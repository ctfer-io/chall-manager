package challenge

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ctfer-io/chall-manager/api/v1/common"
	"github.com/ctfer-io/chall-manager/api/v1/instance"
	"github.com/ctfer-io/chall-manager/global"
	errs "github.com/ctfer-io/chall-manager/pkg/errors"
	"github.com/ctfer-io/chall-manager/pkg/fs"
	"github.com/ctfer-io/chall-manager/pkg/lock"
)

func (store *Store) RetrieveChallenge(ctx context.Context, req *RetrieveChallengeRequest) (*Challenge, error) {
	logger := global.Log()
	ctx = global.WithChallengeID(ctx, req.Id)
	span := trace.SpanFromContext(ctx)

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

	// 2. Lock R challenge
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
	if err := clock.RLock(ctx); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "challenge R lock", zap.Error(multierr.Combine(
			totw.RUnlock(ctx),
			err,
		)))
		return nil, errs.ErrInternalNoSub
	}
	defer func(lock lock.RWLock) {
		if err := lock.RUnlock(ctx); err != nil {
			err := &errs.ErrInternal{Sub: err}
			logger.Error(ctx, "challenge RW unlock", zap.Error(err))
		}
	}(clock)

	// 3. Unlock R TOTW
	if err := totw.RUnlock(ctx); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "TOTW R unlock", zap.Error(err))
		return nil, errs.ErrInternalNoSub
	}
	span.AddEvent("unlocked TOTW")

	// 4. Fetch challenge info
	fschall, err := fs.LoadChallenge(req.Id)
	if err != nil {
		// If challenge not found, is not an error
		if _, ok := err.(*errs.ErrChallengeExist); ok {
			return nil, nil
		}
		if err, ok := err.(*errs.ErrInternal); ok {
			logger.Error(ctx, "loading challenge",
				zap.Error(err),
			)
			return nil, errs.ErrInternalNoSub
		}
		return nil, err
	}

	// 5. For all challenge instances, lock, read, unlock, unlock R ASAP
	clmIsts := map[string]string{}
	ists, err := fs.ListInstances(req.Id)
	if err != nil {
		logger.Error(ctx, "loading instance",
			zap.Error(err),
		)
		return nil, errs.ErrInternalNoSub
	}
	for _, ist := range ists {
		src, err := fs.LookupClaim(req.Id, ist)
		if err != nil {
			// in pool
			continue
		}
		clmIsts[src] = ist
	}
	oists := make([]*instance.Instance, 0, len(clmIsts))
	for sourceID, identity := range clmIsts {
		ctxi := global.WithSourceID(ctx, sourceID)
		fsist, err := fs.LoadInstance(req.Id, identity)
		if err != nil {
			if err, ok := err.(*errs.ErrInternal); ok {
				logger.Error(ctxi, "loading instance",
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
		Timeout:    toPBDuration(fschall.Timeout),
		Until:      toPBTimestamp(fschall.Until),
		Instances:  oists,
		Additional: fschall.Additional,
		Min:        fschall.Min,
		Max:        fschall.Max,
	}, nil
}

func toPBDuration(d *time.Duration) *durationpb.Duration {
	if d == nil {
		return nil
	}
	return durationpb.New(*d)
}

func toPBTimestamp(t *time.Time) *timestamppb.Timestamp {
	if t == nil {
		return nil
	}
	return timestamppb.New(*t)
}
