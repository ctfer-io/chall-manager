package kubernetes_test

import (
	"testing"

	k8s "github.com/ctfer-io/chall-manager/sdk/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const dcVipOnly = `
services:
  node:
    privileged: false
    image: pandatix/vip-only-node:latest
    ports:
      - "3000:3000"
    depends_on:
      - mongo
    environment:
      - MONGO_URI=mongodb://root:5e409bd6c906e75bc961de62d516ca52@mongo:27017/vipOnlyApp?authSource=admin
      - SESSION_SECRET=0A010010D98FDFDJDJHIUAY

  mongo:
    privileged: false
    image: pandatix/vip-only-mongo:latest
    ports:
      - "27017:27017"
    environment:
      MONGO_INITDB_DATABASE: vipOnlyApp
      MONGO_INITDB_ROOT_USERNAME: root
      MONGO_INITDB_ROOT_PASSWORD: 5e409bd6c906e75bc961de62d516ca52
`

func Test_U_Kompose(t *testing.T) {
	t.Parallel()

	var tests = map[string]struct {
		Args      *k8s.KomposeArgs
		ExpectErr bool
	}{
		"nil-args": {
			Args:      nil,
			ExpectErr: true,
		},
		"empty-args": {
			Args:      &k8s.KomposeArgs{},
			ExpectErr: true,
		},
		"vip-only-no-portbinding": {
			Args: &k8s.KomposeArgs{
				Identity: pulumi.String("a0b1c2d3"),
				Hostname: pulumi.String("24hiut25.ctfer.io"),
				YAML:     pulumi.String(dcVipOnly),
				Ports:    nil,
			},
			ExpectErr: true,
		},
		"vip-only-nodeport": {
			Args: &k8s.KomposeArgs{
				Identity: pulumi.String("a0b1c2d3"),
				Hostname: pulumi.String("24hiut25.ctfer.io"),
				YAML:     pulumi.String(dcVipOnly),
				Ports: k8s.PortBindingMapArray{
					"node": {
						k8s.PortBindingArgs{
							Port:       pulumi.Int(3000),
							ExposeType: k8s.ExposeNodePort,
						},
					},
				},
			},
			ExpectErr: false,
		},
		"vip-only-ingress": {
			Args: &k8s.KomposeArgs{
				Identity: pulumi.String("a0b1c2d3"),
				Hostname: pulumi.String("24hiut25.ctfer.io"),
				YAML:     pulumi.String(dcVipOnly),
				Ports: k8s.PortBindingMapArray{
					"node": {
						k8s.PortBindingArgs{
							Port:       pulumi.Int(3000),
							ExposeType: k8s.ExposeIngress,
						},
					},
				},
			},
			ExpectErr: false,
		},
		"loadbalancer": {
			Args: &k8s.KomposeArgs{
				Identity: pulumi.String("a0b1c2d3"),
				Hostname: pulumi.String("24hiut25.ctfer.io"),
				YAML:     pulumi.String(dcVipOnly),
				Ports: k8s.PortBindingMapArray{
					"node": {
						k8s.PortBindingArgs{
							Port:       pulumi.Int(3000),
							ExposeType: k8s.ExposeLoadBalancer,
						},
					},
				},
			},
			ExpectErr: false,
		},
	}

	for testname, tt := range tests {
		t.Run(testname, func(t *testing.T) {
			err := pulumi.RunErr(func(ctx *pulumi.Context) error {
				_, err := k8s.NewKompose(ctx, "kmp-test", tt.Args)
				if tt.ExpectErr {
					require.Error(t, err)
				} else {
					require.NoError(t, err)

					// We cannot run unit tests on URLs (output)
					//
					// This SDK uses a yaml/v2:ConfigGroup to deploy everything from
					// the output of a Kompose call. It does not directly create the
					// objects, especially the core/v1:Service and such even with the
					// mocks{} implementation.
					//
					// We are thus not able to define nodeports, loadbalancer
					// external IP, nor Ingresses... So cannot tests for URLs validity.
				}

				return nil
			}, pulumi.WithMocks("project", "stack", mocks{}))
			assert.NoError(t, err)
		})
	}
}
