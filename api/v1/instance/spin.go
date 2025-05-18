package instance

import (
	"context"
	"time"

	"github.com/ctfer-io/chall-manager/api/v1/common"
	"github.com/ctfer-io/chall-manager/global"
	errs "github.com/ctfer-io/chall-manager/pkg/errors"
	"github.com/ctfer-io/chall-manager/pkg/fs"
	"github.com/ctfer-io/chall-manager/pkg/iac"
	"github.com/ctfer-io/chall-manager/pkg/identity"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/multierr"
	"go.uber.org/zap"
)

// SpinUp is a function that creates a brand new instance in the pool of a challenge.
// Is must be called in a goroutine as it relocks the TOTW/challenge locks.
func SpinUp(ctx context.Context, challengeID string) {
	logger := global.Log()

	// Reset context to acceptable state
	ctx = context.WithoutCancel(ctx)
	ctx = global.WithChallengeID(ctx, challengeID)
	ctx = global.WithoutSourceID(ctx)
	ctx = global.WithoutIdentity(ctx)

	// Track span of spinning up a new instance
	ctx, span := global.Tracer.Start(ctx, "pool-spin-up", trace.WithAttributes(
		attribute.String("challenge_id", challengeID),
	))
	defer span.End()

	// 1. Lock R TOTW
	span.AddEvent("lock TOTW")
	totw, err := common.LockTOTW()
	if err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "build TOTW lock", zap.Error(err))
		return
	}
	defer common.LClose(totw)
	if err := totw.RLock(ctx); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "TOTW R lock", zap.Error(err))
		return
	}
	span.AddEvent("locked TOTW")

	// 2. Lock R challenge
	clock, err := common.LockChallenge(challengeID)
	if err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "build challenge lock", zap.Error(multierr.Combine(
			totw.RUnlock(ctx),
			err,
		)))
		return
	}
	defer common.LClose(clock)
	if err := clock.RLock(ctx); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "challenge R lock", zap.Error(multierr.Combine(
			totw.RUnlock(ctx),
			err,
		)))
		return
	}
	defer func() {
		if err := clock.RUnlock(ctx); err != nil {
			err := &errs.ErrInternal{Sub: err}
			logger.Error(ctx, "challenge R unlock",
				zap.Error(err),
			)
		}
	}()

	// 3. Unlock R TOTW
	if err := totw.RUnlock(ctx); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "TOTW R unlock",
			zap.Error(err),
		)
		return
	}
	span.AddEvent("unlocked TOTW")

	// 4. If challenge does not exist (could have been deleted), return error
	fschall, err := fs.LoadChallenge(challengeID)
	if err != nil {
		logger.Error(ctx, "loading challenge",
			zap.Error(multierr.Combine(
				clock.RUnlock(ctx),
				err,
			)),
		)
		return
	}
	// Skip pre-provision if challenge is expired
	if fschall.Until != nil && time.Now().After(*fschall.Until) {
		return
	}

	// 5. Create identity
	id := identity.New()
	ctx = global.WithIdentity(ctx, id)

	// 10. Spin up instance
	stack, err := iac.NewStack(ctx, id, fschall)
	if err != nil {
		logger.Error(ctx, "building new stack",
			zap.Error(err),
		)
		return
	}
	if err := iac.Additional(ctx, stack, fschall.Additional, nil); err != nil {
		logger.Error(ctx, "configuring additionals on stack",
			zap.Error(err),
		)
		return
	}

	sr, err := stack.Up(ctx)
	if err != nil {
		logger.Error(ctx, "stack up",
			zap.Error(err),
		)
		return
	}

	now := time.Now()
	fsist := &fs.Instance{
		Identity:    id,
		ChallengeID: challengeID,
		Since:       now,
		LastRenew:   now,
		Until:       common.ComputeUntil(fschall.Until, fschall.Timeout),
		Additional:  nil,
	}
	if err := iac.Extract(ctx, stack, sr, fsist); err != nil {
		logger.Error(ctx, "extracting stack info",
			zap.Error(err),
		)
		return
	}

	logger.Info(ctx, "instance registered in pool")
	common.InstancesUDCounter().Add(ctx, 1,
		metric.WithAttributeSet(common.InstanceAttrs(challengeID, "", true)),
	)

	// 11. Save fsist
	if err := fsist.Save(); err != nil {
		logger.Error(ctx, "exporting instance information to filesystem",
			zap.Error(err),
		)
	}
}
