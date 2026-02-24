package instance

import (
	"context"
	sync "sync"

	"github.com/ctfer-io/chall-manager/api/v1/common"
	"github.com/ctfer-io/chall-manager/global"
	errs "github.com/ctfer-io/chall-manager/pkg/errors"
	"github.com/ctfer-io/chall-manager/pkg/fs"
	"github.com/ctfer-io/chall-manager/pkg/lock"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (man *Manager) QueryInstance(req *QueryInstanceRequest, server InstanceManager_QueryInstanceServer) error {
	logger := global.Log()
	ctx := server.Context()
	span := trace.SpanFromContext(ctx)

	// 1. Lock RW TOTW -> R should be sufficient, but we want this query to be as fast as possible
	span.AddEvent("lock TOTW")
	totw, err := common.LockTOTW(ctx)
	if err != nil {
		if totw.IsCanceled(err) {
			return errs.ErrCanceled
		}
		logger.Error(ctx, "build TOTW lock", zap.Error(err))
		return errs.ErrInternalNoSub
	}
	if err := totw.RWLock(ctx); err != nil {
		if totw.IsCanceled(err) {
			return errs.ErrCanceled
		}
		logger.Error(ctx, "TOTW RW lock", zap.Error(err))
		return errs.ErrInternalNoSub
	}
	span.AddEvent("locked TOTW")

	// 2. Fetch all challenges
	fschalls, err := fs.ListChallenges()
	if err != nil {
		logger.Error(ctx, "listing challenges", zap.Error(multierr.Combine(
			err,
			totw.RWUnlock(context.WithoutCancel(ctx)),
		)))
		return errs.ErrInternalNoSub
	}

	// 3. Create "relock" and "work" wait groups for all challenges, and for each
	qs := common.NewQueryServer[*Instance](server)
	relock := &sync.WaitGroup{}
	relock.Add(len(fschalls))
	work := &sync.WaitGroup{}
	work.Add(len(fschalls))
	cerr := make(chan error, len(fschalls))
	for _, challengeID := range fschalls {
		work.Go(func() {
			// Track span of loading stack
			ctx, span := global.Tracer.Start(ctx, "reading-challenge", trace.WithAttributes(
				attribute.String("challenge_id", challengeID),
			))
			defer span.End()

			ctx = global.WithChallengeID(ctx, challengeID)

			// 4.a. Lock R challenge
			clock, err := common.LockChallenge(ctx, challengeID)
			if err != nil {
				if clock.IsCanceled(err) {
					err = nil
				}
				cerr <- err
				relock.Done() // release to avoid dead-lock
				return
			}
			if err := clock.RLock(ctx); err != nil {
				if clock.IsCanceled(err) {
					err = nil
				}
				cerr <- err
				relock.Done() // release to avoid dead-lock
				return
			}
			defer func(lock lock.RWLock) {
				if err := lock.RUnlock(context.WithoutCancel(ctx)); err != nil {
					logger.Error(ctx, "challenge R unlock", zap.Error(err))
				}
			}(clock)

			// 4.b. Done in the "relock wait group"
			relock.Done()

			// 4.c. Fetch challenge instance for this source
			ist, err := fs.FindInstance(challengeID, req.GetSourceId())
			if err != nil {
				if _, ok := err.(*errs.InstanceExist); ok {
					err = nil // no instance was claimed by this source, skip it
				}
				cerr <- err
				return
			}

			fsist, err := fs.LoadInstance(challengeID, ist)
			if err != nil {
				cerr <- err
				return
			}

			var until *timestamppb.Timestamp
			if fsist.Until != nil {
				until = timestamppb.New(*fsist.Until)
			}

			if err := qs.SendMsg(&Instance{
				ChallengeId:    challengeID,
				SourceId:       req.GetSourceId(),
				Since:          timestamppb.New(fsist.Since),
				LastRenew:      timestamppb.New(fsist.LastRenew),
				Until:          until,
				ConnectionInfo: fsist.ConnectionInfo,
				Flag: func() *string { // kept for retrocompatibility enough time for public migration
					if len(fsist.Flags) == 1 {
						return &fsist.Flags[0]
					}
					return nil
				}(),
				Flags:      fsist.Flags,
				Additional: fsist.Additional,
			}); err != nil {
				cerr <- err
				return
			}
		})
	}

	// 5. Once all "relock" done, unlock RW TOTW
	relock.Wait()
	if err := totw.RWUnlock(context.WithoutCancel(ctx)); err != nil {
		logger.Error(ctx, "TOTW RW unlock", zap.Error(err))
		span.RecordError(err)
		// don't return now to avoid having working goroutines after request completion (zombies)
	} else {
		span.AddEvent("unlocked TOTW")
	}

	// 6. Once all "work" done, return error if any
	work.Wait()
	close(cerr)
	var merr error
	for err := range cerr {
		merr = multierr.Append(merr, err)
	}
	if merr != nil {
		logger.Error(ctx, "reading challenge instances", zap.Error(merr))
		return errs.ErrInternalNoSub
	}
	return nil
}
