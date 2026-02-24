package instance

import (
	context "context"
	"os"
	"time"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	ctx = global.WithChallengeID(ctx, req.GetChallengeId())
	ctx = global.WithSourceID(ctx, req.GetSourceId())
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
	clock, err := common.LockChallenge(ctx, req.GetChallengeId())
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
	if err := clock.RLock(ctx); err != nil {
		if clock.IsCanceled(err) {
			// If canceled, we need to recover
			if err := totw.RUnlock(context.WithoutCancel(ctx)); err != nil {
				logger.Error(ctx, "recovering from challenge R lock", zap.Error(err))
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

	// 3. Unlock R TOTW
	if err := totw.RUnlock(context.WithoutCancel(ctx)); err != nil {
		logger.Error(ctx, "TOTW R unlock",
			zap.Error(multierr.Combine(
				clock.RUnlock(context.WithoutCancel(ctx)),
				err,
			)),
		)
		return nil, errs.ErrInternalNoSub
	}
	span.AddEvent("unlocked TOTW")

	// 2. If challenge does not exist, is expired, or already has an instance
	// for the given source, return error.
	fschall, err := fs.LoadChallenge(req.GetChallengeId())
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
	// Reload cache if necessary
	if fschall.Until != nil && time.Now().After(*fschall.Until) {
		if err := clock.RUnlock(context.WithoutCancel(ctx)); err != nil {
			logger.Error(ctx, "unlocking R challenge", zap.Error(err))
		}
		st, err := status.New(codes.FailedPrecondition, "Challenge has expired.").WithDetails(
			&errdetails.ErrorInfo{
				Reason: errs.ReasonChallengeExpired,
				Domain: errs.Domain,
				Metadata: map[string]string{
					"id": req.GetChallengeId(),
				},
			},
			&errdetails.PreconditionFailure{
				Violations: []*errdetails.PreconditionFailure_Violation{
					{
						Type:        "EXPIRATION",
						Subject:     errs.Domain + "/Challenge",
						Description: "Challenge has expired so can no longer process instance requests.",
					},
				},
			},
		)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to build error: %v", err)
		}
		return nil, st.Err()
	}
	if _, err := fs.FindInstance(req.GetChallengeId(), req.GetSourceId()); err == nil {
		if err := clock.RUnlock(context.WithoutCancel(ctx)); err != nil {
			logger.Error(ctx, "unlocking R challenge", zap.Error(err))
		}

		if _, ok := err.(*errs.InstanceExist); ok {
			return nil, err
		}
		logger.Error(ctx, "finding instance", zap.Error(err))
		return nil, errs.ErrInternalNoSub
	}

	// If there are instances in pool, claim one, else deploy
	ists, err := fs.ListInstances(req.GetChallengeId())
	if err != nil {
		logger.Error(ctx, "listing instances",
			zap.Error(multierr.Combine(
				clock.RUnlock(context.WithoutCancel(ctx)),
				err,
			)),
		)
		return nil, errs.ErrInternalNoSub
	}
	pooled := []string{}
	for _, ist := range ists {
		sourceID, err := fs.LookupClaim(req.GetChallengeId(), ist)
		if os.IsNotExist(err) {
			// no claim file => in pool
			pooled = append(pooled, sourceID)
			continue
		}
		if err != nil {
			logger.Error(ctx, "looking up for claim",
				zap.Error(err),
			)
			return nil, errs.ErrInternalNoSub
		}
	}

	if len(pooled) != 0 {
		// We spin one new if there is less in the pool than the minimum requested
		// AND there is either no maximum defined, or we are under the defined maximum
		// threshold. -1 because we claim one from the pool, so we don't count it.
		toSpin := len(pooled)-1 < int(fschall.Min) && (fschall.Max == 0 || len(ists) < int(fschall.Max))

		// Start concurrent routine that will refill the pool in exchange of the
		// one we just claimed, if we are under a defined threshold (i.e. max).
		// Elseway just spin up one more.
		if toSpin {
			go SpinUp(ctx, req.GetChallengeId())
		}

		// Claim from pool
		claimed := pooled[0]
		ctx = global.WithIdentity(ctx, claimed)
		logger.Info(ctx, "claiming instance from pool",
			zap.Bool("spin-up", toSpin),
		)

		if err := fs.Claim(req.GetChallengeId(), claimed, req.GetSourceId()); err != nil {
			logger.Error(ctx, "claiming instance",
				zap.Error(multierr.Combine(
					clock.RUnlock(context.WithoutCancel(ctx)),
					err,
				)),
			)
			return nil, errs.ErrInternalNoSub
		}

		// Lock RW instance
		ctx = global.WithSourceID(ctx, req.GetSourceId())
		ilock, err := common.LockInstance(ctx, req.GetChallengeId(), claimed)
		if err != nil {
			if ilock.IsCanceled(err) {
				// If canceled, we need to recover
				if err := clock.RUnlock(context.WithoutCancel(ctx)); err != nil {
					logger.Error(ctx, "recovering from build instance lock", zap.Error(err))
					return nil, errs.ErrInternalNoSub
				}
				return nil, errs.ErrCanceled // recovery is successful, we can quit safely
			}
			logger.Error(ctx, "build instance lock",
				zap.Error(multierr.Combine(
					clock.RUnlock(context.WithoutCancel(ctx)),
					err,
				)),
			)
			return nil, errs.ErrInternalNoSub
		}
		if err := ilock.RWLock(ctx); err != nil {
			if ilock.IsCanceled(err) {
				// If canceled, we need to recover
				if err := clock.RUnlock(context.WithoutCancel(ctx)); err != nil {
					logger.Error(ctx, "recovering from challenge instance RW lock", zap.Error(err))
					return nil, errs.ErrInternalNoSub
				}
				return nil, errs.ErrCanceled // recovery is successful, we can quit safely
			}
			logger.Error(ctx, "challenge instance RW lock",
				zap.Error(multierr.Combine(
					clock.RUnlock(context.WithoutCancel(ctx)),
					err,
				)),
			)
			return nil, errs.ErrInternalNoSub
		}

		// Load instance
		fsist, err := fs.LoadInstance(req.GetChallengeId(), claimed)
		if err != nil {
			logger.Error(ctx, "challenge instance filesystem load",
				zap.Error(multierr.Combine(
					clock.RUnlock(context.WithoutCancel(ctx)),
					ilock.RWUnlock(context.WithoutCancel(ctx)),
					err,
				)),
			)
			return nil, errs.ErrInternalNoSub
		}

		// Update times and stack
		fsist.Until = common.ComputeUntil(fschall.Until, fschall.Timeout)
		fsist.LastRenew = time.Now()
		if len(req.GetAdditional()) != 0 {
			fsist.Additional = req.GetAdditional()
			if err := iac.Update(ctx, fschall.Scenario, "", fschall, fsist); err != nil {
				logger.Error(ctx, "updating pooled instance",
					zap.Error(multierr.Combine(
						clock.RUnlock(context.WithoutCancel(ctx)),
						ilock.RWUnlock(context.WithoutCancel(ctx)),
						err,
					)),
				)
				return nil, errs.ErrInternalNoSub
			}
		}

		// Unlock RW chall
		if err := clock.RUnlock(context.WithoutCancel(ctx)); err != nil {
			logger.Error(ctx, "unlock R challenge",
				zap.Error(multierr.Combine(
					ilock.RWUnlock(context.WithoutCancel(ctx)),
					err,
				)),
			)
			return nil, errs.ErrInternalNoSub
		}

		// Save fsist
		if err := fsist.Save(); err != nil {
			logger.Error(ctx, "saving challenge instance",
				zap.Error(multierr.Combine(
					ilock.RWUnlock(context.WithoutCancel(ctx)),
					err,
				)),
			)
			return nil, errs.ErrInternalNoSub
		}

		// Unlock RW instance
		if err := ilock.RWUnlock(context.WithoutCancel(ctx)); err != nil {
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
			ChallengeId:    req.GetChallengeId(),
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
			Additional: req.GetAdditional(),
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
	stack, err := iac.NewStack(ctx, fschall, id)
	if err != nil {
		logger.Error(ctx, "building new stack",
			zap.Error(multierr.Combine(
				clock.RUnlock(context.WithoutCancel(ctx)),
				err,
			)),
		)
		return nil, err
	}
	if err := iac.Additional(ctx, stack, fschall.Additional, req.GetAdditional()); err != nil {
		logger.Error(ctx, "configuring additionals on stack",
			zap.Error(multierr.Combine(
				clock.RUnlock(context.WithoutCancel(ctx)),
				err,
			)),
		)
		return nil, err
	}

	sr, err := stack.Up(ctx)
	if err != nil {
		logger.Error(ctx, "stack up",
			zap.Error(multierr.Combine(
				clock.RUnlock(context.WithoutCancel(ctx)),
				err,
			)),
		)
		return nil, err
	}

	now := time.Now()
	fsist := &fs.Instance{
		Identity:    id,
		ChallengeID: req.GetChallengeId(),
		Since:       now,
		LastRenew:   now,
		Until:       common.ComputeUntil(fschall.Until, fschall.Timeout),
		Additional:  req.GetAdditional(),
	}
	if err := stack.Export(ctx, sr, fsist); err != nil {
		logger.Error(ctx, "extracting stack info",
			zap.Error(multierr.Combine(
				clock.RUnlock(context.WithoutCancel(ctx)),
				err,
			)),
		)
		return nil, errs.ErrInternalNoSub
	}

	// Save fsist
	if err := fsist.Save(); err != nil {
		logger.Error(ctx, "exporting instance information to filesystem",
			zap.Error(multierr.Combine(
				clock.RUnlock(context.WithoutCancel(ctx)),
				err,
			)),
		)
		return nil, errs.ErrInternalNoSub
	}
	if err := fsist.Claim(req.GetSourceId()); err != nil {
		logger.Error(ctx, "claiming instance",
			zap.Error(multierr.Combine(
				clock.RUnlock(context.WithoutCancel(ctx)),
				err,
			)),
		)
		return nil, errs.ErrInternalNoSub
	}

	logger.Info(ctx, "instance created successfully")
	common.InstancesUDCounter().Add(ctx, 1,
		metric.WithAttributeSet(common.InstanceAttrs(req.GetChallengeId(), req.GetSourceId(), false)),
	)

	// Unlock RW instance
	if err := clock.RUnlock(context.WithoutCancel(ctx)); err != nil {
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
		ChallengeId:    req.GetChallengeId(),
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
		Additional: req.GetAdditional(),
	}, nil
}
