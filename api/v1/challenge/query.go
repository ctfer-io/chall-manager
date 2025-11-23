package challenge

import (
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ctfer-io/chall-manager/api/v1/common"
	"github.com/ctfer-io/chall-manager/api/v1/instance"
	"github.com/ctfer-io/chall-manager/global"
	errs "github.com/ctfer-io/chall-manager/pkg/errors"
	"github.com/ctfer-io/chall-manager/pkg/fs"
	"github.com/ctfer-io/chall-manager/pkg/lock"
)

func (store *Store) QueryChallenge(_ *emptypb.Empty, server ChallengeStore_QueryChallengeServer) error {
	logger := global.Log()
	ctx := server.Context()
	span := trace.SpanFromContext(ctx)

	// 1. Lock RW TOTW
	span.AddEvent("lock TOTW")
	totw, err := common.LockTOTW(ctx)
	if err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "build TOTW lock", zap.Error(err))
		return errs.ErrLockUnavailable
	}
	defer common.LClose(totw)
	if err := totw.RWLock(ctx); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "TOTW RW lock", zap.Error(err))
		return errs.ErrLockUnavailable
	}
	span.AddEvent("locked TOTW")

	// 2. Fetch all challenges
	ids, err := fs.ListChallenges()
	if err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "listing challenges", zap.Error(err))
		return errs.ErrInternalNoSub
	}

	// 3. Create "relock" and "work" wait groups for all challenges, and for each
	qs := common.NewQueryServer[*Challenge](server)
	relock := &sync.WaitGroup{}
	relock.Add(len(ids))
	work := &sync.WaitGroup{}
	work.Add(len(ids))
	cerr := make(chan error, len(ids))
	for _, id := range ids {
		go func(relock, work *sync.WaitGroup, cerr chan<- error, id string) {
			// Track span of loading stack
			ctx, span := global.Tracer.Start(ctx, "reading-challenge", trace.WithAttributes(
				attribute.String("challenge_id", id),
			))
			defer span.End()

			// 4.d. done in the "work" wait group
			defer work.Done()
			ctx = global.WithChallengeID(ctx, id)

			// 4.a. Lock R challenge
			clock, err := common.LockChallenge(ctx, id)
			if err != nil {
				cerr <- errs.ErrLockUnavailable
				relock.Done() // release to avoid dead-lock
				return
			}
			defer common.LClose(clock)
			if err := clock.RLock(ctx); err != nil {
				cerr <- errs.ErrLockUnavailable
				relock.Done() // release to avoid dead-lock
				return
			}
			defer func(lock lock.RWLock) {
				// 4.e. Unlock R challenge
				if err := lock.RUnlock(ctx); err != nil {
					err := &errs.ErrInternal{Sub: err}
					logger.Error(ctx, "challenge RW unlock", zap.Error(err))
				}
			}(clock)

			// 4.b. done in the "relock" wait group
			relock.Done()

			// 4.c. Read challenge info
			fschall, err := fs.LoadChallenge(id)
			if err != nil {
				cerr <- errs.ErrValidationFailed{Reason: err.Error()}
				return
			}

			// 4.d. Fetch challenge instances
			//      (don't lock and access concurrently, most probably fast enough even at scale)
			//      (if required to perform concurrently, no breaking change so LGTM)
			clmIsts := map[string]string{}
			ists, err := fs.ListInstances(id)
			if err != nil {
				cerr <- errs.ErrInternalNoSub
				return
			}
			for _, ist := range ists {
				src, err := fs.LookupClaim(id, ist)
				if err != nil {
					// in pool
					continue
				}
				clmIsts[src] = ist
			}
			oists := make([]*instance.Instance, 0, len(clmIsts))
			for sourceID, identity := range clmIsts {
				fsist, err := fs.LoadInstance(id, identity)
				if err != nil {
					cerr <- err
					return
				}

				var until *timestamppb.Timestamp
				if fsist.Until != nil {
					until = timestamppb.New(*fsist.Until)
				}
				oists = append(oists, &instance.Instance{
					ChallengeId:    id,
					SourceId:       sourceID,
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
				})
			}

			if err := qs.SendMsg(&Challenge{
				Id:         id,
				Scenario:   fschall.Scenario,
				Timeout:    toPBDuration(fschall.Timeout),
				Until:      toPBTimestamp(fschall.Until),
				Instances:  oists,
				Additional: fschall.Additional,
				Min:        fschall.Min,
				Max:        fschall.Max,
			}); err != nil {
				cerr <- err
				return
			}
		}(relock, work, cerr, id)
	}

	// 5. Once all "relock" done, unlock RW TOTW
	relock.Wait()
	if err := totw.RWUnlock(ctx); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "TOTW RW unlock", zap.Error(err))
		return errs.ErrInternalNoSub
	}
	span.AddEvent("unlocked TOTW")

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
