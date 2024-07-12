package main

import (
	"context"
	"io"
	"os"
	"sync"
	"time"

	"github.com/ctfer-io/chall-manager/api/v1/challenge"
	"github.com/ctfer-io/chall-manager/api/v1/instance"
	"github.com/ctfer-io/chall-manager/global"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	version = "dev"
	commit  = ""
	date    = ""
	builtBy = ""
)

func main() {
	app := &cli.App{
		Name:  "Chall-Manager-Janitor",
		Usage: "Chall-Manager-Janitor is an utility that handles challenges instances death.",
		Flags: []cli.Flag{
			cli.VersionFlag,
			cli.HelpFlag,
			&cli.StringFlag{
				Name:    "url",
				EnvVars: []string{"URL"},
				Usage:   "The chall-manager URL to reach out.",
			},
		},
		Action: run,
		Authors: []*cli.Author{
			{
				Name:  "Lucas Tesson - PandatiX",
				Email: "lucastesson@protonmail.com",
			},
		},
		Version: version,
		Metadata: map[string]any{
			"version": version,
			"commit":  commit,
			"date":    date,
			"builtBy": builtBy,
		},
	}

	if err := app.Run(os.Args); err != nil {
		global.Log().Error(context.Background(), "fatal error",
			zap.Error(err),
		)
		os.Exit(1)
	}
}

func run(ctx *cli.Context) error {
	logger := global.Log()

	cli, _ := grpc.NewClient(ctx.String("url"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	store := challenge.NewChallengeStoreClient(cli)
	manager := instance.NewInstanceManagerClient(cli)
	defer func(cli *grpc.ClientConn) {
		if err := cli.Close(); err != nil {
			logger.Error(ctx.Context, "closing gRPC connection", zap.Error(err))
		}
	}(cli)

	challs, err := store.QueryChallenge(ctx.Context, nil)
	if err != nil {
		return err
	}
	for {
		chall, err := challs.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		ctx := global.WithChallengeId(ctx.Context, chall.Id)

		// Don't janitor if the challenge has no dates configured
		if chall.Dates == nil {
			logger.Info(ctx, "skipping challenge with no dates configured")
			continue
		}

		// Janitor outdated intances
		wg := &sync.WaitGroup{}
		for _, ist := range chall.Instances {
			ctx := global.WithSourceId(ctx, ist.SourceId)

			if time.Now().After(ist.Until.AsTime()) {
				logger.Info(ctx, "janitoring instance")
				wg.Add(1)

				go func(ist *instance.Instance) {
					defer wg.Done()

					if _, err := manager.DeleteInstance(ctx, &instance.DeleteInstanceRequest{
						ChallengeId: ist.ChallengeId,
						SourceId:    ist.SourceId,
					}); err != nil {
						logger.Error(ctx, "deleting challenge instance",
							zap.Error(err),
						)
					}
				}(ist)
			}
		}
		wg.Wait()
	}
	return nil
}
