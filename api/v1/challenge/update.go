package challenge

import (
	"context"
	"maps"
	"os"
	"slices"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
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
	"github.com/ctfer-io/chall-manager/pkg/pool"
)

func (store *Store) UpdateChallenge(ctx context.Context, req *UpdateChallengeRequest) (*Challenge, error) {
	logger := global.Log()
	ctx = global.WithChallengeID(ctx, req.GetId())
	span := trace.SpanFromContext(ctx)

	// 0. Validate request
	um := req.GetUpdateMask()
	if err := common.CheckUpdateMask(um, req); err != nil {
		return nil, err
	}
	// => Pooler boundaries defaults to 0, with proper ordering
	if err := common.CheckPooler(um.GetPaths(), req.GetMin(), req.GetMax()); err != nil {
		return nil, err
	}

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
	span.AddEvent("lock challenge")
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
	span.AddEvent("locked challenge")
	defer func(lock lock.RWLock) {
		if err := lock.RWUnlock(context.WithoutCancel(ctx)); err != nil {
			logger.Error(ctx, "challenge RW unlock", zap.Error(err))
		}
	}(clock)

	// 3. Unlock R TOTW
	if err := totw.RUnlock(context.WithoutCancel(ctx)); err != nil {
		logger.Error(ctx, "TOTW R unlock",
			zap.Error(err),
		)
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

	// 5. Update challenge until/timeout, pooler, or scenario on filesystem
	updateScenario := false
	updateAdditional := false
	if slices.Contains(um.GetPaths(), "scenario") {
		equals, err := global.GetOCIManager().Equals(ctx, fschall.Scenario, req.GetScenario())
		if err != nil {
			logger.Error(ctx, "comparing scenarios",
				zap.Error(err),
			)
			return nil, errs.ErrInternalNoSub
		}
		updateScenario = !equals
	}
	if slices.Contains(um.GetPaths(), "until") {
		fschall.Until = toTime(req.GetUntil())
	}
	if slices.Contains(um.GetPaths(), "timeout") {
		fschall.Timeout = toDuration(req.GetTimeout())
	}
	if slices.Contains(um.GetPaths(), "additional") {
		updateAdditional = !maps.Equal(fschall.Additional, req.GetAdditional())
		fschall.Additional = req.GetAdditional()
	}
	if slices.Contains(um.GetPaths(), "min") {
		fschall.Min = req.GetMin()
	}
	if slices.Contains(um.GetPaths(), "max") {
		fschall.Max = req.GetMax()
	}

	// XXX a different scenario reference is not sufficient as the additional can guide variability
	// (e.g., generic scenario into others paths that might fail)
	var oldScn *string
	if updateScenario {
		oldScn, fschall.Scenario = &fschall.Scenario, req.GetScenario()

		if err := common.Validate(ctx, req.GetScenario(), fschall.Additional); err != nil {
			return nil, err // already handled by the helper
		}
	}

	// 7. Create "work" and "updated" wait groups for all instances and for all claimed
	logger.Info(ctx, "updating challenge",
		zap.Bool("scenario", updateScenario),
		zap.Bool("additional", updateAdditional),
	)
	span.AddEvent("beginning update")

	ists, err := fs.ListInstances(req.GetId())
	if err != nil {
		logger.Error(ctx, "listing instances",
			zap.Error(err),
		)
		return nil, errs.ErrInternalNoSub
	}
	claimed := []string{}
	pooled := []string{}
	for _, ist := range ists {
		_, err := fs.LookupClaim(req.GetId(), ist)
		if os.IsNotExist(err) {
			pooled = append(pooled, ist)
			continue
		}
		if err == nil {
			claimed = append(claimed, ist)
			continue
		}

		logger.Error(ctx, "looking up for claim",
			zap.Error(err),
		)
		return nil, errs.ErrInternalNoSub
	}

	delta := pool.NewDelta(fschall.Min, fschall.Max, int64(len(claimed)), int64(len(pooled)))
	size := len(ists)

	claimedAfterUpdate := make([]string, 0, len(claimed))
	work := &sync.WaitGroup{}
	work.Add(size)
	cerr := make(chan error, size)
	for _, identity := range claimed {
		sourceID, err := fs.LookupClaim(req.GetId(), identity)
		if err != nil {
			// No error should happen as the instance is supposed to be claimed.
			// Send it over the chan in the work group to avoid waiting indefinitely.
			// This skips the normal work that induce a LOT of work (especially the deal with locks).
			work.Go(func() {
				cerr <- err
			})
			continue
		}
		work.Go(func() {
			// Track span of loading stack
			ctx, span := global.Tracer.Start(ctx, "updating-instance", trace.WithAttributes(
				attribute.String("source_id", sourceID),
				attribute.String("identity", identity),
			))
			defer span.End()

			ctx = global.WithSourceID(ctx, sourceID)
			ctx = global.WithIdentity(ctx, identity)

			// 8.a. Lock RW instance
			ilock, err := common.LockInstance(ctx, req.GetId(), identity)
			if err != nil {
				if ilock.IsCanceled(err) {
					err = nil
				}
				cerr <- err
				return
			}
			if err := ilock.RWLock(ctx); err != nil {
				if ilock.IsCanceled(err) {
					err = nil
				}
				cerr <- err
				return
			}
			defer func(lock lock.RWLock) {
				if err := lock.RWUnlock(context.WithoutCancel(ctx)); err != nil {
					logger.Error(ctx, "instance RW unlock", zap.Error(err))
				}
			}(ilock)

			fsist, err := fs.LoadInstance(req.GetId(), identity)
			if err != nil {
				cerr <- err
				return
			}

			// 8.c. Mirror instance's "until" based on the challenge
			fsist.Until = common.ComputeUntil(fschall.Until, fschall.Timeout)

			// 8.d. If scenario is not nil, update it
			scn := fschall.Scenario
			if updateScenario {
				scn = *oldScn
			}

			// Keep track of who is the owner of the instance
			oldID := fsist.Identity

			// Then update if necessary
			var uerr error
			if updateScenario || updateAdditional {
				uerr = iac.Update(ctx, scn, req.GetUpdateStrategy().String(), fschall, fsist)
			}

			// Save potentially updated instance
			newIst := fsist.Identity
			ferr := fsist.Save()

			// (Re-)claim the instance (e.g. can be another one with recreate)
			lerr := fsist.Claim(sourceID)

			claimedAfterUpdate = append(claimedAfterUpdate, newIst)

			// TODO add meter for live-updates

			if err := multierr.Combine(uerr, ferr, lerr); err != nil {
				cerr <- err
				return
			}

			if oldID != newIst {
				// Delete old instance (unused resources)
				oldIst := &fs.Instance{
					ChallengeID: req.GetId(),
					Identity:    oldID,
				}
				if err := oldIst.Delete(); err != nil {
					cerr <- err
					return
				}
			}

			// 8.e. Unlock RW instance
			//      -> defered after 8.a. (fault-tolerance)
			// 8.f. done in the "work" wait group
			///     -> defered at the beginning of goroutine
		})
	}

	// Create new instances if there is no until configured or
	// current calls happens before this until date.
	if fschall.Until == nil || time.Now().Before(*fschall.Until) {
		for range delta.Create {
			// The pool will spin instances and make them available ASAP,
			// but we don't have the time to wait for it now.
			go instance.SpinUp(ctx, req.GetId())
		}
	}

	for _, identity := range pooled[:delta.Delete] {
		work.Go(func() {
			ctx, span := global.Tracer.Start(ctx, "delete-instance", trace.WithAttributes(
				attribute.String("identity", identity),
			))
			defer span.End()

			ctx = global.WithIdentity(ctx, identity)

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
			if err := stack.Import(ctx, fsist); err != nil {
				cerr <- err
				return
			}

			logger.Info(ctx, "deleting instance")

			err = multierr.Combine(
				stack.Down(ctx),
				fsist.Delete(),
			)

			common.InstancesUDCounter().Add(ctx, -1,
				metric.WithAttributeSet(common.InstanceAttrs(req.GetId(), "", true)),
			)

			if err != nil {
				cerr <- err
				return
			}

			logger.Info(ctx, "deleted instance successfully")
		})
	}

	// Update iif required to do so, elseway do nothing
	for _, identity := range pooled[delta.Delete:] {
		work.Go(func() {
			ctx, span := global.Tracer.Start(ctx, "update-instance", trace.WithAttributes(
				attribute.String("identity", identity),
			))
			defer span.End()

			ctx = global.WithIdentity(ctx, identity)

			fsist, err := fs.LoadInstance(req.GetId(), identity)
			if err != nil {
				cerr <- err
				return
			}

			newScn := fschall.Scenario
			if updateScenario {
				newScn = *oldScn
			}
			var uerr error
			if updateScenario || updateAdditional {
				uerr = iac.Update(ctx, newScn, req.GetUpdateStrategy().String(), fschall, fsist)
			}

			cerr <- multierr.Combine(
				uerr,
				fsist.Save(),
			)
		})
	}

	if err := fschall.Save(); err != nil {
		logger.Error(ctx, "exporting challenge information to filesystem",
			zap.Error(err),
		)
		return nil, errs.ErrInternalNoSub
	}

	// 10. Once all "work" done, return response or error if any
	work.Wait()

	close(cerr)
	var merr error
	for err := range cerr {
		merr = multierr.Append(merr, err)
	}
	if merr != nil {
		return nil, merr
	}

	// Don't delete old directory, i.e. the previous scenario, as it could be reused
	// by other challenges.

	if err := fschall.Save(); err != nil {
		logger.Error(ctx, "exporting challenge information to filesystem",
			zap.Error(err),
		)
		return nil, errs.ErrInternalNoSub
	}

	logger.Info(ctx, "challenge updated successfully")

	oists := make([]*instance.Instance, 0, len(claimedAfterUpdate))
	for _, identity := range claimedAfterUpdate {
		sourceID, err := fs.LookupClaim(req.GetId(), identity)
		if err != nil {
			logger.Error(ctx, "looking up for claim",
				zap.Error(err),
			)
			return nil, errs.ErrInternalNoSub
		}

		ctx := global.WithSourceID(ctx, sourceID)

		fsist, err := fs.LoadInstance(req.GetId(), identity)
		if err != nil {
			logger.Error(ctx, "loading instance",
				zap.Error(err),
			)
			return nil, errs.ErrInternalNoSub
		}

		var until *timestamppb.Timestamp
		if fsist.Until != nil {
			until = timestamppb.New(*fsist.Until)
		}
		oists = append(oists, &instance.Instance{
			ChallengeId:    req.GetId(),
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

	return &Challenge{
		Id:         req.GetId(),
		Scenario:   fschall.Scenario,
		Additional: fschall.Additional,
		Min:        fschall.Min,
		Max:        fschall.Max,
		Timeout:    toPBDuration(fschall.Timeout),
		Until:      toPBTimestamp(fschall.Until),
		Instances:  oists,
	}, nil
}
