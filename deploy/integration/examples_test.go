package integration_test

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"testing"

	"github.com/pkg/errors"
	"github.com/pulumi/pulumi/pkg/v3/testing/integration"
	"github.com/stretchr/testify/require"

	"github.com/ctfer-io/chall-manager/api/v1/challenge"
	"github.com/ctfer-io/chall-manager/pkg/scenario"
)

var examples = []string{
	"additional",
	"exposed-monopod",
	"kompose",
	"kubernetes",
	"no-sdk",
	"prebuilt",
	"teeworlds",
}

func Test_I_Examples(t *testing.T) {
	require.NotEmpty(t, Server)

	cwd, _ := os.Getwd()
	exDir := filepath.Join(cwd, "..", "..", "examples")

	// Trigger prebuilt case
	if err := compile(
		filepath.Join(exDir, "teeworlds"),
		filepath.Join(exDir, "prebuilt"),
	); err != nil {
		t.Fatal(err)
	}

	// Run tests
	integration.ProgramTest(t, &integration.ProgramTestOptions{
		Quick:       true,
		SkipRefresh: true,
		StackName:   stackName(t.Name()),
		Dir:         path.Join(cwd, ".."),
		Config: map[string]string{
			"namespace":        os.Getenv("NAMESPACE"),
			"registry":         os.Getenv("REGISTRY"),
			"tag":              os.Getenv("TAG"),
			"romeo-claim-name": os.Getenv("ROMEO_CLAIM_NAME"),
			"oci-insecure":     "true",          // don't mind HTTPS on the CI registry
			"pvc-access-mode":  "ReadWriteOnce", // don't need to scale (+ not possible with kind in CI)
			"expose":           "true",          // make API externally reachable
		},
		ExtraRuntimeValidation: func(t *testing.T, stack integration.RuntimeValidationStackInfo) {
			cli := grpcClient(t, stack.Outputs)
			chlCli := challenge.NewChallengeStoreClient(cli)

			challengeid := randomId()
			ctx := t.Context()

			for _, ex := range examples {
				err := scenario.EncodeOCI(ctx,
					fmt.Sprintf("localhost:5000/example/%s:test", ex), filepath.Join(exDir, ex),
					true, "", "",
				)
				require.NoError(t, err)

				// Create the challenge
				ch, err := chlCli.CreateChallenge(ctx, &challenge.CreateChallengeRequest{
					Id:       challengeid,
					Scenario: fmt.Sprintf("registry:5000/example/%s:test", ex),
				})
				require.NoError(t, err, "during test of example %s, creating challenge", ex)

				// Cannot create an instance under all circumpstances.
				// The genericity layer could not be tested under GitHub Actions
				// without the setup of all existing and future hosting systems,
				// thus has no meaning.

				// Destroy it
				_, err = chlCli.DeleteChallenge(ctx, &challenge.DeleteChallengeRequest{
					Id: ch.Id,
				})
				require.NoError(t, err, "during test of example %s, deleting challenge", ex)
			}
		},
	})
}

func compile(from, to string) error {
	cmd := exec.Command("go", "build", "-o", filepath.Join(to, "main"), "main.go")
	cmd.Dir = from
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "%s", out)
	}
	return nil
}
