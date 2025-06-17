package integration_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/pulumi/pulumi/pkg/v3/testing/integration"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ctfer-io/chall-manager/api/v1/challenge"
)

var examples = []string{
	"additional",
	"exposed-monopod",
	"kompose",
	"kubernetes",
	"no-sdk",
	// prebuilt is not tested as require pre-conditions
	"teeworlds",
}

func Test_I_Examples(t *testing.T) {
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

			challenge_id := randomId()
			ctx := context.Background()

			exDir := filepath.Join(pwd, "..", "..", "examples")
			for _, ex := range examples {
				scn, err := scenario(filepath.Join(exDir, ex))
				if !assert.NoError(t, err, "during test of example %s, building scenario", ex) {
					return
				}

				// Create the challenge
				ch, err := chlCli.CreateChallenge(ctx, &challenge.CreateChallengeRequest{
					Id:       challenge_id,
					Scenario: scn,
				})
				if !assert.NoError(t, err, "during test of example %s, creating challenge", ex) {
					return
				}

				// Cannot create an instance under all circumpstances.
				// The genericity layer could not be tested under GitHub Actions
				// without the setup of all existing and future hosting systems,
				// thus has no meaning.

				// Destroy it
				_, err = chlCli.DeleteChallenge(ctx, &challenge.DeleteChallengeRequest{
					Id: ch.Id,
				})
				if !assert.NoError(t, err, "during test of example %s, deleting challenge", ex) {
					return
				}
			}
		},
	})
}

func scenario(dir string) (string, error) {
	buf := bytes.NewBuffer([]byte{})

	archive := zip.NewWriter(buf)

	// Walk through the source directory.
	if err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		// Ensure the header reflects the file's path within the zip archive.
		fs, err := filepath.Rel(filepath.Dir(dir), path)
		if err != nil {
			return err
		}
		f, err := archive.Create(fs)
		if err != nil {
			return err
		}

		// Open the file.
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		// Copy the file's contents into the archive.
		_, err = io.Copy(f, file)
		if err != nil {
			return err
		}

		return nil
	}); err != nil {
		return "", err
	}

	if err := archive.Close(); err != nil {
		return "", err
	}

	enc := base64.StdEncoding.EncodeToString(buf.Bytes())
	return enc, nil
}
