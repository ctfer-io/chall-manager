package integration_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/ctfer-io/chall-manager/api/v1/challenge"
	"github.com/ctfer-io/chall-manager/api/v1/instance"
	"github.com/pulumi/pulumi/pkg/v3/testing/integration"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func Test_I_Examples(t *testing.T) {
	pwd, _ := os.Getwd()

	cwd, _ := os.Getwd()
	integration.ProgramTest(t, &integration.ProgramTestOptions{
		Quick:       true,
		SkipRefresh: true,
		Dir:         path.Join(cwd, ".."),
		Config: map[string]string{
			"service-type": "NodePort", // enable reaching it out from out the cluster
			"lock-kind":    "etcd",     // production deployment for scalability
		},
		ExtraRuntimeValidation: func(t *testing.T, stack integration.RuntimeValidationStackInfo) {
			exDir := filepath.Join(pwd, "..", "..", "examples")
			dir, err := os.ReadDir(exDir)
			if err != nil {
				t.Fatal(err)
			}
			for _, dfs := range dir {
				port := stack.Outputs["port"].(float64)
				cli, err := grpc.NewClient(fmt.Sprintf("%s:%0.f", Base, port), grpc.WithTransportCredentials(insecure.NewCredentials()))
				if err != nil {
					t.Fatalf("can't reach out the deployment, got: %s", err)
				}
				chlCli := challenge.NewChallengeStoreClient(cli)
				istCli := instance.NewInstanceManagerClient(cli)

				t.Run(dfs.Name(), func(t *testing.T) {
					challenge_id := randomId()
					source_id := randomId()
					ctx := context.Background()
					scn, err := scenario(filepath.Join(exDir, dfs.Name()))
					if err != nil {
						t.Fatalf("got unexpected error: %s", err)
					}

					// Create the challenge
					ch, err := chlCli.CreateChallenge(ctx, &challenge.CreateChallengeRequest{
						Id:       challenge_id,
						Scenario: scn,
					})
					if err != nil {
						t.Fatalf("got unexpected error: %s", err)
					}

					// Create an instance
					if _, err = istCli.CreateInstance(ctx, &instance.CreateInstanceRequest{
						ChallengeId: challenge_id,
						SourceId:    source_id,
					}); err != nil {
						t.Fatalf("got unexpected error: %s", err)
					}

					// Detroy it
					_, err = chlCli.DeleteChallenge(ctx, &challenge.DeleteChallengeRequest{
						Id: ch.Id,
					})
					if err != nil {
						t.Fatalf("got unexpected error: %s", err)
					}
				})
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
