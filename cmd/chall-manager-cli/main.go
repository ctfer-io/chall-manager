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
	"github.com/urfave/cli/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type cliChallKey struct{}
type cliIstKey struct{}

func main() {
	app := cli.App{
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
				Before: func(ctx *cli.Context) error {
					conn, err := grpc.NewClient(ctx.String("url"),
						grpc.WithTransportCredentials(insecure.NewCredentials()),
					)
					if err != nil {
						return err
					}
					cliChall := challenge.NewChallengeStoreClient(conn)

					ctx.Context = context.WithValue(ctx.Context, cliChallKey{}, cliChall)
					return nil
				},
				Subcommands: []*cli.Command{
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
								Name:   "until",
								Layout: "02-01-2006",
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
						Action: func(ctx *cli.Context) error {
							cliChall := ctx.Context.Value(cliChallKey{}).(challenge.ChallengeStoreClient)
							var timeout *durationpb.Duration
							if ctx.IsSet("timeout") {
								timeout = durationpb.New(ctx.Duration("timeout"))
							}
							var until *timestamppb.Timestamp
							if ctx.IsSet("until") {
								until = timestamppb.New(*ctx.Timestamp("until"))
							}
							var add map[string]string
							if ctx.IsSet("additional") {
								slc := ctx.StringSlice("additional")
								add = make(map[string]string, len(slc))
								for _, kv := range slc {
									k, v, _ := strings.Cut(kv, "=")
									add[k] = v
								}
							}

							ref := ctx.String("scenario")
							if ctx.IsSet("directory") {
								var username, password *string
								if ctx.IsSet("username") {
									username = ptr(ctx.String("username"))
								}
								if ctx.IsSet("password") {
									password = ptr(ctx.String("password"))
								}
								if err := scenario.EncodeOCI(ctx.Context,
									ref, ctx.String("directory"),
									ctx.Bool("insecure"), username, password,
								); err != nil {
									return err
								}
							}

							now := time.Now()
							chall, err := cliChall.CreateChallenge(ctx.Context, &challenge.CreateChallengeRequest{
								Id:         ctx.String("id"),
								Scenario:   ref,
								Timeout:    timeout,
								Until:      until,
								Additional: add,
								Min:        ctx.Int64("min"),
								Max:        ctx.Int64("max"),
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
						Action: func(ctx *cli.Context) error {
							cliChall := ctx.Context.Value(cliChallKey{}).(challenge.ChallengeStoreClient)

							chall, err := cliChall.RetrieveChallenge(ctx.Context, &challenge.RetrieveChallengeRequest{
								Id: ctx.String("id"),
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
								Name:   "until",
								Layout: "02-01-2006",
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
								Action: func(_ *cli.Context, strategy string) error {
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
						Action: func(ctx *cli.Context) error {
							cliChall := ctx.Context.Value(cliChallKey{}).(challenge.ChallengeStoreClient)

							ref := ctx.String("scenario")
							if ctx.IsSet("directory") {
								dir := ctx.String("directory")
								var username, password *string
								if ctx.IsSet("username") {
									username = ptr(ctx.String("username"))
								}
								if ctx.IsSet("password") {
									password = ptr(ctx.String("password"))
								}
								if err := scenario.EncodeOCI(ctx.Context,
									ref, dir,
									ctx.Bool("insecure"), username, password,
								); err != nil {
									return err
								}
							}

							// Build request with the FieldMask for fine-grained update
							req := &challenge.UpdateChallengeRequest{
								Id: ctx.String("id"),
							}
							um, err := fieldmaskpb.New(req)
							if err != nil {
								return err
							}
							if ctx.IsSet("scenario") {
								if err := um.Append(req, "scenario"); err != nil {
									return err
								}
								req.Scenario = ptr(ctx.String("scenario"))
							}
							if ctx.IsSet("timeout") {
								if err := um.Append(req, "timeout"); err != nil {
									return err
								}
								req.Timeout = durationpb.New(ctx.Duration("timeout"))
							} else if ctx.Bool("reset-timeout") {
								if err := um.Append(req, "timeout"); err != nil {
									return err
								}
							}
							if ctx.IsSet("until") {
								if err := um.Append(req, "until"); err != nil {
									return err
								}
								req.Until = timestamppb.New(*ctx.Timestamp("until"))
							} else if ctx.Bool("reset-until") {
								if err := um.Append(req, "until"); err != nil {
									return err
								}
							}
							if ctx.IsSet("additional") {
								if err := um.Append(req, "additional"); err != nil {
									return err
								}
								slc := ctx.StringSlice("additional")
								req.Additional = make(map[string]string, len(slc))
								for _, kv := range slc {
									k, v, _ := strings.Cut(kv, "=")
									req.Additional[k] = v
								}
							} else if ctx.Bool("reset-additional") {
								if err := um.Append(req, "additional"); err != nil {
									return err
								}
							}
							if ctx.IsSet("min") {
								if err := um.Append(req, "min"); err != nil {
									return err
								}
								req.Min = ctx.Int64("min")
							}
							if ctx.IsSet("max") {
								if err := um.Append(req, "max"); err != nil {
									return err
								}
								req.Max = ctx.Int64("max")
							}
							switch ctx.String("strategy") {
							case "blue-green":
								req.UpdateStrategy = challenge.UpdateStrategy_blue_green.Enum()
							case "recreate":
								req.UpdateStrategy = challenge.UpdateStrategy_recreate.Enum()
							case "in-place":
								req.UpdateStrategy = challenge.UpdateStrategy_update_in_place.Enum()
							}

							req.UpdateMask = um
							chall, err := cliChall.UpdateChallenge(ctx.Context, req)
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
						Action: func(ctx *cli.Context) error {
							cliChall := ctx.Context.Value(cliChallKey{}).(challenge.ChallengeStoreClient)
							id := ctx.String("id")
							if _, err := cliChall.DeleteChallenge(ctx.Context, &challenge.DeleteChallengeRequest{
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
				Before: func(ctx *cli.Context) error {
					conn, err := grpc.NewClient(ctx.String("url"), grpc.WithTransportCredentials(insecure.NewCredentials()))
					if err != nil {
						return err
					}
					cliIst := instance.NewInstanceManagerClient(conn)

					ctx.Context = context.WithValue(ctx.Context, cliIstKey{}, cliIst)
					return nil
				},
				Subcommands: []*cli.Command{
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
						Action: func(ctx *cli.Context) error {
							cliIst := ctx.Context.Value(cliIstKey{}).(instance.InstanceManagerClient)

							var add map[string]string
							if ctx.IsSet("additional") {
								slc := ctx.StringSlice("additional")
								add = make(map[string]string, len(slc))
								for _, kv := range slc {
									k, v, _ := strings.Cut(kv, "=")
									add[k] = v
								}
							}

							before := time.Now()
							ist, err := cliIst.CreateInstance(ctx.Context, &instance.CreateInstanceRequest{
								ChallengeId: ctx.String("challenge_id"),
								SourceId:    ctx.String("source_id"),
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
						Action: func(ctx *cli.Context) error {
							cliIst := ctx.Context.Value(cliIstKey{}).(instance.InstanceManagerClient)

							if _, err := cliIst.DeleteInstance(ctx.Context, &instance.DeleteInstanceRequest{
								ChallengeId: ctx.String("challenge_id"),
								SourceId:    ctx.String("source_id"),
							}); err != nil {
								return err
							}

							fmt.Printf("[+] Instance <%s,%s> deleted\n", ctx.String("challenge_id"), ctx.String("source_id"))

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
				Action: func(ctx *cli.Context) error {
					ref := ctx.String("scenario")
					dir := ctx.String("directory")

					var username, password *string
					if ctx.IsSet("username") {
						username = ptr(ctx.String("username"))
					}
					if ctx.IsSet("password") {
						password = ptr(ctx.String("password"))
					}

					before := time.Now()
					if err := scenario.EncodeOCI(ctx.Context,
						ref, dir,
						ctx.Bool("insecure"), username, password,
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

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func ptr[T any](t T) *T {
	return &t
}
