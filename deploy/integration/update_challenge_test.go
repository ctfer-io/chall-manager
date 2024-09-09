package integration_test

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"testing"
	"time"

	"github.com/ctfer-io/chall-manager/api/v1/challenge"
	"github.com/ctfer-io/chall-manager/api/v1/instance"
	"github.com/pulumi/pulumi/pkg/v3/testing/integration"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/durationpb"
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
	// Finally, it deletes the instance and after that the challenge.
	//
	// We especially check the composition link between challenge and instance
	// objects i.e. a challenge update affects the instances ; a instance does
	// not delete its challenge.
	// It does not check precisely the respect of the update strategy and how
	// the instance(s) behave through time. It is voluntarly a high level
	// check to serve as a smoke test to ensure all update strategies works.

	var tests = map[string]struct {
		Scenario1      []byte
		Scenario2      []byte
		UpdateStrategy challenge.UpdateStrategy
	}{
		"unchanged-scenario": {
			Scenario1:      scn2024,
			Scenario2:      scn2024,
			UpdateStrategy: challenge.UpdateStrategy_update_in_place,
		},
		"update-in-place": {
			Scenario1:      scn2024,
			Scenario2:      scn2025,
			UpdateStrategy: challenge.UpdateStrategy_update_in_place,
		},
		"blue-green": {
			Scenario1:      scn2024,
			Scenario2:      scn2025,
			UpdateStrategy: challenge.UpdateStrategy_blue_green,
		},
		"recreate": {
			Scenario1:      scn2024,
			Scenario2:      scn2025,
			UpdateStrategy: challenge.UpdateStrategy_recreate,
		},
	}

	cwd, _ := os.Getwd()
	integration.ProgramTest(t, &integration.ProgramTestOptions{
		Quick:       true,
		SkipRefresh: true,
		Dir:         path.Join(cwd, ".."),
		Config: map[string]string{
			"service-type": "NodePort",
		},
		ExtraRuntimeValidation: func(t *testing.T, stack integration.RuntimeValidationStackInfo) {
			port := stack.Outputs["port"].(float64)
			cli, err := grpc.NewClient(fmt.Sprintf("%s:%0.f", Base, port), grpc.WithTransportCredentials(insecure.NewCredentials()))
			if err != nil {
				t.Fatalf("can't reach out the deployment, got: %s", err)
			}
			chlCli := challenge.NewChallengeStoreClient(cli)
			istCli := instance.NewInstanceManagerClient(cli)
			ctx := context.Background()

			for testname, tt := range tests {
				t.Run(testname, func(t *testing.T) {
					assert := assert.New(t)

					challenge_id := randomId()
					source_id := randomId()
					scn1 := base64.StdEncoding.EncodeToString(tt.Scenario1)
					scn2 := base64.StdEncoding.EncodeToString(tt.Scenario2)

					// Create a challenge
					if _, err := chlCli.CreateChallenge(ctx, &challenge.CreateChallengeRequest{
						Id:       challenge_id,
						Scenario: scn1,
						Timeout:  durationpb.New(10 * time.Minute),
						Until:    nil, // no date limit
					}); !assert.Nil(err) {
						t.Fatal("got unexpected error")
					}

					// Create an instance of the challenge
					if _, err := istCli.CreateInstance(ctx, &instance.CreateInstanceRequest{
						ChallengeId: challenge_id,
						SourceId:    source_id,
					}); !assert.Nil(err) {
						t.Fatal("got unexpected error")
					}

					// Update the challenge scenario
					if _, err := chlCli.UpdateChallenge(ctx, &challenge.UpdateChallengeRequest{
						Id:             challenge_id,
						Scenario:       &scn2,
						UpdateStrategy: &tt.UpdateStrategy,
					}); !assert.Nil(err) {
						t.Fatal("got unexpected error")
					}

					// Test the instance is still running
					if _, err := istCli.RetrieveInstance(ctx, &instance.RetrieveInstanceRequest{
						ChallengeId: challenge_id,
						SourceId:    source_id,
					}); !assert.Nil(err) {
						t.Fatal("got unexpected error")
					}

					// Delete instance
					if _, err := istCli.DeleteInstance(ctx, &instance.DeleteInstanceRequest{
						ChallengeId: challenge_id,
						SourceId:    source_id,
					}); !assert.Nil(err) {
						t.Fatal("got unexpected error")
					}

					// Delete challenge
					if _, err := chlCli.DeleteChallenge(ctx, &challenge.DeleteChallengeRequest{
						Id: challenge_id,
					}); !assert.Nil(err) {
						t.Fatal("got unexpected error")
					}
				})
			}
		},
	})
}

func randomId() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
