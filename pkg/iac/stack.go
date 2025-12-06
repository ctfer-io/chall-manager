package iac

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
	"gopkg.in/yaml.v3"

	"github.com/ctfer-io/chall-manager/global"
	errs "github.com/ctfer-io/chall-manager/pkg/errors"
	fsapi "github.com/ctfer-io/chall-manager/pkg/fs"
)

type Stack struct {
	// pulumi auto stack
	pas auto.Stack
}

func NewStack(ctx context.Context, fschall *fsapi.Challenge, id string) (*Stack, error) {
	stack, err := LoadStack(ctx, fschall.Scenario, id)
	if err != nil {
		return nil, &errs.ErrInternal{Sub: err}
	}

	if err := stack.pas.SetAllConfig(ctx, auto.ConfigMap{
		"identity": auto.ConfigValue{Value: id},
	}); err != nil {
		return nil, &errs.ErrInternal{Sub: err}
	}

	return stack, nil
}

// LoadStack upsert a Pulumi stack for a given scenario and instance identity.
func LoadStack(ctx context.Context, scenario, id string) (*Stack, error) {
	// Track span of loading stack
	ctx, span := global.Tracer.Start(ctx, "loading-stack")
	defer span.End()

	// Load the scenario
	dir, err := global.GetOCIManager().Load(ctx, scenario)
	if err != nil {
		return nil, err
	}

	// Get scenario's project name
	b, err := loadPulumiYml(dir)
	if err != nil {
		return nil, &errs.ErrScenario{Sub: errors.Wrap(err, "no Pulumi.yaml/Pulumi.yml file")}
	}
	var yml workspace.Project
	if err := yaml.Unmarshal(b, &yml); err != nil {
		return nil, &errs.ErrScenario{Sub: errors.Wrap(err, "invalid Pulumi YAML content")}
	}

	// Create workspace in scenario directory
	envVars := map[string]string{
		"PULUMI_CONFIG_PASSPHRASE": "",
		"CM_PROJECT":               yml.Name.String(), // necessary to load the configuration
	}
	ws, err := auto.NewLocalWorkspace(ctx,
		auto.WorkDir(dir),
		auto.EnvVars(envVars),
	)
	if err != nil {
		return nil, &errs.ErrInternal{Sub: errors.Wrap(err, "new local workspace")}
	}

	// Build stack
	stackName := auto.FullyQualifiedStackName("organization", yml.Name.String(), id)
	pas, err := auto.UpsertStack(ctx, stackName, ws)
	if err != nil {
		return nil, &errs.ErrInternal{Sub: errors.Wrapf(err, "upsert stack %s", stackName)}
	}
	return &Stack{
		pas: pas,
	}, nil
}

// Additional packs the challenge and instance additional k=v entries together
// then configure them in the stack configuration.
// If the same key is defined in both, the instance's additional k=v is kept.
func Additional(ctx context.Context, stack *Stack, challAdd, istAdd map[string]string) error {
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
	return stack.pas.SetConfig(ctx, "additional", auto.ConfigValue{Value: string(b)})
}

type Result struct {
	sub auto.UpResult
}

func (stack *Stack) Up(ctx context.Context) (*Result, error) {
	res, err := stack.pas.Up(ctx)
	if err != nil {
		return nil, err
	}
	return &Result{
		sub: res,
	}, nil
}

func (stack *Stack) Preview(ctx context.Context) error {
	_, err := stack.pas.Preview(ctx)
	return err
}

func (stack *Stack) Down(ctx context.Context) error {
	_, err := stack.pas.Destroy(ctx)
	return err
}

// Export the state results into the instance, i.e., the connection information,
// flags, and state for later update and delete operations.
func (stack *Stack) Export(ctx context.Context, res *Result, ist *fsapi.Instance) error {
	udp, err := stack.pas.Export(ctx)
	if err != nil {
		return &errs.ErrInternal{Sub: err}
	}
	coninfo, ok := res.sub.Outputs["connection_info"]
	if !ok {
		return &errs.ErrInternal{Sub: err}
	}

	// For migration purposes, we still support "flag" as a valid output for a while.
	// After this arbitrary period, only the "flags" output will be supported.
	flags := []string{}
	if f, ok := res.sub.Outputs["flag"]; ok {
		// If there is a single flag defined, let's use it
		flags = append(flags, f.Value.(string))
	}
	if f, ok := res.sub.Outputs["flags"]; ok {
		if fs, ok := f.Value.([]any); ok {
			for _, f := range fs {
				// Should be a string, else there is a problem
				if _, ok := f.(string); !ok {
					return &errs.ErrInternal{Sub: fmt.Errorf("invalid flag type for %v, should be a string", f)}
				}
				flags = append(flags, f.(string))
			}
		}
		if !ok {
			return &errs.ErrInternal{Sub: fmt.Errorf("invalid flags type, should be an array")}
		}
	}

	ist.State = udp.Deployment
	ist.ConnectionInfo = coninfo.Value.(string)
	ist.Flags = flags
	return nil
}

func (stack *Stack) Import(ctx context.Context, ist *fsapi.Instance) error {
	s, err := json.Marshal(ist.State)
	if err != nil {
		return err
	}

	return stack.pas.Import(ctx, apitype.UntypedDeployment{
		Version:    3,
		Deployment: s,
	})
}

func loadPulumiYml(dir string) ([]byte, error) {
	b, err := os.ReadFile(filepath.Join(dir, "Pulumi.yaml"))
	if err == nil {
		return b, nil
	}
	b, err = os.ReadFile(filepath.Join(dir, "Pulumi.yml"))
	if err == nil {
		return b, nil
	}
	return nil, err // this should not happen as it has been validated by the OCI service
}
