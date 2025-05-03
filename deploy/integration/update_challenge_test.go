package integration_test

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"os"
	"path"
	"testing"
	"time"

	"github.com/pulumi/pulumi/pkg/v3/testing/integration"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ctfer-io/chall-manager/api/v1/challenge"
	"github.com/ctfer-io/chall-manager/api/v1/instance"
)

func Test_I_Update(t *testing.T) {
	// This use case represent an abnormal situation where the Admin/Ops must
	// patch a challenge with ongoing instances. This may be due to invalid
	// configurations, patching an unexpected solve, a security issue, etc.
	// At first it registers a challenge in the store, spins up an instance,
	// update the challenge scenario and test the instance still exist.
	// Finally, it deletes the instance, and after that the challenge.
	//
	// We especially check the composition link between challenge and instance
	// objects i.e. a challenge update affects the instances ; a instance does
	// not delete its challenge.

	require.NotEmpty(t, Server)

	pwd, _ := os.Getwd()
	integration.ProgramTest(t, &integration.ProgramTestOptions{
		Quick:       true,
		SkipRefresh: true,
		Dir:         path.Join(pwd, ".."),
		Config: map[string]string{
			"registry":         os.Getenv("REGISTRY"),
			"tag":              os.Getenv("TAG"),
			"romeo.claim-name": os.Getenv("ROMEO_CLAIM_NAME"),
			"oci-insecure":     "true",          // don't mind HTTPS on the CI registry
			"pvc-access-mode":  "ReadWriteOnce", // don't need to scale (+ not possible with kind in CI)
			"expose":           "true",          // make API externally reachable
		},
		ExtraRuntimeValidation: func(t *testing.T, stack integration.RuntimeValidationStackInfo) {
			cli := grpcClient(t, stack.Outputs)
			chlCli := challenge.NewChallengeStoreClient(cli)
			istCli := instance.NewInstanceManagerClient(cli)
			ctx := context.Background()

			challenge_id := randomId()
			source_id := randomId()

			// Create a challenge
			_, err := chlCli.CreateChallenge(ctx, &challenge.CreateChallengeRequest{
				Id:         challenge_id,
				Scenario:   Scn23Ref,
				Timeout:    durationpb.New(10 * time.Minute),                  // timeout should be large enough
				Until:      timestamppb.New(time.Now().Add(10 * time.Minute)), // no date limit ; condition for #509
				Additional: map[string]string{},                               // No config first
			})
			require.NoError(t, err)

			// Create an instance of the challenge
			_, err = istCli.CreateInstance(ctx, &instance.CreateInstanceRequest{
				ChallengeId: challenge_id,
				SourceId:    source_id,
			})
			require.NoError(t, err)

			// Update the challenge scenario
			req := &challenge.UpdateChallengeRequest{
				Id:             challenge_id,
				Scenario:       &Scn25Ref,
				UpdateStrategy: challenge.UpdateStrategy_blue_green.Enum(),
				Additional: map[string]string{ // some random configuration
					"toto": "toto",
					"tata": "tata",
				},
			}
			req.UpdateMask, err = fieldmaskpb.New(req)
			require.NoError(t, err)
			require.NoError(t, req.UpdateMask.Append(req, "additional"))
			require.NoError(t, req.UpdateMask.Append(req, "scenario"))

			_, err = chlCli.UpdateChallenge(ctx, req)
			require.NoError(t, err)

			// Test the instance is still running
			_, err = istCli.RetrieveInstance(ctx, &instance.RetrieveInstanceRequest{
				ChallengeId: challenge_id,
				SourceId:    source_id,
			})
			require.NoError(t, err)

			// Renew (test for #509 regression)
			_, err = istCli.RenewInstance(ctx, &instance.RenewInstanceRequest{
				ChallengeId: challenge_id,
				SourceId:    source_id,
			})
			require.NoError(t, err)

			// Delete instance
			_, err = istCli.DeleteInstance(ctx, &instance.DeleteInstanceRequest{
				ChallengeId: challenge_id,
				SourceId:    source_id,
			})
			require.NoError(t, err)

			// Delete challenge (should still exist thus no error)
			_, err = chlCli.DeleteChallenge(ctx, &challenge.DeleteChallengeRequest{
				Id: challenge_id,
			})
			require.NoError(t, err)
		},
	})
}

func randomId() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
