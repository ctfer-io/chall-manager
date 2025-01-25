package integration_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path"
	"testing"
	"time"

	"github.com/pulumi/pulumi/pkg/v3/testing/integration"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/durationpb"

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

	cwd, _ := os.Getwd()
	integration.ProgramTest(t, &integration.ProgramTestOptions{
		Quick:       true,
		SkipRefresh: true,
		Dir:         path.Join(cwd, ".."),
		Config: map[string]string{
			"service-type": "NodePort",
		},
		ExtraRuntimeValidation: func(t *testing.T, stack integration.RuntimeValidationStackInfo) {
			require := require.New(t)

			port := stack.Outputs["port"].(float64)
			cli, err := grpc.NewClient(fmt.Sprintf("%s:%0.f", Base, port), grpc.WithTransportCredentials(insecure.NewCredentials()))
			if err != nil {
				t.Fatalf("can't reach out the deployment, got: %s", err)
			}
			chlCli := challenge.NewChallengeStoreClient(cli)
			istCli := instance.NewInstanceManagerClient(cli)
			ctx := context.Background()

			challenge_id := randomId()
			source_id := randomId()

			// Create a challenge
			_, err = chlCli.CreateChallenge(ctx, &challenge.CreateChallengeRequest{
				Id:       challenge_id,
				Scenario: base64.StdEncoding.EncodeToString(scn2024),
				Timeout:  durationpb.New(10 * time.Minute),
				Until:    nil, // no date limit
			})
			require.NoError(err)

			// Create an instance
			_, err = istCli.CreateInstance(ctx, &instance.CreateInstanceRequest{
				ChallengeId: challenge_id,
				SourceId:    source_id,
			})
			require.NoError(err)

			// Update challenge (reduce timeout to a ridiculously low one)
			_, err = chlCli.UpdateChallenge(ctx, &challenge.UpdateChallengeRequest{
				Id:      challenge_id,
				Timeout: durationpb.New(time.Second),
			})
			require.NoError(err)
			// TODO check the instance has a new timeout

			// Delete challenge
			_, err = chlCli.DeleteChallenge(ctx, &challenge.DeleteChallengeRequest{
				Id: challenge_id,
			})
			require.NoError(err)

			// Check instance does not remain
			_, err = istCli.RetrieveInstance(ctx, &instance.RetrieveInstanceRequest{
				ChallengeId: challenge_id,
				SourceId:    source_id,
			})
			require.NoError(err)
		},
	})
}
