package instance

import (
	context "context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ctfer-io/chall-manager/api/v1/common"
	"github.com/ctfer-io/chall-manager/global"
	"github.com/ctfer-io/chall-manager/pkg/fs"
	"github.com/ctfer-io/chall-manager/pkg/iac"
	"github.com/ctfer-io/chall-manager/pkg/lock"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (man *Manager) CreateInstance(ctx context.Context, req *CreateInstanceRequest) (*Instance, error) {
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
	challDir := filepath.Join(global.Conf.Directory, "chall", req.ChallengeId)
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

	// 6. If instance does exist, return error (+ Unlock RW instance, Unlock R challenge)
	idir := filepath.Join(challDir, "instance", req.SourceId)
	if _, err := os.Stat(idir); err == nil {
		return nil, fmt.Errorf("instance %s of challenge %s already exist", req.SourceId, req.ChallengeId)
	}
	if err := os.MkdirAll(idir, os.ModePerm); err != nil {
		logger.Error("create challenge instance",
			zap.String("challenge_id", req.ChallengeId),
			zap.String("source_id", req.SourceId),
			zap.Error(err),
		)
		return nil, common.ErrInternal
	}

	// 7. Pulumi up the instance, write state+metadata to filesystem
	stack, err := iac.NewStack(ctx, fschall, req.SourceId)
	if err != nil {
		return nil, err
	}

	logger.Info("deploying challenge scenario",
		zap.String("challenge_id", req.ChallengeId),
		zap.String("source_id", req.SourceId),
	)
	sr, err := stack.Up(ctx)
	if err != nil {
		logger.Error("stack up",
			zap.String("challenge_id", req.ChallengeId),
			zap.String("source_id", req.SourceId),
			zap.Error(err),
		)
		return nil, common.ErrInternal
	}

	now := time.Now()
	fsist := &fs.Instance{
		ChallengeID: req.ChallengeId,
		SourceID:    req.SourceId,
		Since:       now,
		LastRenew:   now,
		Until:       computeUntil(fschall.Until, fschall.Timeout),
	}
	if err := iac.Write(ctx, stack, sr, fsist); err != nil {
		return nil, common.ErrInternal
	}

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
	// 9. Unlock RW challenge
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

func computeUntil(until *time.Time, timeout *time.Duration) time.Time {
	if until != nil {
		return *until
	}
	if timeout != nil {
		return time.Now().Add(*timeout)
	}
	panic("invalid challenge configuration")
}
