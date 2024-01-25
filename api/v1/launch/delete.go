package launch

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/ctfer-io/chall-manager/global"
	"github.com/pkg/errors"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/emptypb"
	"gopkg.in/yaml.v3"
)

func (server *launcherServer) DeleteLaunch(ctx context.Context, req *LaunchRequest) (*emptypb.Empty, error) {
	// 1. Generate request identity
	id := identity(req.ChallengeId, req.SourceId)

	// 2. Make sure only 1 parallel launch for this challenge (avoid overwriting files
	// during parallel requests handling).
	challLock.Lock()
	mx, ok := challLocks[req.ChallengeId]
	if !ok {
		mx = &sync.Mutex{}
		challLocks[req.ChallengeId] = mx
	}
	mx.Lock()
	defer mx.Unlock()
	challLock.Unlock()

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
	global.Log().Info(
		"destroying challenge scenario",
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
