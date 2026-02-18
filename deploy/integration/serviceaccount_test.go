package integration_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ctfer-io/chall-manager/api/v1/challenge"
	"github.com/ctfer-io/chall-manager/api/v1/instance"
	"github.com/ctfer-io/chall-manager/pkg/scenario"
	"github.com/pulumi/pulumi/pkg/v3/testing/integration"
	"github.com/stretchr/testify/require"
)

func Test_I_ServiceAccount(t *testing.T) {
	// This integration test focuses on using an external ServiceAccount in a Namespace.
	// Doing so, a downstream consumer can provide different capabilities to fit its own
	// scenarios, but also use a Cloud Provider integration (e.g., an AWS IRSA).

	cwd, _ := os.Getwd()

	// Add the scenario into registry
	if err := scenario.EncodeOCI(t.Context(),
		fmt.Sprintf("%s/scenario:sa", registry), filepath.Join(cwd, "serviceaccount", "scenario"),
		true, "", "",
	); err != nil {
		t.Fatal(err)
	}
	ScnRef := "registry:5000/scenario:sa"

	sn := stackName(t.Name())
	integration.ProgramTest(t, &integration.ProgramTestOptions{
		Quick:       true,
		SkipRefresh: true,
		Dir:         filepath.Join(cwd, "serviceaccount"),
		StackName:   sn,
		Config: map[string]string{
			// Don't run it in a given namespace, we don't measure functionalities but infra
			"registry": os.Getenv("REGISTRY"),
			"tag":      os.Getenv("TAG"),
			// Don't plug Romeo to the app, we don't measure functionalities but infra
		},
		ExtraRuntimeValidation: func(t *testing.T, stack integration.RuntimeValidationStackInfo) {
			cli := grpcClient(t, stack.Outputs)
			chlCli := challenge.NewChallengeStoreClient(cli)
			istCli := instance.NewInstanceManagerClient(cli)
			ctx := t.Context()

			challengeID := randomId()
			sourceID := randomId()

			// First create a challenge with a scenario requiring cluster-wide capabilities
			// which are out of the default ServiceAccount scope.
			_, err := chlCli.CreateChallenge(ctx, &challenge.CreateChallengeRequest{
				Id:       challengeID,
				Scenario: ScnRef,
			})
			require.NoError(t, err)

			// Create an instance
			_, err = istCli.CreateInstance(ctx, &instance.CreateInstanceRequest{
				ChallengeId: challengeID,
				SourceId:    sourceID,
			})
			require.NoError(t, err)

			// If no error, the ServiceAccount has been properly used

			// Delete challenge
			_, err = chlCli.DeleteChallenge(ctx, &challenge.DeleteChallengeRequest{
				Id: challengeID,
			})
			require.NoError(t, err)
		},
	})
}
