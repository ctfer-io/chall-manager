package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"time"

	"github.com/ctfer-io/chall-manager/global"
	"github.com/ctfer-io/chall-manager/pkg/lock"
	"github.com/ctfer-io/chall-manager/pkg/state"
	"github.com/pkg/errors"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

var (
	version = "dev"
	commit  = ""
	date    = ""
	builtBy = ""
)

var (
	regId = regexp.MustCompile(`^[a-f0-9]{32}$`) // hexa on 32 characters
)

func main() {
	app := &cli.App{
		Name:  "Chall-Manager-Janitor",
		Usage: "Chall-Manager-Janitor is an utility that handles infrastructures timeouts thus their destructions.",
		Flags: []cli.Flag{
			cli.VersionFlag,
			cli.HelpFlag,
			&cli.StringFlag{
				Name:        "dir",
				Aliases:     []string{"d"},
				EnvVars:     []string{"DIRECTORY"},
				Value:       "cm", // **C**hall-**M**anager, could fail locally as not part of the main go module
				Destination: &global.Conf.Directory,
				Usage:       "Define the volume to read/write stack and states to. It should be sharded across chall-manager and the janitor replicas for HA.",
			},
			&cli.StringFlag{
				Name:        "lock-kind",
				EnvVars:     []string{"LOCK_KIND"},
				Value:       "etcd",
				Destination: &global.Conf.Lock.Kind,
				Usage:       "Define the lock kind to use. It could either be \"ectd\" for Kubernetes-native deployments (recommended) or \"local\" for a flock on the host machine (not scalable but at least handle local replicas).",
				Action: func(ctx *cli.Context, s string) error {
					if !slices.Contains([]string{"etcd", "local"}, s) {
						return errors.New("invalid lock kind value")
					}
					return nil
				},
			},
			&cli.StringSliceFlag{
				Name:    "lock-etcd-endpoints",
				EnvVars: []string{"LOCK_ETCD_ENDPOINTS"},
				Usage:   "Define the etcd endpoints to reach for locks.",
				Action: func(ctx *cli.Context, s []string) error {
					if ctx.String("lock-kind") != "etcd" {
						return errors.New("incompatible lock kind with lock-etcd-endpoints, expect etcd")
					}

					// use action instead of destination to avoid dealing with conversions
					global.Conf.Lock.EtcdEndpoints = s
					return nil
				},
			},
			&cli.StringFlag{
				Name:        "lock-etcd-username",
				EnvVars:     []string{"LOCK_ETCD_USERNAME"},
				Destination: &global.Conf.Lock.EtcdUsername,
				Usage:       "If lock kind is etcd, define the username to use to connect to the etcd cluster.",
				Action: func(ctx *cli.Context, s string) error {
					if ctx.String("lock-kind") != "etcd" {
						return errors.New("incompatible lock kind with lock-etcd-username, expect etcd")
					}
					return nil
				},
			},
			&cli.StringFlag{
				Name:        "lock-etcd-password",
				EnvVars:     []string{"LOCK_ETCD_PASSWORD"},
				Destination: &global.Conf.Lock.EtcdPassword,
				Usage:       "If lock kind is etcd, define the password to use to connect to the etcd cluster.",
				Action: func(ctx *cli.Context, s string) error {
					if ctx.String("lock-kind") != "etcd" {
						return errors.New("incompatible lock kind with lock-etcd-password, expect etcd")
					}
					return nil
				},
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
	// List states in directory and handle each one by one
	cd := filepath.Join(global.Conf.Directory, "states")
	de, err := os.ReadDir(cd)
	if err != nil {
		return err
	}

	for _, f := range de {
		// Should not be a directory or unrelated file, skip it if happens before panic
		if f.IsDir() || !regId.MatchString(f.Name()) {
			continue
		}

		if err := handle(ctx.Context, cd, f.Name()); err != nil {
			return err
		}
	}

	return nil
}

func handle(ctx context.Context, cd, id string) error {
	logger := global.Log()

	// Make sure only 1 parallel launch for this challenge
	// (avoid overwriting files during parallel requests handling).
	rw, err := lock.NewRWLock(id)
	if err != nil {
		return err
	}
	if err := rw.RWLock(); err != nil {
		return err
	}
	defer func(rw lock.RWLock) {
		if err := rw.RWUnlock(); err != nil {
			logger.Error("failed to release lock, could stuck the identity until renewal",
				zap.Error(err),
			)
		}
	}(rw)

	// Open state
	b, err := os.ReadFile(filepath.Join(cd, id))
	if err != nil {
		return err
	}
	st := &state.State{}
	if err := json.Unmarshal(b, st); err != nil {
		return err
	}

	// Look for destruction
	if st.Metadata.Until == nil || st.Metadata.Until.After(time.Now()) {
		return nil
	}
	logger.Info("destructing state",
		zap.String("identity", id),
		zap.Time("until", *st.Metadata.Until),
	)
	stack, err := loadStackWithState(ctx, id, st)
	if err != nil {
		return err
	}
	_, err = stack.Destroy(ctx)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(global.Conf.Directory, "states", id)); err != nil {
		return err
	}

	return nil
}

func loadStackWithState(ctx context.Context, id string, st *state.State) (auto.Stack, error) {
	cd := filepath.Join(global.Conf.Directory, "scenarios", st.Metadata.ChallengeId, st.Metadata.SourceDir)
	b, err := os.ReadFile(filepath.Join(cd, "Pulumi.yaml"))
	if err != nil {
		return auto.Stack{}, err
	}
	type PulumiYaml struct {
		Name string `yaml:"name"`
		// Runtime and Description are not used
	}
	var yml PulumiYaml
	if err := yaml.Unmarshal(b, &yml); err != nil {
		return auto.Stack{}, err
	}

	// Create workspace in decoded+unzipped archive directory
	ws, err := auto.NewLocalWorkspace(ctx,
		auto.WorkDir(cd),
		auto.EnvVars(map[string]string{
			"PULUMI_CONFIG_PASSPHRASE": "",
			"CM_PROJECT":               yml.Name, // necessary to load the configuration
		}),
	)
	if err != nil {
		return auto.Stack{}, err
	}

	// Build stack
	stackName := auto.FullyQualifiedStackName("organization", yml.Name, id)
	stack, err := auto.UpsertStack(ctx, stackName, ws)
	if err != nil {
		return auto.Stack{}, errors.Wrapf(err, "while upserting stack %s", stackName)
	}

	// Load state
	if err := stack.Import(ctx, apitype.UntypedDeployment{
		Version:    3,
		Deployment: st.Pulumi,
	}); err != nil {
		return auto.Stack{}, errors.Wrap(err, "while importing state")
	}

	return stack, nil
}
