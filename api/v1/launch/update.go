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

	// Signal to the Top-Of-The-World lock the request entered the room
	// thus can't get stopped by incoming Query request.
	totw, err := TOTWLock(ctx)
	if err != nil {
		return nil, err
	}
	defer lclose(totw)
	if err := totw.RLock(); err != nil {
		return nil, err
	}
	defer func(totw lock.RWLock) {
		if err := totw.RUnlock(); err != nil {
			logger.Error("failed to release Top-Of-The-World lock, could stuck the whole system",
				zap.Error(err),
			)
		}
	}(totw)

	// Make sure only 1 parallel launch for this challenge instance
	// (avoid overwriting files during parallel requests handling).
	rw, err := lock.NewRWLock(id)
	if err != nil {
		return nil, err
	}
	defer lclose(rw)
	if err := rw.RWLock(); err != nil {
		return nil, err
	}
	defer func(rw lock.RWLock) {
		if err := rw.RWUnlock(); err != nil {
			logger.Error("failed to release lock, could stuck the identity until renewal",
				zap.Error(err),
			)
		}
	}(rw)

	// Load existing state
	st, err := state.Load(id)
	if err != nil {
		return nil, err
	}

	st.Metadata.Until = untilFromNow(req.Dates)

	// Export state
	if err := st.Export(id); err != nil {
		return nil, err
	}

	return response(st), nil
}
