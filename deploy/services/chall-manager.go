package services

import (
	"bytes"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/pkg/errors"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	netwv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/networking/v1"
	yamlv2 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/yaml/v2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"go.uber.org/multierr"

	"github.com/ctfer-io/chall-manager/deploy/common"
	"github.com/ctfer-io/chall-manager/deploy/services/parts"
)

type (
	// ChallManager Micro Service deployed on a Kubernetes infrastructure.
	ChallManager struct {
		pulumi.ResourceState

		// Deployment
		ns *parts.Namespace

		// Parts
		etcd *parts.EtcdCluster
		cm   *parts.ChallManager
		cmj  *parts.ChallManagerJanitor

		// Exposure
		svc       *corev1.Service
		expnetpol *netwv1.NetworkPolicy

		// Interface & ports network policies
		cmToEtcd  *netwv1.NetworkPolicy
		cmjToCm   *netwv1.NetworkPolicy
		cmFromCmj *netwv1.NetworkPolicy
		cmToApi   *yamlv2.ConfigGroup
		allToOtel *netwv1.NetworkPolicy

		// Outputs

		Endpoint    pulumi.StringOutput
		ExposedPort pulumi.IntPtrOutput
		PodLabels   pulumi.StringMapOutput
	}

	// ChallManagerArgs contains all the parametrization of a Chall-Manager
	// MicroService deployment on Kubernetes.
	ChallManagerArgs struct {
		Tag pulumi.StringPtrInput
		tag pulumi.StringOutput

		// Registry define from where to fetch the Chall-Manager Docker images.
		// If set empty, defaults to Docker Hub.
		Registry pulumi.StringPtrInput
		registry pulumi.StringOutput

		// LogLevel defines the level at which to log.
		LogLevel pulumi.StringInput

		Namespace       pulumi.StringInput
		createNamespace bool

		EtcdReplicas pulumi.IntPtrInput
		Replicas     pulumi.IntPtrInput
		replicas     pulumi.IntOutput

		JanitorCron   pulumi.StringInput
		JanitorTicker pulumi.StringInput
		JanitorMode   parts.JanitorMode

		// PVCAccessModes defines the access modes supported by the PVC.
		PVCAccessModes pulumi.StringArrayInput
		pvcAccessModes pulumi.StringArrayOutput

		// PVCStorageSize enable to configure the storage size of the PVC Chall-Manager
		// will write into (store Pulumi stacks, data persistency, ...).
		// Default to 2Gi.
		// See https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/#meaning-of-memory
		// for syntax.
		PVCStorageSize pulumi.StringInput

		// RomeoClaimName, if set, will turn on the coverage export of Chall-Manager for later download.
		RomeoClaimName pulumi.StringInput

		// Kubeconfig is an optional attribute that override the ServiceAccount created
		// by default for Chall-Manager.
		// It can enable giving it access to another cluster, for instance when isolation is required.
		Kubeconfig pulumi.StringInput

		// ServiceAccount is an optional attribute that override the ServiceAccount created
		// by default for Chall-Manager.
		// It can enable giving different permissions, for instance when needing cluster-wide permissions
		// or support for CRDs.
		//
		// Will logically only work when the Namespace is provided too, as this ServiceAccount MUST
		// be namespaced in it.
		ServiceAccount pulumi.StringInput
		serviceAccount pulumi.StringOutput

		// Requests for the Chall-Manager container. For more infos:
		// https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
		Requests pulumi.StringMapInput

		// Limits for the Chall-Manager container. For more infos:
		// https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
		Limits pulumi.StringMapInput

		// A key=value map of additional environment variables to mount in Chall-Manager.
		Envs pulumi.StringMapInput

		// CmToApiServerTemplate is a Go text/template that defines the NetworkPolicy
		// YAML schema to use.
		// If none set, it is defaulted to a cilium.io/v2 CiliumNetworkPolicy.
		CmToApiServerTemplate pulumi.StringPtrInput
		cmToApiServerTemplate pulumi.StringOutput

		Swagger, Expose bool

		Otel *common.OtelArgs

		OCIInsecure bool
		OCIUsername pulumi.StringPtrInput
		OCIPassword pulumi.StringPtrInput
	}
)

const (
	defaultTag      = "latest"
	defaultReplicas = 1

	defaultCmToApiServerTemplate = `
apiVersion: cilium.io/v2
kind: CiliumNetworkPolicy
metadata:
  name: cilium-seed-apiserver-allow-{{ .Stack }}
  namespace: {{ .Namespace }}
spec:
  endpointSelector:
    matchLabels:
    {{- range $k, $v := .PodLabels }}
      {{ $k }}: {{ $v }}
    {{- end }}
  egress:
  - toEntities:
    - kube-apiserver
  - toPorts:
    - ports:
      - port: "6443"
        protocol: TCP
`
)

var defaultPVCAccessModes = []string{
	"ReadWriteMany",
}

// NewChallManager deploys the Chall-Manager service as it is intended to be deployed
// in a production environment, in a Kubernetes cluster.
//
// It is not made to be exposed to outer world (outside of the cluster).
func NewChallManager(ctx *pulumi.Context, name string, args *ChallManagerArgs, opts ...pulumi.ResourceOption) (*ChallManager, error) {
	cm := &ChallManager{}

	args = cm.defaults(args)
	if err := cm.check(args); err != nil {
		return nil, err
	}
	if err := ctx.RegisterComponentResource("ctfer-io:chall-manager", name, cm, opts...); err != nil {
		return nil, err
	}
	opts = append(opts, pulumi.Parent(cm))
	if err := cm.provision(ctx, args, opts...); err != nil {
		return nil, err
	}
	if err := cm.outputs(ctx); err != nil {
		return nil, err
	}
	return cm, nil
}

func (cm *ChallManager) defaults(args *ChallManagerArgs) *ChallManagerArgs {
	if args == nil {
		args = &ChallManagerArgs{}
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

	args.registry = pulumi.String("").ToStringOutput()
	if args.Registry != nil {
		args.registry = args.Registry.ToStringPtrOutput().ApplyT(func(in *string) string {
			// No private registry -> defaults to Docker Hub
			if in == nil {
				return ""
			}

			str := *in
			// If one set, make sure it ends with one '/'
			if str != "" && !strings.HasSuffix(str, "/") {
				str = str + "/"
			}
			return str
		}).(pulumi.StringOutput)
	}

	args.createNamespace = args.Namespace == nil
	if args.Namespace != nil {
		wg := &sync.WaitGroup{}
		wg.Add(1)
		args.Namespace.ToStringOutput().ApplyT(func(ns string) error {
			args.createNamespace = ns == ""
			wg.Done()
			return nil
		})
		wg.Wait()
	}

	args.replicas = pulumi.Int(defaultReplicas).ToIntOutput()
	if args.Replicas != nil {
		args.replicas = args.Replicas.ToIntPtrOutput().ApplyT(func(replicas *int) int {
			if replicas == nil {
				return defaultReplicas
			}
			return *replicas
		}).(pulumi.IntOutput)
	}

	args.pvcAccessModes = pulumi.ToStringArray(defaultPVCAccessModes).ToStringArrayOutput()
	if args.PVCAccessModes != nil {
		args.pvcAccessModes = args.PVCAccessModes.ToStringArrayOutput().ApplyT(func(am []string) []string {
			if len(am) == 0 {
				return defaultPVCAccessModes
			}
			return am
		}).(pulumi.StringArrayOutput)
	}

	args.cmToApiServerTemplate = pulumi.String(defaultCmToApiServerTemplate).ToStringOutput()
	if args.CmToApiServerTemplate != nil {
		args.cmToApiServerTemplate = args.CmToApiServerTemplate.ToStringPtrOutput().ApplyT(func(cmToApiServerTemplate *string) string {
			if cmToApiServerTemplate == nil || *cmToApiServerTemplate == "" {
				return defaultCmToApiServerTemplate
			}
			return *cmToApiServerTemplate
		}).(pulumi.StringOutput)
	}

	args.serviceAccount = pulumi.String("").ToStringOutput()
	if args.ServiceAccount != nil {
		args.serviceAccount = args.ServiceAccount.ToStringOutput()
	}

	return args
}

func (cm *ChallManager) check(args *ChallManagerArgs) error {
	wg := &sync.WaitGroup{}
	checks := 3 // number of checks to perform
	wg.Add(checks)
	cerr := make(chan error, checks)

	// Verify replicas configuration
	pulumi.All(args.replicas, args.EtcdReplicas).ApplyT(func(all []any) error {
		defer wg.Done()

		replicas := all[0].(int)
		var etcdReplicas *int
		if r, ok := all[1].(*int); ok {
			etcdReplicas = r
		}
		if r, ok := all[1].(int); ok {
			etcdReplicas = &r
		}

		if replicas > 1 && (etcdReplicas == nil || *etcdReplicas < 1) {
			cerr <- errors.New("cannot deploy chall-manager replicas (High-Availability) without a distributed lock system (etcd)")
		}
		return nil
	})

	// Verify the template is syntactically valid
	args.cmToApiServerTemplate.ApplyT(func(cmToApiServerTemplate string) error {
		defer wg.Done()

		_, err := template.New("cm-to-apiserver").
			Funcs(sprig.FuncMap()).
			Parse(cmToApiServerTemplate)
		cerr <- err
		return nil
	})

	// Verify that ServiceAccount => Namespace
	args.serviceAccount.ToStringOutput().ApplyT(func(sa string) error {
		defer wg.Done()

		if sa != "" && !args.createNamespace {
			cerr <- errors.New("service account is provided but no namespace is provided")
		}
		return nil
	})

	wg.Wait()
	close(cerr)

	var merr error
	for err := range cerr {
		merr = multierr.Append(merr, err)
	}
	return merr
}

func (cm *ChallManager) provision(ctx *pulumi.Context, args *ChallManagerArgs, opts ...pulumi.ResourceOption) (err error) {
	// Create namespace if required
	namespace := args.Namespace
	if args.createNamespace {
		cm.ns, err = parts.NewNamespace(ctx, "cm-ns", &parts.NamespaceArgs{
			Name: pulumi.String("cm-ns"),
			AdditionalLabels: pulumi.StringMap{
				"app.kubernetes.io/component": pulumi.String("chall-manager"),
				"app.kubernetes.io/part-of":   pulumi.String("chall-manager"),
				"ctfer.io/stack-name":         pulumi.String(ctx.Stack()),
			},
		}, opts...)
		if err != nil {
			return err
		}
		namespace = cm.ns.Name
	}

	// Deploy etcd as the distributed lock/counter solution
	if args.EtcdReplicas != nil {
		var etcdOtel *common.OtelArgs
		if args.Otel != nil {
			etcdOtel = &common.OtelArgs{
				ServiceName: pulumi.Sprintf("%s-etcd", args.Otel.ServiceName),
				Endpoint:    args.Otel.Endpoint,
				Insecure:    args.Otel.Insecure,
			}
		}

		cm.etcd, err = parts.NewEtcdCluster(ctx, "lock", &parts.EtcdArgs{
			Namespace: namespace,
			Registry:  args.Registry,
			Replicas: args.EtcdReplicas.ToIntPtrOutput().Elem().ApplyT(func(replicas int) int {
				if replicas > 0 {
					return replicas
				}
				return 1 // default replicas to 1
			}).(pulumi.IntOutput),
			Otel: etcdOtel,
		}, opts...)
		if err != nil {
			return
		}
	}

	// Deploy the core service
	cmArgs := &parts.ChallManagerArgs{
		Tag:       args.tag,
		Registry:  args.registry,
		Namespace: namespace,
		Replicas: args.replicas.ApplyT(func(replicas int) int {
			if replicas > 0 {
				return replicas
			}
			return 1 // default replicas to 1
		}).(pulumi.IntOutput),
		LogLevel:       args.LogLevel,
		Etcd:           nil,
		Swagger:        args.Swagger,
		PVCAccessModes: args.pvcAccessModes,
		PVCStorageSize: args.PVCStorageSize,
		Otel:           nil,
		RomeoClaimName: args.RomeoClaimName,
		Kubeconfig:     args.Kubeconfig,
		Requests:       args.Requests,
		Limits:         args.Limits,
		Envs:           args.Envs,
		OCIInsecure:    args.OCIInsecure,
		OCIUsername:    args.OCIUsername,
		OCIPassword:    args.OCIPassword,
	}
	if args.EtcdReplicas != nil {
		cmArgs.Etcd = &parts.ChallManagerEtcdArgs{
			Endpoint: cm.etcd.Endpoint,
			Username: cm.etcd.Username,
			Password: cm.etcd.Password,
		}
	}
	if args.Otel != nil {
		cmArgs.Otel = &common.OtelArgs{
			ServiceName: pulumi.Sprintf("%s-chall-manager", args.Otel.ServiceName),
			Endpoint:    args.Otel.Endpoint,
			Insecure:    args.Otel.Insecure,
		}
	}
	cm.cm, err = parts.NewChallManager(ctx, "chall-manager", cmArgs, opts...)
	if err != nil {
		return
	}

	if args.Expose {
		cm.svc, err = corev1.NewService(ctx, "cm-exposed", &corev1.ServiceArgs{
			Metadata: metav1.ObjectMetaArgs{
				Labels:    cm.cm.PodLabels,
				Namespace: namespace,
			},
			Spec: corev1.ServiceSpecArgs{
				Type:     pulumi.String("NodePort"),
				Selector: cm.cm.PodLabels,
				Ports: corev1.ServicePortArray{
					corev1.ServicePortArgs{
						Port: cm.cm.Endpoint.ApplyT(func(edp string) int {
							// On bootstrap there is no valid URL, but port is assigned
							pts := strings.Split(edp, ":")
							p := pts[len(pts)-1]
							port, _ := strconv.Atoi(p)
							return port
						}).(pulumi.IntOutput),
					},
				},
			},
		}, opts...)
		if err != nil {
			return
		}

		// Grant traffic from outside world to Chall-Manager's API
		cm.expnetpol, err = netwv1.NewNetworkPolicy(ctx, "exposed-netpol", &netwv1.NetworkPolicyArgs{
			Metadata: metav1.ObjectMetaArgs{
				Namespace: namespace,
				Labels: pulumi.StringMap{
					"app.kubernetes.io/component": pulumi.String("chall-manager"),
					"app.kubernetes.io/part-of":   pulumi.String("chall-manager"),
					"ctfer.io/stack-name":         pulumi.String(ctx.Stack()),
				},
			},
			Spec: netwv1.NetworkPolicySpecArgs{
				PodSelector: metav1.LabelSelectorArgs{
					MatchLabels: cm.cm.PodLabels,
				},
				PolicyTypes: pulumi.ToStringArray([]string{
					"Ingress",
				}),
				Ingress: netwv1.NetworkPolicyIngressRuleArray{
					netwv1.NetworkPolicyIngressRuleArgs{
						From: netwv1.NetworkPolicyPeerArray{
							netwv1.NetworkPolicyPeerArgs{
								IpBlock: &netwv1.IPBlockArgs{
									Cidr: pulumi.String("0.0.0.0/0"),
								},
							},
						},
						Ports: netwv1.NetworkPolicyPortArray{
							netwv1.NetworkPolicyPortArgs{
								Port:     cm.svc.Spec.Ports().Index(pulumi.Int(0)).Port(),
								Protocol: pulumi.String("TCP"),
							},
						},
					},
				},
			},
		}, opts...)
		if err != nil {
			return err
		}
	}

	// Deploy janitor
	var cmjOtel *common.OtelArgs
	if args.Otel != nil {
		cmjOtel = &common.OtelArgs{
			ServiceName: pulumi.Sprintf("%s-chall-manager-janitor", args.Otel.ServiceName),
			Endpoint:    args.Otel.Endpoint,
			Insecure:    args.Otel.Insecure,
		}
	}
	cm.cmj, err = parts.NewChallManagerJanitor(ctx, "janitor", &parts.ChallManagerJanitorArgs{
		Tag:                  args.tag,
		Registry:             args.registry,
		LogLevel:             args.LogLevel,
		Namespace:            namespace,
		ChallManagerEndpoint: cm.cm.Endpoint,
		Cron:                 args.JanitorCron,
		Ticker:               args.JanitorTicker,
		Mode:                 args.JanitorMode,
		RomeoClaimName:       args.RomeoClaimName,
		Otel:                 cmjOtel,
	}, opts...)
	if err != nil {
		return
	}

	// => NetworkPolicy from chall-manager to etcd
	if args.EtcdReplicas != nil {
		cm.cmToEtcd, err = netwv1.NewNetworkPolicy(ctx, "cm-to-etcd", &netwv1.NetworkPolicyArgs{
			Metadata: metav1.ObjectMetaArgs{
				Namespace: namespace,
				Labels: pulumi.StringMap{
					"app.kubernetes.io/component": pulumi.String("chall-manager"),
					"app.kubernetes.io/part-of":   pulumi.String("chall-manager"),
					"ctfer.io/stack-name":         pulumi.String(ctx.Stack()),
				},
			},
			Spec: netwv1.NetworkPolicySpecArgs{
				PolicyTypes: pulumi.ToStringArray([]string{
					"Egress",
				}),
				PodSelector: metav1.LabelSelectorArgs{
					MatchLabels: cm.cm.PodLabels,
				},
				Egress: netwv1.NetworkPolicyEgressRuleArray{
					netwv1.NetworkPolicyEgressRuleArgs{
						To: netwv1.NetworkPolicyPeerArray{
							netwv1.NetworkPolicyPeerArgs{
								NamespaceSelector: metav1.LabelSelectorArgs{
									MatchLabels: pulumi.StringMap{
										"kubernetes.io/metadata.name": namespace,
									},
								},
								PodSelector: metav1.LabelSelectorArgs{
									MatchLabels: cm.etcd.PodLabels,
								},
							},
						},
						Ports: netwv1.NetworkPolicyPortArray{
							netwv1.NetworkPolicyPortArgs{
								Port:     parseEndpoint(cm.etcd.Endpoint),
								Protocol: pulumi.String("TCP"),
							},
						},
					},
				},
			},
		}, opts...)
		if err != nil {
			return
		}
	}

	// => NetworkPolicy from chall-manager-janitor to chall-manager
	cm.cmjToCm, err = netwv1.NewNetworkPolicy(ctx, "cmj-to-cm", &netwv1.NetworkPolicyArgs{
		Metadata: metav1.ObjectMetaArgs{
			Namespace: namespace,
			Labels: pulumi.StringMap{
				"app.kubernetes.io/component": pulumi.String("chall-manager"),
				"app.kubernetes.io/part-of":   pulumi.String("chall-manager"),
				"ctfer.io/stack-name":         pulumi.String(ctx.Stack()),
			},
		},
		Spec: netwv1.NetworkPolicySpecArgs{
			PolicyTypes: pulumi.ToStringArray([]string{
				"Egress",
			}),
			PodSelector: metav1.LabelSelectorArgs{
				MatchLabels: cm.cmj.PodLabels,
			},
			Egress: netwv1.NetworkPolicyEgressRuleArray{
				netwv1.NetworkPolicyEgressRuleArgs{
					To: netwv1.NetworkPolicyPeerArray{
						netwv1.NetworkPolicyPeerArgs{
							NamespaceSelector: metav1.LabelSelectorArgs{
								MatchLabels: pulumi.StringMap{
									"kubernetes.io/metadata.name": namespace,
								},
							},
							PodSelector: metav1.LabelSelectorArgs{
								MatchLabels: cm.cm.PodLabels,
							},
						},
					},
					Ports: netwv1.NetworkPolicyPortArray{
						netwv1.NetworkPolicyPortArgs{
							Port:     parseEndpoint(cm.cm.Endpoint),
							Protocol: pulumi.String("TCP"),
						},
					},
				},
			},
		},
	}, opts...)
	if err != nil {
		return
	}

	// => From chall-manager to chall-manager-janitor
	cm.cmFromCmj, err = netwv1.NewNetworkPolicy(ctx, "cm-from-cmj", &netwv1.NetworkPolicyArgs{
		Metadata: metav1.ObjectMetaArgs{
			Namespace: namespace,
			Labels: pulumi.StringMap{
				"app.kubernetes.io/component": pulumi.String("chall-manager"),
				"app.kubernetes.io/part-of":   pulumi.String("chall-manager"),
				"ctfer.io/stack-name":         pulumi.String(ctx.Stack()),
			},
		},
		Spec: netwv1.NetworkPolicySpecArgs{
			PolicyTypes: pulumi.ToStringArray([]string{
				"Ingress",
			}),
			PodSelector: metav1.LabelSelectorArgs{
				MatchLabels: cm.cm.PodLabels,
			},
			Ingress: netwv1.NetworkPolicyIngressRuleArray{
				netwv1.NetworkPolicyIngressRuleArgs{
					From: netwv1.NetworkPolicyPeerArray{
						netwv1.NetworkPolicyPeerArgs{
							NamespaceSelector: metav1.LabelSelectorArgs{
								MatchLabels: pulumi.StringMap{
									"kubernetes.io/metadata.name": namespace,
								},
							},
							PodSelector: metav1.LabelSelectorArgs{
								MatchLabels: cm.cmj.PodLabels,
							},
						},
					},
					Ports: netwv1.NetworkPolicyPortArray{
						netwv1.NetworkPolicyPortArgs{
							Port:     parseEndpoint(cm.cm.Endpoint),
							Protocol: pulumi.String("TCP"),
						},
					},
				},
			},
		},
	}, opts...)
	if err != nil {
		return
	}

	// => NetworkPolicy from chall-manager to kube-apiserver through endpoint in
	// default namespace.
	cm.cmToApi, err = yamlv2.NewConfigGroup(ctx, "kube-apiserver-netpol", &yamlv2.ConfigGroupArgs{
		Yaml: pulumi.All(args.cmToApiServerTemplate, namespace, cm.cm.PodLabels).ApplyT(func(all []any) (string, error) {
			cmToApiServerTemplate := all[0].(string)
			namespace := all[1].(string)
			podLabels := all[2].(map[string]string)

			tmpl, _ := template.New("cm-to-apiserver").
				Funcs(sprig.FuncMap()).
				Parse(cmToApiServerTemplate)

			buf := &bytes.Buffer{}
			if err := tmpl.Execute(buf, map[string]any{
				"Stack":     ctx.Stack(),
				"Namespace": namespace,
				"PodLabels": podLabels,
			}); err != nil {
				return "", err
			}
			return buf.String(), nil
		}).(pulumi.StringOutput),
	}, opts...)
	if err != nil {
		return
	}

	if args.Otel != nil {
		cm.allToOtel, err = netwv1.NewNetworkPolicy(ctx, "all-to-otel", &netwv1.NetworkPolicyArgs{
			Metadata: metav1.ObjectMetaArgs{
				Namespace: namespace,
				Labels: pulumi.StringMap{
					"app.kubernetes.io/component": pulumi.String("chall-manager"),
					"app.kubernetes.io/part-of":   pulumi.String("chall-manager"),
					"ctfer.io/stack-name":         pulumi.String(ctx.Stack()),
				},
			},
			Spec: netwv1.NetworkPolicySpecArgs{
				PolicyTypes: pulumi.ToStringArray([]string{
					"Egress",
				}),
				PodSelector: metav1.LabelSelectorArgs{
					MatchLabels: pulumi.StringMap{
						// Following labels are common to all Pods of this deployment
						"app.kubernetes.io/component": pulumi.String("chall-manager"),
						"app.kubernetes.io/part-of":   pulumi.String("chall-manager"),
						"ctfer.io/stack-name":         pulumi.String(ctx.Stack()),
					},
				},
				Egress: netwv1.NetworkPolicyEgressRuleArray{
					netwv1.NetworkPolicyEgressRuleArgs{
						Ports: netwv1.NetworkPolicyPortArray{
							netwv1.NetworkPolicyPortArgs{
								Port:     parseEndpoint(args.Otel.Endpoint),
								Protocol: pulumi.String("TCP"),
							},
						},
					},
				},
			},
		}, opts...)
		if err != nil {
			return
		}
	}

	return
}

func (cm *ChallManager) outputs(ctx *pulumi.Context) error {
	cm.Endpoint = cm.cm.Endpoint
	if cm.svc != nil {
		cm.ExposedPort = cm.svc.Spec.ApplyT(func(spec corev1.ServiceSpec) *int {
			return spec.Ports[0].NodePort
		}).(pulumi.IntPtrOutput)
	}
	cm.PodLabels = cm.cm.PodLabels

	return ctx.RegisterResourceOutputs(cm, pulumi.Map{
		"endpoint":     cm.Endpoint,
		"exposed_port": cm.ExposedPort,
		"podLabels":    cm.PodLabels,
	})
}

// parseEndpoint cuts the input endpoint to return its port.
// Examples:
//   - some.thing:port -> port
//   - dns://some.thing:port -> port
func parseEndpoint(edp pulumi.StringInput) pulumi.IntOutput {
	return edp.ToStringOutput().ApplyT(func(edp string) (int, error) {
		// If it is a URL-formatted endpoint, parse it
		if u, err := url.Parse(edp); err == nil && u.Port() != "" {
			return parsePort(edp, u.Port())
		}

		// Else it should be a cuttable endpoint
		_, pStr, _ := strings.Cut(edp, ":")
		return parsePort(edp, pStr)
	}).(pulumi.IntOutput)
}

func parsePort(edp, port string) (int, error) {
	p, err := strconv.Atoi(port)
	if err != nil {
		return 0, errors.Wrapf(err, "parsing endpoint %s for port", edp)
	}
	return p, nil
}
