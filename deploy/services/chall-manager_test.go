package services_test

import (
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"

	"github.com/ctfer-io/chall-manager/deploy/common"
	"github.com/ctfer-io/chall-manager/deploy/services"
	"github.com/ctfer-io/chall-manager/deploy/services/parts"
)

type mocks struct{}

func (mocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	return args.Name + "_id", args.Inputs, nil
}

func (mocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	return args.Args, nil
}

func Test_U_ChallManager(t *testing.T) {
	t.Parallel()

	var tests = map[string]struct {
		Args *services.ChallManagerArgs
	}{
		"nil-args": {
			Args: nil,
		},
		"empty-args": {
			Args: &services.ChallManagerArgs{},
		},
		"local": {
			Args: &services.ChallManagerArgs{
				Tag:             pulumi.String("alpha-1"),
				PrivateRegistry: pulumi.String("registry.dev1.ctfer-io.lab"),
			},
		},
		"local-otel": {
			Args: &services.ChallManagerArgs{
				Tag:             pulumi.String("alpha-1"),
				PrivateRegistry: pulumi.String("registry.dev1.ctfer-io.lab"),
				Otel: &common.OtelArgs{
					ServiceName: pulumi.String("test"),
					Endpoint:    pulumi.String("http://my.otel.edp:4317"),
					Insecure:    true,
				},
			},
		},
		"etcd": {
			Args: &services.ChallManagerArgs{
				Tag:             pulumi.String("alpha-1"),
				PrivateRegistry: pulumi.String("registry.dev1.ctfer-io.lab"),
				Namespace:       pulumi.String("random-namespace"),
				Replicas:        pulumi.Int(2),
				EtcdReplicas:    pulumi.Int(1),
				Otel: &common.OtelArgs{
					ServiceName: pulumi.String("test"),
					Endpoint:    pulumi.String("http://my.otel.edp:4317"),
					Insecure:    true,
				},
			},
		},
		"etcd-otel": {
			Args: &services.ChallManagerArgs{
				Tag:             pulumi.String("alpha-1"),
				PrivateRegistry: pulumi.String("registry.dev1.ctfer-io.lab"),
				Namespace:       pulumi.String("random-namespace"),
				Replicas:        pulumi.Int(2),
				EtcdReplicas:    pulumi.Int(1),
				Otel: &common.OtelArgs{
					ServiceName: pulumi.String("test"),
					Endpoint:    pulumi.String("http://my.otel.edp:4317"),
					Insecure:    true,
				},
			},
		},
		"public-dev": {
			Args: &services.ChallManagerArgs{
				Swagger: true,
			},
		},
		"cron-and-ticker": {
			Args: &services.ChallManagerArgs{
				JanitorCron:   pulumi.String("0/* * * * *"),
				JanitorTicker: pulumi.String("1m"),
				JanitorMode:   parts.JanitorModeCron,
			},
		},
	}

	for testname, tt := range tests {
		t.Run(testname, func(t *testing.T) {
			assert := assert.New(t)

			err := pulumi.RunErr(func(ctx *pulumi.Context) error {
				cm, err := services.NewChallManager(ctx, "cm-test", tt.Args)
				assert.NoError(err)

				cm.Endpoint.ApplyT(func(edp string) error {
					assert.NotEmpty(edp)
					return nil
				})

				return nil
			}, pulumi.WithMocks("project", "stack", mocks{}))
			assert.NoError(err)
		})
	}
}
