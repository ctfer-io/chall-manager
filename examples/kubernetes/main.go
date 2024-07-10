package main

import (
	"fmt"

	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

const internalPort = 8080

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		cfg := config.New(ctx, "kubernetes")
		config := map[string]string{
			"identity": cfg.Get("identity"),
		}

		labels := pulumi.ToStringMap(map[string]string{
			"identity":  config["identity"],
			"category":  "crypto",
			"challenge": "license-lvl1",
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
										ContainerPort: pulumi.Int(internalPort),
									},
								},
								Env: corev1.EnvVarArray{
									corev1.EnvVarArgs{
										Name:  pulumi.String("PORT"),
										Value: pulumi.Sprintf("%d", internalPort),
									},
								},
							},
						},
					},
				},
			},
		}); err != nil {
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
						Port:       pulumi.Int(internalPort),
						TargetPort: pulumi.Int(internalPort),
					},
				},
			},
		})
		if err != nil {
			return err
		}

		ctx.Export("connection_info", svc.Spec.ApplyT(func(spec *corev1.ServiceSpec) string {
			np := spec.Ports[0].NodePort
			if np == nil {
				return ""
			}
			return fmt.Sprintf("http://brefctf.ctfer-io.lab:%d", *np)
		}).(pulumi.StringOutput))

		return nil
	})
}
