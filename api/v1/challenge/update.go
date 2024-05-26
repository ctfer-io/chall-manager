package challenge

import (
	context "context"
	"os"
	"path/filepath"
	"strings"
	sync "sync"
	"time"

	"github.com/ctfer-io/chall-manager/api/v1/common"
	instance "github.com/ctfer-io/chall-manager/api/v1/instance"
	"github.com/ctfer-io/chall-manager/global"
	errs "github.com/ctfer-io/chall-manager/pkg/errors"
	"github.com/ctfer-io/chall-manager/pkg/fs"
	"github.com/ctfer-io/chall-manager/pkg/iac"
	"github.com/ctfer-io/chall-manager/pkg/identity"
	"github.com/ctfer-io/chall-manager/pkg/lock"
	"github.com/ctfer-io/chall-manager/pkg/scenario"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (store *Store) UpdateChallenge(ctx context.Context, req *UpdateChallengeRequest) (*Challenge, error) {
	logger := global.Log()
	ctx = global.WithChallengeId(ctx, req.Id)

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

	// 2. Lock RW challenge
	clock, err := common.LockChallenge(ctx, req.Id)
	if err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "build challenge lock", zap.Error(multierr.Combine(
			totw.RUnlock(),
			err,
		)))
		return nil, errs.ErrInternalNoSub
	}
	defer common.LClose(clock)
	if err := clock.RWLock(); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "challenge RW lock", zap.Error(multierr.Combine(
			totw.RUnlock(),
			err,
		)))
		return nil, errs.ErrInternalNoSub
	}
	// don't defer unlock, will do it manually for ASAP challenge availability

	// 3. Unlock R TOTW
	if err := totw.RUnlock(); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "TOTW R unlock", zap.Error(multierr.Combine(
			clock.RWUnlock(),
			err,
		)))
		return nil, errs.ErrInternalNoSub
	}

	// 4. If challenge does not exist, return error (+ unlock RW challenge)
	challDir := fs.ChallengeDirectory(req.Id)
	fschall, err := fs.LoadChallenge(req.Id)
	if err != nil {
		if err, ok := err.(*errs.ErrInternal); ok {
			logger.Error(ctx, "loading challenge",
				zap.Error(multierr.Combine(
					clock.RWUnlock(),
					err,
				)),
			)
			return nil, errs.ErrInternalNoSub
		}
		return nil, err
	}

	// 5. Update challenge until/timeout and scenario on filesystem
	fschall.Until, fschall.Timeout = updateDates(req.Dates)
	updateScenario := req.Scenario != nil && fschall.Hash != hash(*req.Scenario)
	var oldDir *string
	if updateScenario {
		// Decode new one
		dir, err := scenario.Decode(ctx, challDir, *req.Scenario)
		if err != nil {
			// Avoid flooding the filesystem
			if err := os.RemoveAll(dir); err != nil {
				err := &errs.ErrInternal{Sub: err}
				logger.Error(ctx, "removing challenge directory",
					zap.Error(err),
				)
			}
			if err, ok := err.(*errs.ErrInternal); ok {
				logger.Error(ctx, "exporting scenario on filesystem",
					zap.Error(err),
				)
				return nil, errs.ErrInternalNoSub
			}
			return nil, err
		}

		// Save new directory (could change in the future, sets up a parachute) and hash
		oldDir, fschall.Directory = ptr(filepath.Join(challDir, strings.Split(fschall.Directory, "/")[6])), dir
		fschall.Hash = hash(*req.Scenario)
	}

	// Tend to transactional operation, try to delete whatever happened
	if oldDir != nil {
		if err := os.RemoveAll(*oldDir); err != nil {
			err := &errs.ErrInternal{Sub: err}
			logger.Error(ctx, "removing challenge old directory",
				zap.Error(err),
			)
		}
	}

	if err := fschall.Save(); err != nil {
		if err := clock.RWUnlock(); err != nil {
			logger.Error(ctx, "challenge RW unlock", zap.Error(err))
		}
		if err, ok := err.(*errs.ErrInternal); ok {
			logger.Error(ctx, "exporting challenge information to filesystem",
				zap.Error(err),
			)
			return nil, errs.ErrInternalNoSub
		}
		return nil, err
	}

	// 6. Fetch challenge instances ids
	iids, err := fs.ListInstances(req.Id)
	if err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "listing instances",
			zap.Error(err),
		)
		return nil, errs.ErrInternalNoSub
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
			ctx := global.WithSourceId(ctx, id)

			// 8.a. Lock RW instance
			ilock, err := common.LockInstance(ctx, req.Id, id)
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
					err := &errs.ErrInternal{Sub: err}
					logger.Error(ctx, "instance RW unlock", zap.Error(err))
				}
			}(ilock)

			// 8.b. done in the "relock" wait group
			relock.Done()

			fsist, err := fs.LoadInstance(req.Id, id)
			if err != nil {
				cerr <- err
				return
			}

			// 8.c. Mirror instance's "until" based on the challenge
			if fschall.Until != nil {
				fsist.Until = fschall.Until
			}
			if fschall.Timeout != nil {
				// Proceed as for an instance renew: until = last_renew+timeout iif current until is not before now
				now := time.Now()
				if fsist.Until == nil || fsist.Until.After(now) {
					u := fsist.LastRenew.Add(*fschall.Timeout)
					fsist.Until = &u
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
				if err := stack.SetAllConfig(ctx, auto.ConfigMap{
					"identity": auto.ConfigValue{
						Value: id,
					},
				}); err != nil {
					cerr <- err
					return
				}

				logger.Info(ctx, "updating instance")

				// Make sure to extract the state whatever happen, or at least try and store
				// it in the FS Instance.
				sr, err := stack.Up(ctx)
				if nerr := iac.Extract(ctx, stack, sr, fsist); nerr != nil {
					if fserr := fsist.Save(); fserr != nil {
						cerr <- multierr.Combine(err, nerr, fserr)
						return
					}
					cerr <- multierr.Combine(err, nerr)
					return
				}
				if err != nil {
					if fserr := fsist.Save(); fserr != nil {
						cerr <- multierr.Combine(err, fserr)
						return
					}
					cerr <- err
					return
				}
			}

			if err := fsist.Save(); err != nil {
				cerr <- err
				return
			}

			var until *timestamppb.Timestamp
			if fsist.Until != nil {
				until = timestamppb.New(*fsist.Until)
			}
			cist <- &instance.Instance{
				ChallengeId:    req.Id,
				SourceId:       id,
				Since:          timestamppb.New(fsist.Since),
				LastRenew:      timestamppb.New(fsist.LastRenew),
				Until:          until,
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
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "challenge RW unlock", zap.Error(err))
		return nil, errs.ErrInternalNoSub
	}

	// 10. Once all "work" done, return response or error if any
	work.Wait()
	close(cerr)
	close(cist)
	var merri, merr error
	for err := range cerr {
		if err, ok := err.(*errs.ErrInternal); ok {
			merri = multierr.Append(merri, err)
			continue
		}
		merr = multierr.Append(merr, err)
	}
	if merri != nil {
		logger.Error(ctx, "updating challenge and its instances",
			zap.Error(err),
		)
		return nil, errs.ErrInternalNoSub
	}
	if merr != nil {
		return nil, merr
	}

	if err := fschall.Save(); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "exporting challenge information to filesystem",
			zap.Error(err),
		)
		return nil, errs.ErrInternalNoSub
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

func ptr[T any](t T) *T {
	return &t
}
