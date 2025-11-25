package instance

import (
	"context"
	"time"

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

func (man *Manager) RenewInstance(ctx context.Context, req *RenewInstanceRequest) (*Instance, error) {
	logger := global.Log()
	ctx = global.WithChallengeID(ctx, req.ChallengeId)
	span := trace.SpanFromContext(ctx)

	// 1. Lock R TOTW
	span.AddEvent("lock TOTW")
	totw, err := common.LockTOTW(ctx)
	if err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "build TOTW lock", zap.Error(err))
		return nil, err
	}
	defer common.LClose(totw)
	if err := totw.RLock(ctx); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "TOTW R lock", zap.Error(err))
		return nil, err
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
		return nil, err
	}
	defer common.LClose(clock)
	if err := clock.RLock(ctx); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "challenge R lock", zap.Error(multierr.Combine(
			totw.RUnlock(ctx),
			err,
		)))
		return nil, err
	}
	defer func(lock lock.RWLock) {
		if err := lock.RUnlock(ctx); err != nil {
			err := &errs.ErrInternal{Sub: err}
			logger.Error(ctx, "challenge R unlock", zap.Error(err))
		}
	}(clock)

	// 3. Unlock R TOTW
	if err := totw.RUnlock(ctx); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "TOTW R unlock", zap.Error(err))
		return nil, err
	}
	span.AddEvent("unlocked TOTW")

	// 4. If challenge does not exist, return error
	fschall, err := fs.LoadChallenge(req.ChallengeId)
	if err != nil {
		if err, ok := err.(*errs.ErrInternal); ok {
			logger.Error(ctx, "loading challenge",
				zap.Error(err),
			)
			return nil, errs.ErrInternalNoSub
		}
		return nil, errs.ErrValidationFailed{Reason: err.Error()}
	}
	id, err := fs.FindInstance(req.ChallengeId, req.SourceId)
	if err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "finding instance",
			zap.Error(err),
		)
		return nil, errs.ErrInternalNoSub
	}

	// 5. Lock RW instance
	ctx = global.WithSourceID(ctx, req.SourceId)
	ctx = global.WithIdentity(ctx, id)
	ilock, err := common.LockInstance(ctx, req.ChallengeId, id)
	if err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "build challenge lock", zap.Error(err))
		return nil, err
	}
	defer common.LClose(ilock)
	if err := ilock.RWLock(ctx); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "challenge instance RW lock", zap.Error(err))
		return nil, err
	}
	defer func(lock lock.RWLock) {
		if err := lock.RWUnlock(ctx); err != nil {
			err := &errs.ErrInternal{Sub: err}
			logger.Error(ctx, "instance RW unlock", zap.Error(err))
		}
	}(ilock)

	// 6. If instance does not exist, return error (+ Unlock RW instance, Unlock R challenge)
	fsist, err := fs.LoadInstance(req.ChallengeId, id)
	if err != nil {
		if err, ok := err.(*errs.ErrInternal); ok {
			logger.Error(ctx, "loading challenge instance",
				zap.Error(err),
			)
			return nil, errs.ErrInternalNoSub
		}
		return nil, errs.ErrValidationFailed{Reason: err.Error()}
	}

	// 7. Set new until to now + challenge.timeout if any
	if fschall.Timeout == nil {
		// This makes sure renewal is possible thanks to a timeout
		return nil, errs.ErrRenewNotAllowed
	}
	now := time.Now()
	if now.After(*fsist.Until) {
		// This makes sure fsist.Until > now <=> fsist.Until-now > 0
		return nil, errs.ErrInstanceExpiredRenew
	}
	fsist.LastRenew = now
	fsist.Until = common.ComputeUntil(fschall.Until, fschall.Timeout)

	logger.Info(ctx, "renewing instance")
	if err := fsist.Save(); err != nil {
		logger.Error(ctx, "exporting instance information to filesystem",
			zap.Error(err),
		)
		return nil, errs.ErrInternalNoSub
	}

	// 8. Unlock RW instance
	//    -> defered after 5 (fault-tolerance)
	// 9. Unlock R challenge
	//    -> defered after 2 (fault-tolerance)

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
		Flag: func() *string { // kept for retrocompatibility enough time for public migration
			if len(fsist.Flags) == 1 {
				return &fsist.Flags[0]
			}
			return nil
		}(),
		Flags: fsist.Flags,
	}, nil
}
