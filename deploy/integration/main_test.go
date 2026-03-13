package integration_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ctfer-io/chall-manager/pkg/scenario"
	"github.com/pkg/errors"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gopkg.in/yaml.v3"
)

const (
	registry = "localhost:5000" // as defined per the kind-config.yaml file
	Scn23Ref = "registry:5000/scenario:23"
	Scn25Ref = "registry:5000/scenario:25"
)

var (
	Server = os.Getenv("SERVER")
)

func TestMain(m *testing.M) {
	if err := testmain(m); err != nil {
		fmt.Printf("FAILED: %s", err)
		os.Exit(1)
	}
}

func testmain(m *testing.M) error {
	if Server == "" {
		fmt.Println("Environment variable SERVER is not set, please indicate the domain name/IP address to reach out the cluster.")
	}

	// Push the scenarios used during tests
	if _, ok := os.LookupEnv("REGISTRY"); ok {
		ctx := context.Background()
		cwd, _ := os.Getwd()
		if err := scenario.EncodeOCI(ctx,
			fmt.Sprintf("%s/scenario:23", registry), filepath.Join(cwd, "scn23"),
			true, "", "",
		); err != nil {
			return errors.Wrap(err, "failed to push scn23")
		}
		if err := scenario.EncodeOCI(ctx,
			fmt.Sprintf("%s/scenario:25", registry), filepath.Join(cwd, "scn25"),
			true, "", "",
		); err != nil {
			return errors.Wrap(err, "failed to push scn25")
		}
	}

	// Compile the stack
	pwd, err := os.Getwd()
	if err != nil {
		return errors.Wrap(err, "get working directory")
	}
	pdir := filepath.Join(pwd, "..")
	cmd := exec.Command("go", "build", "-cover", "-o", "main", "main.go")
	cmd.Dir = pdir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "stack compilation failed with output: %s\n", out)
	}
	defer func() {
		_ = os.Remove(filepath.Join(pdir, "main"))
	}()

	// Re-write the Pulumi.yaml file to use the compiled binary
	b, err := os.ReadFile(filepath.Join(pdir, "Pulumi.yaml"))
	if err != nil {
		return errors.Wrap(err, "could not read Pulumi.yaml file")
	}
	var proj workspace.Project
	if err := yaml.Unmarshal(b, &proj); err != nil {
		return errors.Wrap(err, "invalid Pulumi.yaml content")
	}
	proj.Runtime.SetOption("binary", "./main")
	altered, err := yaml.Marshal(proj)
	if err != nil {
		return errors.Wrap(err, "marshalling Pulumi.yaml content")
	}
	if err := os.WriteFile(filepath.Join(pdir, "Pulumi.yaml"), altered, 0600); err != nil {
		return errors.Wrap(err, "writing back Pulumi.yaml")
	}
	defer func() {
		_ = os.WriteFile(filepath.Join(pdir, "Pulumi.yaml"), b, 0600) //nolint:gosec //#gosec G703 -- Don't bother with tests
	}()

	if code := m.Run(); code != 0 {
		return fmt.Errorf("exit with code %d", code)
	}
	return nil
}

func grpcClient(t *testing.T, outputs map[string]any) *grpc.ClientConn {
	port := fmt.Sprintf("%0.f", outputs["exposed_port"].(float64))
	cli, err := grpc.NewClient(
		fmt.Sprintf("%s:%s", Server, port),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("during gRPC client generation: %s", err)
	}
	return cli
}

func stackName(tname string) (out string) {
	out = tname
	out = strings.TrimPrefix(out, "Test_I_")
	out = strings.ToLower(out)
	return out
}
