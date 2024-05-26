package instance

import (
	context "context"

	json "github.com/goccy/go-json"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	emptypb "google.golang.org/protobuf/types/known/emptypb"

	"github.com/ctfer-io/chall-manager/api/v1/common"
	"github.com/ctfer-io/chall-manager/global"
	errs "github.com/ctfer-io/chall-manager/pkg/errors"
	"github.com/ctfer-io/chall-manager/pkg/fs"
	"github.com/ctfer-io/chall-manager/pkg/iac"
	"github.com/ctfer-io/chall-manager/pkg/identity"
	"github.com/ctfer-io/chall-manager/pkg/lock"
)

func (man *Manager) DeleteInstance(ctx context.Context, req *DeleteInstanceRequest) (*emptypb.Empty, error) {
	logger := global.Log()
	ctx = global.WithChallengeId(ctx, req.ChallengeId)

	// 1. Lock R TOTW
	totw, err := common.LockTOTW(ctx)
	if err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "build TOTW lock", zap.Error(err))
		return nil, errs.ErrInternalNoSub
	}
	defer common.LClose(totw)
	if err := totw.RLock(); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "TOTW R lock", zap.Error(err))
		return nil, errs.ErrInternalNoSub
	}

	// 2. Lock R challenge
	clock, err := common.LockChallenge(ctx, req.ChallengeId)
	if err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "build challenge lock", zap.Error(multierr.Combine(
			totw.RUnlock(),
			err,
		)))
		return nil, errs.ErrInternalNoSub
	}
	defer common.LClose(clock)
	if err := clock.RLock(); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "challenge R lock", zap.Error(multierr.Combine(
			totw.RUnlock(),
			err,
		)))
		return nil, errs.ErrInternalNoSub
	}
	defer func(lock lock.RWLock) {
		if err := lock.RUnlock(); err != nil {
			err := &errs.ErrInternal{Sub: err}
			logger.Error(ctx, "challenge R unlock", zap.Error(err))
		}
	}(clock)

	// 3. Unlock R TOTW
	if err := totw.RUnlock(); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "TOTW R unlock", zap.Error(err))
		return nil, errs.ErrInternalNoSub
	}

	// 4. If challenge does not exist, return error
	fschall, err := fs.LoadChallenge(req.ChallengeId)
	if err != nil {
		if err, ok := err.(*errs.ErrInternal); ok {
			logger.Error(ctx, "loading challenge",
				zap.Error(err),
			)
			return nil, errs.ErrInternalNoSub
		}
		return nil, err
	}

	// 5. Lock RW instance
	ctx = global.WithSourceId(ctx, req.SourceId)
	ilock, err := common.LockInstance(ctx, req.ChallengeId, req.SourceId)
	if err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "build challenge lock", zap.Error(err))
		return nil, errs.ErrInternalNoSub
	}
	defer common.LClose(ilock)
	if err := ilock.RWLock(); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "challenge instance RW lock", zap.Error(err))
		return nil, errs.ErrInternalNoSub
	}
	defer func(lock lock.RWLock) {
		if err := lock.RWUnlock(); err != nil {
			err := &errs.ErrInternal{Sub: err}
			logger.Error(ctx, "instance RW unlock", zap.Error(err))
		}
	}(ilock)

	// 6. Pulumi down the instance, delete state+metadata from filesystem
	fsist, err := fs.LoadInstance(req.ChallengeId, req.SourceId)
	if err != nil {
		if err, ok := err.(*errs.ErrInternal); ok {
			logger.Error(ctx, "loading instance",
				zap.Error(err),
			)
			return nil, errs.ErrInternalNoSub
		}
		return nil, err
	}

	id := identity.Compute(req.ChallengeId, req.SourceId)
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

	// 7. Unlock RW instance
	//    -> defered after 5 (fault-tolerance)
	// 8. Unlock R challenge
	//    -> defered after 2 (fault-tolerance)

	return nil, nil
}
