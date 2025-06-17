package integration_test

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/base64"
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

//go:embed scn2024.zip
var scn2024 []byte

//go:embed scn2025.zip
var scn2025 []byte

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
			istCli := instance.NewInstanceManagerClient(cli)
			ctx := context.Background()

			challenge_id := randomId()
			source_id := randomId()
			scn1 := base64.StdEncoding.EncodeToString(scn2024)
			scn2 := base64.StdEncoding.EncodeToString(scn2025)

			// Create a challenge
			_, err := chlCli.CreateChallenge(ctx, &challenge.CreateChallengeRequest{
				Id:         challenge_id,
				Scenario:   scn1,
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
				Scenario:       &scn2,
				UpdateStrategy: challenge.UpdateStrategy_blue_green.Enum(),
				Additional: map[string]string{ // some random configuration
					"toto": "toto",
					"tata": "tata",
				},
			}
			req.UpdateMask, err = fieldmaskpb.New(req)
			require.NoError(t, err)
			require.NoError(t, req.UpdateMask.Append(req, "additional"))

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
