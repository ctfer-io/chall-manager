package main

import (
	"io"
	"os"
	"time"

	"github.com/ctfer-io/chall-manager/api/v1/challenge"
	"github.com/ctfer-io/chall-manager/api/v1/instance"
	"github.com/ctfer-io/chall-manager/global"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
	"google.golang.org/grpc"
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
		global.Log().Error("fatal error",
			zap.Error(err),
		)
		os.Exit(1)
	}
}

func run(ctx *cli.Context) error {
	logger := global.Log()

	cli, _ := grpc.NewClient(ctx.String("url"))
	store := challenge.NewChallengeStoreClient(cli)
	manager := instance.NewInstanceManagerClient(cli)
	defer func(cli *grpc.ClientConn) {
		if err := cli.Close(); err != nil {
			logger.Error("closing gRPC connection", zap.Error(err))
		}
	}(cli)

	now := time.Now()

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
		for _, ist := range chall.Instances {
			if ist.Until.AsTime().After(now) {
				if _, err := manager.DeleteInstance(ctx.Context, &instance.DeleteInstanceRequest{
					ChallengeId: ist.ChallengeId,
					SourceId:    ist.SourceId,
				}); err != nil {
					logger.Error("deleting challenge instance",
						zap.String("challenge_id", ist.ChallengeId),
						zap.String("source_id", ist.SourceId),
						zap.Error(err),
					)
				}
			}
		}
	}
	return nil
}
