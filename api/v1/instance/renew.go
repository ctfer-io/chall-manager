package instance

import (
	context "context"
	"errors"
	"fmt"
	"time"

	"github.com/ctfer-io/chall-manager/api/v1/common"
	"github.com/ctfer-io/chall-manager/global"
	errs "github.com/ctfer-io/chall-manager/pkg/errors"
	"github.com/ctfer-io/chall-manager/pkg/fs"
	"github.com/ctfer-io/chall-manager/pkg/lock"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (man *Manager) RenewInstance(ctx context.Context, req *RenewInstanceRequest) (*Instance, error) {
	logger := global.Log()

	// 1. Lock R TOTW
	totw, err := common.LockTOTW()
	if err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error("build TOTW lock", zap.Error(err))
		return nil, errs.ErrInternalNoSub
	}
	defer common.LClose(totw)
	if err := totw.RLock(); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error("TOTW R lock", zap.Error(err))
		return nil, errs.ErrInternalNoSub
	}

	// 2. Lock R challenge
	clock, err := common.LockChallenge(req.ChallengeId)
	if err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error("build challenge lock", zap.Error(multierr.Combine(
			totw.RUnlock(),
			err,
		)))
		return nil, errs.ErrInternalNoSub
	}
	defer common.LClose(clock)
	if err := clock.RLock(); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error("challenge R lock", zap.Error(multierr.Combine(
			totw.RUnlock(),
			err,
		)))
		return nil, errs.ErrInternalNoSub
	}
	defer func(lock lock.RWLock) {
		if err := lock.RUnlock(); err != nil {
			err := &errs.ErrInternal{Sub: err}
			logger.Error("challenge R unlock", zap.Error(err))
		}
	}(clock)

	// 3. Unlock R TOTW
	if err := totw.RUnlock(); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error("TOTW R unlock", zap.Error(err))
		return nil, errs.ErrInternalNoSub
	}

	// 4. If challenge does not exist, return error
	fschall, err := fs.LoadChallenge(req.ChallengeId)
	if err != nil {
		if err, ok := err.(*errs.ErrInternal); ok {
			logger.Error("loading challenge",
				zap.String("challenge_id", req.ChallengeId),
				zap.Error(err),
			)
			return nil, errs.ErrInternalNoSub
		}
		return nil, err
	}

	// 5. Lock RW instance
	ilock, err := common.LockInstance(req.ChallengeId, req.SourceId)
	if err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error("build challenge lock", zap.Error(err))
		return nil, errs.ErrInternalNoSub
	}
	defer common.LClose(ilock)
	if err := ilock.RWLock(); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error("challenge instance RW lock", zap.Error(err))
		return nil, errs.ErrInternalNoSub
	}
	defer func(lock lock.RWLock) {
		if err := lock.RWUnlock(); err != nil {
			err := &errs.ErrInternal{Sub: err}
			logger.Error("instance RW unlock", zap.Error(err))
		}
	}(ilock)

	// 6. If instance does not exist, return error (+ Unlock RW instance, Unlock R challenge)
	fsist, err := fs.LoadInstance(req.ChallengeId, req.SourceId)
	if err != nil {
		if err, ok := err.(*errs.ErrInternal); ok {
			logger.Error("loading challenge instance",
				zap.String("challenge_id", req.ChallengeId),
				zap.String("source_id", req.SourceId),
				zap.Error(err),
			)
			return nil, errs.ErrInternalNoSub
		}
		return nil, err
	}

	// 7. Set new until to now + challenge.timeout if any
	if fschall.Timeout == nil {
		// This makes sure renewal is possible thanks to a timeout
		return nil, fmt.Errorf("challenge %s does not accept renewal", req.ChallengeId)
	}
	now := time.Now()
	if now.After(fsist.Until) {
		// This makes sure fsist.Until > now <=> fsist.Until-now > 0
		return nil, errors.New("challenge instance can't be renewed as it expired")
	}
	fsist.LastRenew = now
	fsist.Until = now.Add(*fschall.Timeout)

	if err := fsist.Save(); err != nil {
		logger.Error("exporting instance information to filesystem",
			zap.String("challenge_id", req.ChallengeId),
			zap.String("source_id", req.SourceId),
			zap.Error(err),
		)
		return nil, errs.ErrInternalNoSub
	}

	// 8. Unlock RW instance
	//    -> defered after 5 (fault-tolerance)
	// 9. Unlock R challenge
	//    -> defered after 2 (fault-tolerance)

	return &Instance{
		ChallengeId:    req.ChallengeId,
		SourceId:       req.SourceId,
		Since:          timestamppb.New(fsist.Since),
		LastRenew:      timestamppb.New(fsist.LastRenew),
		Until:          timestamppb.New(fsist.Until),
		ConnectionInfo: fsist.ConnectionInfo,
		Flag:           fsist.Flag,
	}, nil
}
