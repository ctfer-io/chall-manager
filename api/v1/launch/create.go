package launch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/ctfer-io/chall-manager/global"
	"github.com/ctfer-io/chall-manager/pkg/identity"
	"github.com/ctfer-io/chall-manager/pkg/lock"
	"github.com/ctfer-io/chall-manager/pkg/scenario"
	"github.com/ctfer-io/chall-manager/pkg/state"
	"github.com/pkg/errors"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

func (server *launcherServer) CreateLaunch(ctx context.Context, req *LaunchRequest) (*LaunchResponse, error) {
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

	// Check the state does not already exist i.e. could not overwrite it.
	if _, err := os.Stat(filepath.Join(global.Conf.Directory, "states", id)); err == nil {
		return nil, errors.New("state already existing")
	}

	// Decode scenario and build stack from it
	dir, err := scenario.Decode(req.ChallengeId, req.Scenario)
	if err != nil {
		return nil, err
	}
	stack, err := createStack(ctx, filepath.Join(global.Conf.Directory, "scenarios", req.ChallengeId, dir), id)
	if err != nil {
		return nil, err
	}

	// Configure "chall-manager to SDK" API
	if err := stack.SetAllConfig(ctx, auto.ConfigMap{
		"identity": auto.ConfigValue{Value: id},
	}); err != nil {
		return nil, err
	}

	// Deploy resources
	logger.Info("deploying challenge scenario",
		zap.String("challenge_id", req.ChallengeId),
		zap.String("stack_name", stack.Name()),
	)
	sr, err := stack.Up(ctx)
	if err != nil {
		return nil, err
	}

	// Export stack+state for reuse later
	st, err := state.New(ctx, stack, state.StateMetadata{
		ChallengeId: req.ChallengeId,
		SourceId:    req.SourceId,
		SourceDir:   dir,
		Until:       untilFromNow(req.Dates),
	}, sr.Outputs)
	if err != nil {
		return nil, err
	}
	if err := st.Export(id); err != nil {
		return nil, err
	}

	return response(st), nil
}

func createStack(ctx context.Context, dir, id string) (auto.Stack, error) {
	// Get project name
	b, _ := os.ReadFile(filepath.Join(dir, "Pulumi.yaml"))
	type PulumiYaml struct {
		Name    string `yaml:"name"`
		Runtime string `yaml:"runtime"`
		// Description is not used
	}
	var yml PulumiYaml
	if err := yaml.Unmarshal(b, &yml); err != nil {
		return auto.Stack{}, err
	}

	// Check supported runtimes
	if !slices.Contains(global.PulumiRuntimes, yml.Runtime) {
		return auto.Stack{}, fmt.Errorf("got unsupported runtime: %s", yml.Runtime)
	}

	// Create workspace in decoded+unzipped archive directory
	envVars := map[string]string{
		"PULUMI_CONFIG_PASSPHRASE": "",
		"CM_PROJECT":               yml.Name, // necessary to load the configuration
	}
	saToken, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
	if err == nil {
		envVars["CM_SATOKEN"] = string(saToken) // transmit the Kubernetes ServiceAccount projected token to the stack
	}
	ws, err := auto.NewLocalWorkspace(ctx,
		auto.WorkDir(dir),
		auto.EnvVars(envVars),
	)
	if err != nil {
		return auto.Stack{}, err
	}

	// Build stack
	stackName := auto.FullyQualifiedStackName("organization", yml.Name, id)
	stack, err := auto.UpsertStack(ctx, stackName, ws)
	if err != nil {
		return auto.Stack{}, errors.Wrapf(err, "while upserting stack %s", stackName)
	}
	return stack, nil
}
