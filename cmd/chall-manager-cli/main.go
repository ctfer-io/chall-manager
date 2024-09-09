package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/ctfer-io/chall-manager/api/v1/challenge"
	"github.com/ctfer-io/chall-manager/api/v1/instance"
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
								Name: "dir",
							},
							&cli.StringFlag{
								Name: "file",
							},
							&cli.DurationFlag{
								Name: "timeout",
							},
							&cli.TimestampFlag{
								Name:   "until",
								Layout: "02-01-2006",
							},
						},
						Action: func(ctx *cli.Context) error {
							cliChall := ctx.Context.Value(cliChallKey{}).(challenge.ChallengeStoreClient)
							var scn string
							var err error
							if ctx.IsSet("dir") {
								scn, err = scenario(ctx.String("dir"))
								if err != nil {
									return err
								}
							}
							if ctx.IsSet("file") {
								b, err := os.ReadFile(ctx.String("file"))
								if err != nil {
									return err
								}
								scn = base64.StdEncoding.EncodeToString(b)
							}
							var timeout *durationpb.Duration
							if ctx.IsSet("timeout") {
								timeout = durationpb.New(ctx.Duration("timeout"))
							}
							var until *timestamppb.Timestamp
							if ctx.IsSet("until") {
								until = timestamppb.New(*ctx.Timestamp("until"))
							}

							now := time.Now()
							chall, err := cliChall.CreateChallenge(ctx.Context, &challenge.CreateChallengeRequest{
								Id:       ctx.String("id"),
								Scenario: scn,
								Timeout:  timeout,
								Until:    until,
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

							fmt.Printf("[+] Challenge %s created, hash %s, %d instances\n", chall.Id, chall.Hash, len(chall.Instances))

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
								Name: "dir",
							},
							&cli.StringFlag{
								Name: "file",
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
						},
						Action: func(ctx *cli.Context) error {
							cliChall := ctx.Context.Value(cliChallKey{}).(challenge.ChallengeStoreClient)
							var scn *string
							if ctx.IsSet("dir") {
								s, err := scenario(ctx.String("dir"))
								if err != nil {
									return err
								}
								scn = &s
							}
							if ctx.IsSet("file") {
								b, err := os.ReadFile(ctx.String("file"))
								if err != nil {
									return err
								}
								bs := base64.StdEncoding.EncodeToString(b)
								scn = &bs
							}

							// Build request with the FieldMask for fine-grained update
							req := &challenge.UpdateChallengeRequest{
								Id:       ctx.String("id"),
								Scenario: scn,
							}
							um, err := fieldmaskpb.New(req)
							if err != nil {
								return err
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

							chall, err := cliChall.UpdateChallenge(ctx.Context, &challenge.UpdateChallengeRequest{
								Id:         ctx.String("id"),
								Scenario:   scn,
								UpdateMask: um,
							})
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
						},
						Action: func(ctx *cli.Context) error {
							cliIst := ctx.Context.Value(cliIstKey{}).(instance.InstanceManagerClient)

							ist, err := cliIst.CreateInstance(ctx.Context, &instance.CreateInstanceRequest{
								ChallengeId: ctx.String("challenge_id"),
								SourceId:    ctx.String("source_id"),
							})
							if err != nil {
								return err
							}

							fmt.Printf("[+] Instance <%s,%s> created, connect with `%s`\n", ist.ChallengeId, ist.SourceId, ist.ConnectionInfo)

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
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
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
