package instance

import (
	context "context"
	"errors"
	"time"

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
)

func (man *Manager) CreateInstance(ctx context.Context, req *CreateInstanceRequest) (*Instance, error) {
	logger := global.Log()
	ctx = global.WithChallengeID(ctx, req.ChallengeId)
	ctx = global.WithSourceID(ctx, req.SourceId)
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
	clock, err := common.LockChallenge(req.ChallengeId)
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

	// 3. Unlock R TOTW
	if err := totw.RUnlock(ctx); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "TOTW R unlock",
			zap.Error(multierr.Combine(
				clock.RWUnlock(ctx),
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
		err = multierr.Combine(err, clock.RWUnlock(ctx))
		if err, ok := err.(*errs.ErrInternal); ok {
			logger.Error(ctx, "loading challenge",
				zap.Error(err),
			)
			return nil, errs.ErrInternalNoSub
		}
		return nil, err
	}
	if fschall.Until != nil && time.Now().After(*fschall.Until) {
		if err := clock.RWUnlock(ctx); err != nil {
			logger.Error(ctx, "unlocking RW challenge", zap.Error(err))
		}
		return nil, errors.New("challenge is already expired")
	}
	if _, ok := fschall.Instances[req.SourceId]; ok {
		if err := clock.RWUnlock(ctx); err != nil {
			logger.Error(ctx, "unlocking RW challenge", zap.Error(err))
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
				clock.RWUnlock(ctx),
				err,
			)),
		)
		return nil, errs.ErrInternalNoSub
	}
	pool := []string{}
	for _, ist := range ists {
		isClaimed := false
		for _, claimed := range fschall.Instances {
			if ist == claimed {
				isClaimed = true
				break
			}
		}
		if !isClaimed {
			pool = append(pool, ist)
		}
	}

	if len(pool) != 0 {
		// a. Claim from pool
		claimed := pool[0]
		ctx = global.WithIdentity(ctx, claimed)
		if fschall.Instances == nil {
			fschall.Instances = map[string]string{}
		}
		fschall.Instances[req.SourceId] = claimed

		// Start concurrent routine that will refill the pool in exchange of the
		// one we just claimed, if we are under the threshold (i.e. max).
		if len(ists) < int(fschall.Max) {
			go SpinUp(ctx, req.ChallengeId)
		}

		// b. Save fschall
		if err := fschall.Save(); err != nil {
			logger.Error(ctx, "saving challenge on filesystem",
				zap.Error(multierr.Combine(
					clock.RWUnlock(ctx),
					err,
				)),
			)
			return nil, errs.ErrInternalNoSub
		}

		// d. Lock RW instance
		ctx = global.WithSourceID(ctx, req.SourceId)
		ilock, err := common.LockInstance(req.ChallengeId, claimed)
		if err != nil {
			err := &errs.ErrInternal{Sub: err}
			logger.Error(ctx, "build instance lock",
				zap.Error(multierr.Combine(
					clock.RWUnlock(ctx),
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
					clock.RWUnlock(ctx),
					err,
				)),
			)
			return nil, errs.ErrInternalNoSub
		}

		// . Unlock RW chall
		if err := clock.RWUnlock(ctx); err != nil {
			err := &errs.ErrInternal{Sub: err}
			logger.Error(ctx, "unlock RW challenge",
				zap.Error(multierr.Combine(
					ilock.RWUnlock(ctx),
					err,
				)),
			)
			return nil, errs.ErrInternalNoSub
		}

		// f. Load instance
		fsist, err := fs.LoadInstance(req.ChallengeId, claimed)
		if err != nil {
			err := &errs.ErrInternal{Sub: err}
			logger.Error(ctx, "challenge instance filesystem load",
				zap.Error(multierr.Combine(
					ilock.RWUnlock(ctx),
					err,
				)),
			)
			return nil, errs.ErrInternalNoSub
		}

		// g. Update times and stack
		fsist.Until = common.ComputeUntil(fschall.Until, fschall.Timeout)
		fsist.LastRenew = time.Now()
		if len(req.Additional) != 0 {
			fsist.Additional = req.Additional
			if err := iac.Update(ctx, fschall.Directory, "", fschall, fsist); err != nil {
				logger.Error(ctx, "updating pooled instance",
					zap.Error(multierr.Combine(
						ilock.RWUnlock(ctx),
						err,
					)),
				)
				return nil, errs.ErrInternalNoSub
			}
		}

		// h. Save fsit
		if err := fsist.Save(); err != nil {
			logger.Error(ctx, "saving challenge instance on filesystem",
				zap.Error(multierr.Combine(
					ilock.RWUnlock(ctx),
					err,
				)),
			)
			return nil, errs.ErrInternalNoSub
		}

		// i. Unlock RW instance
		if err := ilock.RWUnlock(ctx); err != nil {
			logger.Error(ctx, "instance RW unlock",
				zap.Error(multierr.Combine(
					err,
				)),
			)
			return nil, errs.ErrInternalNoSub
		}

		// k. Respond
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
			Flag:           fsist.Flag,
			Additional:     req.Additional,
		}, nil
	}

	// a. Generate new identity
	id := identity.New()
	ctx = global.WithIdentity(ctx, id)

	// b. Register in fschall
	if fschall.Instances == nil {
		fschall.Instances = map[string]string{}
	}
	fschall.Instances[req.SourceId] = id

	// c. Save fschall
	if err := fschall.Save(); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "saving challenge on filesystem",
			zap.Error(multierr.Combine(
				clock.RWUnlock(ctx),
				err,
			)),
		)
		return nil, errs.ErrInternalNoSub
	}

	// e. Lock RW instance
	ctx = global.WithSourceID(ctx, req.SourceId)
	ilock, err := common.LockInstance(req.ChallengeId, id)
	if err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "build instance lock",
			zap.Error(multierr.Combine(
				clock.RWUnlock(ctx),
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
				clock.RWUnlock(ctx),
				err,
			)),
		)
		return nil, errs.ErrInternalNoSub
	}

	// f. Unlock RW chall
	if err := clock.RWUnlock(ctx); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "unlock RW challenge",
			zap.Error(multierr.Combine(
				ilock.RWUnlock(ctx),
				err,
			)),
		)
		return nil, errs.ErrInternalNoSub
	}

	// g. Spin up
	stack, err := iac.NewStack(ctx, id, fschall)
	if err != nil {
		logger.Error(ctx, "building new stack",
			zap.Error(multierr.Combine(
				ilock.RWUnlock(ctx),
				err,
			)),
		)
		return nil, errs.ErrInternalNoSub
	}
	if err := iac.Additional(ctx, stack, fschall.Additional, req.Additional); err != nil {
		logger.Error(ctx, "configuring additionals on stack",
			zap.Error(multierr.Combine(
				ilock.RWUnlock(ctx),
				err,
			)),
		)
		return nil, errs.ErrInternalNoSub
	}

	logger.Info(ctx, "creating instance")

	sr, err := stack.Up(ctx)
	if err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "stack up",
			zap.Error(multierr.Combine(
				ilock.RWUnlock(ctx),
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
				ilock.RWUnlock(ctx),
				err,
			)),
		)
		return nil, errs.ErrInternalNoSub
	}

	// h. Save fsist
	if err := fsist.Save(); err != nil {
		logger.Error(ctx, "exporting instance information to filesystem",
			zap.Error(multierr.Combine(
				ilock.RWUnlock(ctx),
				err,
			)),
		)
		return nil, errs.ErrInternalNoSub
	}

	logger.Info(ctx, "instance created successfully")
	common.InstancesUDCounter().Add(ctx, 1)

	// i. Unlock RW instance
	if err := ilock.RWUnlock(ctx); err != nil {
		logger.Error(ctx, "instance RW unlock",
			zap.Error(err),
		)
		return nil, errs.ErrInternalNoSub
	}

	// k. Respond
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
		Flag:           fsist.Flag,
		Additional:     req.Additional,
	}, nil
}
