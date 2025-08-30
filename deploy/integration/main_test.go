package integration_test

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ctfer-io/chall-manager/pkg/scenario"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	registry = "localhost:5000" // as defined per the kind-config.yaml file
)

var (
	Server   = ""
	Scn23Ref = "registry:5000/scenario:23"
	Scn25Ref = "registry:5000/scenario:25"
)

func TestMain(m *testing.M) {
	// Get the server address to connect to Chall-Manager instances
	server, ok := os.LookupEnv("SERVER")
	if !ok {
		fmt.Println("Environment variable SERVER is not set, please indicate the domain name/IP address to reach out the cluster.")
	}
	Server = server

	// Push the scenarios used during tests
	if _, ok := os.LookupEnv("REGISTRY"); ok {
		ctx := context.Background()
		cwd, _ := os.Getwd()
		if err := scenario.EncodeOCI(ctx,
			fmt.Sprintf("%s/scenario:23", registry), filepath.Join(cwd, "scn23"),
			true, "", "",
		); err != nil {
			log.Fatalf("Failed to push scn23: %s", err)
		}
		if err := scenario.EncodeOCI(ctx,
			fmt.Sprintf("%s/scenario:25", registry), filepath.Join(cwd, "scn25"),
			true, "", "",
		); err != nil {
			log.Fatalf("Failed to push scn25: %s", err)
		}
	}

	os.Exit(m.Run())
}

func grpcClient(t *testing.T, outputs map[string]any) *grpc.ClientConn {
	port := fmt.Sprintf("%0.f", outputs["exposed_port"].(float64))
	cli, err := grpc.NewClient(
		fmt.Sprintf("%s:%s", Server, port),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("during gRPC client generation: %s", err)
	}
	return cli
}

func stackName(tname string) (out string) {
	out = tname
	out = strings.TrimPrefix(out, "Test_I_")
	out = strings.ToLower(out)
	return out
}
