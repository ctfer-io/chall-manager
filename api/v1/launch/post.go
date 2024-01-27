package launch

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/ctfer-io/chall-manager/global"
	"github.com/ctfer-io/chall-manager/lock"
	"github.com/pkg/errors"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

func (server *launcherServer) CreateLaunch(ctx context.Context, req *LaunchRequest) (*LaunchResponse, error) {
	logger := global.Log()

	// 1. Generate request identity
	id := identity(req.ChallengeId, req.SourceId)

	// 2. Make sure only 1 parallel launch for this challenge instance
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

	// 4. Build stack iif there is none already existing
	if _, err := os.Stat(filepath.Join(global.Conf.StatesDir, id)); err == nil {
		return nil, errors.New("state already existing")
	}
	stack, err := createStack(ctx, req, dir)
	if err != nil {
		return nil, err
	}

	// 5. Configure factory
	if err := stack.SetAllConfig(ctx, auto.ConfigMap{
		"identity": auto.ConfigValue{Value: id},
	}); err != nil {
		return nil, err
	}

	// 6. Call factory
	logger.Info("deploying challenge scenario",
		zap.String("challenge_id", req.ChallengeId),
		zap.String("stack_name", stack.Name()),
	)
	sr, err := stack.Up(ctx)
	if err != nil {
		return nil, err
	}

	// 7. Save stack info
	if err := exportStackState(ctx, stack, id); err != nil {
		return nil, err
	}

	// 8. Build response
	if _, ok := sr.Outputs["connection_info"]; !ok {
		return nil, errors.New("connection_info is not defined in the stack outputs")
	}
	return &LaunchResponse{
		ConnectionInfo: sr.Outputs["connection_info"].Value.(string),
	}, nil
}

func decodeAndUnzip(challID, scenario string) (string, error) {
	// Create challenge directory, delete previous if any
	cd := filepath.Join(global.Conf.ScenarioDir, challID)
	outDir := ""
	if _, err := os.Stat(cd); err == nil {
		if err := os.RemoveAll(cd); err != nil {
			return "", err
		}
	}
	if err := os.Mkdir(cd, os.ModePerm); err != nil {
		return "", err
	}

	// Decode base64
	b, err := base64.StdEncoding.DecodeString(scenario)
	if err != nil {
		return "", err
	}

	// Unzip content into it
	r, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return "", err
	}
	for _, f := range r.File {
		filePath := filepath.Join(cd, f.Name)

		if f.FileInfo().IsDir() {
			if outDir != "" {
				return "", errors.New("archive contain multiple directories, should not occur")
			}
			outDir = f.Name
			if err := os.MkdirAll(filePath, os.ModePerm); err != nil {
				return "", err
			}
			continue
		}

		outFile, err := os.Create(filePath)
		if err != nil {
			return "", err
		}
		defer outFile.Close()

		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		defer rc.Close()

		if _, err := io.Copy(outFile, rc); err != nil {
			return "", err
		}
	}
	return filepath.Join(cd, outDir), nil
}

func createStack(ctx context.Context, req *LaunchRequest, dir string) (auto.Stack, error) {
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
	stackName := auto.FullyQualifiedStackName("organization", yml.Name, "chall-manager-"+req.ChallengeId)
	stack, err := auto.UpsertStack(ctx, stackName, ws)
	if err != nil {
		return auto.Stack{}, errors.Wrapf(err, "while upserting stack %s", stackName)
	}
	return stack, nil
}

func exportStackState(ctx context.Context, stack auto.Stack, identity string) error {
	udp, err := stack.Export(ctx)
	if err != nil {
		return err
	}

	b, _ := udp.Deployment.MarshalJSON()
	return os.WriteFile(filepath.Join(global.Conf.StatesDir, identity), b, os.ModePerm)
}
