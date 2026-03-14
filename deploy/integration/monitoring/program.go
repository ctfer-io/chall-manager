package monitoring

import (
	monservices "github.com/ctfer-io/monitoring/services"
	"github.com/pkg/errors"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	netwv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/networking/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"

	"github.com/ctfer-io/chall-manager/deploy/common"
	"github.com/ctfer-io/chall-manager/deploy/services"
	"github.com/ctfer-io/chall-manager/deploy/services/parts"
)

func Program() {
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
				Endpoint:    mon.OTEL.Endpoint,
				Insecure:    true, // we do not secure communications in this simple setup
			},
		})
		if err != nil {
			return errors.Wrap(err, "deploying chall-manager")
		}

		if _, err := netwv1.NewNetworkPolicy(ctx, "allow-inside-oci", &netwv1.NetworkPolicyArgs{
			Metadata: metav1.ObjectMetaArgs{
				Namespace: pulumi.String(cfg.Namespace),
				Labels: pulumi.StringMap{
					"app.kubernetes.io/component": pulumi.String("chall-manager"),
					"app.kubernetes.io/part-of":   pulumi.String("chall-manager"),
					"ctfer.io/stack-name":         pulumi.String(ctx.Stack()),
				},
			},
			Spec: netwv1.NetworkPolicySpecArgs{
				PodSelector: metav1.LabelSelectorArgs{
					MatchLabels: cm.PodLabels,
				},
				PolicyTypes: pulumi.ToStringArray([]string{
					"Egress",
				}),
				Egress: netwv1.NetworkPolicyEgressRuleArray{
					netwv1.NetworkPolicyEgressRuleArgs{
						Ports: netwv1.NetworkPolicyPortArray{
							netwv1.NetworkPolicyPortArgs{
								Port: pulumi.Int(5000), // we serve the OCI on this port
							},
						},
						To: netwv1.NetworkPolicyPeerArray{
							netwv1.NetworkPolicyPeerArgs{
								IpBlock: netwv1.IPBlockArgs{
									Cidr: pulumi.String("172.16.0.0/12"), // The CIDR the OCI registry lays into as a mirror
								},
							},
						},
					},
				},
			},
		}); err != nil {
			return errors.Wrap(err, "allowing inside OCI traffic")
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
