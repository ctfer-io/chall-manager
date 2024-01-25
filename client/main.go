package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ctfer-io/chall-manager/api/v1/launch"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	var opts []grpc.DialOption
	opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))

	conn, err := grpc.Dial("localhost:8081", opts...)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	client := launch.NewLauncherClient(conn)

	res, err := client.CreateLaunch(context.Background(), &launch.LaunchRequest{
		ChallengeId: "1",
		SourceId:    "1",
		Scenario:    "...",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("res.ConnectionInfo: %v\n", res.ConnectionInfo)
}
