package challenge

import (
	context "context"
	"encoding/json"
	"os"
	"path/filepath"
	sync "sync"
	"time"

	"github.com/ctfer-io/chall-manager/api/v1/common"
	instance "github.com/ctfer-io/chall-manager/api/v1/instance"
	"github.com/ctfer-io/chall-manager/global"
	"github.com/ctfer-io/chall-manager/pkg/fs"
	"github.com/ctfer-io/chall-manager/pkg/iac"
	"github.com/ctfer-io/chall-manager/pkg/identity"
	"github.com/ctfer-io/chall-manager/pkg/lock"
	"github.com/ctfer-io/chall-manager/pkg/scenario"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (store *Store) UpdateChallenge(ctx context.Context, req *UpdateChallengeRequest) (*Challenge, error) {
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

	// 2. Lock RW challenge
	clock, err := common.LockChallenge(req.Id)
	if err != nil {
		logger.Error("build challenge lock", zap.Error(multierr.Combine(
			totw.RUnlock(),
			err,
		)))
		return nil, common.ErrInternal
	}
	defer common.LClose(clock)
	if err := clock.RWLock(); err != nil {
		logger.Error("challenge RW lock", zap.Error(multierr.Combine(
			totw.RUnlock(),
			err,
		)))
		return nil, common.ErrInternal
	}
	// don't defer unlock, will do it manually for ASAP challenge availability

	// 3. Unlock R TOTW
	if err := totw.RUnlock(); err != nil {
		logger.Error("TOTW R unlock", zap.Error(multierr.Combine(
			clock.RWUnlock(),
			err,
		)))
		return nil, common.ErrInternal
	}

	// 4. If challenge does not exist, return error (+ unlock RW challenge)
	challDir := filepath.Join(global.Conf.Directory, "chall", req.Id)
	fschall, err := fs.LoadChallenge(req.Id)
	if err != nil {
		return nil, err
	}

	// 5. Update challenge until/timeout and scenario on filesystem)
	nuntil, ntimeout := updateDates(req.Dates)
	updateUntil := nuntil != nil
	if updateUntil {
		fschall.Until = nuntil
	}
	updateTimeout := ntimeout != nil
	if updateTimeout {
		fschall.Timeout = ntimeout
	}
	updateScenario := req.Scenario != nil && fschall.Hash != hash(*req.Scenario)
	if updateScenario {
		// Remove old scenario
		if err := os.RemoveAll(fschall.Directory); err != nil {
			logger.Error("removing challenge directory", zap.String("id", req.Id), zap.Error(err))
			return nil, common.ErrInternal
		}
		// Decode new one
		dir, err := scenario.Decode(challDir, *req.Scenario)
		if err != nil {
			// XXX should check if fs-related error of due to invalid "scenario" content (i.e. should log or return the error)
			logger.Error("exporting scenario on filesystem", zap.Error(err))
			return nil, common.ErrInternal
		}
		// Save new directory (could change in the future, sets up a parachute) and hash
		fschall.Directory = dir
		fschall.Hash = hash(*req.Scenario)
	}

	fsb, _ := json.Marshal(fschall)
	if err := os.WriteFile(filepath.Join(challDir, "info.json"), fsb, 0644); err != nil {
		logger.Error("exporting challenge information to filesystem",
			zap.String("challenge_id", req.Id),
			zap.Error(err),
		)
		return nil, common.ErrInternal
	}

	// 6. Fetch challenge instances ids
	iids := []string{}
	dir, err := os.ReadDir(filepath.Join(challDir, "instance"))
	if err == nil {
		for _, dfs := range dir {
			iids = append(iids, dfs.Name())
		}
	}

	// 7. Create "relock" and "work" wait groups for all instance, and for each
	relock := &sync.WaitGroup{}
	relock.Add(len(iids))
	work := &sync.WaitGroup{}
	work.Add(len(iids))
	cerr := make(chan error, len(iids))
	cist := make(chan *instance.Instance, len(iids))
	for _, ist := range iids {
		go func(relock, work *sync.WaitGroup, cerr chan<- error, cist chan<- *instance.Instance, id string) {
			defer work.Done()

			// 8.a. Lock RW instance
			ilock, err := common.LockInstance(req.Id, id)
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
				if err := lock.RWUnlock(); err != nil {
					logger.Error("instance RW unlock", zap.Error(err))
				}
			}(ilock)

			// 8.b. done in the "relock" wait group
			relock.Done()

			fsist, err := fs.LoadInstance(req.Id, id)
			if err != nil {
				cerr <- err
				return
			}

			// 8.c. If until/timeout is not nil, update instance.until
			if updateUntil {
				fsist.Until = *fschall.Until
			}
			if updateTimeout {
				now := time.Now()
				// If instance already out of date, let it die
				if now.After(fsist.Until) {
					// new_until = last_renew + new_timeout
					// #1 timeout was 5, now 30:
					// last until = last renew + 5
					// new until = last renew + 30
					// #2 timeout was 30, now 5:
					// last until = last renew + 30
					// new until = last renew + 5
					// If new until is before now, instance will die
					fsist.Until = fsist.LastRenew.Add(*fschall.Timeout)
				}
			}

			// 8.d. If scenario is not nil, update it
			if updateScenario {
				id := identity.Compute(req.Id, id)
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

				logger.Info("updating instance",
					zap.String("source_id", id),
					zap.String("challenge_id", req.Id),
				)

				sr, err := stack.Up(ctx)
				if err != nil {
					cerr <- err
					return
				}

				if err := iac.Write(ctx, stack, sr, fsist); err != nil {
					cerr <- err
					return
				}
			}

			if err := fsist.Save(); err != nil {
				cerr <- err
				return
			}

			cist <- &instance.Instance{
				ChallengeId:    req.Id,
				SourceId:       id,
				Since:          timestamppb.New(fsist.Since),
				LastRenew:      timestamppb.New(fsist.LastRenew),
				Until:          timestamppb.New(fsist.Until),
				ConnectionInfo: fsist.ConnectionInfo,
				Flag:           fsist.Flag,
			}

			// 8.e. Unlock RW instance
			//      -> defered after 8.a. (fault-tolerance)
			// 8.f. done in the "work" wait group
			///     -> defered at the beginning of goroutine
		}(relock, work, cerr, cist, ist)
	}

	// 9. Once all "relock" done, unlock RW challenge
	relock.Wait()
	if err := clock.RWUnlock(); err != nil {
		logger.Error("challenge RW unlock", zap.Error(err))
		return nil, common.ErrInternal
	}

	// 10. Once all "work" done, return response or error if any
	work.Wait()
	close(cerr)
	close(cist)
	var merr error
	for err := range cerr {
		merr = multierr.Append(merr, err)
	}
	if err != nil {
		logger.Error("reading instances",
			zap.String("challenge_id", req.Id),
			zap.Error(err),
		)
		return nil, common.ErrInternal
	}

	if err := fschall.Save(); err != nil {
		logger.Error("exporting challenge information to filesystem",
			zap.String("challenge_id", req.Id),
			zap.Error(err),
		)
		return nil, common.ErrInternal
	}

	ists := make([]*instance.Instance, 0, len(iids))
	for ist := range cist {
		ists = append(ists, ist)
	}
	return &Challenge{
		Id:        req.Id,
		Hash:      fschall.Hash,
		Dates:     toDates(fschall.Until, fschall.Timeout),
		Instances: ists,
	}, nil
}

func updateDates(dates isUpdateChallengeRequest_Dates) (*time.Time, *time.Duration) {
	if until, ok := dates.(*UpdateChallengeRequest_Until); ok {
		t := until.Until.AsTime()
		return &t, nil
	}
	if timeout, ok := dates.(*UpdateChallengeRequest_Timeout); ok {
		d := timeout.Timeout.AsDuration()
		return nil, &d
	}
	return nil, nil
}
