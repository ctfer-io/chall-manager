package integration_test

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var Server string = ""

func TestMain(m *testing.M) {
	server, ok := os.LookupEnv("SERVER")
	if !ok {
		fmt.Println("Environment variable SERVER is not set, please indicate the domain name/IP address to reach out the cluster.")
	}
	Server = server

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
