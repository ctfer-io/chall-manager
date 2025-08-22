package challenge

import (
	"context"
	"fmt"
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
	"github.com/ctfer-io/chall-manager/pkg/iac"
	"github.com/ctfer-io/chall-manager/pkg/lock"
	"github.com/ctfer-io/chall-manager/pkg/scenario"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
)

func (store *Store) CreateChallenge(ctx context.Context, req *CreateChallengeRequest) (*Challenge, error) {
	logger := global.Log()
	ctx = global.WithChallengeID(ctx, req.Id)
	span := trace.SpanFromContext(ctx)

	// 0. Validate request
	// => Pooler boundaries defaults to 0, with proper ordering
	if req.Min < 0 || req.Max < 0 || (req.Min > req.Max && req.Max != 0) {
		return nil, fmt.Errorf("min/max out of bounds: %d/%d", req.Min, req.Max)
	}

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

	// 2. Lock RW challenge
	clock, err := common.LockChallenge(ctx, req.Id)
	if err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "build challenge lock", zap.Error(multierr.Combine(
			totw.RUnlock(ctx),
			err,
		)))
		return nil, errs.ErrInternalNoSub
	}
	defer common.LClose(clock)
	if err := clock.RWLock(ctx); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "challenge RW lock", zap.Error(multierr.Combine(
			totw.RUnlock(ctx),
			err,
		)))
		return nil, errs.ErrInternalNoSub
	}
	defer func(lock lock.RWLock) {
		if err := lock.RWUnlock(ctx); err != nil {
			err := &errs.ErrInternal{Sub: err}
			logger.Error(ctx, "challenge RW unlock", zap.Error(err))
		}
	}(clock)

	// 3. Unlock R TOTW
	if err := totw.RUnlock(ctx); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "TOTW R unlock", zap.Error(err))
		return nil, errs.ErrInternalNoSub
	}
	span.AddEvent("unlocked TOTW")

	// 4. If the challenge already exist, return error
	if err := fs.CheckChallenge(req.Id); err == nil {
		return nil, &errs.ErrChallengeExist{
			ID:    req.Id,
			Exist: true,
		}
	}

	// 5. Prepare challenge
	var dirInitializer *string
	var istack *auto.Stack
	var isr *auto.UpResult
	if req.Initializer != nil {
		logger.Info(ctx, "creating challenge initializer")

		// Decode
		dir, err := scenario.DecodeOCI(ctx,
			req.Id, *req.Initializer, nil,
			global.Conf.OCI.Insecure, global.Conf.OCI.Username, global.Conf.OCI.Password,
		)
		if err != nil {
			err := &errs.ErrInternal{Sub: err}
			logger.Error(ctx, "decoding initializer scenario",
				zap.String("reference", *req.Initializer),
				zap.Error(err),
			)
			return nil, errs.ErrInternalNoSub
		}
		dirInitializer = &dir

		// Then deploy
		stack, err := iac.NewStack(ctx, req.Id, dir)
		if err != nil {
			err := &errs.ErrInternal{Sub: err}
			logger.Error(ctx, "building initializer stack",
				zap.Error(err),
			)
			return nil, errs.ErrInternalNoSub
		}
		istack = &stack

		// don't need additionals for now
		sr, err := stack.Up(ctx)
		if err != nil {
			err := &errs.ErrInternal{Sub: err}
			logger.Error(ctx, "stack up",
				zap.Error(err),
			)
			return nil, errs.ErrInternalNoSub
		}
		isr = &sr
	}

	logger.Info(ctx, "creating challenge")
	dirScenario, err := scenario.DecodeOCI(ctx,
		req.Id, req.Scenario, req.Additional,
		global.Conf.OCI.Insecure, global.Conf.OCI.Username, global.Conf.OCI.Password,
	)
	if err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "decoding scenario",
			zap.String("reference", req.Scenario),
			zap.Error(err),
		)
		return nil, errs.ErrInternalNoSub
	}
	fschall := &fs.Challenge{
		ID:                   req.Id,
		Initializer:          req.Initializer,
		InitializerDirectory: dirInitializer,
		Scenario:             req.Scenario,
		Directory:            dirScenario,
		Timeout:              toDuration(req.Timeout),
		Until:                toTime(req.Until),
		Additional:           req.Additional,
		Min:                  req.Min,
		Max:                  req.Max,
	}

	// If an initializer is set, extract its info
	if req.Initializer != nil {
		if err := iac.ExtractInitializer(ctx, *istack, *isr, fschall); err != nil {
			logger.Error(ctx, "exporting initializer stack into",
				zap.Error(err),
			)
			return nil, errs.ErrInternalNoSub
		}
	}

	// 6. Spin up instances if pool is configured. Lock is acquired at challenge level
	//    hence don't need to be held too.
	for range req.Min {
		go instance.SpinUp(ctx, req.Id)
	}

	// 7. Save challenge on filesystem, and respond to API call
	if err := fschall.Save(); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "exporting challenge information to filesystem",
			zap.Error(err),
		)
		return nil, errs.ErrInternalNoSub
	}

	logger.Info(ctx, "challenge created successfully")
	common.ChallengesUDCounter().Add(ctx, 1)

	chall := &Challenge{
		Id:          req.Id,
		Initializer: req.Initializer,
		Scenario:    req.Scenario,
		Timeout:     req.Timeout,
		Until:       req.Until,
		Instances:   []*instance.Instance{},
		Additional:  req.Additional,
		Min:         req.Min,
		Max:         req.Max,
	}

	// 8. Unlock RW challenge
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
