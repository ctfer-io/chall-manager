package launch

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/ctfer-io/chall-manager/global"
	"github.com/ctfer-io/chall-manager/pkg/identity"
	"github.com/ctfer-io/chall-manager/pkg/lock"
	"github.com/ctfer-io/chall-manager/pkg/scenario"
	"github.com/pkg/errors"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/emptypb"
	"gopkg.in/yaml.v3"
)

func (server *launcherServer) DeleteLaunch(ctx context.Context, req *LaunchRequest) (*emptypb.Empty, error) {
	logger := global.Log()

	// Generate request identity
	id := identity.Compute(req.ChallengeId, req.SourceId)

	// Make sure only 1 parallel launch for this challenge
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

	// Decode scenario, fetch state and build stack from them
	dir, err := scenario.Decode(req.ChallengeId, req.Scenario)
	if err != nil {
		return nil, err
	}
	stack, err := loadStackState(ctx, filepath.Join(global.Conf.Directory, "scenarios", req.ChallengeId, dir), id)
	if err != nil {
		return nil, err
	}

	// Destroy resources
	logger.Info("destroying challenge scenario",
		zap.String("challenge_id", req.ChallengeId),
		zap.String("stack_name", stack.Name()),
	)
	_, err = stack.Destroy(ctx)
	if err != nil {
		return nil, err
	}

	// Delete stack state
	if err := os.RemoveAll(filepath.Join(global.Conf.Directory, "states", id)); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func loadStackState(ctx context.Context, dir, id string) (auto.Stack, error) {
	// Get project name
	b, err := os.ReadFile(filepath.Join(dir, "Pulumi.yaml"))
	if err != nil {
		return auto.Stack{}, err
	}
	type PulumiYaml struct {
		Name string `yaml:"name"`
		// Runtime and Description are not used
	}
	var yml PulumiYaml
	if err := yaml.Unmarshal(b, &yml); err != nil {
		return auto.Stack{}, err
	}

	// Create workspace in decoded+unzipped archive directory
	ws, err := auto.NewLocalWorkspace(ctx,
		auto.WorkDir(dir),
		auto.EnvVars(map[string]string{
			"PULUMI_CONFIG_PASSPHRASE": "",
			"CM_PROJECT":               yml.Name, // necessary to load the configuration
		}),
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

	// Load state
	bs, err := os.ReadFile(filepath.Join(global.Conf.Directory, id))
	if err != nil {
		return auto.Stack{}, err
	}
	if err := stack.Import(ctx, apitype.UntypedDeployment{
		Version:    3,
		Deployment: json.RawMessage(bs),
	}); err != nil {
		return auto.Stack{}, err
	}

	return stack, nil
}
