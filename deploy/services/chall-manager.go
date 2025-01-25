package services

import (
	"errors"
	"strings"
	"sync"

	netwv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/networking/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"go.uber.org/multierr"

	"github.com/ctfer-io/chall-manager/deploy/common"
	"github.com/ctfer-io/chall-manager/deploy/services/parts"
)

type (
	// ChallManager Micro Service deployed on a Kubernetes infrastructure.
	ChallManager struct {
		pulumi.ResourceState

		// Parts
		etcd *parts.EtcdCluster
		cm   *parts.ChallManager
		cmj  *parts.ChallManagerJanitor

		// Interface & ports network policies
		cmToEtcd *netwv1.NetworkPolicy
		cmjToCm  *netwv1.NetworkPolicy

		// Outputs

		Endpoint pulumi.StringOutput
	}

	// ChallManagerArgs contains all the parametrization of a Chall-Manager
	// MicroService deployment on Kubernetes.
	ChallManagerArgs struct {
		Tag pulumi.StringPtrInput
		tag pulumi.StringOutput

		// PrivateRegistry define from where to fetch the Chall-Manager Docker images.
		// If set empty, defaults to Docker Hub.
		// Authentication is not supported, please provide it as Kubernetes-level configuration.
		PrivateRegistry pulumi.StringPtrInput
		privateRegistry pulumi.StringOutput

		Namespace    pulumi.StringInput
		EtcdReplicas pulumi.IntPtrInput
		Replicas     pulumi.IntPtrInput
		replicas     pulumi.IntOutput

		JanitorCron   pulumi.StringPtrInput
		janitorCron   pulumi.StringPtrOutput
		JanitorTicker pulumi.StringPtrInput
		janitorTicker pulumi.StringPtrOutput
		JanitorMode   parts.JanitorMode

		Swagger bool

		Otel *common.OtelArgs
	}
)

const (
	defaultCron = "*/1 * * * *"
)

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
	cm.outputs()

	return cm, nil
}

func (cm *ChallManager) defaults(args *ChallManagerArgs) *ChallManagerArgs {
	if args == nil {
		args = &ChallManagerArgs{}
	}

	if args.Tag == nil || args.Tag.ToStringPtrOutput().OutputState == nil {
		args.tag = pulumi.String("dev").ToStringOutput()
	} else {
		args.tag = args.Tag.ToStringPtrOutput().Elem()
	}

	if args.PrivateRegistry == nil || args.PrivateRegistry.ToStringPtrOutput().OutputState == nil {
		args.privateRegistry = pulumi.String("").ToStringOutput()
	} else {
		args.privateRegistry = args.PrivateRegistry.ToStringPtrOutput().ApplyT(func(in *string) string {
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

	if args.Replicas == nil || args.Replicas.ToIntPtrOutput().OutputState == nil {
		args.replicas = pulumi.Int(1).ToIntOutput()
	} else {
		args.replicas = args.Replicas.ToIntPtrOutput().Elem()
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
		Tag:             args.tag,
		PrivateRegistry: args.privateRegistry,
		Namespace:       args.Namespace,
		Replicas: args.replicas.ApplyT(func(replicas int) int {
			if replicas > 0 {
				return replicas
			}
			return 1 // default replicas to 1
		}).(pulumi.IntOutput),
		Etcd:    nil,
		Swagger: args.Swagger,
		Otel:    nil,
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
		PrivateRegistry:      args.privateRegistry,
		Namespace:            args.Namespace,
		ChallManagerEndpoint: cm.cm.Endpoint,
		Cron:                 args.janitorCron,
		Ticker:               args.janitorTicker,
		Mode:                 args.JanitorMode,
		Otel:                 cmjOtel,
	})
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

func (cm *ChallManager) outputs() {
	cm.Endpoint = cm.cm.Endpoint
}

func ptr[T any](t T) *T {
	return &t
}
