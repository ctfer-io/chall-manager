package instance

import (
	"context"
	"os"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	emptypb "google.golang.org/protobuf/types/known/emptypb"

	"github.com/ctfer-io/chall-manager/api/v1/common"
	"github.com/ctfer-io/chall-manager/global"
	errs "github.com/ctfer-io/chall-manager/pkg/errors"
	"github.com/ctfer-io/chall-manager/pkg/fs"
	"github.com/ctfer-io/chall-manager/pkg/iac"
	"github.com/ctfer-io/chall-manager/pkg/lock"
)

func (man *Manager) DeleteInstance(ctx context.Context, req *DeleteInstanceRequest) (*emptypb.Empty, error) {
	logger := global.Log()
	ctx = global.WithChallengeID(ctx, req.GetChallengeId())
	span := trace.SpanFromContext(ctx)

	// 1. Lock R TOTW
	span.AddEvent("lock TOTW")
	totw, err := common.LockTOTW(ctx)
	if err != nil {
		if totw.IsCanceled(err) {
			return nil, errs.ErrCanceled
		}
		logger.Error(ctx, "build TOTW lock", zap.Error(err))
		return nil, errs.ErrInternalNoSub
	}
	if err := totw.RLock(ctx); err != nil {
		if totw.IsCanceled(err) {
			return nil, errs.ErrCanceled
		}
		logger.Error(ctx, "TOTW R lock", zap.Error(err))
		return nil, errs.ErrInternalNoSub
	}
	span.AddEvent("locked TOTW")

	// 2. Lock RW challenge
	clock, err := common.LockChallenge(ctx, req.GetChallengeId())
	if err != nil {
		if clock.IsCanceled(err) {
			// If canceled, we need to recover
			if err := totw.RUnlock(context.WithoutCancel(ctx)); err != nil {
				logger.Error(ctx, "recovering from build challenge lock", zap.Error(err))
				return nil, errs.ErrInternalNoSub
			}
			return nil, errs.ErrCanceled // recovery is successful, we can quit safely
		}
		logger.Error(ctx, "build challenge lock", zap.Error(multierr.Combine(
			totw.RUnlock(context.WithoutCancel(ctx)),
			err,
		)))
		return nil, errs.ErrInternalNoSub
	}
	if err := clock.RLock(ctx); err != nil {
		if clock.IsCanceled(err) {
			// If canceled, we need to recover
			if err := totw.RUnlock(context.WithoutCancel(ctx)); err != nil {
				logger.Error(ctx, "recovering from challenge R lock", zap.Error(err))
				return nil, errs.ErrInternalNoSub
			}
			return nil, errs.ErrCanceled // recovery is successful, we can quit safely
		}
		logger.Error(ctx, "challenge R lock", zap.Error(multierr.Combine(
			totw.RUnlock(context.WithoutCancel(ctx)),
			err,
		)))
		return nil, errs.ErrInternalNoSub
	}

	// 3. Unlock R TOTW
	if err := totw.RUnlock(context.WithoutCancel(ctx)); err != nil {
		logger.Error(ctx, "TOTW R unlock",
			zap.Error(multierr.Combine(
				clock.RUnlock(context.WithoutCancel(ctx)),
				err,
			)),
		)
		return nil, errs.ErrInternalNoSub
	}
	span.AddEvent("unlocked TOTW")

	// 4. If challenge does not exist, return error
	fschall, err := fs.LoadChallenge(req.GetChallengeId())
	if err != nil {
		if err := clock.RUnlock(context.WithoutCancel(ctx)); err != nil {
			logger.Error(ctx, "unlocking R challenge", zap.Error(err))
		}

		// If challenge not found
		if _, ok := err.(*errs.ChallengeExist); ok {
			return nil, err
		}
		// Else deal with it as an internal server error
		logger.Error(ctx, "loading challenge",
			zap.Error(err),
		)
		return nil, errs.ErrInternalNoSub
	}

	// 5. Lock RW instance
	ctx = global.WithSourceID(ctx, req.GetSourceId())
	id, err := fs.FindInstance(req.GetChallengeId(), req.GetSourceId())
	if err != nil {
		if err := clock.RUnlock(context.WithoutCancel(ctx)); err != nil {
			logger.Error(ctx, "unlocking R challenge", zap.Error(err))
		}

		if _, ok := err.(*errs.InstanceExist); ok {
			return nil, err
		}

		logger.Error(ctx, "finding instance",
			zap.Error(err),
		)
		return nil, errs.ErrInternalNoSub
	}

	ctx = global.WithIdentity(ctx, id)
	ilock, err := common.LockInstance(ctx, req.GetChallengeId(), id)
	if err != nil {
		if ilock.IsCanceled(err) {
			// If canceled, we need to recover
			if err := clock.RUnlock(context.WithoutCancel(ctx)); err != nil {
				logger.Error(ctx, "recovering from challenge R unlock", zap.Error(err))
				return nil, errs.ErrInternalNoSub
			}
			return nil, errs.ErrCanceled // recovery is successful, we can quit safely
		}
		logger.Error(ctx, "build challenge lock",
			zap.Error(multierr.Combine(
				clock.RUnlock(context.WithoutCancel(ctx)),
				err,
			)),
		)
		return nil, errs.ErrInternalNoSub
	}
	if err := ilock.RWLock(ctx); err != nil {
		if ilock.IsCanceled(err) {
			// If canceled, we need to recover
			if err := clock.RUnlock(context.WithoutCancel(ctx)); err != nil {
				logger.Error(ctx, "recovering from challenge instance RW lock", zap.Error(err))
				return nil, errs.ErrInternalNoSub
			}
			return nil, errs.ErrCanceled // recovery is successful, we can quit safely
		}
		logger.Error(ctx, "challenge instance RW lock",
			zap.Error(multierr.Combine(
				clock.RUnlock(context.WithoutCancel(ctx)),
				err,
			)),
		)
		return nil, errs.ErrInternalNoSub
	}
	defer func(lock lock.RWLock) {
		if err := lock.RWUnlock(context.WithoutCancel(ctx)); err != nil {
			logger.Error(ctx, "instance RW unlock", zap.Error(err))
		}
	}(ilock)

	ists, err := fs.ListInstances(req.GetChallengeId())
	if err != nil {
		logger.Error(ctx, "listing instances",
			zap.Error(multierr.Combine(
				clock.RUnlock(context.WithoutCancel(ctx)),
				err,
			)),
		)
		return nil, errs.ErrInternalNoSub
	}
	pooled := []string{}
	for _, ist := range ists {
		sourceID, err := fs.LookupClaim(req.GetChallengeId(), ist)
		if os.IsNotExist(err) {
			// no claim file => in pool
			pooled = append(pooled, sourceID)
			continue
		}
		if err != nil {
			logger.Error(ctx, "looking up for claim",
				zap.Error(err),
			)
			return nil, errs.ErrInternalNoSub
		}
	}

	if err := clock.RUnlock(context.WithoutCancel(ctx)); err != nil {
		logger.Error(ctx, "challenge RW unlock",
			zap.Error(err),
		)
		return nil, errs.ErrInternalNoSub
	}

	// 6. Pulumi down the instance, delete state+metadata from filesystem
	fsist, err := fs.LoadInstance(req.GetChallengeId(), id)
	if err != nil {
		logger.Error(ctx, "loading instance",
			zap.Error(err),
		)
		return nil, errs.ErrInternalNoSub
	}

	// Reload cache if necessary
	stack, err := iac.LoadStack(ctx, fschall.Scenario, id)
	if err != nil {
		logger.Error(ctx, "creating challenge instance stack",
			zap.Error(err),
		)
		return nil, errs.ErrInternalNoSub
	}
	if err := stack.Import(ctx, fsist); err != nil {
		logger.Error(ctx, "unmarshalling Pulumi state",
			zap.Error(err),
		)
		return nil, errs.ErrInternalNoSub
	}

	logger.Info(ctx, "deleting instance")

	if err := stack.Down(ctx); err != nil {
		logger.Error(ctx, "stack down",
			zap.Error(err),
		)
		return nil, err // might be a meaningfull error
	}

	if err := fsist.Delete(); err != nil {
		logger.Error(ctx, "removing instance directory",
			zap.Error(err),
		)
		return nil, errs.ErrInternalNoSub
	}

	logger.Info(ctx, "deleted instance successfully")
	common.InstancesUDCounter().Add(ctx, -1,
		metric.WithAttributeSet(common.InstanceAttrs(req.GetChallengeId(), req.GetSourceId(), false)),
	)

	// Start concurrent routine that will refill the pool if we are now under
	// the threshold (i.e. max).
	// -1 to remove the current deleted instances from filesystem read that
	// happened before.
	//
	// XXX data were captured in a concurrent-safe segment of code, but now it might have drifted a bit. This should be performed in the critical section
	if len(pooled) < int(fschall.Min) && (fschall.Max == 0 || len(ists)-1 < int(fschall.Max)) {
		go SpinUp(ctx, req.GetChallengeId())
	}

	// 7. Unlock RW instance
	//    -> defered after 5 (fault-tolerance)
	// 8. Unlock R challenge
	//    -> defered after 2 (fault-tolerance)

	return nil, nil
}
