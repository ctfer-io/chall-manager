package monitoring

import (
	monservices "github.com/ctfer-io/monitoring/services"
	"github.com/pkg/errors"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"

	"github.com/ctfer-io/chall-manager/deploy/common"
	"github.com/ctfer-io/chall-manager/deploy/services"
	"github.com/ctfer-io/chall-manager/deploy/services/parts"
)

func Run() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		cfg := loadConfig(ctx)

		mon, err := monservices.NewMonitoring(ctx, "monitoring", &monservices.MonitoringArgs{
			ColdExtract: true, // we want to do this to ensure there are datas
		})
		if err != nil {
			return errors.Wrap(err, "deploying monitoring")
		}

		cm, err := services.NewChallManager(ctx, "chall-manager", &services.ChallManagerArgs{
			Namespace:      pulumi.String(cfg.Namespace),
			Tag:            pulumi.String(cfg.Tag),
			Registry:       pulumi.String(cfg.Registry),
			Expose:         true,
			RomeoClaimName: pulumi.String(cfg.RomeoClaimName),
			JanitorMode:    parts.JanitorModeTicker,
			JanitorTicker:  pulumi.String("3s"),
			Otel: &common.OtelArgs{
				ServiceName: pulumi.String(ctx.Stack()),
				Endpoint:    pulumi.Sprintf("dns://%s", mon.OTEL.Endpoint), // we have no specific infra, just reach the collector
				Insecure:    true,                                          // we do not secure communications in this simple setup
			},
		})
		if err != nil {
			return errors.Wrap(err, "deploying chall-manager")
		}

		ctx.Export("mon.namespace", mon.Namespace)
		ctx.Export("mon.cold-extract-pvc-name", mon.OTEL.ColdExtractPVCName)
		ctx.Export("exposed_port", cm.ExposedPort)

		return nil
	})
}

type Config struct {
	Namespace      string
	Tag            string
	Registry       string
	RomeoClaimName string
}

func loadConfig(ctx *pulumi.Context) *Config {
	cfg := config.New(ctx, "")
	return &Config{
		Namespace:      cfg.Get("namespace"),
		Tag:            cfg.Get("tag"),
		Registry:       cfg.Get("registry"),
		RomeoClaimName: cfg.Get("romeo-claim-name"),
	}
}
