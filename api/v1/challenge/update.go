package challenge

import (
	"context"
	"os"
	"slices"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ctfer-io/chall-manager/api/v1/common"
	"github.com/ctfer-io/chall-manager/api/v1/instance"
	"github.com/ctfer-io/chall-manager/global"
	errs "github.com/ctfer-io/chall-manager/pkg/errors"
	"github.com/ctfer-io/chall-manager/pkg/fs"
	"github.com/ctfer-io/chall-manager/pkg/iac"
	"github.com/ctfer-io/chall-manager/pkg/lock"
	"github.com/ctfer-io/chall-manager/pkg/scenario"
)

func (store *Store) UpdateChallenge(ctx context.Context, req *UpdateChallengeRequest) (*Challenge, error) {
	logger := global.Log()
	ctx = global.WithChallengeId(ctx, req.Id)
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
	span.AddEvent("lock challenge")
	clock, err := common.LockChallenge(req.Id)
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
	span.AddEvent("locked challenge")
	// don't defer unlock, will do it manually for ASAP challenge availability

	// 3. Unlock R TOTW
	if err := totw.RUnlock(ctx); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "TOTW R unlock", zap.Error(multierr.Combine(
			clock.RWUnlock(ctx),
			err,
		)))
		return nil, errs.ErrInternalNoSub
	}
	span.AddEvent("unlocked TOTW")

	// 4. If challenge does not exist, return error (+ unlock RW challenge)
	challDir := fs.ChallengeDirectory(req.Id)
	fschall, err := fs.LoadChallenge(req.Id)
	if err != nil {
		if err, ok := err.(*errs.ErrInternal); ok {
			logger.Error(ctx, "loading challenge",
				zap.Error(multierr.Combine(
					clock.RWUnlock(ctx),
					err,
				)),
			)
			return nil, errs.ErrInternalNoSub
		}
		return nil, err
	}

	// 5. Update challenge until/timeout and scenario on filesystem
	um := req.GetUpdateMask()
	if um.IsValid(req) {
		if slices.Contains(um.Paths, "until") {
			if req.Until != nil {
				fschall.Until = toTime(req.Until)
			} else {
				fschall.Until = nil
			}
		}
		if slices.Contains(um.Paths, "timeout") {
			if req.Timeout != nil {
				fschall.Timeout = toDuration(req.Timeout)
			} else {
				fschall.Timeout = nil
			}
		}
	}

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
			if _, ok := err.(*errs.ErrScenario); ok {
				logger.Error(ctx, "invalid scenario", zap.Error(err))
				return nil, errs.ErrScenarioNoSub
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
		// Use "ptr" rather than "&" to avoid confusions, else oldDir and fschall.Directory will be the same
		oldDir, fschall.Directory = ptr(fschall.Directory), dir
		fschall.Hash = hash(*req.Scenario)
	}

	if err := fschall.Save(); err != nil {
		if err := clock.RWUnlock(ctx); err != nil {
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
	logger.Info(ctx, "updating challenge",
		zap.Int("instances", len(iids)),
		zap.Bool("update_scenario", updateScenario),
	)
	relock := &sync.WaitGroup{}
	relock.Add(len(iids))
	work := &sync.WaitGroup{}
	work.Add(len(iids))
	cerr := make(chan error, len(iids))
	cist := make(chan *instance.Instance, len(iids))
	for _, ist := range iids {
		go func(relock, work *sync.WaitGroup, cerr chan<- error, cist chan<- *instance.Instance, id string) {
			// Track span of loading stack
			ctx, span := global.Tracer.Start(ctx, "updating-instance", trace.WithAttributes(
				attribute.String("source_id", id),
			))
			defer span.End()

			defer work.Done()
			ctx = global.WithSourceId(ctx, id)

			// 8.a. Lock RW instance
			ilock, err := common.LockInstance(req.Id, id)
			if err != nil {
				cerr <- err
				relock.Done() // release to avoid dead-lock
				return
			}
			defer common.LClose(ilock)
			if err := ilock.RWLock(ctx); err != nil {
				cerr <- err
				relock.Done() // release to avoid dead-lock
				return
			}
			defer func(lock lock.RWLock) {
				if err := lock.RWUnlock(ctx); err != nil {
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
			fsist.Until = common.ComputeUntil(fschall.Until, fschall.Timeout)

			// 8.d. If scenario is not nil, update it
			if updateScenario {
				if err := iac.Update(ctx, *oldDir, req.UpdateStrategy.String(), fschall, fsist); err != nil {
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
	if err := clock.RWUnlock(ctx); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "challenge RW unlock", zap.Error(err))
		return nil, errs.ErrInternalNoSub
	}
	span.AddEvent("unlocked challenge")

	// 10. Once all "work" done, return response or error if any
	work.Wait()

	// Tend to transactional operation, try to delete whatever happened
	if oldDir != nil {
		if err := os.RemoveAll(*oldDir); err != nil {
			err := &errs.ErrInternal{Sub: err}
			logger.Error(ctx, "removing challenge old directory",
				zap.Error(err),
			)
		}
	}

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

	logger.Info(ctx, "challenge updated successfully")

	ists := make([]*instance.Instance, 0, len(iids))
	for ist := range cist {
		ists = append(ists, ist)
	}
	return &Challenge{
		Id:        req.Id,
		Hash:      fschall.Hash,
		Timeout:   toPBDuration(fschall.Timeout),
		Until:     toPBTimestamp(fschall.Until),
		Instances: ists,
	}, nil
}

func ptr[T any](t T) *T {
	return &t
}
