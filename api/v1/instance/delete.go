package instance

import (
	"context"

	json "github.com/goccy/go-json"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
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
	ctx = global.WithChallengeID(ctx, req.ChallengeId)
	span := trace.SpanFromContext(ctx)

	// 1. Lock R TOTW
	span.AddEvent("lock TOTW")
	totw, err := common.LockTOTW()
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
	clock, err := common.LockChallenge(req.ChallengeId)
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
		logger.Error(ctx, "TOTW R unlock",
			zap.Error(multierr.Combine(
				clock.RUnlock(ctx),
				err,
			)),
		)
		return nil, errs.ErrInternalNoSub
	}
	span.AddEvent("unlocked TOTW")

	// 4. If challenge does not exist, return error
	fschall, err := fs.LoadChallenge(req.ChallengeId)
	if err != nil {
		if err, ok := err.(*errs.ErrInternal); ok {
			logger.Error(ctx, "loading challenge",
				zap.Error(multierr.Combine(
					clock.RUnlock(ctx),
					err,
				)),
			)
			return nil, errs.ErrInternalNoSub
		}
		if err := clock.RUnlock(ctx); err != nil {
			logger.Error(ctx, "reading challenge from filesystem",
				zap.Error(clock.RUnlock(ctx)),
			)
		}
		return nil, err
	}

	// 5. Lock RW instance
	ctx = global.WithSourceID(ctx, req.SourceId)
	id, err := fs.FindInstance(req.ChallengeId, req.SourceId)
	if err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "finding instance",
			zap.Error(multierr.Combine(
				clock.RUnlock(ctx),
				err,
			)),
		)
		return nil, errs.ErrInternalNoSub
	}

	ctx = global.WithIdentity(ctx, id)
	ilock, err := common.LockInstance(req.ChallengeId, id)
	if err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "build challenge lock",
			zap.Error(multierr.Combine(
				clock.RUnlock(ctx),
				err,
			)),
		)
		return nil, errs.ErrInternalNoSub
	}
	defer common.LClose(ilock)
	if err := ilock.RWLock(ctx); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "challenge instance RW lock",
			zap.Error(multierr.Combine(
				clock.RUnlock(ctx),
				err,
			)),
		)
		return nil, errs.ErrInternalNoSub
	}
	defer func(lock lock.RWLock) {
		if err := lock.RWUnlock(ctx); err != nil {
			err := &errs.ErrInternal{Sub: err}
			logger.Error(ctx, "instance RW unlock", zap.Error(err))
		}
	}(ilock)

	ists, err := fs.ListInstances(req.ChallengeId)
	if err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "listing instances",
			zap.Error(multierr.Combine(
				clock.RUnlock(ctx),
				err,
			)),
		)
		return nil, errs.ErrInternalNoSub
	}

	if err := clock.RUnlock(ctx); err != nil {
		logger.Error(ctx, "challenge RW unlock",
			zap.Error(err),
		)
		return nil, errs.ErrInternalNoSub
	}

	// 6. Pulumi down the instance, delete state+metadata from filesystem
	fsist, err := fs.LoadInstance(req.ChallengeId, id)
	if err != nil {
		if err, ok := err.(*errs.ErrInternal); ok {
			logger.Error(ctx, "loading instance",
				zap.Error(err),
			)
			return nil, errs.ErrInternalNoSub
		}
		return nil, err
	}

	stack, err := iac.LoadStack(ctx, fschall.Directory, id)
	if err != nil {
		if err, ok := err.(*errs.ErrInternal); ok {
			logger.Error(ctx, "creating challenge instance stack",
				zap.Error(err),
			)
			return nil, errs.ErrInternalNoSub
		}
		return nil, err
	}
	state, err := json.Marshal(fsist.State)
	if err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "unmarshalling Pulumi state",
			zap.Error(err),
		)
		return nil, errs.ErrInternalNoSub
	}
	if err := stack.Import(ctx, apitype.UntypedDeployment{
		Version:    3,
		Deployment: state,
	}); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "importing state",
			zap.Error(err),
		)
		return nil, errs.ErrInternalNoSub
	}

	logger.Info(ctx, "deleting instance")

	if _, err := stack.Destroy(ctx); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "stack down",
			zap.Error(err),
		)
		return nil, errs.ErrInternalNoSub
	}

	if err := fsist.Delete(); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "removing instance directory",
			zap.Error(err),
		)
		return nil, errs.ErrInternalNoSub
	}

	logger.Info(ctx, "deleted instance successfully")
	common.InstancesUDCounter().Add(ctx, -1)

	// Start concurrent routine that will refill the pool if we are now under
	// the threshold (i.e. max).
	// -1 to remove the current deleted instances from filesystem read that
	// happened before.
	if len(ists)-1 < int(fschall.Max) {
		go SpinUp(ctx, req.ChallengeId)
	}

	// 7. Unlock RW instance
	//    -> defered after 5 (fault-tolerance)
	// 8. Unlock R challenge
	//    -> defered after 2 (fault-tolerance)

	return nil, nil
}
