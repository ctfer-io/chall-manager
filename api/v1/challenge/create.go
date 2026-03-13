package challenge

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	durationpb "google.golang.org/protobuf/types/known/durationpb"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ctfer-io/chall-manager/api/v1/common"
	"github.com/ctfer-io/chall-manager/api/v1/instance"
	"github.com/ctfer-io/chall-manager/global"
	errs "github.com/ctfer-io/chall-manager/pkg/errors"
	"github.com/ctfer-io/chall-manager/pkg/fs"
	"github.com/ctfer-io/chall-manager/pkg/lock"
)

func (store *Store) CreateChallenge(ctx context.Context, req *CreateChallengeRequest) (*Challenge, error) {
	logger := global.Log()
	ctx = global.WithChallengeID(ctx, req.GetId())
	span := trace.SpanFromContext(ctx)

	// 0. Validate request (fake an update mask)
	if err := common.CheckPooler([]string{"min", "max"}, req.GetMin(), req.GetMax()); err != nil {
		return nil, err
	}

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
	clock, err := common.LockChallenge(ctx, req.GetId())
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
	if err := clock.RWLock(ctx); err != nil {
		if clock.IsCanceled(err) {
			// If canceled, we need to recover
			if err := totw.RUnlock(context.WithoutCancel(ctx)); err != nil {
				logger.Error(ctx, "recovering from challenge RW lock", zap.Error(err))
				return nil, errs.ErrInternalNoSub
			}
			return nil, errs.ErrCanceled // recovery is successful, we can quit safely
		}
		logger.Error(ctx, "challenge RW lock", zap.Error(multierr.Combine(
			totw.RUnlock(context.WithoutCancel(ctx)),
			err,
		)))
		return nil, errs.ErrInternalNoSub
	}
	defer func(lock lock.RWLock) {
		if err := lock.RWUnlock(context.WithoutCancel(ctx)); err != nil {
			logger.Error(ctx, "challenge RW unlock", zap.Error(err))
		}
	}(clock)

	// 3. Unlock R TOTW
	if err := totw.RUnlock(context.WithoutCancel(ctx)); err != nil {
		logger.Error(ctx, "TOTW R unlock", zap.Error(err))
		return nil, errs.ErrInternalNoSub
	}
	span.AddEvent("unlocked TOTW")

	// 4. If the challenge already exist, return error
	err = fs.CheckChallenge(req.GetId())
	if err == nil { // challenge exist -> no error
		return nil, &errs.ChallengeExist{
			ID:    req.GetId(),
			Exist: true,
		}
	}
	if _, ok := err.(*errs.ChallengeExist); !ok { // if error is not an ErrChallengeExist, there is a problem
		logger.Error(ctx, "checking challenge", zap.Error(err))
		return nil, errs.ErrInternalNoSub
	}

	// 5. Validate the reference before creation (does not guarantee it will work indefinitely,
	// but at least it works at time point in time thus errors enable faster recovery and debug).
	if err := common.Validate(ctx, req.GetScenario(), req.GetAdditional()); err != nil {
		return nil, err // already handled by the helper
	}

	// 6. Prepare challenge
	logger.Info(ctx, "creating challenge")
	fschall := &fs.Challenge{
		ID:         req.GetId(),
		Scenario:   req.GetScenario(),
		Timeout:    toDuration(req.GetTimeout()),
		Until:      toTime(req.GetUntil()),
		Additional: req.GetAdditional(),
		Min:        req.GetMin(),
		Max:        req.GetMax(),
	}

	// 7. Spin up instances if pool is configured. Lock is acquired at challenge level
	//    hence don't need to be held too.
	for range req.GetMin() {
		go instance.SpinUp(ctx, req.GetId())
	}

	// 8. Save challenge on filesystem, and respond to API call
	if err := fschall.Save(); err != nil {
		logger.Error(ctx, "saving challenge",
			zap.Error(err),
		)
		return nil, errs.ErrInternalNoSub
	}

	logger.Info(ctx, "challenge created successfully")
	common.ChallengesUDCounter().Add(ctx, 1)

	chall := &Challenge{
		Id:         req.GetId(),
		Scenario:   req.GetScenario(),
		Timeout:    req.GetTimeout(),
		Until:      req.GetUntil(),
		Instances:  []*instance.Instance{},
		Additional: req.GetAdditional(),
		Min:        req.GetMin(),
		Max:        req.GetMax(),
	}

	// 9. Unlock RW challenge
	//    -> defered after 2 (fault-tolerance)

	return chall, nil
}

func toDuration(d *durationpb.Duration) *time.Duration {
	if d == nil {
		return nil
	}
	td := d.AsDuration()
	return &td
}

func toTime(d *timestamppb.Timestamp) *time.Time {
	if d == nil {
		return nil
	}
	td := d.AsTime()
	return &td
}
