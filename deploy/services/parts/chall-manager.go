package parts

import (
	"fmt"
	"strings"
	"sync"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"

	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	rbacv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/rbac/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/ctfer-io/chall-manager/deploy/common"
)

type (
	ChallManager struct {
		pulumi.ResourceState

		tgtns   *Namespace
		role    *rbacv1.Role
		sa      *corev1.ServiceAccount
		rb      *rbacv1.RoleBinding
		kubesec *corev1.Secret
		pvc     *corev1.PersistentVolumeClaim
		dep     *appsv1.Deployment
		svc     *corev1.Service

		PodLabels pulumi.StringMapOutput
		Endpoint  pulumi.StringOutput
	}

	ChallManagerArgs struct {
		// Tag defines the specific tag to run chall-manager to.
		// If not specified, defaults to "latest".
		Tag pulumi.StringPtrInput
		tag pulumi.StringOutput

		// Registry define from where to fetch the Chall-Manager Docker images.
		// If set empty, defaults to Docker Hub.
		// Authentication is not supported, please provide it as Kubernetes-level configuration.
		Registry pulumi.StringPtrInput
		registry pulumi.StringOutput

		// LogLevel defines the level at which to log.
		LogLevel pulumi.StringInput
		logLevel pulumi.StringOutput

		// Namespace to which deploy the chall-manager resources.
		// It is different from the namespace the chall-manager will deploy instances to,
		// which will be created on the fly.
		Namespace pulumi.StringInput

		// Replicas of the chall-manager instance. If not specified, default to 1.
		Replicas pulumi.IntPtrInput

		// PVCAccessModes defines the access modes supported by the PVC.
		PVCAccessModes pulumi.StringArrayInput
		pvcAccessModes pulumi.StringArrayOutput

		// PVCStorageSize enable to configure the storage size of the PVC Chall-Manager
		// will write into (store Pulumi stacks, data persistency, ...).
		// Default to 2Gi.
		// See https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/#meaning-of-memory
		// for syntax.
		PVCStorageSize pulumi.StringInput
		pvcStorageSize pulumi.StringOutput

		// RomeoClaimName, if set, will turn on the coverage export of Chall-Manager for later download.
		RomeoClaimName pulumi.StringInput
		mountCoverdir  bool

		// Kubeconfig is an optional attribute that override the ServiceAccount
		// created by default for Chall-Manager.
		Kubeconfig      pulumi.StringInput
		mountKubeconfig bool

		// Requests for the Chall-Manager container. For more infos:
		// https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
		Requests pulumi.StringMapInput
		requests pulumi.StringMapOutput

		// Limits for the Chall-Manager container. For more infos:
		// https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
		Limits pulumi.StringMapInput
		limits pulumi.StringMapOutput

		// A key=value map of additional environment variables to mount in Chall-Manager.
		Envs pulumi.StringMapInput
		envs pulumi.StringMapOutput

		Swagger bool

		Etcd *ChallManagerEtcdArgs

		Otel *common.OtelArgs

		OCIInsecure bool
		OCIUsername pulumi.StringPtrInput
		OCIPassword pulumi.StringPtrInput
	}
	ChallManagerEtcdArgs struct {
		Endpoint pulumi.StringInput
		Username pulumi.StringInput
		Password pulumi.StringInput
	}
)

const (
	port      = 8080
	portKey   = "grpc"
	directory = "/etc/chall-manager"
	coverdir  = "/etc/coverdir"

	defaultPVCStorageSize = "2Gi"
	defaultLogLevel       = "info"
)

var crudVerbs = []string{
	"create",
	"delete",
	"get",
	"list", // required to list resources in namespaces (queries)
	"patch",
	"update",
	"watch", // required to monitor resources when deployed/updated, else will get stucked
}

var defaultPVCAccessModes = []string{
	"ReadWriteMany",
}

// NewChallManager is a Kubernetes resources builder for a Chall-Manager HA instance.
//
// It creates the namespace the Chall-Manager will launch the scenarios into, then all
// the recommended resources for a Kubernetes-native Micro Services deployment.
func NewChallManager(ctx *pulumi.Context, name string, args *ChallManagerArgs, opts ...pulumi.ResourceOption) (*ChallManager, error) {
	cm := &ChallManager{}

	args = cm.defaults(args)
	if err := ctx.RegisterComponentResource("ctfer-io:chall-manager:chall-manager", name, cm, opts...); err != nil {
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

	args.pvcAccessModes = pulumi.ToStringArray(defaultPVCAccessModes).ToStringArrayOutput()
	if args.PVCAccessModes != nil {
		args.pvcAccessModes = args.PVCAccessModes.ToStringArrayOutput().ApplyT(func(am []string) []string {
			if len(am) == 0 {
				return defaultPVCAccessModes
			}
			return am
		}).(pulumi.StringArrayOutput)
	}

	args.pvcStorageSize = pulumi.String(defaultPVCStorageSize).ToStringOutput()
	if args.PVCStorageSize != nil {
		args.pvcStorageSize = args.PVCStorageSize.ToStringOutput().ApplyT(func(size string) string {
			if size == "" {
				return defaultPVCStorageSize
			}
			return size
		}).(pulumi.StringOutput)
	}

	if args.Kubeconfig != nil {
		wg := &sync.WaitGroup{}
		wg.Add(1)
		args.Kubeconfig.ToStringOutput().ApplyT(func(kubeconfig string) error {
			args.mountKubeconfig = kubeconfig != "" // XXX this check could be improved to validate it is syntaxically valid
			wg.Done()
			return nil
		})
		wg.Wait()
	}

	args.logLevel = pulumi.String(defaultLogLevel).ToStringOutput()
	if args.LogLevel != nil {
		args.logLevel = args.LogLevel.ToStringOutput()
	}

	if args.RomeoClaimName != nil {
		wg := &sync.WaitGroup{}
		wg.Add(1)
		args.RomeoClaimName.ToStringOutput().ApplyT(func(rcn string) error {
			args.mountCoverdir = rcn != ""
			wg.Done()
			return nil
		})
		wg.Wait()
	}

	args.requests = pulumi.StringMap{}.ToStringMapOutput()
	if args.Requests != nil {
		args.requests = args.Requests.ToStringMapOutput()
	}

	args.limits = pulumi.StringMap{}.ToStringMapOutput()
	if args.Limits != nil {
		args.limits = args.Limits.ToStringMapOutput()
	}

	args.envs = pulumi.StringMap{}.ToStringMapOutput()
	if args.Envs != nil {
		args.envs = args.Envs.ToStringMapOutput()
	}

	return args
}

func (cm *ChallManager) provision(ctx *pulumi.Context, args *ChallManagerArgs, opts ...pulumi.ResourceOption) (err error) {
	// Start chall-manager cluster
	// Labels: https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/#labels

	// Create a specific provider for distant resources ("target").
	topts := opts
	if args.Kubeconfig != nil {
		extcl, err := kubernetes.NewProvider(ctx, "external-cluster-pv", &kubernetes.ProviderArgs{
			Kubeconfig: args.Kubeconfig,
		})
		if err != nil {
			return err
		}
		topts = append(topts, pulumi.Provider(extcl))
	}

	cm.tgtns, err = NewNamespace(ctx, "target-ns", &NamespaceArgs{
		Name: pulumi.String("cm-target"),
		AdditionalLabels: pulumi.StringMap{
			"app.kubernetes.io/component": pulumi.String("target"),
			"app.kubernetes.io/part-of":   pulumi.String("chall-manager"),
			"ctfer.io/stack-name":         pulumi.String(ctx.Stack()),
		},
	}, topts...)
	if err != nil {
		return
	}

	if !args.mountKubeconfig {
		// => Role, used to create a dedicated service acccount for Chall-Manager
		cm.role, err = rbacv1.NewRole(ctx, "chall-manager-role", &rbacv1.RoleArgs{
			Metadata: metav1.ObjectMetaArgs{
				Namespace: cm.tgtns.Name,
				Labels: pulumi.StringMap{
					"app.kubernetes.io/component": pulumi.String("chall-manager"),
					"app.kubernetes.io/part-of":   pulumi.String("chall-manager"),
					"ctfer.io/stack-name":         pulumi.String(ctx.Stack()),
				},
			},
			Rules: rbacv1.PolicyRuleArray{
				rbacv1.PolicyRuleArgs{
					ApiGroups: pulumi.ToStringArray([]string{
						"",
					}),
					Resources: pulumi.ToStringArray([]string{
						// All the following resources are namespaced.
						"configmaps",
						"endpoints",
						"persistentvolumeclaims",
						"pods",
						"resourcequotas",
						"secrets",
						"services",
					}),
					Verbs: pulumi.ToStringArray(crudVerbs),
				},
				rbacv1.PolicyRuleArgs{
					ApiGroups: pulumi.ToStringArray([]string{
						"apps",
					}),
					Resources: pulumi.ToStringArray([]string{
						"deployments",
						"replicasets",
						"statefulsets",
					}),
					Verbs: pulumi.ToStringArray(crudVerbs),
				},
				rbacv1.PolicyRuleArgs{
					ApiGroups: pulumi.ToStringArray([]string{
						"batch",
					}),
					Resources: pulumi.ToStringArray([]string{
						"cronjobs",
						"jobs",
					}),
					Verbs: pulumi.ToStringArray(crudVerbs),
				},
				rbacv1.PolicyRuleArgs{
					ApiGroups: pulumi.ToStringArray([]string{
						"networking.k8s.io",
					}),
					Resources: pulumi.ToStringArray([]string{
						"ingresses",
						"networkpolicies",
					}),
					Verbs: pulumi.ToStringArray(crudVerbs),
				},
				rbacv1.PolicyRuleArgs{
					ApiGroups: pulumi.ToStringArray([]string{
						"events.k8s.io",
					}),
					Resources: pulumi.ToStringArray([]string{
						"events",
					}),
					Verbs: pulumi.ToStringArray([]string{
						"get",
						"list",
						"watch",
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
				Namespace: args.Namespace,
				Labels: pulumi.StringMap{
					"app.kubernetes.io/component": pulumi.String("chall-manager"),
					"app.kubernetes.io/part-of":   pulumi.String("chall-manager"),
					"ctfer.io/stack-name":         pulumi.String(ctx.Stack()),
				},
			},
		}, opts...)
		if err != nil {
			return
		}

		// => RoleBinding, binds the Role and ServiceAccount
		cm.rb, err = rbacv1.NewRoleBinding(ctx, "chall-manager-role-binding", &rbacv1.RoleBindingArgs{
			Metadata: metav1.ObjectMetaArgs{
				Namespace: cm.tgtns.Name,
				Name: cm.tgtns.Name.ApplyT(func(ns string) string {
					return fmt.Sprintf("ctfer-io:chall-manager:%s", ns) // uniquely identify the target-namespace RoleBinding
				}).(pulumi.StringOutput),
				Labels: pulumi.StringMap{
					"app.kubernetes.io/component": pulumi.String("chall-manager"),
					"app.kubernetes.io/part-of":   pulumi.String("chall-manager"),
					"ctfer.io/stack-name":         pulumi.String(ctx.Stack()),
				},
			},
			RoleRef: rbacv1.RoleRefArgs{
				ApiGroup: pulumi.String("rbac.authorization.k8s.io"),
				Kind:     pulumi.String("Role"),
				Name:     cm.role.Metadata.Name().Elem(),
			},
			Subjects: rbacv1.SubjectArray{
				rbacv1.SubjectArgs{
					Kind:      pulumi.String("ServiceAccount"),
					Name:      cm.sa.Metadata.Name().Elem(),
					Namespace: args.Namespace,
				},
			},
		}, opts...)
		if err != nil {
			return
		}
	} else {
		cm.kubesec, err = corev1.NewSecret(ctx, "kubesec", &corev1.SecretArgs{
			Metadata: metav1.ObjectMetaArgs{
				Namespace: args.Namespace,
				Labels: pulumi.StringMap{
					"app.kubernetes.io/component": pulumi.String("chall-manager"),
					"app.kubernetes.io/part-of":   pulumi.String("chall-manager"),
					"ctfer.io/stack-name":         pulumi.String(ctx.Stack()),
				},
			},
			StringData: pulumi.StringMap{
				"kubeconfig": args.Kubeconfig.ToStringPtrOutput().Elem(),
			},
			Immutable: pulumi.BoolPtr(true), // don't enable live patches
		}, opts...)
		if err != nil {
			return
		}
	}

	// => Deployment
	initCts := corev1.ContainerArray{}
	envs := corev1.EnvVarArray{
		corev1.EnvVarArgs{
			Name:  pulumi.String("PORT"),
			Value: pulumi.Sprintf("%d", port),
		},
		corev1.EnvVarArgs{
			Name:  pulumi.String("SWAGGER"),
			Value: pulumi.Sprintf("%t", args.Swagger),
		},
		corev1.EnvVarArgs{
			Name:  pulumi.String("DIR"),
			Value: pulumi.String(directory),
		},
		corev1.EnvVarArgs{
			Name:  pulumi.String("KUBERNETES_TARGET_NAMESPACE"),
			Value: cm.tgtns.Name,
		},
		corev1.EnvVarArgs{
			Name:  pulumi.String("LOG_LEVEL"),
			Value: args.logLevel,
		},
	}

	if args.Etcd != nil {
		initCts = append(initCts, corev1.ContainerArgs{
			Name:  pulumi.String("wait-etcd"),
			Image: pulumi.Sprintf("%sbitnami/etcd:3.5.16-debian-12-r0", args.registry),
			Command: pulumi.All(args.Etcd.Endpoint, args.Etcd.Username, args.Etcd.Password).ApplyT(func(args []any) []string {
				endpoint := args[0].(string)
				username := args[1].(string)
				password := args[2].(string)

				return []string{
					"/bin/sh", "-c",
					fmt.Sprintf(`until etcdctl --endpoints=http://%s --user=%s --password=%s endpoint health; do
	echo "Waiting for etcd cluster to be ready..."
	sleep 5
	done`, endpoint, username, password),
				}
			}).(pulumi.StringArrayOutput),
		})

		envs = append(envs,
			corev1.EnvVarArgs{
				Name:  pulumi.String("ETCD_ENDPOINT"),
				Value: args.Etcd.Endpoint,
			},
			corev1.EnvVarArgs{
				Name:  pulumi.String("ETCD_USERNAME"),
				Value: args.Etcd.Username,
			},
			corev1.EnvVarArgs{
				Name:  pulumi.String("ETCD_PASSWORD"),
				Value: args.Etcd.Password,
			},
		)
	}

	if args.Otel != nil {
		envs = append(envs,
			corev1.EnvVarArgs{
				Name:  pulumi.String("TRACING"),
				Value: pulumi.String("true"),
			},
			corev1.EnvVarArgs{
				Name:  pulumi.String("OTEL_SERVICE_NAME"),
				Value: args.Otel.ServiceName,
			},
			corev1.EnvVarArgs{
				Name:  pulumi.String("OTEL_EXPORTER_OTLP_ENDPOINT"),
				Value: pulumi.Sprintf("dns://%s", args.Otel.Endpoint),
			},
		)
		if args.Otel.Insecure {
			envs = append(envs,
				corev1.EnvVarArgs{
					Name:  pulumi.String("OTEL_EXPORTER_OTLP_INSECURE"),
					Value: pulumi.String("true"),
				},
			)
		}
	}

	if args.mountKubeconfig {
		envs = append(envs, corev1.EnvVarArgs{
			Name:  pulumi.String("KUBECONFIG"),
			Value: pulumi.String("/etc/kube/kubeconfig"),
		})
	}

	if args.mountCoverdir {
		envs = append(envs, corev1.EnvVarArgs{
			Name:  pulumi.String("GOCOVERDIR"),
			Value: pulumi.String(coverdir),
		})
	}
	if args.OCIInsecure {
		envs = append(envs, corev1.EnvVarArgs{
			Name:  pulumi.String("OCI_INSECURE"),
			Value: pulumi.String("true"),
		})
	}
	if args.OCIUsername != nil && args.OCIPassword != nil {
		envs = append(envs,
			corev1.EnvVarArgs{
				Name:  pulumi.String("OCI_USERNAME"),
				Value: args.OCIUsername,
			},
			corev1.EnvVarArgs{
				Name:  pulumi.String("OCI_PASSWORD"),
				Value: args.OCIPassword,
			},
		)
	}

	// => PersistentVolumeClaim
	cm.pvc, err = corev1.NewPersistentVolumeClaim(ctx, "chall-manager-pvc", &corev1.PersistentVolumeClaimArgs{
		Metadata: metav1.ObjectMetaArgs{
			Namespace: args.Namespace,
			Labels: pulumi.StringMap{
				"app.kubernetes.io/component": pulumi.String("chall-manager"),
				"app.kubernetes.io/part-of":   pulumi.String("chall-manager"),
				"ctfer.io/stack-name":         pulumi.String(ctx.Stack()),
			},
		},
		Spec: corev1.PersistentVolumeClaimSpecArgs{
			AccessModes: args.PVCAccessModes,
			Resources: corev1.VolumeResourceRequirementsArgs{
				Requests: pulumi.StringMap{
					"storage": args.pvcStorageSize,
				},
			},
		},
	}, opts...)
	if err != nil {
		return
	}

	// => Deployment
	cm.PodLabels = pulumi.StringMap{
		"app.kubernetes.io/name":      pulumi.String("chall-manager"),
		"app.kubernetes.io/version":   args.tag,
		"app.kubernetes.io/component": pulumi.String("chall-manager"),
		"app.kubernetes.io/part-of":   pulumi.String("chall-manager"),
		"ctfer.io/stack-name":         pulumi.String(ctx.Stack()),
	}.ToStringMapOutput()
	cm.dep, err = appsv1.NewDeployment(ctx, "chall-manager-deployment", &appsv1.DeploymentArgs{
		Metadata: metav1.ObjectMetaArgs{
			Namespace: args.Namespace,
			Labels: pulumi.StringMap{
				"app.kubernetes.io/name":      pulumi.String("chall-manager"),
				"app.kubernetes.io/version":   args.tag,
				"app.kubernetes.io/component": pulumi.String("chall-manager"),
				"app.kubernetes.io/part-of":   pulumi.String("chall-manager"),
				"ctfer.io/stack-name":         pulumi.String(ctx.Stack()),
			},
		},
		Spec: appsv1.DeploymentSpecArgs{
			Replicas: pulumi.All(args.Replicas).ApplyT(func(all []any) int {
				if replicas, ok := all[0].(*int); ok {
					return *replicas
				}
				return 1 // default replicas to 1
			}).(pulumi.IntOutput),
			Selector: metav1.LabelSelectorArgs{
				MatchLabels: pulumi.StringMap{
					"app.kubernetes.io/name":      pulumi.String("chall-manager"),
					"app.kubernetes.io/version":   args.tag,
					"app.kubernetes.io/component": pulumi.String("chall-manager"),
					"app.kubernetes.io/part-of":   pulumi.String("chall-manager"),
					"ctfer.io/stack-name":         pulumi.String(ctx.Stack()),
				},
			},
			Template: corev1.PodTemplateSpecArgs{
				Metadata: metav1.ObjectMetaArgs{
					Namespace: args.Namespace,
					Labels:    cm.PodLabels,
				},
				Spec: corev1.PodSpecArgs{
					ServiceAccountName: func() pulumi.StringPtrInput {
						if !args.mountKubeconfig {
							return cm.sa.Metadata.Name()
						}
						return nil
					}(),
					InitContainers: initCts,
					Containers: corev1.ContainerArray{
						corev1.ContainerArgs{
							Name:  pulumi.String("chall-manager"),
							Image: pulumi.Sprintf("%sctferio/chall-manager:%s", args.registry, args.tag),
							Env: pulumi.All(envs.ToEnvVarArrayOutput(), args.envs).ApplyT(func(all []any) []corev1.EnvVar {
								envs := all[0].([]corev1.EnvVar)
								for k, v := range all[1].(map[string]string) {
									envs = append(envs, corev1.EnvVar{
										Name:  k,
										Value: &v,
									})
								}
								return envs
							}).(corev1.EnvVarArrayOutput),
							ImagePullPolicy: pulumi.String("Always"),
							Ports: corev1.ContainerPortArray{
								corev1.ContainerPortArgs{
									Name:          pulumi.String(portKey),
									ContainerPort: pulumi.Int(port),
								},
							},
							VolumeMounts: func() corev1.VolumeMountArrayOutput {
								vms := corev1.VolumeMountArray{
									corev1.VolumeMountArgs{
										Name:      pulumi.String("dir"),
										MountPath: pulumi.String(directory),
									},
								}
								if args.mountKubeconfig {
									vms = append(vms, corev1.VolumeMountArgs{
										Name:      pulumi.String("kubeconfig"),
										MountPath: pulumi.String("/etc/kube"),
										ReadOnly:  pulumi.BoolPtr(true),
									})
								}
								if args.mountCoverdir {
									vms = append(vms, corev1.VolumeMountArgs{
										Name:      pulumi.String("coverdir"),
										MountPath: pulumi.String(coverdir),
									})
								}
								return vms.ToVolumeMountArrayOutput()
							}(),
							ReadinessProbe: corev1.ProbeArgs{
								HttpGet: corev1.HTTPGetActionArgs{
									Path: pulumi.String("/healthcheck"),
									Port: pulumi.Int(port),
								},
							},
							Resources: corev1.ResourceRequirementsArgs{
								Requests: args.requests,
								Limits:   args.limits,
							},
						},
					},
					Volumes: func() corev1.VolumeArrayOutput {
						vs := corev1.VolumeArray{
							corev1.VolumeArgs{
								Name: pulumi.String("dir"),
								PersistentVolumeClaim: corev1.PersistentVolumeClaimVolumeSourceArgs{
									ClaimName: cm.pvc.Metadata.Name().Elem(),
								},
							},
						}
						if args.mountKubeconfig {
							vs = append(vs, corev1.VolumeArgs{
								Name: pulumi.String("kubeconfig"),
								Secret: corev1.SecretVolumeSourceArgs{
									SecretName: cm.kubesec.Metadata.Name(),
								},
							})
						}
						if args.mountCoverdir {
							vs = append(vs, corev1.VolumeArgs{
								Name: pulumi.String("coverdir"),
								PersistentVolumeClaim: corev1.PersistentVolumeClaimVolumeSourceArgs{
									ClaimName: args.RomeoClaimName,
								},
							})
						}
						return vs.ToVolumeArrayOutput()
					}(),
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
			Namespace: args.Namespace,
			Labels: pulumi.StringMap{
				"app.kubernetes.io/component": pulumi.String("chall-manager"),
				"app.kubernetes.io/part-of":   pulumi.String("chall-manager"),
				"ctfer.io/stack-name":         pulumi.String(ctx.Stack()),
			},
		},
		Spec: corev1.ServiceSpecArgs{
			ClusterIP: pulumi.String("None"), // Headless, for DNS purposes
			Ports: corev1.ServicePortArray{
				corev1.ServicePortArgs{
					Name: pulumi.String(portKey),
					Port: pulumi.Int(port),
				},
			},
			Selector: cm.dep.Spec.Template().Metadata().Labels(),
		},
	}, opts...)
	if err != nil {
		return
	}

	return
}

func (cm *ChallManager) outputs(ctx *pulumi.Context) error {
	// cm.PodLabels is defined during provisionning such that it can be returned for
	// netpols. Then, they can be created to grant network traffic (cmj->cm[->etcd])
	// necessary for the readiness probe to pass as it can reach the etcd cluster.

	cm.Endpoint = pulumi.Sprintf("%s.%s:%d", cm.svc.Metadata.Name().Elem(), cm.svc.Metadata.Namespace().Elem(), port)

	return ctx.RegisterResourceOutputs(cm, pulumi.Map{
		"podLabels": cm.PodLabels,
		"endpoint":  cm.Endpoint,
	})
}
