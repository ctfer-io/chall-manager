package parts

import (
	"strings"

	"github.com/ctfer-io/chall-manager/deploy/common"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	"github.com/pulumi/pulumi-random/sdk/v4/go/random"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type (
	EtcdCluster struct {
		pulumi.ResourceState

		rand  *random.RandomString
		chart *helm.Chart

		PodLabels pulumi.StringMapOutput
		Endpoint  pulumi.StringOutput
		Username  pulumi.StringOutput
		Password  pulumi.StringOutput
	}

	EtcdArgs struct {
		Namespace pulumi.StringInput
		Replicas  pulumi.IntInput

		Otel *common.OtelArgs
	}
)

func NewEtcdCluster(ctx *pulumi.Context, name string, args *EtcdArgs, opts ...pulumi.ResourceOption) (*EtcdCluster, error) {
	if args == nil {
		args = &EtcdArgs{}
	}

	etcd := &EtcdCluster{}
	if err := ctx.RegisterComponentResource("ctfer-io:chall-manager:etcd", name, etcd, opts...); err != nil {
		return nil, err
	}
	opts = append(opts, pulumi.Parent(etcd))
	if err := etcd.provision(ctx, args, opts...); err != nil {
		return nil, err
	}
	etcd.outputs()

	return etcd, nil
}

func (etcd *EtcdCluster) provision(ctx *pulumi.Context, args *EtcdArgs, opts ...pulumi.ResourceOption) (err error) {
	etcd.rand, err = random.NewRandomString(ctx, "etcd-password", &random.RandomStringArgs{
		Length:  pulumi.Int(16),
		Special: pulumi.Bool(false),
	}, opts...)
	if err != nil {
		return err
	}

	// Chart from https://github.com/bitnami/charts/tree/main/bitnami/etcd
	values := pulumi.Map{
		"containerPorts": pulumi.Map{
			"client": pulumi.Int(2379),
		},
		"auth": pulumi.Map{
			"rbac": pulumi.Map{
				"rootPassword": etcd.rand.Result,
			},
		},
		"replicaCount": args.Replicas,
		"commonLabels": pulumi.StringMap{
			"app.kubernetes.io/part-of": pulumi.String("chall-manager"),
		},
		"podLabels": pulumi.StringMap{
			"app.kubernetes.io/name": pulumi.String("etcd"),
		},
	}
	if args.Otel != nil {
		values["args"] = pulumi.StringArray{
			pulumi.String("etcd"), // execute etcd
			pulumi.String("--experimental-enable-distributed-tracing=true"), // start OpenTelemetry support
			args.Otel.Endpoint.ToStringOutput().ApplyT(func(edp string) string {
				addr, _ := strings.CutPrefix(edp, "http://")
				return "--experimental-distributed-tracing-address=" + addr
			}).(pulumi.StringOutput), // export to OTEL endpoint
			pulumi.String("--experimental-distributed-tracing-sampling-rate=1000000"), // TODO make it configurable
			pulumi.Sprintf("--experimental-distributed-tracing-service-name=%s", args.Otel.ServiceName),
		}
	}
	etcd.chart, err = helm.NewChart(ctx, "etcd-cluster", helm.ChartArgs{
		Chart:     pulumi.String("oci://registry-1.docker.io/bitnamicharts/etcd"),
		Version:   pulumi.String("10.2.17"),
		Namespace: args.Namespace,
		Values:    values,
	}, opts...)
	if err != nil {
		return err
	}

	return nil
}

func (etcd *EtcdCluster) outputs() {
	// Hardcoded values
	// XXX might not be sufficient
	etcd.PodLabels = pulumi.ToStringMap(map[string]string{
		"app.kubernetes.io/name": "etcd",
	}).ToStringMapOutput()
	etcd.Endpoint = pulumi.String("etcd-cluster-headless:2379").ToStringOutput()
	etcd.Username = pulumi.String("root").ToStringOutput()

	// Generated values
	etcd.Password = etcd.rand.Result
}