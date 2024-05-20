package challenge

import (
	context "context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/ctfer-io/chall-manager/api/v1/common"
	"github.com/ctfer-io/chall-manager/global"
	errs "github.com/ctfer-io/chall-manager/pkg/errors"
	"github.com/ctfer-io/chall-manager/pkg/fs"
	"github.com/ctfer-io/chall-manager/pkg/iac"
	"github.com/ctfer-io/chall-manager/pkg/identity"
	"github.com/ctfer-io/chall-manager/pkg/lock"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

func (store *Store) DeleteChallenge(ctx context.Context, req *DeleteChallengeRequest) (*emptypb.Empty, error) {
	logger := global.Log()

	// 1. Lock R TOTW
	totw, err := common.LockTOTW()
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

	// 2. Lock RW challenge
	clock, err := common.LockChallenge(req.Id)
	if err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error("build challenge lock", zap.Error(multierr.Combine(
			totw.RUnlock(),
			err,
		)))
		return nil, errs.ErrInternalNoSub
	}
	defer common.LClose(clock)
	if err := clock.RWLock(); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error("challenge RW lock", zap.Error(multierr.Combine(
			totw.RUnlock(),
			err,
		)))
		return nil, errs.ErrInternalNoSub
	}
	// don't defer unlock, will do it manually for ASAP challenge availability

	// 3. Unlock R TOTW
	if err := totw.RUnlock(); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error("TOTW R unlock", zap.Error(multierr.Combine(
			clock.RWUnlock(),
			err,
		)))
		return nil, errs.ErrInternalNoSub
	}

	// 4. If challenge does not exist, return error (+ unlock RW challenge)
	challDir := filepath.Join(global.Conf.Directory, "chall", req.Id)
	fschall, err := fs.LoadChallenge(req.Id)
	if err != nil {
		if err, ok := err.(*errs.ErrInternal); ok {
			logger.Error("reading challenge from filesystem",
				zap.String("challenge_id", req.Id),
				zap.Error(multierr.Combine(
					clock.RWUnlock(),
					err,
				)),
			)
			return nil, errs.ErrInternalNoSub
		}
		return nil, err
	}

	// 5. Fetch challenge instances (if any started)
	iids := []string{}
	dir, err := os.ReadDir(filepath.Join(challDir, "instance"))
	if err == nil {
		for _, dfs := range dir {
			iids = append(iids, dfs.Name())
		}
	}

	// 6. Create "relock" and "work" wait groups for all instances, and for each
	relock := &sync.WaitGroup{} // track goroutines that overlocked an identity
	relock.Add(len(iids))
	work := &sync.WaitGroup{} // track goroutines that ended dealing with the instances
	work.Add(len(iids))
	cerr := make(chan error, len(iids))
	for _, iid := range iids {
		go func(relock, work *sync.WaitGroup, cerr chan<- error, iid string) {
			// 7.e. done in the "work" wait group
			defer work.Done()

			// 7.a. Lock RW instance
			ilock, err := common.LockInstance(req.Id, iid)
			if err != nil {
				cerr <- err
				relock.Done() // release to avoid dead-lock
				return
			}
			defer common.LClose(ilock)
			if err := ilock.RWLock(); err != nil {
				cerr <- err
				relock.Done() // release to avoid dead-lock
				return
			}
			defer func(lock lock.RWLock) {
				// 7.d. Unlock RW instance
				if err := lock.RWUnlock(); err != nil {
					err := &errs.ErrInternal{Sub: err}
					logger.Error("instance RW unlock", zap.Error(err))
				}
			}(ilock)

			// 7.b. done in the "relock" wait group
			relock.Done()

			// 7.c. delete it
			fsist, err := fs.LoadInstance(req.Id, iid)
			if err != nil {
				cerr <- err
				return
			}

			id := identity.Compute(req.Id, iid)
			stack, err := iac.LoadStack(ctx, fschall.Directory, id)
			if err != nil {
				cerr <- err
				return
			}
			state, _ := json.Marshal(fsist.State)
			if err := stack.Import(ctx, apitype.UntypedDeployment{
				Version:    3,
				Deployment: state,
			}); err != nil {
				cerr <- err
				return
			}
			if _, err := stack.Destroy(ctx); err != nil {
				cerr <- err
				return
			}
		}(relock, work, cerr, iid)
	}

	// 8. Once all "relock" done, unlock RW challenge
	relock.Wait()
	if err := clock.RWUnlock(); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error("challenge RW unlock", zap.Error(err))
		return nil, errs.ErrInternalNoSub
	}

	// 9. Once all "work" done, return response or error if any
	work.Wait()
	close(cerr)
	var merri, merr error
	for err := range cerr {
		if err, ok := err.(*errs.ErrInternal); ok {
			merri = multierr.Append(merri, err)
			continue
		}
		merr = multierr.Append(merr, err)
	}
	if merri != nil {
		logger.Error("reading instances",
			zap.String("challenge_id", req.Id),
			zap.Error(merri),
		)
		if err := os.RemoveAll(challDir); err != nil {
			logger.Error("removing challenge directory",
				zap.String("challenge_id", req.Id),
				zap.Error(err),
			)
		}
		return nil, errs.ErrInternalNoSub
	}
	if merr != nil {
		return nil, merr
	}

	if err := os.RemoveAll(challDir); err != nil {
		logger.Error("removing challenge directory",
			zap.String("challenge_id", req.Id),
			zap.Error(err),
		)
		return nil, errs.ErrInternalNoSub
	}
	return nil, nil
}
