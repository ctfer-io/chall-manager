package components

import (
	"fmt"

	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	batchv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/batch/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	netwv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/networking/v1"
	rbacv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/rbac/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type (
	ChallManager struct {
		pulumi.ResourceState

		tgtns       *corev1.Namespace
		role        *rbacv1.Role
		sa          *corev1.ServiceAccount
		rb          *rbacv1.RoleBinding
		pvc         *corev1.PersistentVolumeClaim
		dep         *appsv1.Deployment
		svc         *corev1.Service
		cjob        *batchv1.CronJob
		npol        *netwv1.NetworkPolicy
		dnspol      *netwv1.NetworkPolicy
		internspol  *netwv1.NetworkPolicy
		internetpol *netwv1.NetworkPolicy

		// Non-mandatory values, used internally to get track of arguments logic results.
		etcd *EtcdCluster

		Port        pulumi.IntPtrOutput
		GatewayPort pulumi.IntPtrOutput
	}

	ChallManagerArgs struct {
		// Tag defines the specific tag to run chall-manager to.
		// If not specified, defaults to "latest".
		Tag pulumi.StringPtrInput
		tag pulumi.StringOutput

		// Namespace to which deploy the chall-manager resources.
		// It is different from the namespace the chall-manager will deploy instances to,
		// which will be created on the fly.
		Namespace pulumi.StringInput

		// Replicas of the chall-manager instance. If not specified, default to 1.
		Replicas pulumi.IntPtrInput

		Gateway bool
		Swagger bool

		// ServiceType enables you to expose your Chall-Manager instance
		// (e.g. "NodePort" will make it reachable in the Kubernetes NodePort range).
		ServiceType pulumi.StringPtrInput

		// LockKind, know what lock strategy to adopt.
		LockKind string

		// EtcdReplicas ; if not specified, default to 1.
		EtcdReplicas pulumi.IntPtrInput

		// JanitorCron is the cron controlling how often the chall-manager-janitor must run.
		// If not set, default to every 15 minutes.
		JanitorCron pulumi.StringPtrInput
		cron        pulumi.StringOutput

		// The Otel Collector (OTLP through gRPC) endpoint to send signals to.
		// If specified, will automatically turn on tracing.
		OTLPEndpoint pulumi.StringInput
		OTLPInsecure bool
	}
)

const (
	port        = 8080
	portKey     = "grpc"
	gwPort      = 9090
	gwPortKey   = "gateway"
	directory   = "/etc/chall-manager"
	defaultCron = "*/1 * * * *"
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

// NewChallManager is a Kubernetes resources builder for a Chall-Manager HA instance.
//
// It creates the namespace the Chall-Manager will launch the scenarios into, then all
// the recommended resources for a Kubernetes-native Micro Services deployment.
func NewChallManager(ctx *pulumi.Context, name string, args *ChallManagerArgs, opts ...pulumi.ResourceOption) (*ChallManager, error) {
	// Validate inputs and defaults if necessary
	if args == nil {
		args = &ChallManagerArgs{}
	}
	if args.JanitorCron == nil || args.JanitorCron == pulumi.String("") {
		args.cron = pulumi.String(defaultCron).ToStringOutput()
	} else {
		args.cron = args.JanitorCron.ToStringPtrOutput().Elem()
	}
	if args.Tag == nil || args.Tag == pulumi.String("") {
		args.tag = pulumi.String("dev").ToStringOutput()
	} else {
		args.tag = args.Tag.ToStringPtrOutput().Elem()
	}

	// Register component resource, provision and export outputs
	cm := &ChallManager{}
	if err := ctx.RegisterComponentResource("ctfer-io:chall-manager:chall-manager", name, cm, opts...); err != nil {
		return nil, err
	}
	opts = append(opts, pulumi.Parent(cm))
	if err := cm.provision(ctx, args, opts...); err != nil {
		return nil, err
	}
	cm.outputs()

	return cm, nil
}

func (cm *ChallManager) provision(ctx *pulumi.Context, args *ChallManagerArgs, opts ...pulumi.ResourceOption) (err error) {
	// Start chall-manager cluster
	// Labels: https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/#labels

	// => Namespace to deploy to
	cm.tgtns, err = corev1.NewNamespace(ctx, "chall-manager-target-ns", &corev1.NamespaceArgs{
		Metadata: metav1.ObjectMetaArgs{
			Labels: pulumi.StringMap{
				"app.kubernetes.io/component": pulumi.String("deploy"),
				"app.kubernetes.io/part-of":   pulumi.String("chall-manager"),
			},
		},
	}, opts...)
	if err != nil {
		return
	}

	// => NetworkPolicy to deny all trafic by default. Scenarios should provide
	// their own network policies to grant necessary trafic.
	cm.npol, err = netwv1.NewNetworkPolicy(ctx, "chall-manager-target-ns-netpol-deny-all", &netwv1.NetworkPolicyArgs{
		Metadata: metav1.ObjectMetaArgs{
			Namespace: cm.tgtns.Metadata.Name(),
			Labels: pulumi.StringMap{
				"app.kubernetes.io/component": pulumi.String("chall-manager"),
				"app.kubernetes.io/part-of":   pulumi.String("chall-manager"),
			},
		},
		Spec: netwv1.NetworkPolicySpecArgs{
			PodSelector: metav1.LabelSelectorArgs{},
			PolicyTypes: pulumi.ToStringArray([]string{
				"Ingress",
				"Egress",
			}),
		},
	}, opts...)
	if err != nil {
		return
	}

	// => NetworkPolicy to grant DNS resolution (complex scenarios could require
	// to reach other pods in the namespace, e.g. not a scenario that fits into
	// the sdk.ctfer.io/ExposedMonopod architecture, which then would use headless
	// services so DNS resolution).
	cm.dnspol, err = netwv1.NewNetworkPolicy(ctx, "chall-manager-target-ns-netpol-dns", &netwv1.NetworkPolicyArgs{
		Metadata: metav1.ObjectMetaArgs{
			Namespace: cm.tgtns.Metadata.Name(),
			Labels: pulumi.StringMap{
				"app.kubernetes.io/component": pulumi.String("chall-manager"),
				"app.kubernetes.io/part-of":   pulumi.String("chall-manager"),
			},
		},
		Spec: netwv1.NetworkPolicySpecArgs{
			PolicyTypes: pulumi.ToStringArray([]string{
				"Egress",
			}),
			PodSelector: metav1.LabelSelectorArgs{},
			Egress: netwv1.NetworkPolicyEgressRuleArray{
				netwv1.NetworkPolicyEgressRuleArgs{
					To: netwv1.NetworkPolicyPeerArray{
						netwv1.NetworkPolicyPeerArgs{
							NamespaceSelector: metav1.LabelSelectorArgs{
								MatchLabels: pulumi.StringMap{
									"kubernetes.io/metadata.name": pulumi.String("kube-system"),
								},
							},
							PodSelector: metav1.LabelSelectorArgs{
								MatchLabels: pulumi.StringMap{
									"k8s-app": pulumi.String("kube-dns"),
								},
							},
						},
					},
					Ports: netwv1.NetworkPolicyPortArray{
						netwv1.NetworkPolicyPortArgs{
							Port:     pulumi.Int(53),
							Protocol: pulumi.String("UDP"),
						},
						netwv1.NetworkPolicyPortArgs{
							Port:     pulumi.Int(53),
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

	// => NetworkPolicy to deny all scenarios from reaching adjacent namespaces
	cm.internspol, err = netwv1.NewNetworkPolicy(ctx, "chall-manager-target-inter-ns-netpol", &netwv1.NetworkPolicyArgs{
		Metadata: metav1.ObjectMetaArgs{
			Namespace: cm.tgtns.Metadata.Name(),
			Labels: pulumi.StringMap{
				"app.kubernetes.io/component": pulumi.String("chall-manager"),
				"app.kubernetes.io/part-of":   pulumi.String("chall-manager"),
			},
		},
		Spec: netwv1.NetworkPolicySpecArgs{
			PodSelector: metav1.LabelSelectorArgs{},
			PolicyTypes: pulumi.ToStringArray([]string{
				"Egress",
			}),
			Egress: netwv1.NetworkPolicyEgressRuleArray{
				netwv1.NetworkPolicyEgressRuleArgs{
					To: netwv1.NetworkPolicyPeerArray{
						netwv1.NetworkPolicyPeerArgs{
							NamespaceSelector: metav1.LabelSelectorArgs{
								MatchExpressions: metav1.LabelSelectorRequirementArray{
									metav1.LabelSelectorRequirementArgs{
										Key:      pulumi.String("kubernetes.io/metadata.name"),
										Operator: pulumi.String("NotIn"),
										Values: pulumi.StringArray{
											cm.tgtns.Metadata.Name().Elem(),
										},
									},
								},
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

	// => NetworkPolicy to grant access to Internet IPs (required to download fonts, images, etc.)
	cm.internetpol, err = netwv1.NewNetworkPolicy(ctx, "chall-manager-internet-netpol", &netwv1.NetworkPolicyArgs{
		Metadata: metav1.ObjectMetaArgs{
			Namespace: cm.tgtns.Metadata.Name(),
			Labels: pulumi.StringMap{
				"app.kubernetes.io/component": pulumi.String("chall-manager"),
				"app.kubernetes.io/part-of":   pulumi.String("chall-manager"),
			},
		},
		Spec: netwv1.NetworkPolicySpecArgs{
			PodSelector: metav1.LabelSelectorArgs{},
			PolicyTypes: pulumi.ToStringArray([]string{
				"Egress",
			}),
			Egress: netwv1.NetworkPolicyEgressRuleArray{
				netwv1.NetworkPolicyEgressRuleArgs{
					To: netwv1.NetworkPolicyPeerArray{
						netwv1.NetworkPolicyPeerArgs{
							IpBlock: netwv1.IPBlockArgs{
								Cidr: pulumi.String("0.0.0.0/0"),
								Except: pulumi.ToStringArray([]string{
									"10.0.0.0/8",     // internal Kubernetes cluster IP range
									"172.16.0.0/12",  // common internal IP range
									"192.168.0.0/16", // common internal IP range
								}),
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

	// Check lock kind
	switch lk := args.LockKind; lk {
	case "", "local":
		args.LockKind = "local" // overwrite, for the empty string case
		// Nothing special to do, it will work by itself

	case "etcd":
		// Start etcd cluster
		cm.etcd, err = NewEtcdCluster(ctx, &EtcdArgs{
			Namespace: args.Namespace,
			Replicas: pulumi.All(args.EtcdReplicas).ApplyT(func(all []any) int {
				if replicas, ok := all[0].(*int); ok {
					return *replicas
				}
				return 1 // default replicas to 1
			}).(pulumi.IntOutput),
			OTLPEndpoint: args.OTLPEndpoint,
			OTLPInsecure: args.OTLPInsecure,
		}, opts...)
		if err != nil {
			return err
		}

	default:
		return fmt.Errorf("invalid lock kind: %s", lk)
	}

	// => Role, used to create a dedicated service acccount for Chall-Manager
	cm.role, err = rbacv1.NewRole(ctx, "chall-manager-role", &rbacv1.RoleArgs{
		Metadata: metav1.ObjectMetaArgs{
			Namespace: cm.tgtns.Metadata.Name(),
			Labels: pulumi.StringMap{
				"app.kubernetes.io/component": pulumi.String("chall-manager"),
				"app.kubernetes.io/part-of":   pulumi.String("chall-manager"),
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
			},
		},
	}, opts...)
	if err != nil {
		return
	}

	// => RoleBinding, binds the Role and ServiceAccount
	cm.rb, err = rbacv1.NewRoleBinding(ctx, "chall-manager-role-binding", &rbacv1.RoleBindingArgs{
		Metadata: metav1.ObjectMetaArgs{
			Namespace: cm.tgtns.Metadata.Name(),
			Labels: pulumi.StringMap{
				"app.kubernetes.io/component": pulumi.String("chall-manager"),
				"app.kubernetes.io/part-of":   pulumi.String("chall-manager"),
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

	// => PersistentVolumeClaim
	cm.pvc, err = corev1.NewPersistentVolumeClaim(ctx, "chall-manager-pvc", &corev1.PersistentVolumeClaimArgs{
		Metadata: metav1.ObjectMetaArgs{
			Namespace: args.Namespace,
			Labels: pulumi.StringMap{
				"app.kubernetes.io/component": pulumi.String("chall-manager"),
				"app.kubernetes.io/part-of":   pulumi.String("chall-manager"),
			},
		},
		Spec: corev1.PersistentVolumeClaimSpecArgs{
			// StorageClassName: pulumi.String("longhorn"),
			AccessModes: pulumi.ToStringArray([]string{
				"ReadWriteMany",
			}),
			Resources: corev1.VolumeResourceRequirementsArgs{
				Requests: pulumi.ToStringMap(map[string]string{
					"storage": "2Gi",
				}),
			},
		},
	}, opts...)
	if err != nil {
		return
	}

	// => Deployment
	initCts := corev1.ContainerArray{}
	envs := corev1.EnvVarArray{
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
			Name:  pulumi.String("GATEWAY_SWAGGER"),
			Value: pulumi.Sprintf("%t", args.Swagger),
		},
		corev1.EnvVarArgs{
			Name:  pulumi.String("DIR"),
			Value: pulumi.String(directory),
		},
		corev1.EnvVarArgs{
			Name:  pulumi.String("LOCK_KIND"),
			Value: pulumi.String(args.LockKind),
		},
		corev1.EnvVarArgs{
			Name:  pulumi.String("GOPRIVATE"),
			Value: pulumi.String("github.com/ctfer-io/chall-manager"),
		},
		corev1.EnvVarArgs{
			Name:  pulumi.String("KUBERNETES_NAMESPACE"),
			Value: cm.tgtns.Metadata.Name(),
		},
	}

	if args.LockKind == "etcd" {
		initCts = append(initCts, corev1.ContainerArgs{
			Name:  pulumi.String("wait-etcd"),
			Image: pulumi.String("bitnami/etcd:3.5.15"),
			Command: pulumi.All(cm.etcd.Endpoint, cm.etcd.Username, cm.etcd.Password).ApplyT(func(args []any) []string {
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
		)
	}

	if args.OTLPEndpoint != nil {
		envs = append(envs,
			corev1.EnvVarArgs{
				Name:  pulumi.String("TRACING"),
				Value: pulumi.String("true"),
			},
			corev1.EnvVarArgs{
				Name:  pulumi.String("OTEL_EXPORTER_OTLP_ENDPOINT"),
				Value: args.OTLPEndpoint,
			},
		)
		if args.OTLPInsecure {
			envs = append(envs,
				corev1.EnvVarArgs{
					Name:  pulumi.String("OTEL_EXPORTER_OTLP_INSECURE"),
					Value: pulumi.String("true"),
				},
			)
		}
	}

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
			Namespace: args.Namespace,
			Labels: pulumi.StringMap{
				"app.kubernetes.io/name":      pulumi.String("chall-manager"),
				"app.kubernetes.io/version":   args.tag,
				"app.kubernetes.io/component": pulumi.String("chall-manager"),
				"app.kubernetes.io/part-of":   pulumi.String("chall-manager"),
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
				},
			},
			Template: corev1.PodTemplateSpecArgs{
				Metadata: metav1.ObjectMetaArgs{
					Namespace: args.Namespace,
					Labels: pulumi.StringMap{
						"app.kubernetes.io/name":      pulumi.String("chall-manager"),
						"app.kubernetes.io/version":   args.tag,
						"app.kubernetes.io/component": pulumi.String("chall-manager"),
						"app.kubernetes.io/part-of":   pulumi.String("chall-manager"),
					},
				},
				Spec: corev1.PodSpecArgs{
					ServiceAccountName: cm.sa.Metadata.Name(),
					InitContainers:     initCts,
					Containers: corev1.ContainerArray{
						corev1.ContainerArgs{
							Name:            pulumi.String("chall-manager"),
							Image:           pulumi.Sprintf("registry.dev1.ctfer-io.lab/ctferio/chall-manager:%s", args.tag), // TODO set proper image ctferio/chall-manager
							ImagePullPolicy: pulumi.String("Always"),
							Command: pulumi.ToStringArray([]string{
								"/bin/bash", "-c",
								"echo \"machine github.com login pandatix password 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\" > /root/.netrc && /chall-manager",
							}),
							Env:   envs,
							Ports: dpar,
							VolumeMounts: corev1.VolumeMountArray{
								corev1.VolumeMountArgs{
									Name:      pulumi.String("dir"),
									MountPath: pulumi.String(directory),
								},
							},
						},
					},
					Volumes: corev1.VolumeArray{
						corev1.VolumeArgs{
							Name: pulumi.String("dir"),
							PersistentVolumeClaim: corev1.PersistentVolumeClaimVolumeSourceArgs{
								ClaimName: cm.pvc.Metadata.Name().Elem(),
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
			Namespace: args.Namespace,
			Labels: pulumi.StringMap{
				"app.kubernetes.io/component": pulumi.String("chall-manager"),
				"app.kubernetes.io/part-of":   pulumi.String("chall-manager"),
			},
		},
		Spec: corev1.ServiceSpecArgs{
			Type:      args.ServiceType,
			ClusterIP: pulumi.String("None"), // Headless, for DNS purposes
			Ports:     spar,
			Selector: pulumi.StringMap{
				"app.kubernetes.io/name":      pulumi.String("chall-manager"),
				"app.kubernetes.io/version":   args.tag,
				"app.kubernetes.io/component": pulumi.String("chall-manager"),
				"app.kubernetes.io/part-of":   pulumi.String("chall-manager"),
			},
		},
	}, opts...)
	if err != nil {
		return
	}

	// => CronJob (janitor)
	cronEnv := corev1.EnvVarArray{
		corev1.EnvVarArgs{
			Name:  pulumi.String("URL"),
			Value: pulumi.Sprintf("%s:%d", cm.svc.Metadata.Name().Elem(), port),
		},
	}
	if args.OTLPEndpoint != nil {
		cronEnv = append(cronEnv,
			corev1.EnvVarArgs{
				Name:  pulumi.String("TRACING"),
				Value: pulumi.String("true"),
			},
			corev1.EnvVarArgs{
				Name:  pulumi.String("OTEL_EXPORTER_OTLP_ENDPOINT"),
				Value: args.OTLPEndpoint,
			},
		)
		if args.OTLPInsecure {
			cronEnv = append(cronEnv,
				corev1.EnvVarArgs{
					Name:  pulumi.String("OTEL_EXPORTER_OTLP_INSECURE"),
					Value: pulumi.String("true"),
				},
			)
		}
	}
	cm.cjob, err = batchv1.NewCronJob(ctx, "chall-manager-janitor", &batchv1.CronJobArgs{
		Metadata: metav1.ObjectMetaArgs{
			Namespace: args.Namespace,
			Labels: pulumi.StringMap{
				"app.kubernetes.io/name":      pulumi.String("chall-manager-janitor"),
				"app.kubernetes.io/version":   args.tag,
				"app.kubernetes.io/component": pulumi.String("chall-manager"),
				"app.kubernetes.io/part-of":   pulumi.String("chall-manager"),
			},
		},
		Spec: batchv1.CronJobSpecArgs{
			Schedule: args.cron,
			JobTemplate: batchv1.JobTemplateSpecArgs{
				Spec: batchv1.JobSpecArgs{
					Template: corev1.PodTemplateSpecArgs{
						Metadata: metav1.ObjectMetaArgs{
							Namespace: args.Namespace,
							Labels: pulumi.StringMap{
								"app.kubernetes.io/name":      pulumi.String("chall-manager-janitor"),
								"app.kubernetes.io/version":   args.tag,
								"app.kubernetes.io/component": pulumi.String("chall-manager"),
								"app.kubernetes.io/part-of":   pulumi.String("chall-manager"),
							},
						},
						Spec: corev1.PodSpecArgs{
							Containers: corev1.ContainerArray{
								corev1.ContainerArgs{
									Name:            pulumi.String("chall-manager-janitor"),
									Image:           pulumi.Sprintf("registry.dev1.ctfer-io.lab/ctferio/chall-manager-janitor:%s", args.tag), // TODO set proper image ctferio/chall-manager-janitor
									ImagePullPolicy: pulumi.String("Always"),
									Env:             cronEnv,
								},
							},
							RestartPolicy: pulumi.String("OnFailure"),
						},
					},
				},
			},
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

func findSpecKeyNodeport(svcSpec corev1.ServiceSpecOutput, key string) pulumi.IntPtrOutput {
	return svcSpec.ApplyT(func(spec corev1.ServiceSpec) *int {
		for _, ports := range spec.Ports {
			if ports.Name != nil && *ports.Name == key {
				return ports.NodePort
			}
		}
		return nil
	}).(pulumi.IntPtrOutput)
}
