package instance

import (
	"context"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ctfer-io/chall-manager/api/v1/common"
	"github.com/ctfer-io/chall-manager/global"
	errs "github.com/ctfer-io/chall-manager/pkg/errors"
	"github.com/ctfer-io/chall-manager/pkg/fs"
	"github.com/ctfer-io/chall-manager/pkg/lock"
)

func (man *Manager) RetrieveInstance(ctx context.Context, req *RetrieveInstanceRequest) (*Instance, error) {
	logger := global.Log()
	ctx = global.WithChallengeID(ctx, req.ChallengeId)
	span := trace.SpanFromContext(ctx)

	// 1. Lock R TOTW
	span.AddEvent("lock TOTW")
	totw, err := common.LockTOTW(ctx)
	if err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "build TOTW lock", zap.Error(err))
		return nil, errs.ErrInternalNoSub
	}
	defer common.LClose(totw)
	if err := totw.RLock(ctx); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "TOTW R lock", zap.Error(err))
		return nil, errs.ErrInternalNoSub
	}
	span.AddEvent("locked TOTW")

	// 2. Lock R challenge
	clock, err := common.LockChallenge(ctx, req.ChallengeId)
	if err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "build challenge lock", zap.Error(multierr.Combine(
			totw.RUnlock(ctx),
			err,
		)))
		return nil, errs.ErrInternalNoSub
	}
	defer common.LClose(clock)
	if err := clock.RLock(ctx); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "challenge R lock", zap.Error(multierr.Combine(
			totw.RUnlock(ctx),
			err,
		)))
		return nil, errs.ErrInternalNoSub
	}

	// 3. Unlock R TOTW
	if err := totw.RUnlock(ctx); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "TOTW R unlock", zap.Error(multierr.Combine(
			clock.RUnlock(ctx),
			err,
		)))
		return nil, errs.ErrInternalNoSub
	}
	span.AddEvent("unlocked TOTW")

	// 4. If challenge/instance does not exist, return error
	id, err := fs.FindInstance(req.ChallengeId, req.SourceId)
	if err != nil {
		// If instance not found, is not an error
		if _, ok := err.(*errs.ErrInstanceExist); ok {
			if err := clock.RUnlock(ctx); err != nil {
				err := &errs.ErrInternal{Sub: err}
				logger.Error(ctx, "retrieving unknown instance",
					zap.Error(err),
				)
			}
			return nil, nil
		}
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "finding instance",
			zap.Error(multierr.Combine(
				clock.RUnlock(ctx),
				err,
			)),
		)
		return nil, errs.ErrInternalNoSub
	}

	// 4. Lock R instance
	ctx = global.WithSourceID(ctx, req.SourceId)
	ctx = global.WithIdentity(ctx, id)
	ilock, err := common.LockInstance(ctx, req.ChallengeId, id)
	if err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "build challenge lock", zap.Error(multierr.Combine(
			clock.RUnlock(ctx),
			err,
		)))
		return nil, errs.ErrInternalNoSub
	}
	defer common.LClose(ilock)
	if err := ilock.RLock(ctx); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "challenge instance RW lock", zap.Error(multierr.Combine(
			clock.RUnlock(ctx),
			err,
		)))
		return nil, errs.ErrInternalNoSub
	}
	defer func(lock lock.RWLock) {
		if err := lock.RUnlock(ctx); err != nil {
			err := &errs.ErrInternal{Sub: err}
			logger.Error(ctx, "instance RW unlock", zap.Error(err))
		}
	}(ilock)

	// 5. Unlock R challenge
	if nerr := clock.RUnlock(ctx); nerr != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "challenge R unlock", zap.Error(err))
		return nil, errs.ErrInternalNoSub
	}

	// 6. If instance does not exist, return error
	fsist, err := fs.LoadInstance(req.ChallengeId, id)
	if err != nil {
		// If instance not found, is not an error
		if _, ok := err.(*errs.ErrInstanceExist); ok {
			return nil, nil
		}
		if err, ok := err.(*errs.ErrInternal); ok {
			logger.Error(ctx, "loading challenge instance",
				zap.Error(err),
			)
			return nil, errs.ErrInternalNoSub
		}
		return nil, err
	}

	// 7. Unlock R instance
	//    -> defered after 4 (fault-tolerance)

	var until *timestamppb.Timestamp
	if fsist.Until != nil {
		until = timestamppb.New(*fsist.Until)
	}
	return &Instance{
		ChallengeId:    req.ChallengeId,
		SourceId:       req.SourceId,
		Since:          timestamppb.New(fsist.Since),
		LastRenew:      timestamppb.New(fsist.LastRenew),
		Until:          until,
		ConnectionInfo: fsist.ConnectionInfo,
		Flag:           fsist.Flag,
		Additional:     fsist.Additional,
	}, nil
}
