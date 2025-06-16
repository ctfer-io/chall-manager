package main

import (
	"fmt"
	"os"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	netwv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/networking/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

const port = 8080

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		cfg := config.New(ctx, "kubernetes")
		config := map[string]string{
			"identity": cfg.Get("identity"),
		}

		opts := []pulumi.ResourceOption{}
		if k8sns, ok := os.LookupEnv("KUBERNETES_TARGET_NAMESPACE"); ok {
			pv, err := kubernetes.NewProvider(ctx, "target", &kubernetes.ProviderArgs{
				Namespace: pulumi.String(k8sns),
			})
			if err != nil {
				return err
			}
			opts = append(opts, pulumi.Provider(pv))
		}

		labels := pulumi.ToStringMap(map[string]string{
			"chall-manager.ctfer.io/identity": config["identity"],
			"category":                        "crypto",
			"challenge":                       "license-lvl1",
		})
		if _, err := appsv1.NewDeployment(ctx, "example", &appsv1.DeploymentArgs{
			Metadata: metav1.ObjectMetaArgs{
				Labels: labels,
			},
			Spec: appsv1.DeploymentSpecArgs{
				Selector: metav1.LabelSelectorArgs{
					MatchLabels: labels,
				},
				Template: corev1.PodTemplateSpecArgs{
					Metadata: metav1.ObjectMetaArgs{
						Labels: labels,
					},
					Spec: corev1.PodSpecArgs{
						Containers: corev1.ContainerArray{
							corev1.ContainerArgs{
								Name:  pulumi.String("license-lvl1"),
								Image: pulumi.String("pandatix/license-lvl1:latest"),
								Ports: corev1.ContainerPortArray{
									corev1.ContainerPortArgs{
										ContainerPort: pulumi.Int(port),
									},
								},
								Env: corev1.EnvVarArray{
									corev1.EnvVarArgs{
										Name:  pulumi.String("PORT"),
										Value: pulumi.Sprintf("%d", port),
									},
								},
							},
						},
					},
				},
			},
		}, opts...); err != nil {
			return err
		}

		svc, err := corev1.NewService(ctx, "example", &corev1.ServiceArgs{
			Metadata: metav1.ObjectMetaArgs{
				Labels: labels,
			},
			Spec: corev1.ServiceSpecArgs{
				Selector: labels,
				Type:     pulumi.String("NodePort"),
				Ports: corev1.ServicePortArray{
					corev1.ServicePortArgs{
						Port:       pulumi.Int(port),
						TargetPort: pulumi.Int(port),
					},
				},
			},
		}, opts...)
		if err != nil {
			return err
		}

		// Don't forget to expose the pod for outer trafic !
		if _, err = netwv1.NewNetworkPolicy(ctx, "example", &netwv1.NetworkPolicyArgs{
			Metadata: metav1.ObjectMetaArgs{
				Labels: labels,
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
								IpBlock: netwv1.IPBlockArgs{
									Cidr: pulumi.String("0.0.0.0/0"),
								},
							},
						},
						Ports: netwv1.NetworkPolicyPortArray{
							netwv1.NetworkPolicyPortArgs{
								Port: pulumi.Int(port),
							},
						},
					},
				},
			},
		}, opts...); err != nil {
			return err
		}

		ctx.Export("connection_info", svc.Spec.ApplyT(func(spec corev1.ServiceSpec) string {
			np := spec.Ports[0].NodePort
			if np == nil {
				return ""
			}
			return fmt.Sprintf("http://brefctf.ctfer.io:%d", *np)
		}).(pulumi.StringOutput))

		return nil
	})
}
