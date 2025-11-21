package instance

import (
	context "context"
	"errors"
	"time"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ctfer-io/chall-manager/api/v1/common"
	"github.com/ctfer-io/chall-manager/global"
	errs "github.com/ctfer-io/chall-manager/pkg/errors"
	"github.com/ctfer-io/chall-manager/pkg/fs"
	"github.com/ctfer-io/chall-manager/pkg/iac"
	"github.com/ctfer-io/chall-manager/pkg/identity"
	"github.com/ctfer-io/chall-manager/pkg/scenario"
)

func (man *Manager) CreateInstance(ctx context.Context, req *CreateInstanceRequest) (*Instance, error) {
	logger := global.Log()
	ctx = global.WithChallengeID(ctx, req.ChallengeId)
	ctx = global.WithSourceID(ctx, req.SourceId)
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

	// 2. Lock RW challenge
	clock, err := common.LockChallenge(ctx, req.ChallengeId)
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
		logger.Error(ctx, "challenge RW lock", zap.Error(multierr.Combine(
			totw.RUnlock(ctx),
			err,
		)))
		return nil, errs.ErrInternalNoSub
	}

	// 3. Unlock R TOTW
	if err := totw.RUnlock(ctx); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "TOTW R unlock",
			zap.Error(multierr.Combine(
				clock.RUnlock(ctx),
				err,
			)),
		)
		return nil, errs.ErrInternalNoSub
	}
	span.AddEvent("unlocked TOTW")

	// 2. If challenge does not exist, is expired, or already has an instance
	// for the given source, return error.
	fschall, err := fs.LoadChallenge(req.ChallengeId)
	if err != nil {
		err = multierr.Combine(err, clock.RUnlock(ctx))
		if err, ok := err.(*errs.ErrInternal); ok {
			logger.Error(ctx, "loading challenge",
				zap.Error(err),
			)
			return nil, errs.ErrInternalNoSub
		}
		return nil, err
	}
	// Reload cache if necessary
	if _, err := scenario.DecodeOCI(ctx,
		fschall.ID, fschall.Scenario, req.Additional,
		global.Conf.OCI.Insecure, global.Conf.OCI.Username, global.Conf.OCI.Password,
	); err != nil {
		logger.Error(ctx, "decoding scenario",
			zap.String("reference", fschall.Scenario),
			zap.Error(err),
		)
		return nil, errs.ErrInternalNoSub
	}
	if fschall.Until != nil && time.Now().After(*fschall.Until) {
		if err := clock.RUnlock(ctx); err != nil {
			logger.Error(ctx, "unlocking R challenge", zap.Error(err))
		}
		return nil, errors.New("challenge is already expired")
	}
	if _, err := fs.FindInstance(req.ChallengeId, req.SourceId); err == nil {
		if err := clock.RUnlock(ctx); err != nil {
			logger.Error(ctx, "unlocking R challenge", zap.Error(err))
		}
		return nil, &errs.ErrInstanceExist{
			ChallengeID: req.ChallengeId,
			SourceID:    req.SourceId,
			Exist:       true,
		}
	}

	// If there are instances in pool, claim one, else deploy
	ists, err := fs.ListInstances(req.ChallengeId)
	if err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "listing instances",
			zap.Error(multierr.Combine(
				clock.RUnlock(ctx),
				err,
			)),
		)
		return nil, errs.ErrInternalNoSub
	}
	pool := []string{}
	for _, ist := range ists {
		sourceID, _ := fs.LookupClaim(req.ChallengeId, ist)
		isClaimed := sourceID != ""
		if !isClaimed {
			pool = append(pool, ist)
		}
	}

	if len(pool) != 0 {
		// We spin one new if there is less in the pool than the minimum requested
		// AND there is either no maximum defined, or we are under the defined maximum
		// threshold. -1 because we claim one from the pool, so we don't count it.
		toSpin := len(pool)-1 < int(fschall.Min) && (fschall.Max == 0 || len(ists) < int(fschall.Max))

		// Start concurrent routine that will refill the pool in exchange of the
		// one we just claimed, if we are under a defined threshold (i.e. max).
		// Elseway just spin up one more.
		if toSpin {
			go SpinUp(ctx, req.ChallengeId)
		}

		// Claim from pool
		claimed := pool[0]
		ctx = global.WithIdentity(ctx, claimed)
		logger.Info(ctx, "claiming instance from pool",
			zap.Bool("spin-up", toSpin),
		)

		if err := fs.Claim(req.ChallengeId, claimed, req.SourceId); err != nil {
			err := &errs.ErrInternal{Sub: err}
			logger.Error(ctx, "claiming instance",
				zap.Error(multierr.Combine(
					clock.RUnlock(ctx),
					err,
				)),
			)
			return nil, errs.ErrInternalNoSub
		}

		// Lock RW instance
		ctx = global.WithSourceID(ctx, req.SourceId)
		ilock, err := common.LockInstance(ctx, req.ChallengeId, claimed)
		if err != nil {
			err := &errs.ErrInternal{Sub: err}
			logger.Error(ctx, "build instance lock",
				zap.Error(multierr.Combine(
					clock.RUnlock(ctx),
					err,
				)),
			)
			return nil, errs.ErrInternalNoSub
		}
		defer common.LClose(ilock)
		if err := ilock.RWLock(ctx); err != nil {
			err := &errs.ErrInternal{Sub: err}
			logger.Error(ctx, "challenge instance RW lock",
				zap.Error(multierr.Combine(
					clock.RUnlock(ctx),
					err,
				)),
			)
			return nil, errs.ErrInternalNoSub
		}

		// Load instance
		fsist, err := fs.LoadInstance(req.ChallengeId, claimed)
		if err != nil {
			err := &errs.ErrInternal{Sub: err}
			logger.Error(ctx, "challenge instance filesystem load",
				zap.Error(multierr.Combine(
					clock.RUnlock(ctx),
					ilock.RWUnlock(ctx),
					err,
				)),
			)
			return nil, errs.ErrInternalNoSub
		}

		// Update times and stack
		fsist.Until = common.ComputeUntil(fschall.Until, fschall.Timeout)
		fsist.LastRenew = time.Now()
		if len(req.Additional) != 0 {
			fsist.Additional = req.Additional
			if err := iac.Update(ctx, fschall.Directory, "", fschall, fsist); err != nil {
				logger.Error(ctx, "updating pooled instance",
					zap.Error(multierr.Combine(
						clock.RUnlock(ctx),
						ilock.RWUnlock(ctx),
						err,
					)),
				)
				return nil, errs.ErrInternalNoSub
			}
		}

		// Unlock RW chall
		if err := clock.RUnlock(ctx); err != nil {
			err := &errs.ErrInternal{Sub: err}
			logger.Error(ctx, "unlock R challenge",
				zap.Error(multierr.Combine(
					ilock.RWUnlock(ctx),
					err,
				)),
			)
			return nil, errs.ErrInternalNoSub
		}

		// Save fsit
		if err := fsist.Save(); err != nil {
			logger.Error(ctx, "saving challenge instance on filesystem",
				zap.Error(multierr.Combine(
					ilock.RWUnlock(ctx),
					err,
				)),
			)
			return nil, errs.ErrInternalNoSub
		}

		// Unlock RW instance
		if err := ilock.RWUnlock(ctx); err != nil {
			logger.Error(ctx, "instance RW unlock",
				zap.Error(multierr.Combine(
					err,
				)),
			)
			return nil, errs.ErrInternalNoSub
		}

		// Respond
		var until *timestamppb.Timestamp
		if fsist.Until != nil {
			until = timestamppb.New(*fsist.Until)
		}
		return &Instance{
			ChallengeId:    req.ChallengeId,
			SourceId:       req.SourceId,
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
			Additional: req.Additional,
		}, nil
	}

	// Generate new identity
	id := identity.New()
	ctx = global.WithIdentity(ctx, id)
	logger.Info(ctx, "creating new instance")

	// No need to refine lock -> instance is unique per the identity.
	// We MUST NOT release the clock until the instance is up & running,
	// elseway the challenge could be deleted even if we are working on it.

	// Spin up
	stack, err := iac.NewStack(ctx, id, fschall)
	if err != nil {
		logger.Error(ctx, "building new stack",
			zap.Error(multierr.Combine(
				clock.RUnlock(ctx),
				err,
			)),
		)
		return nil, errs.ErrInternalNoSub
	}
	if err := iac.Additional(ctx, stack, fschall.Additional, req.Additional); err != nil {
		logger.Error(ctx, "configuring additionals on stack",
			zap.Error(multierr.Combine(
				clock.RUnlock(ctx),
				err,
			)),
		)
		return nil, errs.ErrInternalNoSub
	}

	sr, err := stack.Up(ctx)
	if err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "stack up",
			zap.Error(multierr.Combine(
				clock.RUnlock(ctx),
				fs.Wash(fschall.Directory, id),
				err,
			)),
		)
		return nil, errs.ErrInternalNoSub
	}

	now := time.Now()
	fsist := &fs.Instance{
		Identity:    id,
		ChallengeID: req.ChallengeId,
		Since:       now,
		LastRenew:   now,
		Until:       common.ComputeUntil(fschall.Until, fschall.Timeout),
		Additional:  req.Additional,
	}
	if err := iac.Extract(ctx, stack, sr, fsist); err != nil {
		logger.Error(ctx, "extracting stack info",
			zap.Error(multierr.Combine(
				clock.RUnlock(ctx),
				err,
			)),
		)
		return nil, errs.ErrInternalNoSub
	}

	// Save fsist
	if err := fsist.Save(); err != nil {
		logger.Error(ctx, "exporting instance information to filesystem",
			zap.Error(multierr.Combine(
				clock.RUnlock(ctx),
				err,
			)),
		)
		return nil, errs.ErrInternalNoSub
	}
	if err := fsist.Claim(req.SourceId); err != nil {
		logger.Error(ctx, "claiming instance",
			zap.Error(multierr.Combine(
				clock.RUnlock(ctx),
				err,
			)),
		)
		return nil, errs.ErrInternalNoSub
	}

	logger.Info(ctx, "instance created successfully")
	common.InstancesUDCounter().Add(ctx, 1,
		metric.WithAttributeSet(common.InstanceAttrs(req.ChallengeId, req.SourceId, false)),
	)

	// Unlock RW instance
	if err := clock.RUnlock(ctx); err != nil {
		logger.Error(ctx, "challenge R unlock",
			zap.Error(err),
		)
		return nil, errs.ErrInternalNoSub
	}

	// Respond
	var until *timestamppb.Timestamp
	if fsist.Until != nil {
		until = timestamppb.New(*fsist.Until)
	}
	return &Instance{
		ChallengeId:    req.ChallengeId,
		SourceId:       req.SourceId,
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
		Additional: req.Additional,
	}, nil
}
