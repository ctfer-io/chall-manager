package components

import (
	"strings"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	"github.com/pulumi/pulumi-random/sdk/v4/go/random"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type (
	EtcdCluster struct {
		rand  *random.RandomString
		chart *helm.Chart

		Endpoint pulumi.StringOutput
		Username pulumi.StringOutput
		Password pulumi.StringOutput
	}

	EtcdArgs struct {
		Namespace pulumi.StringInput
		Replicas  pulumi.IntInput

		// The Otel Collector (OTLP through gRPC) endpoint to send signals to.
		// If specified, will automatically turn on tracing.
		OTLPEndpoint pulumi.StringInput
		OTLPInsecure bool
	}
)

func NewEtcdCluster(ctx *pulumi.Context, args *EtcdArgs, opts ...pulumi.ResourceOption) (*EtcdCluster, error) {
	if args == nil {
		args = &EtcdArgs{}
	}

	etcd := &EtcdCluster{}
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
	etcd.chart, err = helm.NewChart(ctx, "etcd-cluster", helm.ChartArgs{
		Chart:     pulumi.String("oci://registry-1.docker.io/bitnamicharts/etcd"),
		Version:   pulumi.String("9.10.3"),
		Namespace: args.Namespace,
		Values: pulumi.Map{
			"containerPorts": pulumi.Map{
				"client": pulumi.Int(2379),
			},
			"auth": pulumi.Map{
				"rbac": pulumi.Map{
					"rootPassword": etcd.rand.Result,
				},
			},
			"replicaCount": args.Replicas,
			"args": pulumi.StringArray{
				pulumi.String("etcd"), // execute etcd
				pulumi.String("--experimental-enable-distributed-tracing=true"), // start OpenTelemetry support
				pulumi.Sprintf("%s", args.OTLPEndpoint.ToStringOutput().ApplyT(func(edp string) string {
					addr, _ := strings.CutPrefix(edp, "http://")
					return "--experimental-distributed-tracing-address=" + addr
				}).(pulumi.StringOutput)), // export to OTEL endpoint
				pulumi.String("--experimental-distributed-tracing-sampling-rate=1000000"),
			},
			"commonLabels": pulumi.StringMap{
				"app.kubernetes.io/part-of": pulumi.String("chall-manager"),
			},
			"podLabels": pulumi.StringMap{
				"app.kubernetes.io/name": pulumi.String("etcd"),
			},
		},
	}, opts...)
	if err != nil {
		return err
	}

	return nil
}

func (etcd *EtcdCluster) outputs() {
	// Hardcoded  values
	etcd.Endpoint = pulumi.String("etcd-cluster-headless:2379").ToStringOutput()
	etcd.Username = pulumi.String("root").ToStringOutput()

	// Generated values
	etcd.Password = etcd.rand.Result
}
