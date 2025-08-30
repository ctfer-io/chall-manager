package iac

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"

	"github.com/ctfer-io/chall-manager/global"
	errs "github.com/ctfer-io/chall-manager/pkg/errors"
	"github.com/ctfer-io/chall-manager/pkg/fs"
	"github.com/pkg/errors"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
	"gopkg.in/yaml.v3"
)

func NewStack(ctx context.Context, id string, fschall *fs.Challenge) (auto.Stack, error) {
	stack, err := LoadStack(ctx, fschall.Directory, id)
	if err != nil {
		return auto.Stack{}, &errs.ErrInternal{Sub: err}
	}

	if err := stack.SetAllConfig(ctx, auto.ConfigMap{
		"identity": auto.ConfigValue{Value: id},
	}); err != nil {
		return auto.Stack{}, &errs.ErrInternal{Sub: err}
	}

	return stack, nil
}

func LoadStack(ctx context.Context, dir, id string) (auto.Stack, error) {
	// Track span of loading stack
	ctx, span := global.Tracer.Start(ctx, "loading-stack")
	defer span.End()

	// Get project name
	b, fname, err := loadPulumiYml(dir)
	if err != nil {
		return auto.Stack{}, &errs.ErrInternal{Sub: errors.Wrap(err, "invalid scenario")}
	}
	var yml workspace.Project
	if err := yaml.Unmarshal(b, &yml); err != nil {
		return auto.Stack{}, &errs.ErrInternal{Sub: errors.Wrap(err, "invalid Pulumi yaml content")}
	}

	// Build Go binaries if possible
	if yml.Runtime.Name() == "go" {
		if _, ok := yml.Runtime.Options()["binary"]; !ok {
			if err := compile(dir); err != nil {
				return auto.Stack{}, err
			}
			yml.Runtime.SetOption("binary", "./main")

			b, err = yaml.Marshal(yml)
			if err != nil {
				return auto.Stack{}, &errs.ErrInternal{Sub: err}
			}
			if err := os.WriteFile(filepath.Join(dir, fname), b, 0o600); err != nil {
				return auto.Stack{}, &errs.ErrInternal{Sub: err}
			}
		}
	}
	// Make it executable (OCI does not natively copy permissions)
	if err := os.Chmod(filepath.Join(dir, "main"), 0o766); err != nil {
		return auto.Stack{}, err
	}

	// Check supported runtimes
	if !slices.Contains(global.PulumiRuntimes, yml.Runtime.Name()) {
		return auto.Stack{}, fmt.Errorf("got unsupported runtime: %s", yml.Runtime.Name())
	}

	// Create workspace in scenario directory
	envVars := map[string]string{
		"PULUMI_CONFIG_PASSPHRASE": "",
		"CM_PROJECT":               yml.Name.String(), // necessary to load the configuration
	}
	if global.Conf.OCI.Username != "" {
		envVars["OCI_USERNAME"] = global.Conf.OCI.Username
	}
	if global.Conf.OCI.Password != "" {
		envVars["OCI_PASSWORD"] = global.Conf.OCI.Password
	}
	ws, err := auto.NewLocalWorkspace(ctx,
		auto.WorkDir(dir),
		auto.EnvVars(envVars),
	)
	if err != nil {
		return auto.Stack{}, &errs.ErrInternal{Sub: errors.Wrap(err, "new local workspace")}
	}

	// Build stack
	stackName := auto.FullyQualifiedStackName("organization", yml.Name.String(), id)
	stack, err := auto.UpsertStack(ctx, stackName, ws)
	if err != nil {
		return auto.Stack{}, &errs.ErrInternal{Sub: errors.Wrapf(err, "upsert stack %s", stackName)}
	}
	return stack, nil
}

func Extract(ctx context.Context, stack auto.Stack, sr auto.UpResult, fsist *fs.Instance) error {
	udp, err := stack.Export(ctx)
	if err != nil {
		return &errs.ErrInternal{Sub: err}
	}
	coninfo, ok := sr.Outputs["connection_info"]
	if !ok {
		return &errs.ErrInternal{Sub: err}
	}
	var flag *string
	if f, ok := sr.Outputs["flag"]; ok {
		ff := f.Value.(string)
		flag = &ff
	}

	fsist.State = udp.Deployment
	fsist.ConnectionInfo = coninfo.Value.(string)
	fsist.Flag = flag
	return nil
}

func Additional(ctx context.Context, stack auto.Stack, challAdd, istAdd map[string]string) error {
	// Merge configuration, override challenge one with instance if necessary
	cm := map[string]string{}
	for k, v := range challAdd {
		cm[k] = v
	}
	for k, v := range istAdd {
		cm[k] = v
	}

	// Marshal in object
	b, err := json.Marshal(cm)
	if err != nil {
		return err
	}

	// Set in additional configuration
	return stack.SetConfig(ctx, "additional", auto.ConfigValue{Value: string(b)})
}

func loadPulumiYml(dir string) ([]byte, string, error) {
	b, err := os.ReadFile(filepath.Join(dir, "Pulumi.yaml"))
	if err == nil {
		return b, "Pulumi.yaml", nil
	}
	b, err = os.ReadFile(filepath.Join(dir, "Pulumi.yml"))
	if err == nil {
		return b, "Pulumi.yml", nil
	}
	return nil, "", err
}

func compile(dir string) error {
	mainPath := filepath.Join(dir, "main")
	cmd := exec.Command("go", "build", "-o", mainPath, mainPath+".go")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "%s", out)
	}
	return nil
}
