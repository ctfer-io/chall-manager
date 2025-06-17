package integration_test

import (
	"context"
	"encoding/base64"
	"os"
	"path"
	"testing"
	"time"

	"github.com/pulumi/pulumi/pkg/v3/testing/integration"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"github.com/ctfer-io/chall-manager/api/v1/challenge"
	"github.com/ctfer-io/chall-manager/api/v1/instance"
)

func Test_I_Standard(t *testing.T) {
	// This use case represent a normal use of the chall-manager yet reduced
	// to a single challenge and instance.
	// At first, it register a challenge in the store, spins up an instance,
	// update the challenge info and delete it
	//
	// We especially check the composition link between challenge and instance
	// objects i.e. a challenge update affects the instances ; a challenge delete
	// drops in cascade the instances.

	require.NotEmpty(t, Server)

	cwd, _ := os.Getwd()
	integration.ProgramTest(t, &integration.ProgramTestOptions{
		Quick:       true,
		SkipRefresh: true,
		Dir:         path.Join(cwd, ".."),
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

			challengeID := randomId()
			sourceID := randomId()

			// Create a challenge
			_, err := chlCli.CreateChallenge(ctx, &challenge.CreateChallengeRequest{
				Id:       challengeID,
				Scenario: base64.StdEncoding.EncodeToString(scn2024),
				Timeout:  durationpb.New(10 * time.Minute),
				Until:    nil, // no date limit
			})
			require.NoError(t, err)

			// Create an instance
			_, err = istCli.CreateInstance(ctx, &instance.CreateInstanceRequest{
				ChallengeId: challengeID,
				SourceId:    sourceID,
			})
			require.NoError(t, err)

			// Get another one, just to see if it is consistent (e.g. don't return
			// this instance infos, and when no known, return the nearly-nil value
			// instead of an error: nothing went wrong, simply this call has no valid
			// value to return).
			_, err = istCli.RetrieveInstance(ctx, &instance.RetrieveInstanceRequest{
				ChallengeId: challengeID,
				SourceId:    sourceID + sourceID, // won't exist as sourceID is non-empty
			})
			require.NoError(t, err)

			// Update challenge (reduce timeout to a ridiculously low one)
			req := &challenge.UpdateChallengeRequest{
				Id:      challengeID,
				Timeout: durationpb.New(time.Second),
			}
			req.UpdateMask, err = fieldmaskpb.New(req)
			require.NoError(t, err)
			require.NoError(t, req.UpdateMask.Append(req, "timeout"))

			_, err = chlCli.UpdateChallenge(ctx, req)
			require.NoError(t, err)
			// TODO check the instance has a new timeout

			// Delete challenge
			_, err = chlCli.DeleteChallenge(ctx, &challenge.DeleteChallengeRequest{
				Id: challengeID,
			})
			require.NoError(t, err)

			// Check instance call is not valid as challenge does not exist
			_, err = istCli.RetrieveInstance(ctx, &instance.RetrieveInstanceRequest{
				ChallengeId: challengeID,
				SourceId:    sourceID,
			})
			require.Error(t, err)
		},
	})
}
