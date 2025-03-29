package kubernetes

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math/rand"
	"path/filepath"
	"slices"
	"sync"

	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	netwv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/networking/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"go.uber.org/multierr"
)

type (
	ExposedMonopod struct {
		pulumi.ResourceState

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
		envs corev1.EnvVarArrayOutput

		// Files is a list of additional files to inject in the pod
		// filesystem on the pods. Key is the file absolute path,
		// value is its content.
		// Can be used to provision a per-instance flag.
		//
		// WARNING: provisionning a file in a directory makes adjacent
		// files unavailable.
		// For more info, refer to https://kubernetes.io/docs/tasks/configure-pod-container/configure-pod-configmap/#populate-a-volume-with-data-stored-in-a-configmap
		Files pulumi.StringMapInput

		// FromCIDR can be configured to specify an IP range that will
		// be able to access the pod.
		// TODO @NicoFgrx support it when ExposeIngress too
		FromCIDR pulumi.StringPtrInput
		fromCIDR pulumi.StringOutput

		ExposeType ExposeType

		// IngressAnnotations is a set of additional annotations to
		// put on the ingress, if the `ExposeType` is set to
		// `ExposeIngress`.
		IngressAnnotations pulumi.StringMapInput
		ingressAnnotations pulumi.StringMapOutput

		// IngressNamespace must be configured to the namespace in
		// which the ingress (e.g. nginx, traefik) is deployed.
		IngressNamespace pulumi.StringInput

		// IngressLabels must be configured to the labels of the ingress
		// pods (e.g. app=traefik, ...).
		IngressLabels pulumi.StringMapInput

		// LimitCPU is an optional value, yet recommended to avoid resources exhaustion
		// i.e. DoS.
		// It defines the limit of CPU time allocated to this Pod.
		LimitCPU pulumi.StringInput

		// LimitMemory is an optional value, yet recommended to avoid resources
		// exhaustion i.e. DoS.
		// It defines the limit of RAM memory allocated to this Pod.
		LimitMemory pulumi.StringInput
	}

	ExposeType string
)

const (
	ExposeNodePort ExposeType = "NodePort"
	ExposeIngress  ExposeType = "Ingress"

	defaultCIDR = "0.0.0.0/0"
)

// NewExposedMonopod builds the Kubernetes resources for an [*ExposedMonopod].
// It fits the best cases of web or pwn challenges where only 1 pod is required.
func NewExposedMonopod(ctx *pulumi.Context, name string, args *ExposedMonopodArgs, opts ...pulumi.ResourceOption) (*ExposedMonopod, error) {
	emp := &ExposedMonopod{}

	args = emp.defaults(args)
	if err := emp.check(args); err != nil {
		return nil, err
	}
	if err := ctx.RegisterComponentResource("ctfer-io:chall-manager/sdk:kubernetes.ExposedMonopod", name, emp, opts...); err != nil {
		return nil, err
	}
	opts = append(opts, pulumi.Parent(emp))
	if err := emp.provision(ctx, args, opts...); err != nil {
		return nil, err
	}
	if err := emp.outputs(ctx, args); err != nil {
		return nil, err
	}
	return emp, nil
}

func (emp *ExposedMonopod) defaults(args *ExposedMonopodArgs) *ExposedMonopodArgs {
	if args == nil {
		args = &ExposedMonopodArgs{}
	}
	if args.Identity == nil {
		args.Identity = pulumi.String("")
	}
	if args.Hostname == nil {
		args.Hostname = pulumi.String("")
	}
	if args.Image == nil {
		args.Image = pulumi.String("")
	}
	if args.Port == nil {
		args.Port = pulumi.Int(0)
	}
	if args.ExposeType == "" {
		args.ExposeType = ExposeNodePort
	}

	args.envs = corev1.EnvVarArrayOutput{}
	if args.Envs != nil {
		args.envs = args.Envs.ToStringMapOutput().ApplyT(func(envs map[string]string) []corev1.EnvVar {
			outs := make([]corev1.EnvVar, 0, len(envs))
			for k, v := range envs {
				outs = append(outs, corev1.EnvVar{
					Name:  k,
					Value: &v,
				})
			}
			return outs
		}).(corev1.EnvVarArrayOutput)
	}

	args.ingressAnnotations = pulumi.StringMapOutput{}
	if args.IngressAnnotations != nil {
		args.ingressAnnotations = args.IngressAnnotations.ToStringMapOutput().ApplyT(func(annotations map[string]string) map[string]string {
			// Do not wait for an IP, it could be provided without Pulumi being aware
			annotations["pulumi.com/skipAwait"] = "true"
			return annotations
		}).(pulumi.StringMapOutput)
	}

	args.fromCIDR = pulumi.String(defaultCIDR).ToStringOutput()
	if args.FromCIDR != nil {
		args.fromCIDR = args.FromCIDR.ToStringPtrOutput().ApplyT(func(cidr *string) string {
			if cidr == nil || *cidr == "" {
				return defaultCIDR
			}
			return *cidr
		}).(pulumi.StringOutput)
	}
	return args
}

func (emp *ExposedMonopod) check(args *ExposedMonopodArgs) error {
	wg := &sync.WaitGroup{}
	checks := 4 // number of checks to perform
	wg.Add(checks)
	cerr := make(chan error, checks)

	args.Identity.ToStringOutput().ApplyT(func(id string) error {
		defer wg.Done()

		if id == "" {
			cerr <- errors.New("identity could not be empty")
		}
		return nil
	})
	args.Hostname.ToStringOutput().ApplyT(func(hostname string) error {
		defer wg.Done()

		if hostname == "" {
			cerr <- errors.New("hostname could not be empty")
		}
		return nil
	})
	args.Image.ToStringOutput().ApplyT(func(image string) error {
		defer wg.Done()

		if image == "" {
			cerr <- errors.New("image could not be empty")
		}
		return nil
	})
	args.Port.ToIntOutput().ApplyT(func(port int) error {
		defer wg.Done()

		if port < 1 || port > 65535 {
			cerr <- fmt.Errorf("port %d is out of bounds", port)
		}
		return nil
	})

	wg.Wait()
	close(cerr)

	var merr error
	for err := range cerr {
		merr = multierr.Append(merr, err)
	}
	if !slices.Contains([]ExposeType{ExposeNodePort, ExposeIngress}, args.ExposeType) {
		merr = multierr.Append(merr, fmt.Errorf("unsupported expose type, got %s", args.ExposeType))
	}
	return merr
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
					if lbl, ok := all[1].(string); ok && lbl != "" {
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
				if lbl, ok := all[1].(string); ok && lbl != "" {
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
							Env:          args.envs,
							VolumeMounts: vmounts,
							Resources: corev1.ResourceRequirementsArgs{
								Limits: pulumi.All(args.LimitCPU, args.LimitMemory).ApplyT(func(all []any) (out map[string]string) {
									if all[0] != nil {
										if cpu, ok := all[0].(*string); ok && *cpu != "" {
											out["cpu"] = *cpu
										}
									}
									if all[1] != nil {
										if memory, ok := all[1].(*string); ok && *memory != "" {
											out["memory"] = *memory
										}
									}
									return
								}).(pulumi.StringMapOutput),
							},
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
				if lbl, ok := all[1].(string); ok && lbl != "" {
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
	case ExposeNodePort:
		emp.ntp, err = netwv1.NewNetworkPolicy(ctx, "emp-ntp", &netwv1.NetworkPolicyArgs{
			Metadata: metav1.ObjectMetaArgs{
				Labels: labels,
				Name: pulumi.All(args.Identity, args.Label).ApplyT(func(all []any) string {
					id := all[0].(string)
					if lbl, ok := all[1].(string); ok && lbl != "" {
						return fmt.Sprintf("emp-ntp-%s-%s", lbl, id)
					}
					return fmt.Sprintf("emp-ntp-%s", id)
				}).(pulumi.StringOutput),
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

	case ExposeIngress:
		emp.ing, err = netwv1.NewIngress(ctx, "emp-ing", &netwv1.IngressArgs{
			Metadata: metav1.ObjectMetaArgs{
				Labels: labels,
				Name: pulumi.All(args.Identity, args.Label).ApplyT(func(all []any) string {
					id := all[0].(string)
					if lbl, ok := all[1].(string); ok && lbl != "" {
						return fmt.Sprintf("emp-ing-%s-%s", lbl, id)
					}
					return fmt.Sprintf("emp-ing-%s", id)
				}).(pulumi.StringOutput),
				Annotations: args.ingressAnnotations,
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

		emp.ntp, err = netwv1.NewNetworkPolicy(ctx, "emp-ntp", &netwv1.NetworkPolicyArgs{
			Metadata: metav1.ObjectMetaArgs{
				Labels: labels,
				Name: pulumi.All(args.Identity, args.Label).ApplyT(func(all []any) string {
					id := all[0].(string)
					if lbl, ok := all[1].(string); ok && lbl != "" {
						return fmt.Sprintf("emp-ntp-%s-%s", lbl, id)
					}
					return fmt.Sprintf("emp-ntp-%s", id)
				}).(pulumi.StringOutput),
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
								NamespaceSelector: metav1.LabelSelectorArgs{
									MatchLabels: pulumi.StringMap{
										"kubernetes.io/metadata.name": args.IngressNamespace,
									},
								},
								PodSelector: metav1.LabelSelectorArgs{
									MatchLabels: args.IngressLabels,
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
			return
		}
	}

	return nil
}

func (emp *ExposedMonopod) outputs(ctx *pulumi.Context, args *ExposedMonopodArgs) error {
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

	return ctx.RegisterResourceOutputs(emp, pulumi.Map{
		"url": emp.URL,
	})
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
