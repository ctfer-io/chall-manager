package kubernetes

import (
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	netwv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/networking/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type (
	ExposedMonopod struct {
		dep *appsv1.Deployment
		svc *corev1.Service
		ing *netwv1.Ingress
		ntp *netwv1.NetworkPolicy

		// URL to reach the exposed monopod to.
		// Content depends on the ExposeType argument.
		URL pulumi.StringOutput
	}

	ExposedMonopodArgs struct {
		// Challenge instance attributes

		Identity string
		Hostname string

		// Kubernetes attributes

		Namespace string
		Image     string
		Port      int
		FromCIDR  string

		ExposeType ExposeType
	}

	ExposeType int
)

const (
	ExposeNodePort ExposeType = iota
	ExposeIngress
)

// NewExposedMonopod builds the Kubernetes resources for an exposed monopod.
// It fits the best cases of web or pwn challenges where only 1 pod is required.
func NewExposedMonopod(ctx *pulumi.Context, args *ExposedMonopodArgs, opts ...pulumi.ResourceOption) (*ExposedMonopod, error) {
	if args == nil {
		args = &ExposedMonopodArgs{}
	}

	emp := &ExposedMonopod{}
	if err := emp.provision(ctx, args, opts...); err != nil {
		return nil, err
	}
	emp.outputs(args)

	return emp, nil
}

func (emp *ExposedMonopod) provision(ctx *pulumi.Context, args *ExposedMonopodArgs, opts ...pulumi.ResourceOption) (err error) {
	// Uniquely identify the resources with labels
	labels := pulumi.ToStringMap(map[string]string{
		"instanciated-by":        "chall-manager",
		"chall-manager-sdk/kind": "exposed-monopod",
		"identity":               args.Identity,
	})
	ns := pulumi.String(args.Namespace)

	// => Deployment
	emp.dep, err = appsv1.NewDeployment(ctx, "emp-dep-"+args.Identity, &appsv1.DeploymentArgs{
		Metadata: metav1.ObjectMetaArgs{
			Name:      pulumi.String("emp-dep-" + args.Identity),
			Namespace: ns,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpecArgs{
			Selector: metav1.LabelSelectorArgs{
				MatchLabels: labels,
			},
			Replicas: pulumi.Int(1),
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{
					Namespace: ns,
					Labels:    labels,
				},
				Spec: &corev1.PodSpecArgs{
					Containers: corev1.ContainerArray{
						corev1.ContainerArgs{
							Name:  pulumi.String(args.Identity),
							Image: pulumi.String(args.Image),
							Ports: corev1.ContainerPortArray{
								corev1.ContainerPortArgs{
									ContainerPort: pulumi.Int(args.Port),
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

	// Basic exposure
	svcType := pulumi.StringPtr("")
	if args.ExposeType == ExposeNodePort {
		svcType = pulumi.StringPtr("NodePort")
	}
	svcName := pulumi.String("emp-svc-" + args.Identity)
	emp.svc, err = corev1.NewService(ctx, "emp-svc-"+args.Identity, &corev1.ServiceArgs{
		Metadata: metav1.ObjectMetaArgs{
			Labels:    labels,
			Name:      svcName,
			Namespace: ns,
		},
		Spec: &corev1.ServiceSpecArgs{
			Type:     svcType,
			Selector: labels,
			Ports: corev1.ServicePortArray{
				corev1.ServicePortArgs{
					TargetPort: pulumi.Int(args.Port),
					Port:       pulumi.Int(args.Port),
				},
			},
		},
	}, opts...)
	if err != nil {
		return
	}

	// Specific exposures
	switch args.ExposeType {
	case ExposeIngress:
		emp.ing, err = netwv1.NewIngress(ctx, "emp-ing-"+args.Identity, &netwv1.IngressArgs{
			Metadata: metav1.ObjectMetaArgs{
				Labels:    labels,
				Name:      pulumi.String("emp-ing-" + args.Identity),
				Namespace: ns,
				Annotations: pulumi.ToStringMap(map[string]string{
					"traefik.ingress.kubernetes.io/router.entrypoints": "web", // TODO make this configurable
					"pulumi.com/skipAwait":                             "true",
				}),
			},
			Spec: netwv1.IngressSpecArgs{
				Rules: netwv1.IngressRuleArray{
					netwv1.IngressRuleArgs{
						Host: pulumi.Sprintf("%s.%s", args.Identity, args.Hostname),
						Http: netwv1.HTTPIngressRuleValueArgs{
							Paths: netwv1.HTTPIngressPathArray{
								netwv1.HTTPIngressPathArgs{
									Path:     pulumi.String("/"),
									PathType: pulumi.String("Prefix"),
									Backend: netwv1.IngressBackendArgs{
										Service: netwv1.IngressServiceBackendArgs{
											Name: svcName,
											Port: netwv1.ServiceBackendPortArgs{
												Number: pulumi.Int(args.Port),
											},
										},
									},
								},
							},
						},
					},
				},
				// Tls: netwv1.IngressTLSArray{
				// 	netwv1.IngressTLSArgs{
				// 		// XXX remove this or handle it properly
				// 		// The TLS secret is defered to Traefik for TLS unpacking
				// 		// SecretName: pulumi.String("domain-tls-secret"),
				// 	},
				// },
			},
		}, opts...)
		if err != nil {
			return err
		}
	}

	emp.ntp, err = netwv1.NewNetworkPolicy(ctx, "emp-ntp-"+args.Identity, &netwv1.NetworkPolicyArgs{
		Metadata: metav1.ObjectMetaArgs{
			Labels:    labels,
			Name:      pulumi.String("emp-ntp-" + args.Identity),
			Namespace: ns,
		},
		Spec: netwv1.NetworkPolicySpecArgs{
			PodSelector: metav1.LabelSelectorArgs{
				MatchLabels: labels,
			},
			PolicyTypes: pulumi.ToStringArray([]string{
				"Ingress",
			}),
			Ingress: netwv1.NetworkPolicyIngressRuleArray{
				netwv1.NetworkPolicyIngressRuleArgs{
					From: netwv1.NetworkPolicyPeerArray{
						netwv1.NetworkPolicyPeerArgs{
							IpBlock: netwv1.IPBlockPtr(&netwv1.IPBlockArgs{
								Cidr: pulumi.String(args.FromCIDR),
							}),
						},
					},
					Ports: netwv1.NetworkPolicyPortArray{
						netwv1.NetworkPolicyPortArgs{
							Port: pulumi.Int(args.Port),
						},
					},
				},
			},
		},
	}, opts...)
	if err != nil {
		return err
	}

	return nil
}

func (emp *ExposedMonopod) outputs(args *ExposedMonopodArgs) {
	switch args.ExposeType {
	case ExposeNodePort:
		emp.URL = pulumi.Sprintf("%s:%d", args.Hostname, args.Port)
	case ExposeIngress:
		emp.URL = pulumi.Sprintf("%s.%s", args.Identity, args.Hostname)
	}
}
