package kubernetes_test

import (
	"net/url"
	"sync"
	"testing"

	k8s "github.com/ctfer-io/chall-manager/sdk/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_U_ExposedMultipod(t *testing.T) {
	t.Parallel()

	var tests = map[string]struct {
		Args      *k8s.ExposedMultipodArgs
		ExpectErr bool
	}{
		"nil-args": {
			Args:      nil,
			ExpectErr: true,
		},
		"empty-args": {
			Args:      &k8s.ExposedMultipodArgs{},
			ExpectErr: true,
		},
		"vip-only": {
			Args: &k8s.ExposedMultipodArgs{
				Identity: pulumi.String("a0b1c2d3"),
				Hostname: pulumi.String("ctfer.io"),
				Containers: k8s.ContainerMap{
					"mongo": k8s.ContainerArgs{
						Image: pulumi.String("web/vip-only-mongo:v0.1.0"),
						Ports: k8s.PortBindingArray{
							k8s.PortBindingArgs{
								Port: pulumi.Int(27017),
							},
						},
					},
					"node": k8s.ContainerArgs{
						Image: pulumi.String("web/vip-only-node:v0.1.0"),
						Ports: k8s.PortBindingArray{
							k8s.PortBindingArgs{
								Port:       pulumi.Int(3000),
								ExposeType: k8s.ExposeNodePort,
							},
						},
						Files: pulumi.StringMap{
							"/app/flag.txt": pulumi.String("24HIUT{To0_W3ak_c#yp7o}"),
						},
						Envs: k8s.PrinterMap{
							"MONGODB_URI": k8s.NewPrinter(
								"mongodb://root:qlaod3a5sdha6s8d6@%s:27017/vipOnlyApp?authSource=admin",
								"mongo",
							),
						},
					},
				},
				Rules: k8s.RuleArray{
					k8s.RuleArgs{
						From: pulumi.String("node"),
						To:   pulumi.String("mongo"),
						On:   pulumi.Int(27017),
					},
				},
			},
		},
		"cycles": {
			Args: &k8s.ExposedMultipodArgs{
				Identity: pulumi.String("a0b1c2d3"),
				Hostname: pulumi.String("ctfer.io"),
				Containers: k8s.ContainerMap{
					"a": k8s.ContainerArgs{
						Image: pulumi.String("a"),
						Ports: k8s.PortBindingArray{
							k8s.PortBindingArgs{
								Port: pulumi.Int(8080),
							},
						},
						Envs: k8s.PrinterMap{
							"OTHER": k8s.NewPrinter("%s:8080", "b"),
						},
					},
					"b": k8s.ContainerArgs{
						Image: pulumi.String("b"),
						Ports: k8s.PortBindingArray{
							k8s.PortBindingArgs{
								Port: pulumi.Int(8080),
							},
						},
						Envs: k8s.PrinterMap{
							"OTHER": k8s.NewPrinter("%s:8080", "a"),
						},
					},
				},
				Rules: k8s.RuleArray{
					// Don't define any.
					// SCC could arise from container envs' printers cross-requirement.
					// It is not a problem from the networking rule perspective: two
					// containers can discuss together without stucking each other's
					// deployment.
				},
			},
			ExpectErr: true,
		},
		"complex": {
			Args: &k8s.ExposedMultipodArgs{
				Identity: pulumi.String("a0b1c2d3"),
				Hostname: pulumi.String("ctfer.io"),
				Containers: k8s.ContainerMap{
					"a": k8s.ContainerArgs{
						Image: pulumi.String("a"),
						Ports: k8s.PortBindingArray{
							k8s.PortBindingArgs{
								Port:       pulumi.Int(8080),
								ExposeType: k8s.ExposeIngress,
							},
						},
					},
					"b": k8s.ContainerArgs{
						Image: pulumi.String("b"),
						Ports: k8s.PortBindingArray{
							k8s.PortBindingArgs{
								Port: pulumi.Int(8080),
							},
							k8s.PortBindingArgs{
								Port: pulumi.Int(8081),
							},
						},
					},
					"c": k8s.ContainerArgs{
						Image: pulumi.String("c"),
						Ports: k8s.PortBindingArray{
							k8s.PortBindingArgs{
								Port:       pulumi.Int(8080),
								ExposeType: k8s.ExposeNodePort,
							},
						},
					},
				},
				Rules: k8s.RuleArray{
					k8s.RuleArgs{
						From: pulumi.String("a"),
						To:   pulumi.String("b"),
						On:   pulumi.Int(8080),
					},
					k8s.RuleArgs{
						From: pulumi.String("a"),
						To:   pulumi.String("b"),
						On:   pulumi.Int(8081),
					},
					k8s.RuleArgs{
						From: pulumi.String("a"),
						To:   pulumi.String("c"),
						On:   pulumi.Int(8080),
					},
				},
			},
			ExpectErr: false,
		},
		"printer": {
			Args: &k8s.ExposedMultipodArgs{
				Identity: pulumi.String("a0b1c2d3"),
				Hostname: pulumi.String("ctfer.io"),
				Containers: k8s.ContainerMap{
					"a": k8s.ContainerArgs{
						Image: pulumi.String("a"),
						Ports: k8s.PortBindingArray{
							k8s.PortBindingArgs{
								Port:       pulumi.Int(8080),
								ExposeType: k8s.ExposeNodePort,
							},
						},
					},
					"b": k8s.ContainerArgs{
						Image: pulumi.String("b"),
						Ports: k8s.PortBindingArray{
							k8s.PortBindingArgs{
								Port: pulumi.Int(8080),
							},
						},
						Envs: k8s.PrinterMap{
							"DIRECT":      k8s.NewPrinter("http://%s", "a"),
							"PORT":        k8s.NewPrinter("http://%s", "a:8080"),
							"PORTBINDING": k8s.NewPrinter("http://%s", "a:8080/TCP"),
						},
					},
				},
				Rules: k8s.RuleArray{
					k8s.RuleArgs{
						From: pulumi.String("b"),
						To:   pulumi.String("a"),
						On:   pulumi.Int(8080),
					},
				},
			},
			ExpectErr: false,
		},
		"printer-unexisting": {
			Args: &k8s.ExposedMultipodArgs{
				Identity: pulumi.String("a0b1c2d3"),
				Hostname: pulumi.String("ctfer.io"),
				Containers: k8s.ContainerMap{
					"a": k8s.ContainerArgs{
						Image: pulumi.String("a"),
						Ports: k8s.PortBindingArray{
							k8s.PortBindingArgs{
								Port:       pulumi.Int(8080),
								ExposeType: k8s.ExposeNodePort,
							},
						},
						Envs: k8s.PrinterMap{
							"OTHER": k8s.NewPrinter("http://%s", "pouet"),
						},
					},
					"b": k8s.ContainerArgs{
						Image: pulumi.String("b"),
						Ports: k8s.PortBindingArray{
							k8s.PortBindingArgs{
								Port: pulumi.Int(8080),
							},
						},
						Envs: k8s.PrinterMap{
							"DIRECT": k8s.NewPrinter("http://%s", "a"),
						},
					},
				},
				Rules: k8s.RuleArray{
					k8s.RuleArgs{
						From: pulumi.String("b"),
						To:   pulumi.String("a"),
						On:   pulumi.Int(8080),
					},
				},
			},
			ExpectErr: true,
		},
	}

	for testname, tt := range tests {
		t.Run(testname, func(t *testing.T) {
			assert := assert.New(t)
			require := require.New(t)

			err := pulumi.RunErr(func(ctx *pulumi.Context) error {
				emp, err := k8s.NewExposedMultipod(ctx, "emp-test", tt.Args)
				if tt.ExpectErr {
					require.Error(err)
				} else {
					require.NoError(err)

					// Run tests
					wg := sync.WaitGroup{}
					wg.Add(1)
					emp.URLs.ApplyT(func(svcs map[string]map[string]string) error {
						defer wg.Done()

						all := []string{}
						for name, urls := range svcs {
							t.Logf("URLs of %s: %v\n", name, urls)

							for _, edp := range urls {
								all = append(all, edp)

								_, err := url.Parse(edp)
								assert.NoError(err)
							}
						}
						assert.NotEmpty(all)
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
