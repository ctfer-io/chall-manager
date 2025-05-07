package challenge

import (
	context "context"
	"sync"

	json "github.com/goccy/go-json"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
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
	ctx = global.WithChallengeID(ctx, req.Id)
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
	fschall, err := fs.LoadChallenge(req.Id)
	if err != nil {
		if err, ok := err.(*errs.ErrInternal); ok {
			logger.Error(ctx, "reading challenge from filesystem",
				zap.Error(multierr.Combine(
					clock.RWUnlock(ctx),
					err,
				)),
			)
			return nil, errs.ErrInternalNoSub
		}
		if err := clock.RWUnlock(ctx); err != nil {
			logger.Error(ctx, "reading challenge from filesystem",
				zap.Error(clock.RWUnlock(ctx)),
			)
		}
		return nil, err
	}

	// 5. Create "relock" and "work" wait groups for all instances, and for each
	ists, err := fs.ListInstances(req.Id)
	if err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "listing instances",
			zap.Error(multierr.Combine(
				clock.RWUnlock(ctx),
				err,
			)),
		)
		return nil, errs.ErrInternalNoSub
	}

	logger.Info(ctx, "deleting challenge",
		zap.Int("instances", len(ists)),
	)
	relock := &sync.WaitGroup{} // track goroutines that overlocked an identity
	relock.Add(len(ists))
	work := &sync.WaitGroup{} // track goroutines that ended dealing with the instances
	work.Add(len(ists))
	cerr := make(chan error, len(ists))
	for _, identity := range ists {
		go func(relock, work *sync.WaitGroup, cerr chan<- error, identity string) {
			// 6.e. done in the "work" wait group
			defer work.Done()

			// 6.a. Lock RW instance
			ilock, err := common.LockInstance(req.Id, identity)
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
				// 6.d. Unlock RW instance
				if err := lock.RWUnlock(ctx); err != nil {
					err := &errs.ErrInternal{Sub: err}
					logger.Error(ctx, "instance RW unlock", zap.Error(err))
				}
			}(ilock)

			// 6.b. done in the "relock" wait group
			relock.Done()

			// 6.c. delete it
			fsist, err := fs.LoadInstance(req.Id, identity)
			if err != nil {
				cerr <- err
				return
			}

			stack, err := iac.LoadStack(ctx, fschall.Directory, identity)
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

			common.InstancesUDCounter().Add(ctx, -1)
		}(relock, work, cerr, identity)
	}

	// 7. Once all "relock" done, unlock RW challenge
	relock.Wait()
	if err := clock.RWUnlock(ctx); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "challenge RW unlock", zap.Error(err))
		return nil, errs.ErrInternalNoSub
	}

	// 8. Once all "work" done, return response or error if any
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
		logger.Error(ctx, "reading instances",
			zap.Error(merri),
		)
		if err := fschall.Delete(); err != nil {
			logger.Error(ctx, "removing challenge directory",
				zap.Error(err),
			)
			return nil, errs.ErrInternalNoSub
		}
		return nil, errs.ErrInternalNoSub
	}
	if merr != nil {
		return nil, merr
	}

	if err := fschall.Delete(); err != nil {
		logger.Error(ctx, "removing challenge directory",
			zap.Error(err),
		)
		return nil, errs.ErrInternalNoSub
	}

	logger.Info(ctx, "challenge deleted successfully")
	common.ChallengesUDCounter().Add(ctx, -1)

	return nil, nil
}
