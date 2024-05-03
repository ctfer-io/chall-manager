package instance

import (
	context "context"

	"github.com/ctfer-io/chall-manager/api/v1/common"
	"github.com/ctfer-io/chall-manager/global"
	"github.com/ctfer-io/chall-manager/pkg/fs"
	"github.com/ctfer-io/chall-manager/pkg/lock"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (man *Manager) RetrieveInstance(ctx context.Context, req *RetrieveInstanceRequest) (*Instance, error) {
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

	// 3. Unlock R TOTW
	if err := totw.RUnlock(); err != nil {
		logger.Error("TOTW R unlock", zap.Error(multierr.Combine(
			clock.RUnlock(),
			err,
		)))
		return nil, common.ErrInternal
	}

	// 4. If challenge does not exist, return error
	if _, err := fs.LoadChallenge(req.ChallengeId); err != nil {
		return nil, err
	}

	// 4. Lock R instance
	ilock, err := common.LockInstance(req.ChallengeId, req.SourceId)
	if err != nil {
		logger.Error("build challenge lock", zap.Error(multierr.Combine(
			clock.RUnlock(),
			err,
		)))
		return nil, common.ErrInternal
	}
	defer common.LClose(ilock)
	if err := ilock.RLock(); err != nil {
		logger.Error("challenge instance RW lock", zap.Error(multierr.Combine(
			clock.RUnlock(),
			err,
		)))
		return nil, common.ErrInternal
	}
	defer func(lock lock.RWLock) {
		if err := lock.RUnlock(); err != nil {
			logger.Error("instance RW unlock", zap.Error(err))
		}
	}(ilock)

	// 5. Unlock R challenge
	if nerr := clock.RUnlock(); nerr != nil {
		logger.Error("challenge R unlock", zap.Error(err))
		return nil, common.ErrInternal
	}

	// 6. If instance does not exist, return error
	fsist, err := fs.LoadInstance(req.ChallengeId, req.SourceId)
	if err != nil {
		return nil, err
	}

	// 7. Unlock R instance
	//    -> defered after 4 (fault-tolerance)

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
