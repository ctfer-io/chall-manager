package challenge

import (
	"context"
	"fmt"
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
	"github.com/ctfer-io/chall-manager/pkg/scenario"
	json "github.com/goccy/go-json"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
)

func (store *Store) UpdateChallenge(ctx context.Context, req *UpdateChallengeRequest) (*Challenge, error) {
	logger := global.Log()
	ctx = global.WithChallengeID(ctx, req.Id)
	span := trace.SpanFromContext(ctx)

	// 0. Validate request
	// => Pooler boundaries defaults to 0, with proper ordering
	if req.Min < 0 || req.Max < 0 || (req.Min > req.Max && req.Max != 0) {
		return nil, fmt.Errorf("min/max out of bounds: %d/%d", req.Min, req.Max)
	}

	// 1. Lock R TOTW
	span.AddEvent("lock TOTW")
	totw, err := common.LockTOTW(ctx)
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
	clock, err := common.LockChallenge(ctx, req.Id)
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
	defer func(lock lock.RWLock) {
		if err := lock.RWUnlock(ctx); err != nil {
			err := &errs.ErrInternal{Sub: err}
			logger.Error(ctx, "challenge RW unlock", zap.Error(err))
		}
	}(clock)

	// 3. Unlock R TOTW
	if err := totw.RUnlock(ctx); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "TOTW R unlock",
			zap.Error(err),
		)
		return nil, errs.ErrInternalNoSub
	}
	span.AddEvent("unlocked TOTW")

	// 4. If challenge does not exist, return error (+ unlock RW challenge)
	fschall, err := fs.LoadChallenge(req.Id)
	if err != nil {
		if err, ok := err.(*errs.ErrInternal); ok {
			logger.Error(ctx, "loading challenge",
				zap.Error(err),
			)
			return nil, errs.ErrInternalNoSub
		}
		return nil, err
	}

	// 5. Update challenge until/timeout, pooler, or scenario on filesystem
	updateScenario := false
	updateAdditional := false
	um := req.GetUpdateMask()
	if um.IsValid(req) {
		if slices.Contains(um.Paths, "scenario") {
			equals, err := scenario.Equals(
				fschall.Scenario, *req.Scenario,
				global.Conf.OCI.Insecure,
				global.Conf.OCI.Username, global.Conf.OCI.Password,
			)
			if err != nil {
				err := &errs.ErrInternal{Sub: err}
				logger.Error(ctx, "comparing scenarios",
					zap.Error(err),
				)
				return nil, errs.ErrInternalNoSub
			}
			updateScenario = !equals
		}
		if slices.Contains(um.Paths, "until") {
			fschall.Until = toTime(req.Until)
		}
		if slices.Contains(um.Paths, "timeout") {
			fschall.Timeout = toDuration(req.Timeout)
		}
		if slices.Contains(um.Paths, "additional") {
			updateAdditional = !maps.Equal(fschall.Additional, req.Additional)
			fschall.Additional = req.Additional
		}
		if slices.Contains(um.Paths, "min") {
			fschall.Min = req.Min
		}
		if slices.Contains(um.Paths, "max") {
			fschall.Max = req.Max
		}
	}

	var oldDir *string
	if updateScenario {
		// Decode new one
		dir, err := scenario.DecodeOCI(ctx,
			req.Id, *req.Scenario, fschall.Additional,
			global.Conf.OCI.Insecure, global.Conf.OCI.Username, global.Conf.OCI.Password,
		)
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
		fschall.Scenario = *req.Scenario
		oldDir, fschall.Directory = ptr(fschall.Directory), dir
	}

	// 7. Create "work" and "updated" wait groups for all instances and for all claimed
	logger.Info(ctx, "updating challenge",
		zap.Bool("scenario", updateScenario),
		zap.Bool("additional", updateAdditional),
	)
	if req.UpdateStrategy == nil {
		req.UpdateStrategy = UpdateStrategy_update_in_place.Enum()
	}

	ists, err := fs.ListInstances(req.Id)
	if err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "listing instances",
			zap.Error(err),
		)
		return nil, errs.ErrInternalNoSub
	}
	claimed := []string{}
	pooled := []string{}
	for _, ist := range ists {
		sourceID, _ := fs.LookupClaim(req.Id, ist)
		isClaimed := sourceID != ""
		if isClaimed {
			claimed = append(claimed, ist)
		} else {
			pooled = append(pooled, ist)
		}
	}

	delta := pool.NewDelta(fschall.Min, fschall.Max, int64(len(claimed)), int64(len(pooled)))
	size := len(ists)

	claimedAfterUpdate := make([]string, 0, len(claimed))
	work := &sync.WaitGroup{}
	work.Add(size)
	cerr := make(chan error, size)
	for _, identity := range claimed {
		sourceID, _ := fs.LookupClaim(req.Id, identity)
		go func(work *sync.WaitGroup, cerr chan<- error, sourceID, identity string) {
			// Track span of loading stack
			ctx, span := global.Tracer.Start(ctx, "updating-instance", trace.WithAttributes(
				attribute.String("source_id", sourceID),
				attribute.String("identity", identity),
			))
			defer span.End()

			defer work.Done()
			ctx = global.WithSourceID(ctx, sourceID)
			ctx = global.WithIdentity(ctx, identity)

			// 8.a. Lock RW instance
			ilock, err := common.LockInstance(ctx, req.Id, identity)
			if err != nil {
				cerr <- err
				return
			}
			defer common.LClose(ilock)
			if err := ilock.RWLock(ctx); err != nil {
				cerr <- err
				return
			}
			defer func(lock lock.RWLock) {
				if err := lock.RWUnlock(ctx); err != nil {
					err := &errs.ErrInternal{Sub: err}
					logger.Error(ctx, "instance RW unlock", zap.Error(err))
				}
			}(ilock)

			fsist, err := fs.LoadInstance(req.Id, identity)
			if err != nil {
				cerr <- err
				return
			}

			// 8.c. Mirror instance's "until" based on the challenge
			fsist.Until = common.ComputeUntil(fschall.Until, fschall.Timeout)

			// 8.d. If scenario is not nil, update it
			ndir := fschall.Directory
			if updateScenario {
				ndir = *oldDir
			}

			// Keep track of who is the owner of the instance
			oldID := fsist.Identity

			// Then update if necessary
			if updateScenario || updateAdditional {
				if err := iac.Update(ctx, ndir, req.UpdateStrategy.String(), fschall, fsist); err != nil {
					cerr <- err
					return
				}
			}

			// Save potentially updated instance
			newIst := fsist.Identity
			if err := fsist.Save(); err != nil {
				cerr <- err
				return
			}

			// (Re-)claim the instance (e.g. can be another one with recreate)
			if err := fsist.Claim(sourceID); err != nil {
				if _, ok := err.(*fs.ErrAlreadyClaimed); !ok {
					cerr <- err
					return
				}
			}

			claimedAfterUpdate = append(claimedAfterUpdate, newIst)

			if oldID != newIst {
				// Delete old instance (unused resources)
				oldIst := &fs.Instance{
					ChallengeID: req.Id,
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
		}(work, cerr, sourceID, identity)
	}

	// Create new instances if there is no until configured or
	// current calls happens before this until date.
	if fschall.Until == nil || time.Now().Before(*fschall.Until) {
		for range delta.Create {
			// The pool will spin instances and make them available ASAP,
			// but we don't have the time to wait for it now.
			go instance.SpinUp(ctx, req.Id)
		}
	}

	for _, identity := range pooled[:delta.Delete] {
		go func(work *sync.WaitGroup, cerr chan<- error, identity string) {
			ctx, span := global.Tracer.Start(ctx, "delete-instance", trace.WithAttributes(
				attribute.String("identity", identity),
			))
			defer span.End()

			defer work.Done()
			ctx = global.WithIdentity(ctx, identity)

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
			state, err := json.Marshal(fsist.State)
			if err != nil {
				cerr <- err
				return
			}
			if err := stack.Import(ctx, apitype.UntypedDeployment{
				Version:    3,
				Deployment: state,
			}); err != nil {
				cerr <- err
				return
			}

			logger.Info(ctx, "deleting instance")

			if _, err := stack.Destroy(ctx); err != nil {
				cerr <- err
				return
			}

			if err := fsist.Delete(); err != nil {
				cerr <- err
				return
			}

			// Wash Pulumi files
			if err := fs.Wash(fschall.Directory, identity); err != nil {
				cerr <- err
				return
			}

			logger.Info(ctx, "deleted instance successfully")
			common.InstancesUDCounter().Add(ctx, -1,
				metric.WithAttributeSet(common.InstanceAttrs(req.Id, "", true)),
			)
		}(work, cerr, identity)
	}

	// Update iif required to do so, elseway do nothing
	for _, identity := range pooled[delta.Delete:] {
		go func(work *sync.WaitGroup, cerr chan<- error, identity string) {
			ctx, span := global.Tracer.Start(ctx, "update-instance", trace.WithAttributes(
				attribute.String("identity", identity),
			))
			defer span.End()

			defer work.Done()
			ctx = global.WithIdentity(ctx, identity)

			fsist, err := fs.LoadInstance(req.Id, identity)
			if err != nil {
				cerr <- err
				return
			}

			ndir := fschall.Directory
			if updateScenario {
				ndir = *oldDir
			}
			if updateScenario || updateAdditional {
				if err := iac.Update(ctx, ndir, req.UpdateStrategy.String(), fschall, fsist); err != nil {
					cerr <- err
					return
				}
			}

			if err := fsist.Save(); err != nil {
				cerr <- err
				return
			}
		}(work, cerr, identity)
	}

	if err := fschall.Save(); err != nil {
		if err, ok := err.(*errs.ErrInternal); ok {
			logger.Error(ctx, "exporting challenge information to filesystem",
				zap.Error(err),
			)
			return nil, errs.ErrInternalNoSub
		}
		return nil, err
	}

	// 10. Once all "work" done, return response or error if any
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
		logger.Error(ctx, "updating challenge and its instances",
			zap.Error(merri),
		)
		return nil, errs.ErrInternalNoSub
	}
	if merr != nil {
		return nil, merr
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
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "exporting challenge information to filesystem",
			zap.Error(err),
		)
		return nil, errs.ErrInternalNoSub
	}

	logger.Info(ctx, "challenge updated successfully")

	oists := make([]*instance.Instance, 0, len(claimedAfterUpdate))
	for _, identity := range claimedAfterUpdate {
		sourceID, _ := fs.LookupClaim(req.Id, identity)
		ctx := global.WithSourceID(ctx, sourceID)

		fsist, err := fs.LoadInstance(req.Id, identity)
		if err != nil {
			if err, ok := err.(*errs.ErrInternal); ok {
				logger.Error(ctx, "loading instance",
					zap.Error(err),
				)
				return nil, errs.ErrInternalNoSub
			}
			return nil, err
		}

		var until *timestamppb.Timestamp
		if fsist.Until != nil {
			until = timestamppb.New(*fsist.Until)
		}
		oists = append(oists, &instance.Instance{
			ChallengeId:    req.Id,
			SourceId:       sourceID,
			Since:          timestamppb.New(fsist.Since),
			LastRenew:      timestamppb.New(fsist.LastRenew),
			Until:          until,
			ConnectionInfo: fsist.ConnectionInfo,
			Flag:           fsist.Flag,
			Additional:     fsist.Additional,
		})
	}

	return &Challenge{
		Id:         req.Id,
		Scenario:   fschall.Scenario,
		Additional: fschall.Additional,
		Min:        fschall.Min,
		Max:        fschall.Max,
		Timeout:    toPBDuration(fschall.Timeout),
		Until:      toPBTimestamp(fschall.Until),
		Instances:  oists,
	}, nil
}

func ptr[T any](t T) *T {
	return &t
}
