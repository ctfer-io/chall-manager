package integration_test

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pulumi/pulumi/pkg/v3/testing/integration"
	"github.com/stretchr/testify/assert"
)

func Test_I_Launch(t *testing.T) {
	cwd, _ := os.Getwd()
	integration.ProgramTest(t, &integration.ProgramTestOptions{
		Quick:       true,
		SkipRefresh: true,
		Dir:         path.Join(cwd, ".."),
		Config: map[string]string{
			"service-type": "NodePort",
		},
		ExtraRuntimeValidation: func(t *testing.T, stack integration.RuntimeValidationStackInfo) {
			assert := assert.New(t)

			base := fmt.Sprintf("http://%s:%.0f/api/v1", Base, stack.Outputs["port"].(float64))
			client := http.Client{}
			scn := scenario("../../examples/no-sdk/deploy")
			challenge_id := "1"
			source_id := "1"

			// Create a scenario instance
			{
				req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/launch", base), jsonify(map[string]any{
					"challenge_id": challenge_id,
					"source_id":    source_id,
					"scenario":     scn,
				}))
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

			// Get back its info
			{
				req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/launch/%s/%s", base, challenge_id, source_id), nil)
				res, err := client.Do(req)
				if !assert.Nil(err) {
					t.Fatal("got unexpected error")
				}
				defer res.Body.Close()

				if !assert.Equal(http.StatusOK, res.StatusCode) {
					t.Fatalf("invalid response status code: %d", res.StatusCode)
				}
			}

			// Delete it
			{
				req, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/launch", base), jsonify(map[string]any{
					"challenge_id": challenge_id,
					"source_id":    source_id,
					"scenario":     scn,
				}))
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

			// Get back its info to make sure it is deleted
			{
				req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/launch/%s/%s", base, challenge_id, source_id), nil)
				res, err := client.Do(req)
				if !assert.Nil(err) {
					t.Fatal("got unexpected error")
				}
				defer res.Body.Close()

				if !assert.NotEqual(http.StatusOK, res.StatusCode) {
					t.Fatalf("invalid response status code: %d", res.StatusCode)
				}
			}
		},
	})
}

func scenario(path string) string {
	pwd, _ := os.Getwd()
	cd := filepath.Join(pwd, path)

	// Zip files
	buf := &bytes.Buffer{}
	w := zip.NewWriter(buf)
	defer w.Close()

	walker := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		f, err := w.Create(strings.TrimPrefix(path, cd))
		if err != nil {
			return err
		}

		_, err = io.Copy(f, file)
		return err
	}
	if err := filepath.Walk(cd, walker); err != nil {
		panic(err)
	}
	w.Close()

	// Encode b64
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func jsonify(req any) io.Reader {
	b, err := json.Marshal(req)
	if err != nil {
		panic(err)
	}
	return bytes.NewBuffer(b)
}
