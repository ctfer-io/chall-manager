package components

import (
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

		Port pulumi.IntPtrOutput
	}

	ChallManagerArgs struct {
		Namespace pulumi.StringInput
		Replicas  pulumi.IntInput

		// ServiceType enables you to expose your Chall-Manager instance
		// (e.g. "NodePort" will make it reachable in the Kubernetes NodePort range).
		ServiceType pulumi.StringPtrInput
	}
)

// NewChallManager is a Kubernetes resources builder for a Chall-Manager HA instance.
//
// It creates the namespace the Chall-Manager will launch the scenarios into, then all
// the recommended ressources for a Kubernetes-native deployment in this first.
func NewChallManager(ctx *pulumi.Context, args *ChallManagerArgs, opts ...pulumi.ResourceOption) (*ChallManager, error) {
	if args == nil {
		args = &ChallManagerArgs{}
	}

	cm := &ChallManager{}
	if err := cm.provision(ctx, args, opts...); err != nil {
		return nil, err
	}
	cm.outputs()

	return cm, nil
}

func (cm *ChallManager) provision(ctx *pulumi.Context, args *ChallManagerArgs, opts ...pulumi.ResourceOption) (err error) {
	const port = 8080
	const statesDir = "/etc/chall-manager/states"

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
							},
							Ports: corev1.ContainerPortArray{
								corev1.ContainerPortArgs{
									ContainerPort: pulumi.Int(port),
								},
							},
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
	cm.svc, err = corev1.NewService(ctx, "chall-manager-service", &corev1.ServiceArgs{
		Metadata: metav1.ObjectMetaArgs{
			Name:      pulumi.String("chall-manager-service"),
			Namespace: ns,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpecArgs{
			Type: args.ServiceType,
			Ports: corev1.ServicePortArray{
				corev1.ServicePortArgs{
					Port: pulumi.Int(port),
				},
			},
			Selector: labels,
		},
	}, opts...)
	if err != nil {
		return
	}

	return
}

func (cm *ChallManager) outputs() {
	cm.Port = cm.svc.Spec.ApplyT(func(spec *corev1.ServiceSpec) *int {
		return spec.Ports[0].NodePort
	}).(pulumi.IntPtrOutput)
}
