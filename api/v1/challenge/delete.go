package challenge

import (
	context "context"
	"sync"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/ctfer-io/chall-manager/api/v1/common"
	"github.com/ctfer-io/chall-manager/global"
	errs "github.com/ctfer-io/chall-manager/pkg/errors"
	"github.com/ctfer-io/chall-manager/pkg/fs"
	"github.com/ctfer-io/chall-manager/pkg/iac"
	"github.com/ctfer-io/chall-manager/pkg/lock"
)

func (store *Store) DeleteChallenge(ctx context.Context, req *DeleteChallengeRequest) (*emptypb.Empty, error) {
	logger := global.Log()
	ctx = global.WithChallengeID(ctx, req.GetId())
	span := trace.SpanFromContext(ctx)

	// 1. Lock R TOTW
	span.AddEvent("lock TOTW")
	totw, err := common.LockTOTW(ctx)
	if err != nil {
		if totw.IsCanceled(err) {
			return nil, errs.ErrCanceled
		}
		logger.Error(ctx, "build TOTW lock", zap.Error(err))
		return nil, errs.ErrInternalNoSub
	}
	if err := totw.RLock(ctx); err != nil {
		if totw.IsCanceled(err) {
			return nil, errs.ErrCanceled
		}
		logger.Error(ctx, "TOTW R lock", zap.Error(err))
		return nil, errs.ErrInternalNoSub
	}
	span.AddEvent("locked TOTW")

	// 2. Lock RW challenge
	clock, err := common.LockChallenge(ctx, req.GetId())
	if err != nil {
		if clock.IsCanceled(err) {
			// If canceled, we need to recover
			if err := totw.RUnlock(context.WithoutCancel(ctx)); err != nil {
				logger.Error(ctx, "recovering from build challenge lock", zap.Error(err))
				return nil, errs.ErrInternalNoSub
			}
			return nil, errs.ErrCanceled // recovery is successful, we can quit safely
		}
		logger.Error(ctx, "build challenge lock", zap.Error(multierr.Combine(
			totw.RUnlock(context.WithoutCancel(ctx)),
			err,
		)))
		return nil, errs.ErrInternalNoSub
	}
	if err := clock.RWLock(ctx); err != nil {
		if clock.IsCanceled(err) {
			// If canceled, we need to recover
			if err := totw.RUnlock(context.WithoutCancel(ctx)); err != nil {
				logger.Error(ctx, "recovering from challenge RW lock", zap.Error(err))
				return nil, errs.ErrInternalNoSub
			}
			return nil, errs.ErrCanceled // recovery is successful, we can quit safely
		}
		logger.Error(ctx, "challenge RW lock", zap.Error(multierr.Combine(
			totw.RUnlock(context.WithoutCancel(ctx)),
			err,
		)))
		return nil, errs.ErrInternalNoSub
	}
	defer func(lock lock.RWLock) {
		if err := lock.RWUnlock(context.WithoutCancel(ctx)); err != nil {
			logger.Error(ctx, "challenge RW unlock", zap.Error(err))
		}
	}(clock)

	// 3. Unlock R TOTW
	if err := totw.RUnlock(context.WithoutCancel(ctx)); err != nil {
		logger.Error(ctx, "TOTW R unlock", zap.Error(err))
		return nil, errs.ErrInternalNoSub
	}
	span.AddEvent("unlocked TOTW")

	// 4. If challenge does not exist, return error (+ unlock RW challenge)
	fschall, err := fs.LoadChallenge(req.GetId())
	if err != nil {
		// If challenge not found
		if _, ok := err.(*errs.ChallengeExist); ok {
			return nil, err
		}
		// Else deal with it as an internal server error
		logger.Error(ctx, "loading challenge",
			zap.Error(err),
		)
		return nil, errs.ErrInternalNoSub
	}

	// 5. Create "relock" and "work" wait groups for all instances, and for each
	ists, err := fs.ListInstances(req.GetId())
	if err != nil {
		logger.Error(ctx, "listing instances", zap.Error(err))
		return nil, errs.ErrInternalNoSub
	}

	logger.Info(ctx, "deleting challenge",
		zap.Int("instances", len(ists)),
	)
	work := &sync.WaitGroup{} // track goroutines that ended dealing with the instances
	work.Add(len(ists))
	cerr := make(chan error, len(ists))
	for _, identity := range ists {
		go func(work *sync.WaitGroup, cerr chan<- error, identity string) {
			// 6.b. done in the "work" wait group
			defer work.Done()

			// 6.a. delete it
			fsist, err := fs.LoadInstance(req.GetId(), identity)
			if err != nil {
				cerr <- err
				return
			}

			stack, err := iac.LoadStack(ctx, fschall.Scenario, identity)
			if err != nil {
				cerr <- err
				return
			}

			err = stack.Down(ctx)
			// Don't return it fast else it won't update metrics

			sourceID, lerr := fs.LookupClaim(fsist.ChallengeID, fsist.Identity)
			if err != nil {
				if serr, ok := err.(*errs.InstanceExist); ok && !serr.Exist {
					// It's fine, the instance is in pool!
				} else {
					err = multierr.Combine(err, lerr)
				}
			}

			cerr <- err

			common.InstancesUDCounter().Add(ctx, -1,
				metric.WithAttributeSet(common.InstanceAttrs(req.GetId(), sourceID, sourceID != "")),
			)
		}(work, cerr, identity)
	}

	// 7. Once all "work" done, return response or error if any
	work.Wait()
	close(cerr)
	var merr error
	for err := range cerr {
		merr = multierr.Append(merr, err)
	}
	merr = multierr.Combine(merr, fschall.Delete())
	if merr != nil {
		logger.Error(ctx, "removing challenge directory",
			zap.Error(merr),
		)
		return nil, errs.ErrInternalNoSub
	}

	logger.Info(ctx, "challenge deleted successfully")
	common.ChallengesUDCounter().Add(ctx, -1)

	return nil, nil
}
