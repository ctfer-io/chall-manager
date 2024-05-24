package challenge

import (
	context "context"
	"os"
	"path/filepath"
	"time"

	"github.com/ctfer-io/chall-manager/api/v1/common"
	instance "github.com/ctfer-io/chall-manager/api/v1/instance"
	"github.com/ctfer-io/chall-manager/global"
	errs "github.com/ctfer-io/chall-manager/pkg/errors"
	"github.com/ctfer-io/chall-manager/pkg/fs"
	"github.com/ctfer-io/chall-manager/pkg/lock"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (store *Store) RetrieveChallenge(ctx context.Context, req *RetrieveChallengeRequest) (*Challenge, error) {
	logger := global.Log()

	// 1. Lock R TOTW
	totw, err := common.LockTOTW(ctx)
	if err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error("build TOTW lock", zap.Error(err))
		return nil, errs.ErrInternalNoSub
	}
	defer common.LClose(totw)
	if err := totw.RLock(); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error("TOTW R lock", zap.Error(err))
		return nil, errs.ErrInternalNoSub
	}

	// 2. Lock R challenge
	clock, err := common.LockChallenge(ctx, req.Id)
	if err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error("build challenge lock", zap.Error(multierr.Combine(
			totw.RUnlock(),
			err,
		)))
		return nil, errs.ErrInternalNoSub
	}
	defer common.LClose(clock)
	if err := clock.RLock(); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error("challenge R lock", zap.Error(multierr.Combine(
			totw.RUnlock(),
			err,
		)))
		return nil, errs.ErrInternalNoSub
	}
	defer func(lock lock.RWLock) {
		if err := lock.RUnlock(); err != nil {
			err := &errs.ErrInternal{Sub: err}
			logger.Error("challenge RW unlock", zap.Error(err))
		}
	}(clock)

	// 3. Unlock R TOTW
	if err := totw.RUnlock(); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error("TOTW R unlock", zap.Error(err))
		return nil, errs.ErrInternalNoSub
	}

	// 4. Fetch challenge info
	challDir := fs.ChallengeDirectory(req.Id)
	fschall, err := fs.LoadChallenge(req.Id)
	if err != nil {
		if err, ok := err.(*errs.ErrInternal); ok {
			logger.Error("loading challenge",
				zap.String("challenge_id", req.Id),
				zap.Error(err),
			)
			return nil, errs.ErrInternalNoSub
		}
		return nil, err
	}

	// 5. For all challenge instances, lock, read, unlock, unlock R ASAP
	iids := []string{}
	dir, err := os.ReadDir(filepath.Join(challDir, "instance"))
	if err == nil {
		for _, dfs := range dir {
			iids = append(iids, dfs.Name())
		}
	}
	ists := make([]*instance.Instance, 0, len(iids))
	for _, iid := range iids {
		fsist, err := fs.LoadInstance(req.Id, iid)
		if err != nil {
			if err, ok := err.(*errs.ErrInternal); ok {
				logger.Error("loading instance",
					zap.String("challenge_id", req.Id),
					zap.String("source_id", iid),
					zap.Error(err),
				)
				return nil, errs.ErrInternalNoSub
			}
			return nil, err
		}

		var until *timestamppb.Timestamp
		if fsist.Until != nil {
			until = timestamppb.New(*fsist.Until)
		}
		ists = append(ists, &instance.Instance{
			ChallengeId:    req.Id,
			SourceId:       iid,
			Since:          timestamppb.New(fsist.Since),
			LastRenew:      timestamppb.New(fsist.LastRenew),
			Until:          until,
			ConnectionInfo: fsist.ConnectionInfo,
			Flag:           fsist.Flag,
		})
	}

	return &Challenge{
		Id:        req.Id,
		Hash:      fschall.Hash,
		Dates:     toDates(fschall.Until, fschall.Timeout),
		Instances: ists,
	}, nil
}

func toDates(until *time.Time, timeout *time.Duration) isChallenge_Dates {
	if until != nil {
		return &Challenge_Until{
			Until: timestamppb.New(*until),
		}
	}
	if timeout != nil {
		return &Challenge_Timeout{
			Timeout: durationpb.New(*timeout),
		}
	}
	return nil
}
