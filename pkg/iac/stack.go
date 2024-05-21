package iac

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/ctfer-io/chall-manager/global"
	errs "github.com/ctfer-io/chall-manager/pkg/errors"
	"github.com/ctfer-io/chall-manager/pkg/fs"
	"github.com/ctfer-io/chall-manager/pkg/identity"
	"github.com/pkg/errors"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"gopkg.in/yaml.v3"
)

func NewStack(ctx context.Context, fschall *fs.Challenge, sourceId string) (auto.Stack, error) {
	id := identity.Compute(fschall.ID, sourceId)
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
	// Get project name
	b, err := os.ReadFile(filepath.Join(dir, "Pulumi.yaml"))
	if err != nil {
		return auto.Stack{}, &errs.ErrInternal{Sub: errors.Wrap(err, "invalid scenario")}
	}
	type PulumiYaml struct {
		Name    string `yaml:"name"`
		Runtime string `yaml:"runtime"`
		// Description is not used
	}
	var yml PulumiYaml
	if err := yaml.Unmarshal(b, &yml); err != nil {
		return auto.Stack{}, &errs.ErrInternal{Sub: errors.Wrap(err, "invalid Pulumi yaml content")}
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
	if global.Conf.OCI.RegistryURL != nil {
		envVars["OCI_REGISTRY_URL"] = *global.Conf.OCI.RegistryURL
	}
	if global.Conf.OCI.Username != nil {
		envVars["OCI_REGISTRY_USERNAME"] = *global.Conf.OCI.Username
	}
	if global.Conf.OCI.Password != nil {
		envVars["OCI_REGISTRY_PASSWORD"] = *global.Conf.OCI.Password
	}
	ws, err := auto.NewLocalWorkspace(ctx,
		auto.WorkDir(dir),
		auto.EnvVars(envVars),
	)
	if err != nil {
		return auto.Stack{}, &errs.ErrInternal{Sub: errors.Wrap(err, "new local workspace")}
	}

	// Build stack
	stackName := auto.FullyQualifiedStackName("organization", yml.Name, id)
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
