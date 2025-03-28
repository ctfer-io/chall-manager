package kubernetes_test

import (
	"net/url"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	k8s "github.com/ctfer-io/chall-manager/sdk/kubernetes"
)

func Test_U_ExposedMonopod(t *testing.T) {
	t.Parallel()

	var tests = map[string]struct {
		Args      *k8s.ExposedMonopodArgs
		ExpectErr bool
	}{
		"nil-args": {
			Args:      nil,
			ExpectErr: true,
		},
		"empty-args": {
			Args:      &k8s.ExposedMonopodArgs{},
			ExpectErr: true,
		},
		"basic": {
			Args: &k8s.ExposedMonopodArgs{
				Identity: pulumi.String("a0b1c2d3"),
				Hostname: pulumi.String("ctfer.io"),
				Image:    pulumi.String("pandatix/licence-lvl1:latest"),
				Port:     pulumi.Int(8080),
			},
		},
		"labeled": {
			Args: &k8s.ExposedMonopodArgs{
				Identity: pulumi.String("a0b1c2d3"),
				Label:    pulumi.String("something"),
				Hostname: pulumi.String("ctfer.io"),
				Image:    pulumi.String("pandatix/licence-lvl1:latest"),
				Port:     pulumi.Int(8080),
			},
		},
		"ingress": {
			Args: &k8s.ExposedMonopodArgs{
				Identity:   pulumi.String("a0b1c2d3"),
				Hostname:   pulumi.String("ctfer.io"),
				Image:      pulumi.String("pandatix/licence-lvl1:latest"),
				Port:       pulumi.Int(8080),
				ExposeType: k8s.ExposeIngress,
				// Following are examples based on Nginx IngressController
				IngressAnnotations: pulumi.StringMap{
					"kubernetes.io/ingress.class":                  pulumi.String("nginx"),
					"nginx.ingress.kubernetes.io/backend-protocol": pulumi.String("HTTP"),
					"nginx.ingress.kubernetes.io/ssl-redirect":     pulumi.String("true"),
					"nginx.ingress.kubernetes.io/proxy-body-size":  pulumi.String("50m"),
				},
				IngressNamespace: pulumi.String("ingress-nginx"),
				IngressLabels: pulumi.ToStringMap(map[string]string{
					"app.kubernetes.io/component": "controller",
					"app.kubernetes.io/instance":  "ingress-nginx",
				}),
			},
		},
		"with-limits": {
			Args: &k8s.ExposedMonopodArgs{
				Identity:    pulumi.String("a0b1c2d3"),
				Hostname:    pulumi.String("ctfer.io"),
				Image:       pulumi.String("pandatix/licence-lvl1:latest"),
				Port:        pulumi.Int(8080),
				LimitCPU:    pulumi.String("128Mi"),
				LimitMemory: pulumi.String("500m"),
			},
		},
		"from-cidr": {
			Args: &k8s.ExposedMonopodArgs{
				Identity: pulumi.String("a0b1c2d3"),
				Hostname: pulumi.String("ctfer.io"),
				Image:    pulumi.String("pandatix/licence-lvl1:latest"),
				Port:     pulumi.Int(8080),
				FromCIDR: pulumi.String("192.168.1.0/24"),
			},
		},
		"with-envs-and-files": {
			Args: &k8s.ExposedMonopodArgs{
				Identity: pulumi.String("a0b1c2d3"),
				Hostname: pulumi.String("ctfer.io"),
				Image:    pulumi.String("pandatix/licence-lvl1:latest"),
				Port:     pulumi.Int(8080),
				Envs: pulumi.StringMap{
					"FLAG": pulumi.String("BREFCTF{some-flag}"),
				},
				Files: pulumi.StringMap{
					"/etc/shadow": pulumi.String("root:!:20009:0:99999:7:::\n"),
				},
			},
		},
	}

	for testname, tt := range tests {
		t.Run(testname, func(t *testing.T) {
			assert := assert.New(t)
			require := require.New(t)

			err := pulumi.RunErr(func(ctx *pulumi.Context) error {
				emp, err := k8s.NewExposedMonopod(ctx, "emp-test", tt.Args)
				if tt.ExpectErr {
					require.Error(err)
				} else {
					require.NoError(err)

					emp.URL.ApplyT(func(edp string) error {
						_, err := url.Parse(edp)
						assert.NoError(err)
						return nil
					})
				}

				return nil
			}, pulumi.WithMocks("project", "stack", mocks{}))
			assert.NoError(err)
		})
	}
}
