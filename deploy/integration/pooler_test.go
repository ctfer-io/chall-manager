package integration_test

import (
	"os"
	"path"
	"testing"
	"time"

	"github.com/ctfer-io/chall-manager/api/v1/challenge"
	"github.com/ctfer-io/chall-manager/api/v1/instance"
	"github.com/pulumi/pulumi/pkg/v3/testing/integration"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func Test_I_UpdatePooler(t *testing.T) {
	// This use case represent a usage of the Pooler which should deploy
	// instances of challenges ahead of sources requesting them, improving
	// performances.
	// At first it registers a challenge with a non-empty pool and a maximum
	// number of instances until the pooler is disabled. Then instances are
	// claimed. The pooler maximum number is then decreased (e.g. statistics
	// showed there is no need for so large pool).
	//
	// We especially check the update is possible, and claim is extremely
	// fast when the pooler is used.

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
			ctx := t.Context()

			challenge_id := randomId()
			source_id1 := randomId()
			source_id2 := randomId()

			// Create a challenge
			_, err := chlCli.CreateChallenge(ctx, &challenge.CreateChallengeRequest{
				Id:       challenge_id,
				Scenario: Scn23Ref,
				Min:      2,
				Max:      4,
			})
			require.NoError(t, err)

			// Sleep enough just to make sure the pool has time to fill
			time.Sleep(20 * time.Second)

			// Create an instance of the challenge (should be fast i.e. <1s)
			before := time.Now()
			_, err = istCli.CreateInstance(ctx, &instance.CreateInstanceRequest{
				ChallengeId: challenge_id,
				SourceId:    source_id1,
			})
			dur := time.Since(before)

			require.NoError(t, err)
			assert.Condition(t, func() (success bool) {
				// We expect the pooler to make available instances claimed in under
				// a second, preferably under half a second.
				return dur < 500*time.Millisecond
			})

			// Update the challenge pooler
			req := &challenge.UpdateChallengeRequest{
				Id:  challenge_id,
				Max: 1,
			}
			req.UpdateMask, err = fieldmaskpb.New(req)
			require.NoError(t, err)
			require.NoError(t, req.UpdateMask.Append(req, "max"))

			_, err = chlCli.UpdateChallenge(ctx, req)
			require.NoError(t, err)

			// Create another instance (pool has been exhausted)
			before = time.Now()
			_, err = istCli.CreateInstance(ctx, &instance.CreateInstanceRequest{
				ChallengeId: challenge_id,
				SourceId:    source_id2,
			})
			dur = time.Since(before)

			require.NoError(t, err)
			assert.Condition(t, func() (success bool) {
				// Should be longer when none in pool, here the scenario is so small
				// that a "long time" is around 2 seconds.
				return dur > 2*time.Second
			})

			// Delete instances
			_, err = istCli.DeleteInstance(ctx, &instance.DeleteInstanceRequest{
				ChallengeId: challenge_id,
				SourceId:    source_id1,
			})
			require.NoError(t, err)
			_, err = istCli.DeleteInstance(ctx, &instance.DeleteInstanceRequest{
				ChallengeId: challenge_id,
				SourceId:    source_id2,
			})
			require.NoError(t, err)

			// Update the challenge for it to have an until -> no instances will be pooled
			req = &challenge.UpdateChallengeRequest{
				Id:    challenge_id,
				Until: timestamppb.Now(),
			}
			req.UpdateMask, err = fieldmaskpb.New(req)
			require.NoError(t, err)
			require.NoError(t, req.UpdateMask.Append(req, "until"))

			_, err = chlCli.UpdateChallenge(ctx, req)
			require.NoError(t, err)

			// Then turn it back on
			req = &challenge.UpdateChallengeRequest{
				Id: challenge_id,
			}
			req.UpdateMask, err = fieldmaskpb.New(req)
			require.NoError(t, err)
			require.NoError(t, req.UpdateMask.Append(req, "until"))

			_, err = chlCli.UpdateChallenge(ctx, req)
			require.NoError(t, err)

			// Delete challenge (should still exist thus no error)
			_, err = chlCli.DeleteChallenge(ctx, &challenge.DeleteChallengeRequest{
				Id: challenge_id,
			})
			require.NoError(t, err)
		},
	})
}
