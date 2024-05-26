package challenge

import (
	context "context"
	"crypto/md5"
	"encoding/hex"
	"time"

	"github.com/ctfer-io/chall-manager/api/v1/common"
	instance "github.com/ctfer-io/chall-manager/api/v1/instance"
	"github.com/ctfer-io/chall-manager/global"
	errs "github.com/ctfer-io/chall-manager/pkg/errors"
	"github.com/ctfer-io/chall-manager/pkg/fs"
	"github.com/ctfer-io/chall-manager/pkg/lock"
	"github.com/ctfer-io/chall-manager/pkg/scenario"
	"go.uber.org/multierr"
	"go.uber.org/zap"
)

func (store *Store) CreateChallenge(ctx context.Context, req *CreateChallengeRequest) (*Challenge, error) {
	logger := global.Log()
	ctx = global.WithChallengeId(ctx, req.Id)

	// 1. Lock R TOTW
	totw, err := common.LockTOTW(ctx)
	if err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "build TOTW lock", zap.Error(err))
		return nil, errs.ErrInternalNoSub
	}
	defer common.LClose(totw)
	if err := totw.RLock(); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "TOTW R lock", zap.Error(err))
		return nil, errs.ErrInternalNoSub
	}

	// 2. Lock RW challenge
	clock, err := common.LockChallenge(ctx, req.Id)
	if err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "build challenge lock", zap.Error(multierr.Combine(
			totw.RUnlock(),
			err,
		)))
		return nil, errs.ErrInternalNoSub
	}
	defer common.LClose(clock)
	if err := clock.RWLock(); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "challenge RW lock", zap.Error(multierr.Combine(
			totw.RUnlock(),
			err,
		)))
		return nil, errs.ErrInternalNoSub
	}
	defer func(lock lock.RWLock) {
		if err := lock.RWUnlock(); err != nil {
			err := &errs.ErrInternal{Sub: err}
			logger.Error(ctx, "challenge RW unlock", zap.Error(err))
		}
	}(clock)

	// 3. Unlock R TOTW
	if err := totw.RUnlock(); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "TOTW R unlock", zap.Error(err))
		return nil, errs.ErrInternalNoSub
	}

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
		Until:     untilString(req.Dates),
		Timeout:   timeoutString(req.Dates),
	}
	if err := fschall.Save(); err != nil {
		err := &errs.ErrInternal{Sub: err}
		logger.Error(ctx, "exporting challenge information to filesystem",
			zap.Error(err),
		)
		return nil, errs.ErrInternalNoSub
	}

	chall := &Challenge{
		Id:        req.Id,
		Hash:      h,
		Dates:     datesIO(req.Dates),
		Instances: []*instance.Instance{},
	}

	// 6. Unlock RW challenge
	//    -> defered after 2 (fault-tolerance)

	return chall, nil
}

func untilString(dates isCreateChallengeRequest_Dates) *time.Time {
	until, ok := dates.(*CreateChallengeRequest_Until)
	if !ok {
		return nil
	}
	t := until.Until.AsTime()
	return &t
}

func timeoutString(dates isCreateChallengeRequest_Dates) *time.Duration {
	timeout, ok := dates.(*CreateChallengeRequest_Timeout)
	if !ok {
		return nil
	}
	d := timeout.Timeout.AsDuration()
	return &d
}

func datesIO(dates isCreateChallengeRequest_Dates) isChallenge_Dates {
	if d, ok := dates.(*CreateChallengeRequest_Until); ok {
		return &Challenge_Until{
			Until: d.Until,
		}
	}
	if d, ok := dates.(*CreateChallengeRequest_Timeout); ok {
		return &Challenge_Timeout{
			Timeout: d.Timeout,
		}
	}
	return nil
}

func hash(scenario string) string {
	h := md5.New()
	h.Write([]byte(scenario))
	return hex.EncodeToString(h.Sum(nil))
}
