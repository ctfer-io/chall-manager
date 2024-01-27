package launch

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/ctfer-io/chall-manager/global"
	"github.com/ctfer-io/chall-manager/lock"
	"github.com/pkg/errors"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/emptypb"
	"gopkg.in/yaml.v3"
)

func (server *launcherServer) DeleteLaunch(ctx context.Context, req *LaunchRequest) (*emptypb.Empty, error) {
	logger := global.Log()

	// 1. Generate request identity
	id := identity(req.ChallengeId, req.SourceId)

	// 2. Make sure only 1 parallel launch for this challenge
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

	// 3. Decode+Unzip scenario
	dir, err := decodeAndUnzip(req.ChallengeId, req.Scenario)
	if err != nil {
		return nil, err
	}

	// 4. Load stack
	stack, err := loadStackState(ctx, req, dir, id)
	if err != nil {
		return nil, err
	}

	// 5. Call factory
	logger.Info("destroying challenge scenario",
		zap.String("challenge_id", req.ChallengeId),
		zap.String("stack_name", stack.Name()),
	)
	_, err = stack.Destroy(ctx)
	if err != nil {
		return nil, err
	}

	// 6. Delete stack info
	if err := os.RemoveAll(filepath.Join(global.Conf.StatesDir, id)); err != nil {
		return nil, err
	}

	// 7. Build response (empty body)
	return &emptypb.Empty{}, nil
}

func loadStackState(ctx context.Context, req *LaunchRequest, dir, identity string) (auto.Stack, error) {
	// Get project name
	b, err := os.ReadFile(filepath.Join(dir, "Pulumi.yaml"))
	type PulumiYaml struct {
		Name string `yaml:"name"`
		// Runtime and Description are not used
	}
	if err != nil {
		return auto.Stack{}, err
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
			"CM_PROJECT":               yml.Name, // necessary to load the configuration // TODO provide a way to configure the challenge
		}),
	)
	if err != nil {
		return auto.Stack{}, err
	}

	// Build stack
	stackName := auto.FullyQualifiedStackName("organization", yml.Name, "chall-manager-"+req.ChallengeId)
	stack, err := auto.UpsertStack(ctx, stackName, ws)
	if err != nil {
		return auto.Stack{}, errors.Wrapf(err, "while upserting stack %s", stackName)
	}

	// Load state
	bs, err := os.ReadFile(filepath.Join(global.Conf.StatesDir, identity))
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
