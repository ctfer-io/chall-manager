package instance

import (
	context "context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/ctfer-io/chall-manager/api/v1/common"
	"github.com/ctfer-io/chall-manager/global"
	"github.com/ctfer-io/chall-manager/pkg/fs"
	"github.com/ctfer-io/chall-manager/pkg/iac"
	"github.com/ctfer-io/chall-manager/pkg/identity"
	"github.com/ctfer-io/chall-manager/pkg/lock"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

func (man *Manager) DeleteInstance(ctx context.Context, req *DeleteInstanceRequest) (*emptypb.Empty, error) {
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

	// 6. Pulumi down the instance, delete state+metadata from filesystem
	idir := filepath.Join(challDir, "instance", req.SourceId)
	fsist, err := fs.LoadInstance(req.ChallengeId, req.SourceId)
	if err != nil {
		return nil, err
	}

	id := identity.Compute(req.ChallengeId, req.SourceId)
	stack, err := iac.LoadStack(ctx, fschall.Directory, id)
	if err != nil {
		logger.Error("create challenge instance stack",
			zap.String("challenge_id", req.ChallengeId),
			zap.String("source_id", req.SourceId),
			zap.Error(err),
		)
		return nil, common.ErrInternal
	}
	state, _ := json.Marshal(fsist.State)
	if err := stack.Import(ctx, apitype.UntypedDeployment{
		Version:    3,
		Deployment: state,
	}); err != nil {
		logger.Error("importing state",
			zap.String("challenge_id", req.ChallengeId),
			zap.String("source_id", req.SourceId),
			zap.Error(err),
		)
		return nil, common.ErrInternal
	}

	logger.Info("deleting instance",
		zap.String("source_id", req.SourceId),
		zap.String("challenge_id", req.ChallengeId),
	)

	if _, err := stack.Destroy(ctx); err != nil {
		logger.Error("stack down",
			zap.String("challenge_id", req.ChallengeId),
			zap.String("source_id", req.SourceId),
			zap.Error(err),
		)
		return nil, common.ErrInternal
	}

	if err := os.RemoveAll(idir); err != nil {
		logger.Error("removing instance directory",
			zap.String("challenge_id", req.ChallengeId),
			zap.String("source_id", req.SourceId),
			zap.Error(err),
		)
		return nil, err
	}

	// 7. Unlock RW instance
	//    -> defered after 5 (fault-tolerance)
	// 8. Unlock R challenge
	//    -> defered after 2 (fault-tolerance)

	return nil, nil
}
