package integration_test

import (
	"crypto/rand"
	_ "embed"
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
			"oci-insecure":     "true",          // don't mind HTTPS on the CI registry
			"pvc-access-mode":  "ReadWriteOnce", // don't need to scale (+ not possible with kind in CI)
			"expose":           "true",          // make API externally reachable
			// Following config values are defined, seems like due to a bug in Pulumi loading config
			"etcd.replicas": "1",
			"oci.insecure":  "true",
			"otel.insecure": "true",
		},
		Secrets: map[string]string{
			"kubeconfig": "",
		},
		ExtraRuntimeValidation: func(t *testing.T, stack integration.RuntimeValidationStackInfo) {
			cli := grpcClient(t, stack.Outputs)
			chlCli := challenge.NewChallengeStoreClient(cli)
			istCli := instance.NewInstanceManagerClient(cli)
			ctx := t.Context()

			// Launch all 3 in parallel -> closer to reality (load) + reduce time
			wg := &sync.WaitGroup{}
			wg.Add(3)
			test := func(strat *challenge.UpdateStrategy) {
				defer wg.Done()

				challengeID := randomId()
				sourceID := randomId()

				// Create a challenge
				_, err := chlCli.CreateChallenge(ctx, &challenge.CreateChallengeRequest{
					Id:         challengeID,
					Scenario:   Scn23Ref,
					Timeout:    durationpb.New(10 * time.Minute),                  // timeout should be large enough
					Until:      timestamppb.New(time.Now().Add(10 * time.Minute)), // no date limit ; condition for #509
					Additional: map[string]string{},                               // No config first
				})
				require.NoError(t, err, "challenge %s, strategy: %s", challengeID, strat.String())

				// Create an instance of the challenge
				beforeIst, err := istCli.CreateInstance(ctx, &instance.CreateInstanceRequest{
					ChallengeId: challengeID,
					SourceId:    sourceID,
				})
				require.NoError(t, err, "challenge %s, source %s, strategy: %s", challengeID, sourceID, strat.String())

				// Update the challenge scenario
				req := &challenge.UpdateChallengeRequest{
					Id:             challengeID,
					Scenario:       &Scn25Ref,
					UpdateStrategy: strat,
					Additional: map[string]string{ // some random configuration
						"toto": "toto",
						"tata": "tata",
					},
				}
				req.UpdateMask, err = fieldmaskpb.New(req)
				require.NoError(t, err, "challenge %s, strategy: %s", challengeID, strat.String())
				require.NoError(t, req.UpdateMask.Append(req, "additional"), "challenge %s, strategy: %s", challengeID, strat.String())
				require.NoError(t, req.UpdateMask.Append(req, "scenario"), "challenge %s, strategy: %s", challengeID, strat.String())

				_, err = chlCli.UpdateChallenge(ctx, req)
				require.NoError(t, err, "challenge %s, strategy: %s", challengeID, strat.String())

				// Test the instance is still running
				afterIst, err := istCli.RetrieveInstance(ctx, &instance.RetrieveInstanceRequest{
					ChallengeId: challengeID,
					SourceId:    sourceID,
				})
				require.NoError(t, err, "challenge %s, source %s, strategy: %s", challengeID, sourceID, strat.String())

				// Check it has changed (test for #621 regression)
				assert.NotEqual(t, beforeIst.ConnectionInfo, afterIst.ConnectionInfo, "challenge %s, strategy: %s", challengeID, strat.String())

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
					ChallengeId: challengeID,
					SourceId:    sourceID,
				})
				require.NoError(t, err, "challenge %s, source %s, strategy: %s", challengeID, sourceID, strat.String())

				// Delete instance
				_, err = istCli.DeleteInstance(ctx, &instance.DeleteInstanceRequest{
					ChallengeId: challengeID,
					SourceId:    sourceID,
				})
				require.NoError(t, err, "challenge %s, source %s, strategy: %s", challengeID, sourceID, strat.String())

				// Delete challenge (should still exist thus no error)
				_, err = chlCli.DeleteChallenge(ctx, &challenge.DeleteChallengeRequest{
					Id: challengeID,
				})
				require.NoError(t, err, "challenge %s, strategy: %s", challengeID, strat.String())
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
