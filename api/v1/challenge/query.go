package challenge

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/ctfer-io/chall-manager/api/v1/common"
	instance "github.com/ctfer-io/chall-manager/api/v1/instance"
	"github.com/ctfer-io/chall-manager/global"
	errs "github.com/ctfer-io/chall-manager/pkg/errors"
	"github.com/ctfer-io/chall-manager/pkg/fs"
	"github.com/ctfer-io/chall-manager/pkg/lock"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (store *Store) QueryChallenge(_ *emptypb.Empty, server ChallengeStore_QueryChallengeServer) error {
	logger := global.Log()

	// 1. Lock RW TOTW
	totw, err := common.LockTOTW(server.Context())
	if err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error("build TOTW lock", zap.Error(err))
		return errs.ErrInternalNoSub
	}
	defer common.LClose(totw)
	if err := totw.RWLock(); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error("TOTW RW lock", zap.Error(err))
		return errs.ErrInternalNoSub
	}

	// 2. Fetch all challenges
	idhs := []string{}
	dir, err := os.ReadDir(filepath.Join(global.Conf.Directory, "chall"))
	if err == nil {
		for _, dfs := range dir {
			idhs = append(idhs, dfs.Name())
		}
	}

	// 3. Create "relock" and "work" wait groups for all challenges, and for each
	relock := &sync.WaitGroup{}
	relock.Add(len(idhs))
	work := &sync.WaitGroup{}
	work.Add(len(idhs))
	cerr := make(chan error, len(idhs))
	for _, idh := range idhs {
		go func(relock, work *sync.WaitGroup, cerr chan<- error, idh string) {
			// 4.d. done in the "work" wait group
			defer work.Done()

			// Find back challenge id from its hash -> read idh/info.json -> .id
			id, err := fs.IdOfChallenge(idh)
			if err != nil {
				cerr <- err
				relock.Done() // release to avoid dead-lock
				return
			}

			// 4.a. Lock R challenge
			clock, err := common.LockChallenge(server.Context(), id)
			if err != nil {
				cerr <- err
				relock.Done() // release to avoid dead-lock
				return
			}
			defer common.LClose(clock)
			if err := clock.RLock(); err != nil {
				cerr <- err
				relock.Done() // release to avoid dead-lock
				return
			}
			defer func(lock lock.RWLock) {
				// 4.e. Unlock R challenge
				if err := lock.RUnlock(); err != nil {
					err := &errs.ErrInternal{Sub: err}
					logger.Error("challenge RW unlock", zap.Error(err))
				}
			}(clock)

			// 4.b. done in the "relock" wait group
			relock.Done()

			// 4.c. Read challenge info
			challDir := fs.ChallengeDirectory(id)
			fschall, err := fs.LoadChallenge(id)
			if err != nil {
				cerr <- err
				return
			}

			// 4.d. Fetch challenge instances
			//      (don't lock and access concurrently, most probably fast enough even at scale)
			//      (if required to perform concurrently, no breaking change so LGTM)
			iids := []string{}
			dir, err := os.ReadDir(filepath.Join(challDir, fs.InstanceSubdir))
			if err == nil {
				for _, dfs := range dir {
					iids = append(iids, dfs.Name())
				}
			}
			ists := make([]*instance.Instance, 0, len(iids))
			for _, iid := range iids {
				fsist, err := fs.LoadInstance(id, iid)
				if err != nil {
					cerr <- err
					return
				}

				var until *timestamppb.Timestamp
				if fsist.Until != nil {
					until = timestamppb.New(*fsist.Until)
				}
				ists = append(ists, &instance.Instance{
					ChallengeId:    id,
					SourceId:       iid,
					Since:          timestamppb.New(fsist.Since),
					LastRenew:      timestamppb.New(fsist.LastRenew),
					Until:          until,
					ConnectionInfo: fsist.ConnectionInfo,
					Flag:           fsist.Flag,
				})
			}

			if err := server.Send(&Challenge{
				Id:        id,
				Hash:      fschall.Hash,
				Dates:     toDates(fschall.Until, fschall.Timeout),
				Instances: ists,
			}); err != nil {
				cerr <- err
				return
			}
		}(relock, work, cerr, idh)
	}

	// 5. Once all "relock" done, unlock RW TOTW
	relock.Wait()
	if err := totw.RWUnlock(); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error("TOTW RW unlock", zap.Error(err))
		return errs.ErrInternalNoSub
	}

	// 6. Once all "work" done, return error if any
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
		logger.Error("reading challenges", zap.Error(err))
		return errs.ErrInternalNoSub
	}
	return merr // should remain nil as does not depend on user inputs, but makes it future-proof
}
