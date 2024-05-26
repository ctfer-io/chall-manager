package challenge

import (
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
	ctx := server.Context()

	// 1. Lock RW TOTW
	totw, err := common.LockTOTW(ctx)
	if err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "build TOTW lock", zap.Error(err))
		return errs.ErrInternalNoSub
	}
	defer common.LClose(totw)
	if err := totw.RWLock(); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "TOTW RW lock", zap.Error(err))
		return errs.ErrInternalNoSub
	}

	// 2. Fetch all challenges
	ids, err := fs.ListChallenges()
	if err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "listing challenges", zap.Error(err))
		return errs.ErrInternalNoSub
	}

	// 3. Create "relock" and "work" wait groups for all challenges, and for each
	relock := &sync.WaitGroup{}
	relock.Add(len(ids))
	work := &sync.WaitGroup{}
	work.Add(len(ids))
	cerr := make(chan error, len(ids))
	for _, id := range ids {
		go func(relock, work *sync.WaitGroup, cerr chan<- error, id string) {
			// 4.d. done in the "work" wait group
			defer work.Done()
			ctx := global.WithChallengeId(ctx, id)

			// 4.a. Lock R challenge
			clock, err := common.LockChallenge(ctx, id)
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
					logger.Error(ctx, "challenge RW unlock", zap.Error(err))
				}
			}(clock)

			// 4.b. done in the "relock" wait group
			relock.Done()

			// 4.c. Read challenge info
			fschall, err := fs.LoadChallenge(id)
			if err != nil {
				cerr <- err
				return
			}

			// 4.d. Fetch challenge instances
			//      (don't lock and access concurrently, most probably fast enough even at scale)
			//      (if required to perform concurrently, no breaking change so LGTM)
			iids, err := fs.ListInstances(id)
			if err != nil {
				cerr <- err
				return
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
		}(relock, work, cerr, id)
	}

	// 5. Once all "relock" done, unlock RW TOTW
	relock.Wait()
	if err := totw.RWUnlock(); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "TOTW RW unlock", zap.Error(err))
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
		logger.Error(ctx, "reading challenges", zap.Error(err))
		return errs.ErrInternalNoSub
	}
	return merr // should remain nil as does not depend on user inputs, but makes it future-proof
}
