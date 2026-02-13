package kubernetes_test

import (
	"net/url"
	"sync"
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
				Container: k8s.ContainerArgs{
					Image: pulumi.String("pandatix/license-lvl1:latest"),
					Ports: k8s.PortBindingArray{
						k8s.PortBindingArgs{
							Port:       pulumi.Int(8080),
							ExposeType: k8s.ExposeNodePort,
						},
					},
				},
			},
		},
		"labeled": {
			Args: &k8s.ExposedMonopodArgs{
				Identity: pulumi.String("a0b1c2d3"),
				Label:    pulumi.String("something"),
				Hostname: pulumi.String("ctfer.io"),
				Container: k8s.ContainerArgs{
					Image: pulumi.String("pandatix/license-lvl1:latest"),
					Ports: k8s.PortBindingArray{
						k8s.PortBindingArgs{
							Port:       pulumi.Int(8080),
							ExposeType: k8s.ExposeNodePort,
						},
					},
				},
			},
		},
		"ingress": {
			Args: &k8s.ExposedMonopodArgs{
				Identity: pulumi.String("a0b1c2d3"),
				Hostname: pulumi.String("ctfer.io"),
				Container: k8s.ContainerArgs{
					Image: pulumi.String("pandatix/license-lvl1:latest"),
					Ports: k8s.PortBindingArray{
						k8s.PortBindingArgs{
							Port:       pulumi.Int(8080),
							ExposeType: k8s.ExposeIngress,
							// Based upon default Nginx IngressController settings
							Annotations: pulumi.StringMap{
								"kubernetes.io/ingress.class":                  pulumi.String("nginx"),
								"nginx.ingress.kubernetes.io/backend-protocol": pulumi.String("HTTP"),
								"nginx.ingress.kubernetes.io/ssl-redirect":     pulumi.String("true"),
								"nginx.ingress.kubernetes.io/proxy-body-size":  pulumi.String("50m"),
							},
						},
					},
				},
				// Based upon default Nginx IngressController settings
				IngressNamespace: pulumi.String("ingress-nginx"),
				IngressLabels: pulumi.ToStringMap(map[string]string{
					"app.kubernetes.io/component": "controller",
					"app.kubernetes.io/instance":  "ingress-nginx",
				}),
			},
		},
		"with-limits": {
			Args: &k8s.ExposedMonopodArgs{
				Identity: pulumi.String("a0b1c2d3"),
				Hostname: pulumi.String("ctfer.io"),
				Container: k8s.ContainerArgs{
					Image: pulumi.String("pandatix/license-lvl1:latest"),
					Ports: k8s.PortBindingArray{
						k8s.PortBindingArgs{
							Port:       pulumi.Int(8080),
							ExposeType: k8s.ExposeNodePort,
						},
					},
					Limits: pulumi.StringMap{
						"cpu":    pulumi.String("128Mi"),
						"memory": pulumi.String("500m"),
					},
				},
			},
		},
		"from-cidr": {
			Args: &k8s.ExposedMonopodArgs{
				Identity: pulumi.String("a0b1c2d3"),
				Hostname: pulumi.String("ctfer.io"),
				Container: k8s.ContainerArgs{
					Image: pulumi.String("pandatix/license-lvl1:latest"),
					Ports: k8s.PortBindingArray{
						k8s.PortBindingArgs{
							Port:       pulumi.Int(8080),
							ExposeType: k8s.ExposeNodePort,
						},
					},
				},
				FromCIDR: pulumi.String("192.168.1.0/24"),
			},
		},
		"with-envs-and-files": {
			Args: &k8s.ExposedMonopodArgs{
				Identity: pulumi.String("a0b1c2d3"),
				Hostname: pulumi.String("ctfer.io"),
				Container: k8s.ContainerArgs{
					Image: pulumi.String("pandatix/license-lvl1:latest"),
					Ports: k8s.PortBindingArray{
						k8s.PortBindingArgs{
							Port:       pulumi.Int(8080),
							ExposeType: k8s.ExposeNodePort,
						},
					},
					Envs: k8s.PrinterMap{
						"FLAG": k8s.ToPrinter(pulumi.String("BREFCTF{some-flag}")),
					},
					Files: pulumi.StringMap{
						"/etc/shadow": pulumi.String("root:!:20009:0:99999:7:::\n"),
					},
				},
			},
		},
		"no-ports": {
			Args: &k8s.ExposedMonopodArgs{
				Identity: pulumi.String("a0b1c2d3"),
				Hostname: pulumi.String("ctfer.io"),
				Container: k8s.ContainerArgs{
					Image: pulumi.String("pandatix/license-lvl1:latest"),
					Ports: nil,
				},
			},
			ExpectErr: true,
		},
		"many-ports": {
			Args: &k8s.ExposedMonopodArgs{
				Identity: pulumi.String("a0b1c2d3"),
				Hostname: pulumi.String("ctfer.io"),
				Container: k8s.ContainerArgs{
					Image: pulumi.String("pandatix/license-lvl1:latest"),
					Ports: k8s.PortBindingArray{
						k8s.PortBindingArgs{
							Port:       pulumi.Int(8080),
							ExposeType: k8s.ExposeNodePort,
						},
						k8s.PortBindingArgs{
							Port:       pulumi.Int(8081),
							ExposeType: k8s.ExposeNodePort,
						},
					},
				},
			},
			ExpectErr: false,
		},
		"shared-port": {
			Args: &k8s.ExposedMonopodArgs{
				Identity: pulumi.String("a0b1c2d3"),
				Hostname: pulumi.String("ctfer.io"),
				Container: k8s.ContainerArgs{
					Image: pulumi.String("pandatix/license-lvl1:latest"),
					Ports: k8s.PortBindingArray{
						k8s.PortBindingArgs{
							Port:       pulumi.Int(8080),
							Protocol:   pulumi.String("TCP"),
							ExposeType: k8s.ExposeNodePort,
						},
						k8s.PortBindingArgs{
							Port:       pulumi.Int(8080),
							Protocol:   pulumi.String("UDP"),
							ExposeType: k8s.ExposeNodePort,
						},
					},
				},
			},
			ExpectErr: false,
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

					// Run tests
					wg := sync.WaitGroup{}
					wg.Add(1)
					emp.URLs.ApplyT(func(urls map[string]string) error {
						defer wg.Done()

						assert.NotEmpty(urls)
						t.Logf("URLs: %v\n", urls)
						for _, edp := range urls {
							_, err := url.Parse(edp)
							assert.NoError(err)
						}
						return nil
					})
					wg.Wait()
				}

				return nil
			}, pulumi.WithMocks("project", "stack", mocks{}))
			assert.NoError(err)
		})
	}
}
