package integration_test

import (
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"testing"

	"github.com/pulumi/pulumi/pkg/v3/testing/integration"
	"github.com/stretchr/testify/assert"
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
		UpdateStrategy string
	}{
		"unchanged-scenario": {
			Scenario1:      scn2024,
			Scenario2:      scn2024,
			UpdateStrategy: "update_in_place",
		},
		"update-in-place": {
			Scenario1:      scn2024,
			Scenario2:      scn2025,
			UpdateStrategy: "update_in_place",
		},
		"blue-green": {
			Scenario1:      scn2024,
			Scenario2:      scn2025,
			UpdateStrategy: "blue_green",
		},
		"recreate": {
			Scenario1:      scn2024,
			Scenario2:      scn2025,
			UpdateStrategy: "recreate",
		},
	}

	cwd, _ := os.Getwd()
	integration.ProgramTest(t, &integration.ProgramTestOptions{
		Quick:       true,
		SkipRefresh: true,
		Dir:         path.Join(cwd, ".."),
		Config: map[string]string{
			"service-type": "NodePort",
			"gateway":      "true",
		},
		ExtraRuntimeValidation: func(t *testing.T, stack integration.RuntimeValidationStackInfo) {
			port := stack.Outputs["gw-port"].(float64)
			base := fmt.Sprintf("http://%s:%.0f/api/v1", Base, port)
			client := http.Client{}

			for testname, tt := range tests {
				t.Run(testname, func(t *testing.T) {
					assert := assert.New(t)

					challenge_id := randomId()
					source_id := randomId()
					scn1 := base64.StdEncoding.EncodeToString(tt.Scenario1)
					scn2 := base64.StdEncoding.EncodeToString(tt.Scenario2)

					// Create a challenge
					{
						req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/challenge", base), jsonify(map[string]any{
							"id":              challenge_id,
							"scenario":        scn1,
							"update_strategy": tt.UpdateStrategy,
							"timeout":         "600s",
						}))
						if !assert.Nil(err) {
							t.Fatal("got unexpected error")
						}
						req.Header.Set("Content-Type", "application/json")
						res, err := client.Do(req)
						if !assert.Nil(err) {
							t.Fatal("got unexpected error")
						}
						defer res.Body.Close()

						var resp any
						err = json.NewDecoder(res.Body).Decode(&resp)
						if !assert.Nil(err) {
							t.Fatalf("got unexpected error with value %v", resp)
						}
						if !assert.Equal(http.StatusOK, res.StatusCode) {
							t.Fatalf("got REST JSON API error: %v", resp)
						}
					}

					// Create an instance
					{
						req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/instance", base), jsonify(map[string]any{
							"challenge_id": challenge_id,
							"source_id":    source_id,
						}))
						if !assert.Nil(err) {
							t.Fatal("got unexpected error")
						}
						req.Header.Set("Content-Type", "application/json")
						res, err := client.Do(req)
						if !assert.Nil(err) {
							t.Fatal("got unexpected error")
						}
						defer res.Body.Close()

						var resp any
						err = json.NewDecoder(res.Body).Decode(&resp)
						if !assert.Nil(err) {
							t.Fatalf("got unexpected error with value %v", resp)
						}
						if !assert.Equal(http.StatusOK, res.StatusCode) {
							t.Fatalf("got REST JSON API error: %v", resp)
						}
					}

					// Update the challenge scenario
					{
						req, err := http.NewRequest(http.MethodPatch, fmt.Sprintf("%s/challenge/%s", base, challenge_id), jsonify(map[string]any{
							"scenario": scn2,
						}))
						if !assert.Nil(err) {
							t.Fatal("got unexpected error")
						}
						req.Header.Set("Content-Type", "application/json")
						res, err := client.Do(req)
						if !assert.Nil(err) {
							t.Fatal("got unexpected error")
						}
						defer res.Body.Close()

						var resp any
						err = json.NewDecoder(res.Body).Decode(&resp)
						if !assert.Nil(err) {
							t.Fatalf("got unexpected error with value %v", resp)
						}
						if !assert.Equal(http.StatusOK, res.StatusCode) {
							t.Fatalf("got REST JSON API error: %v", resp)
						}
					}

					// Test the instance is still running
					{
						req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/instance/%s/%s", base, challenge_id, source_id), nil)
						if !assert.Nil(err) {
							t.Fatal("got unexpected error")
						}
						res, err := client.Do(req)
						if !assert.Nil(err) {
							t.Fatal("got unexpected error")
						}
						defer res.Body.Close()

						var resp any
						err = json.NewDecoder(res.Body).Decode(&resp)
						if !assert.Nil(err) {
							t.Fatalf("got unexpected error with value %v", resp)
						}
						if !assert.Equal(http.StatusOK, res.StatusCode) {
							t.Fatalf("got REST JSON API error: %v", resp)
						}
					}

					// Delete instance
					{
						req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/instance/%s/%s", base, challenge_id, source_id), nil)
						if !assert.Nil(err) {
							t.Fatal("got unexpected error")
						}
						res, err := client.Do(req)
						if !assert.Nil(err) {
							t.Fatal("got unexpected error")
						}
						defer res.Body.Close()

						var resp any
						err = json.NewDecoder(res.Body).Decode(&resp)
						if !assert.Nil(err) {
							t.Fatalf("got unexpected error with value %v", resp)
						}
						if !assert.Equal(http.StatusOK, res.StatusCode) {
							t.Fatalf("got REST JSON API error: %v", resp)
						}
					}

					// Delete challenge
					{
						req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/challenge/%s", base, challenge_id), nil)
						if !assert.Nil(err) {
							t.Fatal("got unexpected error")
						}
						req.Header.Set("Content-Type", "application/json")
						res, err := client.Do(req)
						if !assert.Nil(err) {
							t.Fatal("got unexpected error")
						}
						defer res.Body.Close()

						var resp any
						err = json.NewDecoder(res.Body).Decode(&resp)
						if !assert.Nil(err) {
							t.Fatalf("got unexpected error with value %v", resp)
						}
						if !assert.Equal(http.StatusOK, res.StatusCode) {
							t.Fatalf("got REST JSON API error: %v", resp)
						}
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
