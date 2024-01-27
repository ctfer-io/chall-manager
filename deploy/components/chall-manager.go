package components

import (
	"fmt"

	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes/meta/v1"
	rbacv1 "github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes/rbac/v1"
	"github.com/pulumi/pulumi-random/sdk/v4/go/random"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type (
	ChallManager struct {
		ns      *corev1.Namespace
		cr      *rbacv1.ClusterRole
		sa      *corev1.ServiceAccount
		crb     *rbacv1.ClusterRoleBinding
		pvc     *corev1.PersistentVolumeClaim
		salt    *random.RandomId
		saltSec *corev1.Secret
		dep     *appsv1.Deployment
		svc     *corev1.Service
		etcd    *EtcdCluster

		Port        pulumi.IntPtrOutput
		GatewayPort pulumi.IntPtrOutput
	}

	ChallManagerArgs struct {
		Namespace pulumi.StringInput
		// Replicas of the chall-manager instance. If not specified, default to 3.
		Replicas pulumi.IntInput
		Gateway  bool

		// ServiceType enables you to expose your Chall-Manager instance
		// (e.g. "NodePort" will make it reachable in the Kubernetes NodePort range).
		ServiceType pulumi.StringPtrInput

		// EtcdReplicas ; if not specified, default to 3.
		EtcdReplicas pulumi.IntInput
	}
)

const (
	port      = 8080
	portKey   = "grpc"
	gwPort    = 9090
	gwPortKey = "gateway"
	statesDir = "/etc/chall-manager/states"
)

// NewChallManager is a Kubernetes resources builder for a Chall-Manager HA instance.
//
// It creates the namespace the Chall-Manager will launch the scenarios into, then all
// the recommended ressources for a Kubernetes-native deployment in this first.
func NewChallManager(ctx *pulumi.Context, args *ChallManagerArgs, opts ...pulumi.ResourceOption) (*ChallManager, error) {
	if args == nil {
		args = &ChallManagerArgs{}
	}
	args.Replicas = defaultInt(args.Replicas, 3)
	args.EtcdReplicas = defaultInt(args.EtcdReplicas, 3)

	cm := &ChallManager{}
	if err := cm.provision(ctx, args, opts...); err != nil {
		return nil, err
	}
	cm.outputs()

	return cm, nil
}

func (cm *ChallManager) provision(ctx *pulumi.Context, args *ChallManagerArgs, opts ...pulumi.ResourceOption) (err error) {
	// Start etcd cluster
	cm.etcd, err = NewEtcdCluster(ctx, &EtcdArgs{
		Namespace: args.Namespace.ToStringOutput(),
		Replicas:  args.EtcdReplicas,
	}, opts...)
	if err != nil {
		return err
	}

	// Start chall-manager cluster
	labels := pulumi.StringMap{
		"app": pulumi.String("chall-manager"),
	}

	// => Namespace
	cm.ns, err = corev1.NewNamespace(ctx, "chall-manager-ns", &corev1.NamespaceArgs{
		Metadata: metav1.ObjectMetaArgs{
			Name:   args.Namespace,
			Labels: labels,
		},
	}, opts...)
	if err != nil {
		return
	}
	ns := cm.ns.ToNamespaceOutput().Metadata().Name()

	// => ClusterRole, used to create a dedicated service acccount for Chall-Manager
	cm.cr, err = rbacv1.NewClusterRole(ctx, "chall-manager-role", &rbacv1.ClusterRoleArgs{
		Metadata: metav1.ObjectMetaArgs{
			Name:      pulumi.String("chall-manager-role"),
			Namespace: ns,
			Labels:    labels,
		},
		Rules: rbacv1.PolicyRuleArray{
			// TODO review policy rules
			rbacv1.PolicyRuleArgs{
				ApiGroups: pulumi.ToStringArray([]string{
					"",
				}),
				Resources: pulumi.ToStringArray([]string{
					"services",
					"endpoints",
					"secrets",
				}),
				Verbs: pulumi.ToStringArray([]string{
					"get",
					"list",
					"watch",
				}),
			},
			rbacv1.PolicyRuleArgs{
				ApiGroups: pulumi.ToStringArray([]string{
					"extensions",
					"networking.k8s.io",
				}),
				Resources: pulumi.ToStringArray([]string{
					"ingresses",
					"ingressclasses",
				}),
				Verbs: pulumi.ToStringArray([]string{
					"get",
					"list",
					"watch",
				}),
			},
			rbacv1.PolicyRuleArgs{
				ApiGroups: pulumi.ToStringArray([]string{
					"extensions",
					"networking.k8s.io",
				}),
				Resources: pulumi.ToStringArray([]string{
					"ingresses/status",
				}),
				Verbs: pulumi.ToStringArray([]string{
					"update",
				}),
			},
		},
	}, opts...)
	if err != nil {
		return
	}

	// => ServiceAccount
	cm.sa, err = corev1.NewServiceAccount(ctx, "chall-manager-account", &corev1.ServiceAccountArgs{
		Metadata: metav1.ObjectMetaArgs{
			Name:      pulumi.String("chall-manager-account"),
			Namespace: ns,
			Labels:    labels,
		},
	}, opts...)
	if err != nil {
		return
	}

	// => ClusterRoleBinding, binds the ClusterRole and ServiceAccount
	cm.crb, err = rbacv1.NewClusterRoleBinding(ctx, "chall-manager-role-binding", &rbacv1.ClusterRoleBindingArgs{
		Metadata: metav1.ObjectMetaArgs{
			Name:      pulumi.String("chall-manager-role-binding"),
			Namespace: ns,
			Labels:    labels,
		},
		RoleRef: rbacv1.RoleRefArgs{
			ApiGroup: pulumi.String("rbac.authorization.k8s.io"),
			Kind:     pulumi.String("ClusterRole"),
			Name:     pulumi.String("chall-manager-role"),
		},
		Subjects: rbacv1.SubjectArray{
			rbacv1.SubjectArgs{
				Kind:      pulumi.String("ServiceAccount"),
				Name:      pulumi.String("chall-manager-account"),
				Namespace: ns,
			},
		},
	}, opts...)
	if err != nil {
		return
	}

	// => PersistentVolumeClaim
	cm.pvc, err = corev1.NewPersistentVolumeClaim(ctx, "chall-manager-pvc", &corev1.PersistentVolumeClaimArgs{
		Metadata: metav1.ObjectMetaArgs{
			Name:      pulumi.String("chall-manager-pvc"),
			Namespace: ns,
			Labels:    labels,
		},
		Spec: corev1.PersistentVolumeClaimSpecArgs{
			// StorageClassName: pulumi.String("longhorn"),
			AccessModes: pulumi.ToStringArray([]string{
				"ReadWriteMany",
			}),
			Resources: corev1.ResourceRequirementsArgs{
				Requests: pulumi.ToStringMap(map[string]string{
					"storage": "2Gi",
				}),
			},
		},
	}, opts...)
	if err != nil {
		return
	}

	// => Random string (salt)
	cm.salt, err = random.NewRandomId(ctx, "chall-manager-salt", &random.RandomIdArgs{
		ByteLength: pulumi.Int(16),
	}, opts...)
	if err != nil {
		return
	}
	// => Secret (salt)
	cm.saltSec, err = corev1.NewSecret(ctx, "chall-manager-salt-secret", &corev1.SecretArgs{
		Metadata: metav1.ObjectMetaArgs{
			Name:      pulumi.String("chall-manager-salt-secret"),
			Namespace: ns,
			Labels:    labels,
		},
		StringData: pulumi.ToStringMapOutput(map[string]pulumi.StringOutput{
			"salt": cm.salt.B64Std,
		}),
	}, opts...)
	if err != nil {
		return
	}

	// => Deployment
	dpar := corev1.ContainerPortArray{
		corev1.ContainerPortArgs{
			Name:          pulumi.String(portKey),
			ContainerPort: pulumi.Int(port),
		},
	}
	if args.Gateway {
		dpar = append(dpar, corev1.ContainerPortArgs{
			Name:          pulumi.String(gwPortKey),
			ContainerPort: pulumi.Int(gwPort),
		})
	}
	cm.dep, err = appsv1.NewDeployment(ctx, "chall-manager-deployment", &appsv1.DeploymentArgs{
		Metadata: metav1.ObjectMetaArgs{
			Name:      pulumi.String("chall-manager-deployment"),
			Namespace: ns,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpecArgs{
			Replicas: args.Replicas,
			Selector: metav1.LabelSelectorArgs{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpecArgs{
				Metadata: metav1.ObjectMetaArgs{
					Namespace: ns,
					Labels:    labels,
				},
				Spec: corev1.PodSpecArgs{
					ServiceAccountName: pulumi.String("chall-manager-account"),
					InitContainers: corev1.ContainerArray{
						corev1.ContainerArgs{
							Name:  pulumi.String("wait-etcd"),
							Image: pulumi.String("bitnami/etcd:3.5.11"),
							Command: cm.etcd.Endpoint.ApplyT(func(endpoint string) []string {
								return []string{
									"/bin/sh", "-c",
									fmt.Sprintf(`until etcdctl --endpoints=http://%s endpoint health; do
	echo "Waiting for etcd cluster to be ready..."
	sleep 5
done`, endpoint),
								}
							}).(pulumi.StringArrayOutput),
						},
					},
					Containers: corev1.ContainerArray{
						corev1.ContainerArgs{
							Name:            pulumi.String("chall-manager"),
							Image:           pulumi.String("pandatix/chall-manager:v0.1.0"), // TODO set proper image ctferio/chall-manager
							ImagePullPolicy: pulumi.String("Always"),
							Env: corev1.EnvVarArray{
								corev1.EnvVarArgs{
									Name:  pulumi.String("PORT"),
									Value: pulumi.Sprintf("%d", port),
								},
								corev1.EnvVarArgs{
									Name:  pulumi.String("GATEWAY"),
									Value: pulumi.Sprintf("%t", args.Gateway),
								},
								corev1.EnvVarArgs{
									Name:  pulumi.String("GATEWAY_PORT"),
									Value: pulumi.Sprintf("%d", gwPort),
								},
								corev1.EnvVarArgs{
									Name:  pulumi.String("STATES_DIR"),
									Value: pulumi.String(statesDir),
								},
								corev1.EnvVarArgs{
									Name: pulumi.String("SALT"),
									ValueFrom: corev1.EnvVarSourceArgs{
										SecretKeyRef: corev1.SecretKeySelectorArgs{
											Name: cm.saltSec.Metadata.Name(),
											Key:  pulumi.String("salt"),
										},
									},
								},
								corev1.EnvVarArgs{
									Name:  pulumi.String("LOCK_ETCD_ENDPOINTS"),
									Value: cm.etcd.Endpoint,
								},
								corev1.EnvVarArgs{
									Name:  pulumi.String("LOCK_ETCD_USERNAME"),
									Value: cm.etcd.Username,
								},
								corev1.EnvVarArgs{
									Name:  pulumi.String("LOCK_ETCD_PASSWORD"),
									Value: cm.etcd.Password,
								},
							},
							Ports: dpar,
							VolumeMounts: corev1.VolumeMountArray{
								corev1.VolumeMountArgs{
									Name:      pulumi.String("states"),
									MountPath: pulumi.String(statesDir),
								},
							},
						},
					},
					Volumes: corev1.VolumeArray{
						corev1.VolumeArgs{
							Name: pulumi.String("states"),
							PersistentVolumeClaim: corev1.PersistentVolumeClaimVolumeSourceArgs{
								ClaimName: cm.pvc.Metadata.Name().Elem().ToStringOutput(),
							},
						},
					},
				},
			},
		},
	}, opts...)
	if err != nil {
		return
	}

	// => Service
	spar := corev1.ServicePortArray{
		corev1.ServicePortArgs{
			Name: pulumi.String(portKey),
			Port: pulumi.Int(port),
		},
	}
	if args.Gateway {
		spar = append(spar, corev1.ServicePortArgs{
			Name: pulumi.String(gwPortKey),
			Port: pulumi.Int(gwPort),
		})
	}
	cm.svc, err = corev1.NewService(ctx, "chall-manager-service", &corev1.ServiceArgs{
		Metadata: metav1.ObjectMetaArgs{
			Name:      pulumi.String("chall-manager-service"),
			Namespace: ns,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpecArgs{
			Type:     args.ServiceType,
			Ports:    spar,
			Selector: labels,
		},
	}, opts...)
	if err != nil {
		return
	}

	return
}

func (cm *ChallManager) outputs() {
	cm.Port = findSpecKeyNodeport(cm.svc.Spec, portKey)
	cm.GatewayPort = findSpecKeyNodeport(cm.svc.Spec, gwPortKey)
}

func defaultInt(arg pulumi.IntInput, def int) pulumi.IntOutput {
	if arg == nil {
		return pulumi.Int(def).ToIntOutput()
	}
	return arg.ToIntOutput().ApplyT(func(argv int) int {
		if argv < 1 {
			return def
		}
		return argv
	}).(pulumi.IntOutput)
}

func findSpecKeyNodeport(svcSpec corev1.ServiceSpecPtrOutput, key string) pulumi.IntPtrOutput {
	return svcSpec.ApplyT(func(spec *corev1.ServiceSpec) *int {
		for _, ports := range spec.Ports {
			if ports.Name != nil && *ports.Name == key {
				return ports.NodePort
			}
		}
		return nil
	}).(pulumi.IntPtrOutput)
}
