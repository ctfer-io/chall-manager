package challenge

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"os"
	"time"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	durationpb "google.golang.org/protobuf/types/known/durationpb"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ctfer-io/chall-manager/api/v1/common"
	"github.com/ctfer-io/chall-manager/api/v1/instance"
	"github.com/ctfer-io/chall-manager/global"
	errs "github.com/ctfer-io/chall-manager/pkg/errors"
	"github.com/ctfer-io/chall-manager/pkg/fs"
	"github.com/ctfer-io/chall-manager/pkg/lock"
	"github.com/ctfer-io/chall-manager/pkg/scenario"
)

func (store *Store) CreateChallenge(ctx context.Context, req *CreateChallengeRequest) (*Challenge, error) {
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
	defer func(lock lock.RWLock) {
		if err := lock.RWUnlock(ctx); err != nil {
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

	// 4. If the challenge already exist, return error
	if err := fs.CheckChallenge(req.Id); err == nil {
		return nil, &errs.ErrChallengeExist{
			ID:    req.Id,
			Exist: true,
		}
	}
	challDir := fs.ChallengeDirectory(req.Id)

	// 5. Save challenge
	logger.Info(ctx, "creating challenge")
	dir, err := scenario.Decode(ctx, challDir, req.Scenario)
	if err != nil {
		// Make sure to remove the challenge info, avoid inconsistency
		if err := os.RemoveAll(challDir); err != nil {
			return nil, &errs.ErrInternal{Sub: err}
		}
		if _, ok := err.(*errs.ErrScenario); ok {
			logger.Error(ctx, "invalid scenario", zap.Error(err))
			return nil, errs.ErrScenarioNoSub
		}
		if _, ok := err.(*errs.ErrInternal); ok {
			logger.Error(ctx, "decoding scenario", zap.Error(err))
			return nil, errs.ErrInternalNoSub
		}
		return nil, err
	}
	h := hash(req.Scenario)
	fschall := &fs.Challenge{
		ID:        req.Id,
		Directory: dir,
		Hash:      h,
		Timeout:   toDuration(req.Timeout),
		Until:     toTime(req.Until),
	}
	if err := fschall.Save(); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "exporting challenge information to filesystem",
			zap.Error(err),
		)
		return nil, errs.ErrInternalNoSub
	}

	logger.Info(ctx, "challenge created successfully")
	common.ChallengesUDCounter().Add(ctx, 1)

	chall := &Challenge{
		Id:        req.Id,
		Hash:      h,
		Timeout:   req.Timeout,
		Until:     req.Until,
		Instances: []*instance.Instance{},
	}

	// 6. Unlock RW challenge
	//    -> defered after 2 (fault-tolerance)

	return chall, nil
}

func toDuration(d *durationpb.Duration) *time.Duration {
	if d == nil {
		return nil
	}
	td := d.AsDuration()
	return &td
}

func toTime(d *timestamppb.Timestamp) *time.Time {
	if d == nil {
		return nil
	}
	td := d.AsTime()
	return &td
}

func hash(scenario string) string {
	h := md5.New()
	h.Write([]byte(scenario))
	return hex.EncodeToString(h.Sum(nil))
}
