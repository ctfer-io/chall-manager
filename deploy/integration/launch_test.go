package integration_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"testing"

	"github.com/pulumi/pulumi/pkg/v3/testing/integration"
	"github.com/stretchr/testify/assert"
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
			"gateway":      "true",
		},
		ExtraRuntimeValidation: func(t *testing.T, stack integration.RuntimeValidationStackInfo) {
			assert := assert.New(t)

			port := stack.Outputs["gw-port"].(float64)
			base := fmt.Sprintf("http://%s:%.0f/api/v1", Base, port)
			client := http.Client{}
			challenge_id := randomId()
			source_id := randomId()

			// Create a challenge
			{
				req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/challenge", base), jsonify(map[string]any{
					"id":       challenge_id,
					"scenario": scn2024,
					"timeout":  "600s",
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

			// Update challenge (reduce timeout to a ridiculously low one)
			{
				req, err := http.NewRequest(http.MethodPatch, fmt.Sprintf("%s/challenge/%s", base, challenge_id), jsonify(map[string]any{
					"timeout": "3s",
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
				// TODO check the instance has a new timeout
			}

			// Delete challenge
			{
				req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/challenge/%s", base, challenge_id), nil)
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

			// Check instance does not remain
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
				// An API error is not related to HTTP
				if !assert.Nil(err) {
					t.Fatalf("got unexpected error with value %v", resp)
				}
				// An API error returns a non-OK status code
				if !assert.Equal(http.StatusInternalServerError, res.StatusCode) {
					t.Fatalf("got REST JSON API error: %v", resp)
				}
				// An API error returns a response with context about what happened
				if !assert.NotNil(resp) {
					t.Fatalf("got content for deleted instance: %+v", resp)
				}
			}
		},
	})
}

func jsonify(req any) io.Reader {
	b, err := json.Marshal(req)
	if err != nil {
		panic(err)
	}
	return bytes.NewBuffer(b)
}
