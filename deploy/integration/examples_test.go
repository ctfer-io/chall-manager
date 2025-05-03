package integration_test

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"testing"

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
	// prebuilt is not tested as require pre-conditions
	"teeworlds",
}

func Test_I_Examples(t *testing.T) {
	require.NotEmpty(t, Server)

	pwd, _ := os.Getwd()
	integration.ProgramTest(t, &integration.ProgramTestOptions{
		Quick:       true,
		SkipRefresh: true,
		Dir:         path.Join(pwd, ".."),
		StackName:   stackName(t.Name()),
		Config: map[string]string{
			"namespace":        os.Getenv("NAMESPACE"),
			"registry":         os.Getenv("REGISTRY"),
			"tag":              os.Getenv("TAG"),
			"romeo-claim-name": os.Getenv("ROMEO_CLAIM_NAME"),
			"pvc-access-mode":  "ReadWriteOnce", // don't need to scale (+ not possible with kind in CI)
			"expose":           "true",          // make API externally reachable
		},
		ExtraRuntimeValidation: func(t *testing.T, stack integration.RuntimeValidationStackInfo) {
			cli := grpcClient(t, stack.Outputs)
			chlCli := challenge.NewChallengeStoreClient(cli)

			challenge_id := randomId()
			ctx := t.Context()

			exDir := filepath.Join(pwd, "..", "..", "examples")
			for _, ex := range examples {
				ref := fmt.Sprintf("%s/example/%s:test", os.Getenv("REGISTRY"), ex)
				err := scenario.EncodeOCI(ctx, ref, exDir, nil, nil)
				require.NoError(t, err)

				res, err := http.Get(fmt.Sprintf("http://%s/v2/_catalog", os.Getenv("REGISTRY")))
				if err != nil {
					t.Fatal(err)
				}
				defer res.Body.Close()
				b, err := io.ReadAll(res.Body)
				if err != nil {
					t.Fatal(err)
				}
				fmt.Printf("registry catalog: %s\n", b)

				// Create the challenge
				ch, err := chlCli.CreateChallenge(ctx, &challenge.CreateChallengeRequest{
					Id:       challenge_id,
					Scenario: ref,
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
