package challenge

import (
	context "context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ctfer-io/chall-manager/api/v1/common"
	instance "github.com/ctfer-io/chall-manager/api/v1/instance"
	"github.com/ctfer-io/chall-manager/global"
	"github.com/ctfer-io/chall-manager/pkg/fs"
	"github.com/ctfer-io/chall-manager/pkg/lock"
	"github.com/ctfer-io/chall-manager/pkg/scenario"
	"go.uber.org/multierr"
	"go.uber.org/zap"
)

func (store *Store) CreateChallenge(ctx context.Context, req *CreateChallengeRequest) (*Challenge, error) {
	logger := global.Log()

	// 1. Lock R TOTW
	totw, err := common.LockTOTW()
	if err != nil {
		logger.Error("build TOTW lock", zap.Error(err))
		return nil, common.ErrInternal
	}
	defer common.LClose(totw)
	if err := totw.RLock(); err != nil {
		logger.Error("TOTW R lock", zap.Error(err))
		return nil, common.ErrInternal
	}

	// 2. Lock RW challenge
	clock, err := common.LockChallenge(req.Id)
	if err != nil {
		logger.Error("build challenge lock", zap.Error(multierr.Combine(
			totw.RUnlock(),
			err,
		)))
		return nil, common.ErrInternal
	}
	defer common.LClose(clock)
	if err := clock.RWLock(); err != nil {
		logger.Error("challenge RW lock", zap.Error(multierr.Combine(
			totw.RUnlock(),
			err,
		)))
		return nil, common.ErrInternal
	}
	defer func(lock lock.RWLock) {
		if err := lock.RWUnlock(); err != nil {
			logger.Error("challenge RW unlock", zap.Error(err))
		}
	}(clock)

	// 3. Unlock R TOTW
	if err := totw.RUnlock(); err != nil {
		logger.Error("TOTW R unlock", zap.Error(err))
		return nil, common.ErrInternal
	}

	// 4. If the challenge already exist, return error
	challDir := filepath.Join(global.Conf.Directory, "chall", req.Id)
	if _, err := os.Stat(challDir); err == nil {
		return nil, fmt.Errorf("challenge %s already exist", req.Id)
	}

	// 5. Save challenge
	logger.Info("creating challenge", zap.String("id", req.Id))
	if err := os.Mkdir(challDir, os.ModePerm); err != nil {
		logger.Error("creating challenge directory", zap.Error(err))
		return nil, common.ErrInternal
	}
	dir, err := scenario.Decode(challDir, req.Scenario)
	if err != nil {
		// XXX should check if fs-related error of due to invalid "scenario" content (i.e. should log or return the error)
		logger.Error("exporting scenario on filesystem", zap.Error(err))
		return nil, common.ErrInternal
	}
	h := hash(req.Scenario)
	fschall := &fs.Challenge{
		ID:        req.Id,
		Directory: filepath.Join(challDir, dir),
		Hash:      h,
		Until:     untilString(req.Dates),
		Timeout:   timeoutString(req.Dates),
	}
	if err := fschall.Save(); err != nil {
		logger.Error("exporting challenge information to filesystem",
			zap.String("challenge_id", req.Id),
			zap.Error(err),
		)
		return nil, err
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