package integration_test

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path"
	"sync"
	"testing"
	"time"

	"github.com/pulumi/pulumi/pkg/v3/testing/integration"
	"github.com/stretchr/testify/assert"
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
			istCli := instance.NewInstanceManagerClient(cli)
			ctx := context.Background()

			scn1 := base64.StdEncoding.EncodeToString(scn2024)
			scn2 := base64.StdEncoding.EncodeToString(scn2025)

			// Launch all 3 in parallel -> closer to reality (load) + reduce time
			wg := &sync.WaitGroup{}
			wg.Add(3)
			test := func(strat *challenge.UpdateStrategy) {
				defer wg.Done()

				challenge_id := randomId()
				source_id := randomId()

				// Create a challenge
				_, err := chlCli.CreateChallenge(ctx, &challenge.CreateChallengeRequest{
					Id:         challenge_id,
					Scenario:   scn1,
					Timeout:    durationpb.New(10 * time.Minute),                  // timeout should be large enough
					Until:      timestamppb.New(time.Now().Add(10 * time.Minute)), // no date limit ; condition for #509
					Additional: map[string]string{},                               // No config first
				})
				require.NoError(t, err, "strategy: %s", strat.String())

				// Create an instance of the challenge
				beforeIst, err := istCli.CreateInstance(ctx, &instance.CreateInstanceRequest{
					ChallengeId: challenge_id,
					SourceId:    source_id,
				})
				require.NoError(t, err, "strategy: %s", strat.String())

				// Update the challenge scenario
				req := &challenge.UpdateChallengeRequest{
					Id:             challenge_id,
					Scenario:       &scn2,
					UpdateStrategy: strat,
					Additional: map[string]string{ // some random configuration
						"toto": "toto",
						"tata": "tata",
					},
				}
				req.UpdateMask, err = fieldmaskpb.New(req)
				require.NoError(t, err, "strategy: %s", strat.String())
				require.NoError(t, req.UpdateMask.Append(req, "additional"), "strategy: %s", strat.String())

				_, err = chlCli.UpdateChallenge(ctx, req)
				require.NoError(t, err, "strategy: %s", strat.String())

				// Test the instance is still running
				afterIst, err := istCli.RetrieveInstance(ctx, &instance.RetrieveInstanceRequest{
					ChallengeId: challenge_id,
					SourceId:    source_id,
				})
				require.NoError(t, err, "strategy: %s", strat.String())

				// Check it has changed (test for #621 regression)
				assert.NotEqual(t, beforeIst.ConnectionInfo, afterIst.ConnectionInfo, "strategy: %s", strat.String())

				// Update nothing, but with additionals (test for #764 regression)
				before := time.Now()
				_, err = chlCli.UpdateChallenge(ctx, req)
				dur := time.Since(before)

				require.NoError(t, err)
				assert.Condition(t, func() (success bool) {
					// We expect a no-update request to do nothing, especially not update
					// running instances.
					return dur < 500*time.Millisecond
				})

				// Renew (test for #509 regression)
				_, err = istCli.RenewInstance(ctx, &instance.RenewInstanceRequest{
					ChallengeId: challenge_id,
					SourceId:    source_id,
				})
				require.NoError(t, err, "strategy: %s", strat.String())

				// Delete instance
				_, err = istCli.DeleteInstance(ctx, &instance.DeleteInstanceRequest{
					ChallengeId: challenge_id,
					SourceId:    source_id,
				})
				require.NoError(t, err, "strategy: %s", strat.String())

				// Delete challenge (should still exist thus no error)
				_, err = chlCli.DeleteChallenge(ctx, &challenge.DeleteChallengeRequest{
					Id: challenge_id,
				})
				require.NoError(t, err, "strategy: %s", strat.String())
			}
			go test(challenge.UpdateStrategy_update_in_place.Enum())
			go test(challenge.UpdateStrategy_blue_green.Enum())
			go test(challenge.UpdateStrategy_recreate.Enum())
			wg.Wait()
		},
	})
}

func randomId() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
