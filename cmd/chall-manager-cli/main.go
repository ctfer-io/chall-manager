package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"strings"
	"time"

	"github.com/ctfer-io/chall-manager/api/v1/challenge"
	"github.com/ctfer-io/chall-manager/api/v1/instance"
	"github.com/ctfer-io/chall-manager/pkg/scenario"
	"github.com/urfave/cli/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type cliChallKey struct{}
type cliIstKey struct{}

func main() {
	cmd := cli.Command{
		Name: "chall-manager-cli",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "url",
				Usage:    "The URL to reach out the chall-manager instance/cluster.",
				Required: true,
			},
		},
		Commands: []*cli.Command{
			{
				Name: "challenge",
				Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
					conn, err := grpc.NewClient(cmd.String("url"),
						grpc.WithTransportCredentials(insecure.NewCredentials()),
					)
					if err != nil {
						return ctx, err
					}
					cliChall := challenge.NewChallengeStoreClient(conn)

					ctx = context.WithValue(ctx, cliChallKey{}, cliChall)
					return ctx, nil
				},
				Commands: []*cli.Command{
					{
						Name: "create",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "id",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "scenario",
								Required: true,
							},
							&cli.StringFlag{
								Name:    "directory",
								Aliases: []string{"dir"},
							},
							&cli.StringFlag{
								Name:  "username",
								Usage: "The username to use for pushing the scenario to the OCI registry.",
							},
							&cli.StringFlag{
								Name:  "password",
								Usage: "The password to use for pushing the scenario to the OCI registry.",
							},
							&cli.BoolFlag{
								Name:  "insecure",
								Usage: "If turned on, use insecure push mode for OCI registry.",
							},
							&cli.DurationFlag{
								Name: "timeout",
							},
							&cli.TimestampFlag{
								Name: "until",
								Config: cli.TimestampConfig{
									Layouts: []string{"02-01-2006", time.RFC3339},
								},
							},
							&cli.StringSliceFlag{
								Name: "additional",
							},
							&cli.Int64Flag{
								Name:  "min",
								Value: 0,
							},
							&cli.Int64Flag{
								Name:  "max",
								Value: 0,
							},
						},
						Action: func(ctx context.Context, cmd *cli.Command) error {
							cliChall := ctx.Value(cliChallKey{}).(challenge.ChallengeStoreClient)
							var timeout *durationpb.Duration
							if cmd.IsSet("timeout") {
								timeout = durationpb.New(cmd.Duration("timeout"))
							}
							var until *timestamppb.Timestamp
							if cmd.IsSet("until") {
								until = timestamppb.New(cmd.Timestamp("until"))
							}
							var add map[string]string
							if cmd.IsSet("additional") {
								slc := cmd.StringSlice("additional")
								add = make(map[string]string, len(slc))
								for _, kv := range slc {
									k, v, _ := strings.Cut(kv, "=")
									add[k] = v
								}
							}

							username := cmd.String("username")
							password := cmd.String("password")

							ref := cmd.String("scenario")
							if cmd.IsSet("directory") {
								if err := scenario.EncodeOCI(ctx,
									ref, cmd.String("directory"),
									cmd.Bool("insecure"), username, password,
								); err != nil {
									return err
								}
							}

							now := time.Now()
							chall, err := cliChall.CreateChallenge(ctx, &challenge.CreateChallengeRequest{
								Id:         cmd.String("id"),
								Scenario:   ref,
								Timeout:    timeout,
								Until:      until,
								Additional: add,
								Min:        cmd.Int64("min"),
								Max:        cmd.Int64("max"),
							}, grpc.MaxCallSendMsgSize(math.MaxInt64))
							if err != nil {
								return err
							}
							after := time.Now()
							fmt.Printf("duration: %v\n", after.Sub(now))

							fmt.Printf("[+] Challenge %s created\n", chall.Id)

							return nil
						},
					}, {
						Name: "retrieve",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "id",
								Required: true,
							},
						},
						Action: func(ctx context.Context, cmd *cli.Command) error {
							cliChall := ctx.Value(cliChallKey{}).(challenge.ChallengeStoreClient)

							chall, err := cliChall.RetrieveChallenge(ctx, &challenge.RetrieveChallengeRequest{
								Id: cmd.String("id"),
							})
							if err != nil {
								return err
							}

							fmt.Printf("[+] Challenge %s created using %s, %d instances\n", chall.Id, chall.Scenario, len(chall.Instances))

							return nil
						},
					}, {
						Name: "update",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "id",
								Required: true,
							},
							&cli.StringFlag{
								Name: "scenario",
							},
							&cli.StringFlag{
								Name:    "directory",
								Aliases: []string{"dir"},
							},
							&cli.StringFlag{
								Name:  "username",
								Usage: "The username to use for pushing the scenario to the OCI registry.",
							},
							&cli.StringFlag{
								Name:  "password",
								Usage: "The password to use for pushing the scenario to the OCI registry.",
							},
							&cli.BoolFlag{
								Name:  "insecure",
								Usage: "If turned on, use insecure push mode for OCI registry.",
							},
							&cli.DurationFlag{
								Name: "timeout",
							},
							&cli.BoolFlag{
								Name: "reset-timeout",
							},
							&cli.TimestampFlag{
								Name: "until",
								Config: cli.TimestampConfig{
									Layouts: []string{"02-01-2006", time.RFC3339},
								},
							},
							&cli.BoolFlag{
								Name: "reset-until",
							},
							&cli.StringSliceFlag{
								Name: "additional",
							},
							&cli.BoolFlag{
								Name: "reset-additional",
							},
							&cli.StringFlag{
								Name:  "strategy",
								Value: "in-place",
								Action: func(_ context.Context, _ *cli.Command, strategy string) error {
									switch strategy {
									case "blue-green", "recreate", "in-place":
										// everything is fine
										return nil
									default:
										return fmt.Errorf("unsupported update strategy: %s", strategy)
									}
								},
							},
							&cli.Int64Flag{
								Name:  "min",
								Value: 0,
							},
							&cli.Int64Flag{
								Name:  "max",
								Value: 0,
							},
						},
						Action: func(ctx context.Context, cmd *cli.Command) error {
							cliChall := ctx.Value(cliChallKey{}).(challenge.ChallengeStoreClient)

							ref := cmd.String("scenario")
							if cmd.IsSet("directory") {
								dir := cmd.String("directory")
								username := cmd.String("username")
								password := cmd.String("password")

								if err := scenario.EncodeOCI(ctx,
									ref, dir,
									cmd.Bool("insecure"), username, password,
								); err != nil {
									return err
								}
							}

							// Build request with the FieldMask for fine-grained update
							req := &challenge.UpdateChallengeRequest{
								Id: cmd.String("id"),
							}
							um, err := fieldmaskpb.New(req)
							if err != nil {
								return err
							}
							if cmd.IsSet("scenario") {
								if err := um.Append(req, "scenario"); err != nil {
									return err
								}
								req.Scenario = ptr(cmd.String("scenario"))
							}
							if cmd.IsSet("timeout") {
								if err := um.Append(req, "timeout"); err != nil {
									return err
								}
								req.Timeout = durationpb.New(cmd.Duration("timeout"))
							} else if cmd.Bool("reset-timeout") {
								if err := um.Append(req, "timeout"); err != nil {
									return err
								}
							}
							if cmd.IsSet("until") {
								if err := um.Append(req, "until"); err != nil {
									return err
								}
								req.Until = timestamppb.New(cmd.Timestamp("until"))
							} else if cmd.Bool("reset-until") {
								if err := um.Append(req, "until"); err != nil {
									return err
								}
							}
							if cmd.IsSet("additional") {
								if err := um.Append(req, "additional"); err != nil {
									return err
								}
								slc := cmd.StringSlice("additional")
								req.Additional = make(map[string]string, len(slc))
								for _, kv := range slc {
									k, v, _ := strings.Cut(kv, "=")
									req.Additional[k] = v
								}
							} else if cmd.Bool("reset-additional") {
								if err := um.Append(req, "additional"); err != nil {
									return err
								}
							}
							if cmd.IsSet("min") {
								if err := um.Append(req, "min"); err != nil {
									return err
								}
								req.Min = cmd.Int64("min")
							}
							if cmd.IsSet("max") {
								if err := um.Append(req, "max"); err != nil {
									return err
								}
								req.Max = cmd.Int64("max")
							}
							switch cmd.String("strategy") {
							case "blue-green":
								req.UpdateStrategy = challenge.UpdateStrategy_blue_green.Enum()
							case "recreate":
								req.UpdateStrategy = challenge.UpdateStrategy_recreate.Enum()
							case "in-place":
								req.UpdateStrategy = challenge.UpdateStrategy_update_in_place.Enum()
							}

							req.UpdateMask = um
							chall, err := cliChall.UpdateChallenge(ctx, req)
							if err != nil {
								return err
							}

							fmt.Printf("[~] Challenge %s updated\n", chall.Id)

							return nil
						},
					}, {
						Name: "delete",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "id",
								Required: true,
							},
						},
						Action: func(ctx context.Context, cmd *cli.Command) error {
							cliChall := ctx.Value(cliChallKey{}).(challenge.ChallengeStoreClient)
							id := cmd.String("id")
							if _, err := cliChall.DeleteChallenge(ctx, &challenge.DeleteChallengeRequest{
								Id: id,
							}); err != nil {
								return err
							}

							fmt.Printf("[-] Challenge %s deleted\n", id)

							return nil
						},
					},
				},
			}, {
				Name: "instance",
				Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
					conn, err := grpc.NewClient(cmd.String("url"), grpc.WithTransportCredentials(insecure.NewCredentials()))
					if err != nil {
						return ctx, err
					}
					cliIst := instance.NewInstanceManagerClient(conn)

					ctx = context.WithValue(ctx, cliIstKey{}, cliIst)
					return ctx, nil
				},
				Commands: []*cli.Command{
					{
						Name: "create",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "challenge_id",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "source_id",
								Required: true,
							},
							&cli.StringSliceFlag{
								Name: "additional",
							},
						},
						Action: func(ctx context.Context, cmd *cli.Command) error {
							cliIst := ctx.Value(cliIstKey{}).(instance.InstanceManagerClient)

							var add map[string]string
							if cmd.IsSet("additional") {
								slc := cmd.StringSlice("additional")
								add = make(map[string]string, len(slc))
								for _, kv := range slc {
									k, v, _ := strings.Cut(kv, "=")
									add[k] = v
								}
							}

							before := time.Now()
							ist, err := cliIst.CreateInstance(ctx, &instance.CreateInstanceRequest{
								ChallengeId: cmd.String("challenge_id"),
								SourceId:    cmd.String("source_id"),
								Additional:  add,
							})
							fmt.Printf("duration: %s\n", time.Since(before))
							if err != nil {
								return err
							}

							fmt.Printf(
								"[+] Instance <%s,%s> created, connect with `%s`\n",
								ist.ChallengeId,
								ist.SourceId,
								ist.ConnectionInfo,
							)

							return nil
						},
					}, {
						Name: "delete",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "challenge_id",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "source_id",
								Required: true,
							},
						},
						Action: func(ctx context.Context, cmd *cli.Command) error {
							cliIst := ctx.Value(cliIstKey{}).(instance.InstanceManagerClient)

							if _, err := cliIst.DeleteInstance(ctx, &instance.DeleteInstanceRequest{
								ChallengeId: cmd.String("challenge_id"),
								SourceId:    cmd.String("source_id"),
							}); err != nil {
								return err
							}

							fmt.Printf("[+] Instance <%s,%s> deleted\n", cmd.String("challenge_id"), cmd.String("source_id"))

							return nil
						},
					},
				},
			}, {
				Name: "scenario",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "scenario",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "directory",
						Aliases:  []string{"dir"},
						Required: true,
					},
					&cli.StringFlag{
						Name:  "username",
						Usage: "The username to use for pushing the scenario to the OCI registry.",
					},
					&cli.StringFlag{
						Name:  "password",
						Usage: "The password to use for pushing the scenario to the OCI registry.",
					},
					&cli.BoolFlag{
						Name:  "insecure",
						Usage: "If turned on, use insecure push mode for OCI registry.",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					ref := cmd.String("scenario")
					dir := cmd.String("directory")

					username := cmd.String("username")
					password := cmd.String("password")

					before := time.Now()
					if err := scenario.EncodeOCI(ctx,
						ref, dir,
						cmd.Bool("insecure"), username, password,
					); err != nil {
						return err
					}
					fmt.Printf("duration: %v\n", time.Since(before))
					fmt.Printf("Scenario pushed as %s from %s\n", ref, dir)

					return nil
				},
			},
		},
	}

	ctx := context.Background()
	if err := cmd.Run(ctx, os.Args); err != nil {
		log.Fatal(err)
	}
}

func ptr[T any](t T) *T {
	return &t
}
