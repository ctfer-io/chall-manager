package parts

import (
	"strings"

	"github.com/ctfer-io/chall-manager/deploy/common"
	batchv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/batch/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type (
	ChallManagerJanitor struct {
		pulumi.ResourceState

		cjob *batchv1.CronJob

		PodLabels pulumi.StringMapOutput
	}

	ChallManagerJanitorArgs struct {
		// Tag defines the specific tag to run chall-manager to.
		// If not specified, defaults to "latest".
		Tag pulumi.StringPtrInput
		tag pulumi.StringOutput

		// PrivateRegistry define from where to fetch the Chall-Manager Docker images.
		// If set empty, defaults to Docker Hub.
		// Authentication is not supported, please provide it as Kubernetes-level configuration.
		PrivateRegistry pulumi.StringPtrInput
		privateRegistry pulumi.StringOutput

		// Namespace to which deploy the chall-manager resources.
		// It is different from the namespace the chall-manager will deploy instances to,
		// which will be created on the fly.
		Namespace pulumi.StringInput

		// ChallManagerEndpoint to reach the gRPC API of chall-manager.
		ChallManagerEndpoint pulumi.StringInput

		// Cron is the cron controlling how often the chall-manager-janitor must run.
		// If not set, default to every 15 minutes.
		Cron pulumi.StringPtrInput
		cron pulumi.StringOutput

		Otel *common.OtelArgs
	}
)

const (
	defaultCron = "*/1 * * * *"
)

func NewChallManagerJanitor(ctx *pulumi.Context, name string, args *ChallManagerJanitorArgs, opts ...pulumi.ResourceOption) (*ChallManagerJanitor, error) {
	if args == nil {
		args = &ChallManagerJanitorArgs{}
	}
	if args.Tag == nil || args.Tag == pulumi.String("") {
		args.tag = pulumi.String("dev").ToStringOutput()
	} else {
		args.tag = args.Tag.ToStringPtrOutput().Elem()
	}
	if args.Cron == nil || args.Cron == pulumi.String("") {
		args.cron = pulumi.String(defaultCron).ToStringOutput()
	} else {
		args.cron = args.Cron.ToStringPtrOutput().Elem()
	}
	if args.PrivateRegistry == nil || args.PrivateRegistry == pulumi.String("") {
		args.privateRegistry = pulumi.String("").ToStringOutput()
	} else {
		args.privateRegistry = args.PrivateRegistry.ToStringPtrOutput().ApplyT(func(in *string) string {
			str := *in

			// If one set, make sure it ends with one '/'
			if str != "" && !strings.HasSuffix(str, "/") {
				str = str + "/"
			}
			return str
		}).(pulumi.StringOutput)
	}

	cmj := &ChallManagerJanitor{}
	if err := ctx.RegisterComponentResource("ctfer-io:chall-manager:chall-manager-janitor", name, cmj, opts...); err != nil {
		return nil, err
	}
	opts = append(opts, pulumi.Parent(cmj))
	if err := cmj.provision(ctx, args, opts...); err != nil {
		return nil, err
	}
	cmj.outputs()

	return cmj, nil
}

func (cmj *ChallManagerJanitor) provision(ctx *pulumi.Context, args *ChallManagerJanitorArgs, opts ...pulumi.ResourceOption) (err error) {
	// => CronJob (janitor)
	cronEnv := corev1.EnvVarArray{
		corev1.EnvVarArgs{
			Name:  pulumi.String("URL"),
			Value: args.ChallManagerEndpoint,
		},
	}
	if args.Otel != nil {
		cronEnv = append(cronEnv,
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
			cronEnv = append(cronEnv,
				corev1.EnvVarArgs{
					Name:  pulumi.String("OTEL_EXPORTER_OTLP_INSECURE"),
					Value: pulumi.String("true"),
				},
			)
		}
	}
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
						Spec: corev1.PodSpecArgs{
							Containers: corev1.ContainerArray{
								corev1.ContainerArgs{
									Name:            pulumi.String("chall-manager-janitor"),
									Image:           pulumi.Sprintf("%sctferio/chall-manager-janitor:%s", args.privateRegistry, args.tag),
									ImagePullPolicy: pulumi.String("Always"),
									Env:             cronEnv,
								},
							},
							RestartPolicy: pulumi.String("OnFailure"),
						},
					},
				},
			},
		},
	}, opts...)
	if err != nil {
		return
	}

	return
}

func (cmj *ChallManagerJanitor) outputs() {
	cmj.PodLabels = cmj.cjob.Spec.JobTemplate().Metadata().Labels()
}
