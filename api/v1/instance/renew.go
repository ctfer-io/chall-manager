package instance

import (
	context "context"
	"fmt"
	"time"

	"github.com/ctfer-io/chall-manager/api/v1/common"
	"github.com/ctfer-io/chall-manager/global"
	"github.com/ctfer-io/chall-manager/pkg/fs"
	"github.com/ctfer-io/chall-manager/pkg/lock"
	"github.com/pkg/errors"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (man *Manager) RenewInstance(ctx context.Context, req *RenewInstanceRequest) (*Instance, error) {
	logger := global.Log()

	// 1. Lock R TOTW
	totw, err := common.LockTOTW()
	if err != nil {
		logger.Error("build TOTW lock", zap.Error(err))
		return nil, common.ErrInternal
	}
	defer common.LClose(totw)
	if err := totw.RLock(); err != nil {
		logger.Error("TOTW R lock", zap.Error(err))
		return nil, common.ErrInternal
	}

	// 2. Lock R challenge
	clock, err := common.LockChallenge(req.ChallengeId)
	if err != nil {
		logger.Error("build challenge lock", zap.Error(multierr.Combine(
			totw.RUnlock(),
			err,
		)))
		return nil, common.ErrInternal
	}
	defer common.LClose(clock)
	if err := clock.RLock(); err != nil {
		logger.Error("challenge R lock", zap.Error(multierr.Combine(
			totw.RUnlock(),
			err,
		)))
		return nil, common.ErrInternal
	}
	defer func(lock lock.RWLock) {
		if err := lock.RUnlock(); err != nil {
			logger.Error("challenge R unlock", zap.Error(err))
		}
	}(clock)

	// 3. Unlock R TOTW
	if err := totw.RUnlock(); err != nil {
		logger.Error("TOTW R unlock", zap.Error(err))
		return nil, common.ErrInternal
	}

	// 4. If challenge does not exist, return error
	fschall, err := fs.LoadChallenge(req.ChallengeId)
	if err != nil {
		return nil, err
	}

	// 5. Lock RW instance
	ilock, err := common.LockInstance(req.ChallengeId, req.SourceId)
	if err != nil {
		logger.Error("build challenge lock", zap.Error(err))
		return nil, common.ErrInternal
	}
	defer common.LClose(ilock)
	if err := ilock.RWLock(); err != nil {
		logger.Error("challenge instance RW lock", zap.Error(err))
		return nil, common.ErrInternal
	}
	defer func(lock lock.RWLock) {
		if err := lock.RWUnlock(); err != nil {
			logger.Error("instance RW unlock", zap.Error(err))
		}
	}(ilock)

	// 6. If instance does not exist, return error (+ Unlock RW instance, Unlock R challenge)
	fsist, err := fs.LoadInstance(req.ChallengeId, req.SourceId)
	if err != nil {
		return nil, err
	}

	// 7. If not (challenge.timeout + instance.until - now < 2 * challenge.timeout), return error (+Unlock RW intance, Unlock R challenge)
	//    For instance, if the challenge timeout is set to 30 minutes and the instance
	//    has a remaining of 5 minutes, we check that 30+5 < 2*30 <=> 5 < 30, thus
	//    we grant renewal. This avoids infinite renewal at once i.e. a player spamming
	//    the renew button to grant its instance infinite uptime.
	if fschall.Timeout != nil {
		// This makes sure renewal is possible thanks to a timeout
		return nil, fmt.Errorf("challenge %s does not accept renewal", req.ChallengeId)
	}
	now := time.Now()
	fsist.LastRenew = now
	if now.After(fsist.Until) {
		// This makes sure fsist.Until > now <=> fsist.Until-now > 0
		return nil, errors.New("challenge instance can't be renewed")
	}
	remaining := fsist.Until.Sub(now)
	if remaining >= *fschall.Timeout {
		return nil, errors.New("challenge instance can't be renewed")
	}
	fsist.Until = fsist.Until.Add(*fschall.Timeout)

	if err := fsist.Save(); err != nil {
		logger.Error("exporting instance information to filesystem",
			zap.String("challenge_id", req.ChallengeId),
			zap.String("source_id", req.SourceId),
			zap.Error(err),
		)
		return nil, common.ErrInternal
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
