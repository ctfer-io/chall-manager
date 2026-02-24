package instance

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/multierr"
	"go.uber.org/zap"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ctfer-io/chall-manager/api/v1/common"
	"github.com/ctfer-io/chall-manager/global"
	errs "github.com/ctfer-io/chall-manager/pkg/errors"
	"github.com/ctfer-io/chall-manager/pkg/fs"
	"github.com/ctfer-io/chall-manager/pkg/lock"
)

func (man *Manager) RenewInstance(ctx context.Context, req *RenewInstanceRequest) (*Instance, error) {
	logger := global.Log()
	ctx = global.WithChallengeID(ctx, req.GetChallengeId())
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

	// 2. Lock R challenge
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
		logger.Error(ctx, "challenge R lock", zap.Error(multierr.Combine(
			totw.RUnlock(context.WithoutCancel(ctx)),
			err,
		)))
		return nil, errs.ErrInternalNoSub
	}
	defer func(lock lock.RWLock) {
		if err := lock.RUnlock(context.WithoutCancel(ctx)); err != nil {
			logger.Error(ctx, "challenge R unlock", zap.Error(err))
		}
	}(clock)

	// 3. Unlock R TOTW
	if err := totw.RUnlock(context.WithoutCancel(ctx)); err != nil {
		logger.Error(ctx, "TOTW R unlock", zap.Error(err))
		return nil, errs.ErrInternalNoSub
	}
	span.AddEvent("unlocked TOTW")

	// 4. If challenge does not exist, return error
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
	id, err := fs.FindInstance(req.GetChallengeId(), req.GetSourceId())
	if err != nil {
		if _, ok := err.(*errs.InstanceExist); ok {
			return nil, err
		}

		logger.Error(ctx, "finding instance", zap.Error(err))
		return nil, errs.ErrInternalNoSub
	}

	// 5. Lock RW instance
	ctx = global.WithSourceID(ctx, req.GetSourceId())
	ctx = global.WithIdentity(ctx, id)
	ilock, err := common.LockInstance(ctx, req.GetChallengeId(), id)
	if err != nil {
		if ilock.IsCanceled(err) {
			return nil, errs.ErrCanceled
		}
		logger.Error(ctx, "build challenge lock", zap.Error(err))
		return nil, errs.ErrInternalNoSub
	}
	if err := ilock.RWLock(ctx); err != nil {
		if ilock.IsCanceled(err) {
			return nil, errs.ErrCanceled
		}
		logger.Error(ctx, "challenge instance RW lock", zap.Error(err))
		return nil, errs.ErrInternalNoSub
	}
	defer func(lock lock.RWLock) {
		if err := lock.RWUnlock(context.WithoutCancel(ctx)); err != nil {
			logger.Error(ctx, "instance RW unlock", zap.Error(err))
		}
	}(ilock)

	// 6. If instance does not exist, return error (+ Unlock RW instance, Unlock R challenge)
	fsist, err := fs.LoadInstance(req.GetChallengeId(), id)
	if err != nil {
		logger.Error(ctx, "loading challenge instance",
			zap.Error(err),
		)
		return nil, errs.ErrInternalNoSub
	}

	// 7. Set new until to now + challenge.timeout if any
	if fschall.Timeout == nil {
		// This makes sure renewal is possible thanks to a timeout
		st, err := status.New(codes.FailedPrecondition, "Challenge does not accept renewal.").WithDetails(
			&errdetails.ErrorInfo{
				Reason: errs.ReasonChallengeNoRenewal,
				Domain: errs.Domain,
				Metadata: map[string]string{
					"id": req.GetChallengeId(),
				},
			},
			&errdetails.PreconditionFailure{
				Violations: []*errdetails.PreconditionFailure_Violation{
					{
						Type:        "RENEWAL",
						Subject:     errs.Domain + "/Challenge",
						Description: "Challenge has no timeout thus cannot be renewed.",
					},
				},
			},
		)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to build error: %v", err)
		}
		return nil, st.Err()
	}

	now := time.Now()
	if fsist.Until != nil && now.After(*fsist.Until) {
		// This makes sure renewal is not possible once expired
		st, err := status.New(codes.FailedPrecondition, "Instance can't be renewed as it expired.").WithDetails(
			&errdetails.ErrorInfo{
				Reason: errs.ReasonInstanceExpired,
				Domain: errs.Domain,
				Metadata: map[string]string{
					"challenge_id": req.GetChallengeId(),
					"source_id":    req.GetSourceId(),
				},
			},
			&errdetails.PreconditionFailure{
				Violations: []*errdetails.PreconditionFailure_Violation{
					{
						Type:        "EXPIRATION",
						Subject:     errs.Domain + "/Instance",
						Description: "Instance has expired so can no longer process renewal request.",
					},
				},
			},
		)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to build error: %v", err)
		}
		return nil, st.Err()
	}

	fsist.LastRenew = now
	fsist.Until = common.ComputeUntil(fschall.Until, fschall.Timeout)

	logger.Info(ctx, "renewing instance")
	if err := fsist.Save(); err != nil {
		logger.Error(ctx, "exporting instance information to filesystem",
			zap.Error(err),
		)
		return nil, errs.ErrInternalNoSub
	}

	// 8. Unlock RW instance
	//    -> defered after 5 (fault-tolerance)
	// 9. Unlock R challenge
	//    -> defered after 2 (fault-tolerance)

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
		Flags: fsist.Flags,
	}, nil
}
