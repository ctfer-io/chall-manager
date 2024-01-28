package launch

import (
	"context"

	"github.com/ctfer-io/chall-manager/global"
	"github.com/ctfer-io/chall-manager/pkg/identity"
	"github.com/ctfer-io/chall-manager/pkg/lock"
	"github.com/ctfer-io/chall-manager/pkg/state"
	"go.uber.org/zap"
)

func (server *launcherServer) UpdateLaunch(ctx context.Context, req *UpdateLaunchRequest) (*LaunchResponse, error) {
	logger := global.Log()

	// Generate request identity
	id := identity.Compute(req.ChallengeId, req.SourceId)

	// Make sure only 1 parallel launch for this challenge instance
	// (avoid overwriting files during parallel requests handling).
	release, err := lock.Acquire(ctx, id)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := release(); err != nil {
			logger.Error("failed to release lock, could stuck the identity until renewal",
				zap.Error(err),
			)
		}
	}()

	// Load existing state
	st, err := state.Load(id)
	if err != nil {
		return nil, err
	}

	st.Metadata.Until = untilFromNow(req.Dates)

	// TODO export state

	return response(st), nil
}
