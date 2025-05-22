package services

import (
	"errors"
	"strconv"
	"strings"
	"sync"

	netwv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/networking/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"go.uber.org/multierr"

	"github.com/ctfer-io/chall-manager/deploy/common"
	"github.com/ctfer-io/chall-manager/deploy/services/parts"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	v1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
)

type (
	// ChallManager Micro Service deployed on a Kubernetes infrastructure.
	ChallManager struct {
		pulumi.ResourceState

		// Parts
		etcd *parts.EtcdCluster
		cm   *parts.ChallManager
		cmj  *parts.ChallManagerJanitor

		// Exposure
		svc *corev1.Service

		// Interface & ports network policies
		cmToEtcd *netwv1.NetworkPolicy
		cmjToCm  *netwv1.NetworkPolicy

		// Outputs

		Endpoint    pulumi.StringOutput
		ExposedPort pulumi.IntPtrOutput
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

		Namespace    pulumi.StringInput
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

		// Kubeconfig is an optional attribute that override the ServiceAccount
		// created by default for Chall-Manager.
		Kubeconfig pulumi.StringInput

		Swagger, Expose bool

		Otel *common.OtelArgs
	}
)

const (
	defaultTag      = "dev"
	defaultReplicas = 1
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

	return args
}

func (cm *ChallManager) check(args *ChallManagerArgs) error {
	wg := &sync.WaitGroup{}
	checks := 1 // number of checks to perform
	wg.Add(checks)
	cerr := make(chan error, checks)

	pulumi.All(args.replicas, args.EtcdReplicas).ApplyT(func(all []any) error {
		defer wg.Done()

		replicas := all[0].(int)
		var etcdReplicas *int
		if r, ok := all[1].(*int); ok {
			etcdReplicas = r
		}
		if r, ok := all[1].(int); ok {
			etcdReplicas = ptr(r)
		}

		if replicas > 1 && (etcdReplicas == nil || *etcdReplicas < 1) {
			cerr <- errors.New("cannot deploy chall-manager replicas (High-Availability) without a distributed lock system (etcd)")
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
			Namespace: args.Namespace,
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
		Namespace: args.Namespace,
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
			Metadata: v1.ObjectMetaArgs{
				Labels:    cm.cm.PodLabels,
				Namespace: args.Namespace,
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
		Namespace:            args.Namespace,
		ChallManagerEndpoint: cm.cm.Endpoint,
		Cron:                 args.JanitorCron,
		Ticker:               args.JanitorTicker,
		Mode:                 args.JanitorMode,
		Otel:                 cmjOtel,
	}, opts...)
	if err != nil {
		return
	}

	// => NetworkPolicy from chall-manager to etcd
	if args.EtcdReplicas != nil {
		// cm.cmToEtcd, err = netwv1.NewNetworkPolicy(ctx, "cm-to-etcd", &netwv1.NetworkPolicyArgs{
		// 	Metadata: metav1.ObjectMetaArgs{
		// 		Namespace: args.Namespace,
		// 		Labels: pulumi.StringMap{
		// 			"app.kubernetes.io/components": pulumi.String("chall-manager"),
		// 			"app.kubernetes.io/part-of":    pulumi.String("chall-manager"),
		// 		},
		// 	},
		// 	Spec: netwv1.NetworkPolicySpecArgs{
		// 		PolicyTypes: pulumi.ToStringArray([]string{
		// 			"Egress",
		// 		}),
		// 		PodSelector: metav1.LabelSelectorArgs{
		// 			MatchLabels: cm.cm.PodLabels,
		// 		},
		// 		Egress: netwv1.NetworkPolicyEgressRuleArray{
		// 			netwv1.NetworkPolicyEgressRuleArgs{
		// 				To: netwv1.NetworkPolicyPeerArray{
		// 					netwv1.NetworkPolicyPeerArgs{
		// 						NamespaceSelector: metav1.LabelSelectorArgs{
		// 							MatchLabels: pulumi.StringMap{
		// 								"kubernetes.io/metadata.name": args.Namespace,
		// 							},
		// 						},
		// 						PodSelector: metav1.LabelSelectorArgs{
		// 							MatchLabels: cm.etcd.PodLabels,
		// 						},
		// 					},
		// 				},
		// 				Ports: netwv1.NetworkPolicyPortArray{
		// 					netwv1.NetworkPolicyPortArgs{
		// 						Port: cm.etcd.Endpoint.ApplyT(func(edp string) int {
		// 							_, port, _ := strings.Cut(edp, ":")
		// 							iport, _ := strconv.Atoi(port)
		// 							return iport
		// 						}).(pulumi.IntOutput),
		// 						Protocol: pulumi.String("TCP"),
		// 					},
		// 				},
		// 			},
		// 		},
		// 	},
		// }, opts...)
		// if err != nil {
		// 	return
		// }
	}

	// // => NetworkPolicy from chall-manager-janitor to chall-manager
	// cm.cmjToCm, err = netwv1.NewNetworkPolicy(ctx, "cmj-to-cm", &netwv1.NetworkPolicyArgs{
	// 	Metadata: metav1.ObjectMetaArgs{
	// 		Namespace: args.Namespace,
	// 		Labels: pulumi.StringMap{
	// 			"app.kubernetes.io/components": pulumi.String("chall-manager"),
	// 			"app.kubernetes.io/part-of":    pulumi.String("chall-manager"),
	// 		},
	// 	},
	// 	Spec: netwv1.NetworkPolicySpecArgs{
	// 		PolicyTypes: pulumi.ToStringArray([]string{
	// 			"Egress",
	// 		}),
	// 		PodSelector: metav1.LabelSelectorArgs{
	// 			MatchLabels: cm.cmj.PodLabels,
	// 		},
	// 		Egress: netwv1.NetworkPolicyEgressRuleArray{
	// 			netwv1.NetworkPolicyEgressRuleArgs{
	// 				To: netwv1.NetworkPolicyPeerArray{
	// 					netwv1.NetworkPolicyPeerArgs{
	// 						NamespaceSelector: metav1.LabelSelectorArgs{
	// 							MatchLabels: pulumi.StringMap{
	// 								"kubernetes.io/metadata.name": args.Namespace,
	// 							},
	// 						},
	// 						PodSelector: metav1.LabelSelectorArgs{
	// 							MatchLabels: cm.cm.PodLabels,
	// 						},
	// 					},
	// 				},
	// 				Ports: netwv1.NetworkPolicyPortArray{
	// 					netwv1.NetworkPolicyPortArgs{
	// 						Port: cm.cm.Endpoint.ApplyT(func(edp string) int {
	// 							_, port, _ := strings.Cut(edp, ":")
	// 							iport, _ := strconv.Atoi(port)
	// 							return iport
	// 						}).(pulumi.IntOutput),
	// 						Protocol: pulumi.String("TCP"),
	// 					},
	// 				},
	// 			},
	// 		},
	// 	},
	// }, opts...)
	// if err != nil {
	// 	return
	// }

	return
}

func (cm *ChallManager) outputs(ctx *pulumi.Context) error {
	cm.Endpoint = cm.cm.Endpoint
	if cm.svc != nil {
		cm.ExposedPort = cm.svc.Spec.ApplyT(func(spec corev1.ServiceSpec) *int {
			return spec.Ports[0].NodePort
		}).(pulumi.IntPtrOutput)
	}

	return ctx.RegisterResourceOutputs(cm, pulumi.Map{
		"endpoint":     cm.Endpoint,
		"exposed_port": cm.ExposedPort,
	})
}

func ptr[T any](t T) *T {
	return &t
}
