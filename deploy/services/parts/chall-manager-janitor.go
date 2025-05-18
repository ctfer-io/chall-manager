package parts

import (
	"fmt"
	"strings"

	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	batchv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/batch/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/ctfer-io/chall-manager/deploy/common"
)

type (
	ChallManagerJanitor struct {
		pulumi.ResourceState

		cjob *batchv1.CronJob
		dep  *appsv1.Deployment

		PodLabels pulumi.StringMapOutput
	}

	ChallManagerJanitorArgs struct {
		// Tag defines the specific tag to run chall-manager to.
		// If not specified, defaults to "latest".
		Tag pulumi.StringPtrInput
		tag pulumi.StringOutput

		// Registry define from where to fetch the Chall-Manager Docker images.
		// If set empty, defaults to Docker Hub.
		// Authentication is not supported, please provide it as Kubernetes-level configuration.
		Registry pulumi.StringPtrInput
		registry pulumi.StringOutput

		// Namespace to which deploy the chall-manager resources.
		// It is different from the namespace the chall-manager will deploy instances to,
		// which will be created on the fly.
		Namespace pulumi.StringInput

		// ChallManagerEndpoint to reach the chall-manager API.
		ChallManagerEndpoint pulumi.StringInput

		// Cron is the cron controlling how often the chall-manager-janitor must run.
		// If not set, default to every 15 minutes.
		Cron   pulumi.StringPtrInput
		cron   pulumi.StringOutput
		Ticker pulumi.StringPtrInput
		ticker pulumi.StringOutput
		Mode   JanitorMode

		Otel *common.OtelArgs
	}
)

type JanitorMode string

var (
	JanitorModeCron   JanitorMode = "cron"
	JanitorModeTicker JanitorMode = "ticker"
)

const (
	defaultTag    = "dev"
	defaultCron   = "*/1 * * * *"
	defaultTicker = "1m"
)

func NewChallManagerJanitor(ctx *pulumi.Context, name string, args *ChallManagerJanitorArgs, opts ...pulumi.ResourceOption) (*ChallManagerJanitor, error) {
	cmj := &ChallManagerJanitor{}

	args = cmj.defaults(args)
	if err := ctx.RegisterComponentResource("ctfer-io:chall-manager:chall-manager-janitor", name, cmj, opts...); err != nil {
		return nil, err
	}
	opts = append(opts, pulumi.Parent(cmj))
	if err := cmj.provision(ctx, args, opts...); err != nil {
		return nil, err
	}
	cmj.outputs(ctx, args)

	return cmj, nil
}

func (cmj *ChallManagerJanitor) defaults(args *ChallManagerJanitorArgs) *ChallManagerJanitorArgs {
	if args == nil {
		args = &ChallManagerJanitorArgs{}
	}

	args.tag = pulumi.String(defaultTag).ToStringOutput()
	if args.Tag != nil {
		args.tag = args.Tag.ToStringPtrOutput().ApplyT(func(tag *string) string {
			if tag == nil || *tag == "" {
				return defaultTag
			}
			return *tag
		}).(pulumi.StringOutput)
	}

	args.cron = pulumi.String(defaultCron).ToStringOutput()
	if args.Cron != nil {
		args.cron = args.Cron.ToStringPtrOutput().ApplyT(func(cron *string) string {
			if cron == nil || *cron == "" {
				return defaultCron
			}
			return *cron
		}).(pulumi.StringOutput)
	}

	args.ticker = pulumi.String(defaultTicker).ToStringOutput()
	if args.Ticker != nil {
		args.ticker = args.Ticker.ToStringPtrOutput().ApplyT(func(ticker *string) string {
			if ticker == nil || *ticker == "" {
				return defaultTicker
			}
			return *ticker
		}).(pulumi.StringOutput)
	}

	if args.Mode == JanitorMode("") {
		args.Mode = JanitorModeCron
	}

	args.registry = pulumi.String("").ToStringOutput()
	if args.Registry != nil {
		args.registry = args.Registry.ToStringPtrOutput().ApplyT(func(in *string) string {
			str := *in

			// If one set, make sure it ends with one '/'
			if str != "" && !strings.HasSuffix(str, "/") {
				str = str + "/"
			}
			return str
		}).(pulumi.StringOutput)
	}

	return args
}

func (cmj *ChallManagerJanitor) provision(ctx *pulumi.Context, args *ChallManagerJanitorArgs, opts ...pulumi.ResourceOption) (err error) {
	// => CronJob (janitor)
	envs := corev1.EnvVarArray{
		corev1.EnvVarArgs{
			Name:  pulumi.String("URL"),
			Value: args.ChallManagerEndpoint,
		},
	}
	if args.Otel != nil {
		envs = append(envs,
			corev1.EnvVarArgs{
				Name:  pulumi.String("TRACING"),
				Value: pulumi.String("true"),
			},
			corev1.EnvVarArgs{
				Name:  pulumi.String("OTEL_SERVICE_NAME"),
				Value: args.Otel.ServiceName,
			},
			corev1.EnvVarArgs{
				Name:  pulumi.String("OTEL_EXPORTER_OTLP_ENDPOINT"),
				Value: args.Otel.Endpoint,
			},
		)
		if args.Otel.Insecure {
			envs = append(envs,
				corev1.EnvVarArgs{
					Name:  pulumi.String("OTEL_EXPORTER_OTLP_INSECURE"),
					Value: pulumi.String("true"),
				},
			)
		}
	}
	if args.Mode == JanitorModeTicker {
		envs = append(envs, corev1.EnvVarArgs{
			Name:  pulumi.String("TICKER"),
			Value: args.ticker,
		})
	}

	restartPolicy := "OnFailure"
	if args.Mode == JanitorModeTicker {
		restartPolicy = "Always"
	}
	podSpecArgs := corev1.PodSpecArgs{
		InitContainers: corev1.ContainerArray{
			corev1.ContainerArgs{
				Name:  pulumi.String("readiness"),
				Image: pulumi.Sprintf("%slibrary/busybox:1.28", args.registry),
				Command: pulumi.ToStringArray([]string{
					"/bin/sh",
					"-c",
					`healthcheck() { curl --silent --fail --connect-timeout 5 "$URL" > /dev/null; }`,
					`i=1; while [ "$i" -le "$MAX_RETRIES" ]; do if healthcheck; then exit 0; fi; sleep $SLEEP_INTERVAL; i=$((i + 1)); done; exit 1`,
				}),
				Env: corev1.EnvVarArray{
					corev1.EnvVarArgs{
						Name: pulumi.String("URL"),
						// TODO make http or https configurable
						Value: pulumi.Sprintf("http://%s/healthcheck", args.ChallManagerEndpoint),
					},
					corev1.EnvVarArgs{
						Name:  pulumi.String("MAX_RETRIES"),
						Value: pulumi.String("60"),
					},
					corev1.EnvVarArgs{
						Name:  pulumi.String("SLEEP_INTERVAL"),
						Value: pulumi.String("2"),
					},
				},
			},
		},
		Containers: corev1.ContainerArray{
			corev1.ContainerArgs{
				Name:            pulumi.String("chall-manager-janitor"),
				Image:           pulumi.Sprintf("%sctferio/chall-manager-janitor:%s", args.registry, args.tag),
				Env:             envs,
				ImagePullPolicy: pulumi.String("Always"),
			},
		},
		RestartPolicy: pulumi.String(restartPolicy),
	}

	switch args.Mode {
	case JanitorModeCron:
		cmj.cjob, err = batchv1.NewCronJob(ctx, "chall-manager-janitor", &batchv1.CronJobArgs{
			Metadata: metav1.ObjectMetaArgs{
				Namespace: args.Namespace,
				Labels: pulumi.StringMap{
					"app.kubernetes.io/name":      pulumi.String("chall-manager-janitor"),
					"app.kubernetes.io/version":   args.tag,
					"app.kubernetes.io/component": pulumi.String("chall-manager"),
					"app.kubernetes.io/part-of":   pulumi.String("chall-manager"),
				},
			},
			Spec: batchv1.CronJobSpecArgs{
				Schedule: args.cron,
				JobTemplate: batchv1.JobTemplateSpecArgs{
					Spec: batchv1.JobSpecArgs{
						Template: corev1.PodTemplateSpecArgs{
							Metadata: metav1.ObjectMetaArgs{
								Namespace: args.Namespace,
								Labels: pulumi.StringMap{
									"app.kubernetes.io/name":      pulumi.String("chall-manager-janitor"),
									"app.kubernetes.io/version":   args.tag,
									"app.kubernetes.io/component": pulumi.String("chall-manager"),
									"app.kubernetes.io/part-of":   pulumi.String("chall-manager"),
								},
							},
							Spec: podSpecArgs,
						},
					},
				},
			},
		}, opts...)
		if err != nil {
			return
		}

	case JanitorModeTicker:
		cmj.dep, err = appsv1.NewDeployment(ctx, "chall-manager-janitor", &appsv1.DeploymentArgs{
			Metadata: metav1.ObjectMetaArgs{
				Namespace: args.Namespace,
				Labels: pulumi.StringMap{
					"app.kubernetes.io/name":      pulumi.String("chall-manager-janitor"),
					"app.kubernetes.io/version":   args.tag,
					"app.kubernetes.io/component": pulumi.String("chall-manager"),
					"app.kubernetes.io/part-of":   pulumi.String("chall-manager"),
				},
			},
			Spec: appsv1.DeploymentSpecArgs{
				Selector: metav1.LabelSelectorArgs{
					MatchLabels: pulumi.StringMap{
						"app.kubernetes.io/name":      pulumi.String("chall-manager-janitor"),
						"app.kubernetes.io/version":   args.tag,
						"app.kubernetes.io/component": pulumi.String("chall-manager"),
						"app.kubernetes.io/part-of":   pulumi.String("chall-manager"),
					},
				},
				Template: corev1.PodTemplateSpecArgs{
					Metadata: metav1.ObjectMetaArgs{
						Namespace: args.Namespace,
						Labels: pulumi.StringMap{
							"app.kubernetes.io/name":      pulumi.String("chall-manager-janitor"),
							"app.kubernetes.io/version":   args.tag,
							"app.kubernetes.io/component": pulumi.String("chall-manager"),
							"app.kubernetes.io/part-of":   pulumi.String("chall-manager"),
						},
					},
					Spec: podSpecArgs,
				},
			},
		}, opts...)
		if err != nil {
			return
		}

	default:
		return fmt.Errorf("unsupported janitor mode %s", args.Mode)
	}

	return
}

func (cmj *ChallManagerJanitor) outputs(ctx *pulumi.Context, args *ChallManagerJanitorArgs) error {
	switch args.Mode {
	case JanitorModeCron:
		cmj.PodLabels = cmj.cjob.Spec.JobTemplate().Metadata().Labels()

	case JanitorModeTicker:
		cmj.PodLabels = cmj.dep.Metadata.Labels()
	}

	cmj.PodLabels = cmj.cjob.Spec.JobTemplate().Metadata().Labels()

	return ctx.RegisterResourceOutputs(cmj, pulumi.Map{
		"podLabels": cmj.PodLabels,
	})
}
