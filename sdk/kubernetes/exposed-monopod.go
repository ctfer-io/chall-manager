package kubernetes

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/rand"
	"path/filepath"

	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	netwv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/networking/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type (
	ExposedMonopod struct {
		cfg *corev1.ConfigMap
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

		Identity pulumi.StringInput
		Label    pulumi.StringInput
		Hostname pulumi.StringInput

		// Kubernetes attributes

		Image pulumi.StringInput
		Port  pulumi.IntInput
		// Envs is a list of additional environment variables to
		// set on the pods. Key is the varenv name, value is its
		// value.
		// Can be used to provision a per-instance flag.
		Envs pulumi.StringMapInput
		// Files is a list of additional files to inject in the pod
		// filesystem on the pods. Key is the file absolute path,
		// value is its content.
		// Can be used to provision a per-instance flag.
		Files    pulumi.StringMapInput
		FromCIDR pulumi.StringPtrInput
		fromCIDR pulumi.StringOutput

		ExposeType ExposeType

		// IngressAnnotations is a set of additional annotations to
		// put on the ingress, if the `ExposeType` is set to
		// `ExposeIngress`.
		IngressAnnotations pulumi.StringMapInput
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
	if args.Envs == nil {
		args.Envs = pulumi.StringMap{}
	}
	if args.FromCIDR == nil || args.FromCIDR == pulumi.String("") {
		args.fromCIDR = pulumi.String("0.0.0.0/0").ToStringOutput()
	} else {
		args.fromCIDR = args.FromCIDR.ToStringPtrOutput().Elem()
	}
	if args.IngressAnnotations == nil {
		args.IngressAnnotations = pulumi.StringMap{}
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
	labels := pulumi.StringMap{
		"chall-manager.ctfer.io/kind":     pulumi.String("exposed-monopod"),
		"chall-manager.ctfer.io/identity": args.Identity,
	}

	// => ConfigMap
	var vmounts corev1.VolumeMountArrayOutput
	var vs corev1.VolumeArrayOutput
	if args.Files != nil {
		emp.cfg, err = corev1.NewConfigMap(ctx, "emp-cfg", &corev1.ConfigMapArgs{
			Immutable: pulumi.BoolPtr(true),
			Metadata: metav1.ObjectMetaArgs{
				Name: pulumi.All(args.Identity, args.Label).ApplyT(func(all []any) string {
					id := all[0].(string)
					lbl := all[1].(string)

					if lbl != "" {
						return fmt.Sprintf("emp-cfg-%s-%s", lbl, id)
					}
					return fmt.Sprintf("emp-cfg-%s", id)
				}).(pulumi.StringOutput),
				Labels: labels,
			},
			Data: args.Files.ToStringMapOutput().ApplyT(func(mp map[string]string) map[string]string {
				out := map[string]string{}
				for dst, content := range mp {
					out[randName(dst)] = content
				}
				return out
			}).(pulumi.StringMapOutput),
		}, opts...)
		if err != nil {
			return
		}

		vmounts = args.Files.ToStringMapOutput().ApplyT(func(mp map[string]string) []corev1.VolumeMount {
			vmounts := make([]corev1.VolumeMount, 0, len(mp))
			for dst := range mp {
				vmounts = append(vmounts, corev1.VolumeMount{
					Name:      randName(dst),
					MountPath: filepath.Dir(dst),
					ReadOnly:  ptr(true), // injected files should not be mutated, else already handled by the challenge
				})
			}
			return vmounts
		}).(corev1.VolumeMountArrayOutput)
		vs = pulumi.All(args.Files, emp.cfg.Metadata).ApplyT(func(all []any) []corev1.Volume {
			mp := all[0].(map[string]string)
			cfgMeta := all[1].(metav1.ObjectMeta)

			vs := make([]corev1.Volume, 0, len(mp))
			for dst := range mp {
				rn := randName(dst)
				vs = append(vs, corev1.Volume{
					Name: rn,
					ConfigMap: &corev1.ConfigMapVolumeSource{
						Name:        cfgMeta.Name,
						DefaultMode: ptr(0444), // -r--r--r--
						Items: []corev1.KeyToPath{
							{
								Key:  rn,
								Path: filepath.Base(dst),
							},
						},
					},
				})
			}
			return vs
		}).(corev1.VolumeArrayOutput)
	}

	// => Deployment
	emp.dep, err = appsv1.NewDeployment(ctx, "emp-dep", &appsv1.DeploymentArgs{
		Metadata: metav1.ObjectMetaArgs{
			Name: pulumi.All(args.Identity, args.Label).ApplyT(func(all []any) string {
				id := all[0].(string)
				lbl := all[1].(string)

				if lbl != "" {
					return fmt.Sprintf("emp-dep-%s-%s", lbl, id)
				}
				return fmt.Sprintf("emp-dep-%s", id)
			}).(pulumi.StringOutput),
			Labels: labels,
		},
		Spec: appsv1.DeploymentSpecArgs{
			Selector: metav1.LabelSelectorArgs{
				MatchLabels: labels,
			},
			Replicas: pulumi.Int(1),
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{
					Labels: labels,
				},
				Spec: &corev1.PodSpecArgs{
					Containers: corev1.ContainerArray{
						corev1.ContainerArgs{
							Name:  args.Identity,
							Image: args.Image,
							Ports: corev1.ContainerPortArray{
								corev1.ContainerPortArgs{
									ContainerPort: args.Port,
								},
							},
							Env: args.Envs.ToStringMapOutput().ApplyT(func(envs map[string]string) []corev1.EnvVar {
								outs := make([]corev1.EnvVar, 0, len(envs))
								for k, v := range envs {
									outs = append(outs, corev1.EnvVar{
										Name:  k,
										Value: &v,
									})
								}
								return outs
							}).(corev1.EnvVarArrayOutput),
							VolumeMounts: vmounts,
						},
					},
					Volumes: vs,
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
	emp.svc, err = corev1.NewService(ctx, "emp-svc", &corev1.ServiceArgs{
		Metadata: metav1.ObjectMetaArgs{
			Labels: labels,
			Name: pulumi.All(args.Identity, args.Label).ApplyT(func(all []any) string {
				id := all[0].(string)
				lbl := all[1].(string)

				if lbl != "" {
					return fmt.Sprintf("emp-svc-%s-%s", lbl, id)
				}
				return fmt.Sprintf("emp-svc-%s", id)
			}).(pulumi.StringOutput),
		},
		Spec: &corev1.ServiceSpecArgs{
			Type:     svcType,
			Selector: labels,
			Ports: corev1.ServicePortArray{
				corev1.ServicePortArgs{
					TargetPort: args.Port,
					Port:       args.Port,
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
		emp.ing, err = netwv1.NewIngress(ctx, "emp-ing", &netwv1.IngressArgs{
			Metadata: metav1.ObjectMetaArgs{
				Labels: labels,
				Name: pulumi.All(args.Identity, args.Label).ApplyT(func(all []any) string {
					id := all[0].(string)
					lbl := all[1].(string)

					if lbl != "" {
						return fmt.Sprintf("emp-ing-%s-%s", lbl, id)
					}
					return fmt.Sprintf("emp-ing-%s", id)
				}).(pulumi.StringOutput),
				Annotations: args.IngressAnnotations.ToStringMapOutput().ApplyT(func(annotations map[string]string) map[string]string {
					annotations["pulumi.com/skipAwait"] = "true"
					return annotations
				}).(pulumi.StringMapOutput),
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
											Name: emp.svc.Metadata.Name().Elem(),
											Port: netwv1.ServiceBackendPortArgs{
												Number: args.Port,
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
			return err
		}
	}

	emp.ntp, err = netwv1.NewNetworkPolicy(ctx, "emp-ntp", &netwv1.NetworkPolicyArgs{
		Metadata: metav1.ObjectMetaArgs{
			Labels: labels,
			Name:   pulumi.Sprintf("emp-ntp-%s", args.Identity),
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
							IpBlock: &netwv1.IPBlockArgs{
								Cidr: args.fromCIDR,
							},
						},
					},
					Ports: netwv1.NetworkPolicyPortArray{
						netwv1.NetworkPolicyPortArgs{
							Port: args.Port,
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
		np := emp.svc.Spec.ApplyT(func(spec corev1.ServiceSpec) int {
			if spec.Ports[0].NodePort == nil {
				return 0
			}
			return *spec.Ports[0].NodePort
		}).(pulumi.IntOutput)
		emp.URL = pulumi.Sprintf("%s:%d", args.Hostname, np)
	case ExposeIngress:
		emp.URL = pulumi.Sprintf("%s.%s", args.Identity, args.Hostname)
	}
}

func randName(seed string) string {
	sb := []byte(seed)
	s := int64(binary.BigEndian.Uint64(sb))
	prng := rand.New(rand.NewSource(s))

	b := make([]byte, 8)
	_, _ = prng.Read(b)
	return hex.EncodeToString(b)
}

func ptr[T any](t T) *T {
	return &t
}
