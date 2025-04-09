package kubernetes

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	netwv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/networking/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"go.uber.org/multierr"
)

// NewExposedMultipod builds the Kubernetes resources for an [*ExposedMultipod].
// It fits the best cases of advanced web setups, cloud-based infrastructures, etc.
func NewExposedMultipod(ctx *pulumi.Context, name string, args *ExposedMultipodArgs, opts ...pulumi.ResourceOption) (*ExposedMultipod, error) {
	emp := &ExposedMultipod{}
	if err := ctx.RegisterComponentResource("ctfer-io:chall-manager/sdk:kubernetes.ExposedMultipod", name, emp, opts...); err != nil {
		return nil, err
	}
	opts = append(opts, pulumi.Parent(emp))

	sub, err := newExposedMultipod(ctx, args, opts...)
	if err != nil {
		return nil, err
	}
	emp.sub = sub

	emp.URLs = emp.sub.URLs
	if err := ctx.RegisterResourceOutputs(emp, pulumi.Map{
		"urls": emp.URLs,
	}); err != nil {
		return nil, err
	}

	return emp, nil
}

type (
	ExposedMultipod struct {
		pulumi.ResourceState

		sub *exposedMultipod

		// URLs is a map of URL exposed by a container, identified by its name.
		URLs pulumi.StringMapMapOutput
	}

	ExposedMultipodArgs struct {
		// Challenge instance attributes

		Identity pulumi.StringInput
		Label    pulumi.StringInput
		Hostname pulumi.StringInput

		// Containers to deploy in the exposed network.
		Containers ContainerMapInput

		// Rules of networking between containers.
		// They define the allowed traffic enforced by network policies.
		Rules RuleArrayInput

		// FromCIDR can be configured to specify an IP range that will
		// be able to access the pod.
		// TODO @NicoFgrx support it when ExposeIngress too
		FromCIDR pulumi.StringPtrInput
		fromCIDR pulumi.StringOutput

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
	}
)

type exposedMultipod struct {
	cfgs     []*corev1.ConfigMap
	deps     appsv1.DeploymentMapOutput
	svcs     ServiceMapMapOutput
	svcMetas ObjectMetaMapMapOutput
	svcSpecs ServiceSpecMapMapOutput
	ings     IngressMapMapOutput
	ingSpecs IngressSpecMapMapOutput
	ntps     []*netwv1.NetworkPolicy

	// URLs is a map of URL exposed by a container, identified by its name.
	URLs pulumi.StringMapMapOutput
}

func newExposedMultipod(ctx *pulumi.Context, args *ExposedMultipodArgs, opts ...pulumi.ResourceOption) (*exposedMultipod, error) {
	emp := &exposedMultipod{
		// Must init map else nil-pointer dereference
		deps:     appsv1.DeploymentMap{}.ToDeploymentMapOutput(),
		svcs:     ServiceMapMap{}.ToServiceMapMapOutput(),
		svcMetas: ObjectMetaMapMap{}.ToObjectMetaMapMapOutput(),
		svcSpecs: ServiceSpecMapMap{}.ToServiceSpecMapMapOutput(),
		ings:     IngressMapMap{}.ToIngressMapMapOutput(),
		ingSpecs: IngressSpecMapMap{}.ToIngressSpecMapMapOutput(),
		URLs:     pulumi.StringMapMap{}.ToStringMapMapOutput(),
	}

	args = emp.defaults(args)
	if err := emp.check(args); err != nil {
		return nil, err
	}
	if err := emp.provision(ctx, args, opts...); err != nil {
		return nil, err
	}
	emp.outputs(ctx, args)

	return emp, nil
}

func (emp *exposedMultipod) defaults(args *ExposedMultipodArgs) *ExposedMultipodArgs {
	if args == nil {
		args = &ExposedMultipodArgs{}
	}
	if args.Identity == nil {
		args.Identity = pulumi.String("")
	}
	if args.Hostname == nil {
		args.Hostname = pulumi.String("")
	}
	if args.Containers == nil {
		args.Containers = ContainerMap{}
	}
	if args.Rules == nil {
		args.Rules = RuleArray{}
	}

	args.ingressAnnotations = pulumi.StringMap{}.ToStringMapOutput()
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

func (emp *exposedMultipod) check(args *ExposedMultipodArgs) error {
	wg := &sync.WaitGroup{}
	checks := 6 // number of checks to perform
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
	args.Containers.ToContainerMapOutput().ApplyT(func(containers map[string]Container) (merr error) {
		defer wg.Done()

		for _, c := range containers {
			merr = multierr.Append(merr, c.Check())
		}
		cerr <- merr
		return nil
	})
	// Check if rules target existing containers
	pulumi.All(args.Containers, args.Rules).ApplyT(func(all []any) (merr error) {
		defer wg.Done()

		containers := all[0].(map[string]Container)
		rules := all[1].([]Rule)

		for _, rule := range rules {
			rp := rule.Protocol
			if rp == "" {
				rp = "TCP"
			}
			if _, ok := containers[rule.From]; !ok {
				merr = multierr.Append(merr, fmt.Errorf(
					"from %[1]s to %[2]s, %[3]d/%[4]s, %[1]s not found",
					rule.From, rule.To, rule.On, rp,
				))
			}
			to, ok := containers[rule.To]
			if !ok {
				merr = multierr.Append(merr,
					fmt.Errorf(
						"from %[1]s to %[2]s, %[3]d/%[4]s, %[2]s not found",
						rule.From, rule.To, rule.On, rp,
					),
				)
			} else {
				found := false
				for _, p := range to.Ports {
					prot := p.Protocol
					if prot == "" {
						prot = "TCP"
					}
					if rule.On == p.Port && rp == prot {
						found = true
						break
					}
				}
				if !found {
					merr = multierr.Append(merr,
						fmt.Errorf(
							"from %[1]s to %[2]s, %[3]d/%[4]s not found exposed by %[2]s",
							rule.From, rule.To, rule.On, rule.Protocol,
						),
					)
				}
			}
		}
		cerr <- merr
		return nil
	})
	// Ensure DAG of printer's services
	args.Containers.ToContainerMapOutput().ApplyT(func(containers map[string]Container) error {
		defer wg.Done()

		// Construct functional dependency graph declared through printer
		// dependencies.
		depMap := map[string]map[string]struct{}{}
		for name, container := range containers {
			if _, ok := depMap[name]; !ok {
				depMap[name] = map[string]struct{}{}
			}
			for _, env := range container.Envs {
				for _, svc := range env.Services {
					// Keep only the container name, thus drop potential port binding
					svcName, _, _ := strings.Cut(svc, ":")

					depMap[name][svcName] = struct{}{}
					if _, ok := depMap[svcName]; !ok {
						depMap[svcName] = map[string]struct{}{}
					}
				}
			}
		}

		// Build dependency graph
		dcs := []*DepCon{}
		for name, d := range depMap {
			deps := []string{}
			for dep := range d {
				deps = append(deps, dep)
			}
			dcs = append(dcs, &DepCon{
				name: name,
				deps: deps,
			})
		}

		// Topological sort
		_, err := Sort(dcs)
		cerr <- err
		return nil
	})
	// Ensure printer references existing <container,port,protocol>
	args.Containers.ToContainerMapOutput().ApplyT(func(containers map[string]Container) (merr error) {
		defer wg.Done()

		for _, container := range containers {
			for _, printer := range container.Envs {
				for _, name := range printer.Services {
					// Get service by name
					svcName, pb, _ := strings.Cut(name, ":")
					svc, ok := containers[svcName]
					if !ok {
						merr = multierr.Append(merr, fmt.Errorf("printer reference unexisting container %s", svcName))
						continue
					}

					// If no port binding specified, with only 1 port, will be infered.
					// Works when container is referenced only by its name.
					if pb == "" && len(svc.Ports) == 1 {
						continue
					}

					// Look for corresponding port binding
					port, prot, _ := strings.Cut(pb, "/")
					if prot == "" {
						prot = "TCP"
					}

					found := false
					for _, pb := range svc.Ports {
						pbProt := pb.Protocol
						if pbProt == "" {
							pbProt = "TCP"
						}
						if strconv.Itoa(pb.Port) == port && pbProt == prot {
							found = true
							break
						}
					}
					if !found {
						merr = multierr.Append(merr, fmt.Errorf("container %s does not contain a port binding on %s/%s", svcName, port, prot))
					}
				}
			}
		}
		cerr <- merr
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

func (emp *exposedMultipod) provision(ctx *pulumi.Context, args *ExposedMultipodArgs, opts ...pulumi.ResourceOption) (err error) {
	// Start with a topological sort of containers to ensure proper dependency
	// resolution.
	// It ensures there is no cycles that will stuck the dynamic resolver,
	// leading to Denial of Service.
	order := args.Containers.ToContainerMapOutput().ApplyT(func(containers map[string]Container) []string {
		// Construct functional dependency graph declared through printer dependencies.
		depMap := map[string]map[string]struct{}{}
		for name, container := range containers {
			if _, ok := depMap[name]; !ok {
				depMap[name] = map[string]struct{}{}
			}
			for _, env := range container.Envs {
				for _, svc := range env.Services {
					// Keep only the container name, thus drop potential port binding
					svcName, _, _ := strings.Cut(svc, ":")

					depMap[name][svcName] = struct{}{}
					if _, ok := depMap[svcName]; !ok {
						depMap[svcName] = map[string]struct{}{}
					}
				}
			}
		}

		// Build dependency graph
		dcs := []*DepCon{}
		for name, d := range depMap {
			deps := []string{}
			for dep := range d {
				deps = append(deps, dep)
			}
			dcs = append(dcs, &DepCon{
				name: name,
				deps: deps,
			})
		}

		// Topological sort
		dcs, _ = Sort(dcs)

		// Output order
		out := make([]string, len(dcs))
		for i, d := range dcs {
			out[i] = d.name
		}
		return out
	}).(pulumi.StringArrayOutput)

	for i := 0; i < lenP(order); i++ {
		name := order.Index(pulumi.Int(i))
		rawName := raw(name)
		container := args.Containers.ToContainerMapOutput().MapIndex(name)

		// Uniquely identify the resources with labels
		labels := pulumi.StringMap{
			"chall-manager.ctfer.io/kind":     pulumi.String("exposed-multipod"),
			"chall-manager.ctfer.io/identity": args.Identity,
			"app.kubernetes.io/name":          name,
		}

		// => ConfigMap
		var vmounts corev1.VolumeMountArrayOutput
		var vs corev1.VolumeArrayOutput
		if container.HasFiles() {
			cfg, err := corev1.NewConfigMap(ctx, fmt.Sprintf("emp-cfg-%s", rawName), &corev1.ConfigMapArgs{
				Immutable: pulumi.BoolPtr(true),
				Metadata: metav1.ObjectMetaArgs{
					Name: pulumi.All(args.Identity, args.Label, name).ApplyT(func(all []any) string {
						id := all[0].(string)
						name := all[2].(string)
						if lbl, ok := all[1].(string); ok && lbl != "" {
							return fmt.Sprintf("emp-cfg-%s-%s-%s", lbl, id, name)
						}
						return fmt.Sprintf("emp-cfg-%s-%s", id, name)
					}).(pulumi.StringOutput),
					Labels: labels,
				},
				Data: container.Files().ApplyT(func(mp map[string]string) map[string]string {
					out := map[string]string{}
					for dst, content := range mp {
						out[randName(dst)] = content
					}
					return out
				}).(pulumi.StringMapOutput),
			}, opts...)
			if err != nil {
				return err
			}

			vmounts = container.Files().ApplyT(func(mp map[string]string) []corev1.VolumeMount {
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
			vs = pulumi.All(container.Files(), cfg.Metadata).ApplyT(func(all []any) []corev1.Volume {
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
			emp.cfgs = append(emp.cfgs, cfg)
		}

		// => Deployment
		dep, err := appsv1.NewDeployment(ctx, fmt.Sprintf("emp-dep-%s", rawName), &appsv1.DeploymentArgs{
			Metadata: metav1.ObjectMetaArgs{
				Name: pulumi.All(args.Identity, args.Label, name).ApplyT(func(all []any) string {
					id := all[0].(string)
					name := all[2].(string)
					if lbl, ok := all[1].(string); ok && lbl != "" {
						return fmt.Sprintf("emp-dep-%s-%s-%s", lbl, id, name)
					}
					return fmt.Sprintf("emp-dep-%s-%s", id, name)
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
								Image: container.Image(),
								Ports: container.Ports().ApplyT(func(pbs []PortBinding) []corev1.ContainerPort {
									out := make([]corev1.ContainerPort, 0, len(pbs))
									for _, pb := range pbs {
										if pb.Protocol == "" {
											pb.Protocol = "TCP"
										}
										out = append(out, corev1.ContainerPort{
											ContainerPort: pb.Port,
											Protocol:      &pb.Protocol,
										})
									}
									return out
								}).(corev1.ContainerPortArrayOutput),
								Env: container.Envs().Print(emp.svcMetas).ApplyT(func(mp map[string]string) []corev1.EnvVar {
									out := make([]corev1.EnvVar, 0, len(mp))
									for k, v := range mp {
										out = append(out, corev1.EnvVar{
											Name:  k,
											Value: ptr(v),
										})
									}
									return out
								}).(corev1.EnvVarArrayOutput),
								VolumeMounts: vmounts,
								Resources: corev1.ResourceRequirementsArgs{
									Limits: pulumi.All(container.LimitCPU(), container.LimitMemory()).ApplyT(func(all []any) map[string]string {
										out := map[string]string{}
										if cpu, ok := all[0].(*string); ok && cpu != nil && *cpu != "" {
											out["cpu"] = *cpu
										}
										if mem, ok := all[1].(*string); ok && mem != nil && *mem != "" {
											out["memory"] = *mem
										}
										return out
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
			return err
		}
		emp.deps = pulumi.All(emp.deps, name, dep).ApplyT(func(all []any) map[string]*appsv1.Deployment {
			deps := all[0].(map[string]*appsv1.Deployment)
			deps[all[1].(string)] = all[2].(*appsv1.Deployment)
			return deps
		}).(appsv1.DeploymentMapOutput)

		// Expose ports
		l := container.Ports().Len()
		svcs := corev1.ServiceMap{}.ToServiceMapOutput()
		svcMetas := ObjectMetaMap{}.ToObjectMetaMapOutput()
		svcSpecs := ServiceSpecMap{}.ToServiceSpecMapOutput()
		ings := netwv1.IngressMap{}.ToIngressMapOutput()
		ingSpecs := IngressSpecMap{}.ToIngressSpecMapOutput()
		for i := 0; i < l; i++ {
			p := container.Ports().Index(pulumi.Int(i))

			svcType := pulumi.String("")
			pet := p.ExposeType().Raw()
			if pet == ExposeNodePort {
				svcType = pulumi.String("NodePort")
			}

			svc, err := corev1.NewService(ctx, fmt.Sprintf("emp-svc-%s-%d", rawName, i), &corev1.ServiceArgs{
				Metadata: metav1.ObjectMetaArgs{
					Labels: labels,
					Name: pulumi.All(args.Identity, args.Label, name, p.Port(), p.Protocol()).ApplyT(func(all []any) string {
						id := all[0].(string)
						name := all[2].(string)
						port := all[3].(int)
						prot := all[4].(string)
						if lbl, ok := all[1].(string); ok && lbl != "" {
							return fmt.Sprintf("emp-svc-%s-%s-%s-%d-%s", lbl, id, name, port, prot)
						}
						return fmt.Sprintf("emp-svc-%s-%s-%d-%s", id, name, port, prot)
					}).(pulumi.StringOutput),
				},
				Spec: &corev1.ServiceSpecArgs{
					Type:     svcType,
					Selector: labels,
					Ports: corev1.ServicePortArray{
						corev1.ServicePortArgs{
							TargetPort: p.Port(),
							Port:       p.Port(),
							Protocol:   p.Protocol(),
						},
					},
				},
			}, opts...)
			if err != nil {
				return err
			}
			svcs = pulumi.All(svcs, p, svc).ApplyT(func(all []any) map[string]*corev1.Service {
				svcs := all[0].(map[string]*corev1.Service)
				pb := all[1].(PortBinding)
				svc := all[2].(*corev1.Service)

				prot := pb.Protocol
				if prot == "" {
					prot = "TCP"
				}

				svcs[fmt.Sprintf("%d/%s", pb.Port, prot)] = svc
				return svcs
			}).(corev1.ServiceMapOutput)
			svcMetas = pulumi.All(svcMetas, p, svc.Metadata).ApplyT(func(all []any) map[string]metav1.ObjectMeta {
				svcs := all[0].(map[string]metav1.ObjectMeta)
				pb := all[1].(PortBinding)
				svc := all[2].(metav1.ObjectMeta)

				prot := pb.Protocol
				if prot == "" {
					prot = "TCP"
				}

				svcs[fmt.Sprintf("%d/%s", pb.Port, prot)] = svc
				return svcs
			}).(ObjectMetaMapOutput)
			svcSpecs = pulumi.All(svcSpecs, p, svc.Spec).ApplyT(func(all []any) map[string]corev1.ServiceSpec {
				svcs := all[0].(map[string]corev1.ServiceSpec)
				pb := all[1].(PortBinding)
				svc := all[2].(corev1.ServiceSpec)

				prot := pb.Protocol
				if prot == "" {
					prot = "TCP"
				}

				svcs[fmt.Sprintf("%d/%s", pb.Port, prot)] = svc
				return svcs
			}).(ServiceSpecMapOutput)

			// Specific exposures
			switch pet {
			case ExposeNodePort:
				ntp, err := netwv1.NewNetworkPolicy(ctx, fmt.Sprintf("emp-ntp-%s-%d", rawName, i), &netwv1.NetworkPolicyArgs{
					Metadata: metav1.ObjectMetaArgs{
						Labels: labels,
						Name: pulumi.All(args.Identity, args.Label, name, p.Port(), p.Protocol()).ApplyT(func(all []any) string {
							id := all[0].(string)
							name := all[2].(string)
							port := all[3].(int)
							prot := all[4].(string)
							if lbl, ok := all[1].(string); ok && lbl != "" {
								return fmt.Sprintf("emp-ntp-%s-%s-%s-%d-%s", lbl, id, name, port, prot)
							}
							return fmt.Sprintf("emp-ntp-%s-%s-%d-%s", id, name, port, prot)
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
										Port:     p.Port(),
										Protocol: p.Protocol(),
									},
								},
							},
						},
					},
				}, opts...)
				if err != nil {
					return err
				}
				emp.ntps = append(emp.ntps, ntp)

			case ExposeIngress:
				ing, err := netwv1.NewIngress(ctx, fmt.Sprintf("emp-ing-%s-%d", rawName, i), &netwv1.IngressArgs{
					Metadata: metav1.ObjectMetaArgs{
						Labels: labels,
						Name: pulumi.All(args.Identity, args.Label, name).ApplyT(func(all []any) string {
							id := all[0].(string)
							name := all[2].(string)
							if lbl, ok := all[1].(string); ok && lbl != "" {
								return fmt.Sprintf("emp-ing-%s-%s-%s", lbl, id, name)
							}
							return fmt.Sprintf("emp-ing-%s-%s", id, name)
						}).(pulumi.StringOutput),
						Annotations: args.ingressAnnotations,
					},
					Spec: netwv1.IngressSpecArgs{
						Rules: netwv1.IngressRuleArray{
							netwv1.IngressRuleArgs{
								Host: pulumi.Sprintf("%s.%s", pulumi.All(args.Identity, name, p).ApplyT(func(all []any) string {
									// Combine the identity, the container name and the port binding
									// to generate a pseudo-random string.
									id := all[0].(string)
									name := all[1].(string)
									p := all[2].(PortBinding)
									if p.Protocol == "" {
										p.Protocol = "TCP"
									}

									// Generate a hash of the seed, keep only first bytes (same lenght as
									// identity to avoid fingerprinting scenario on ingress name).
									seed := fmt.Sprintf("%s-%s-%d/%s", id, name, p.Port, p.Protocol)
									return randName(seed)[:len(id)]
								}).(pulumi.StringOutput), args.Hostname),
								Http: netwv1.HTTPIngressRuleValueArgs{
									Paths: netwv1.HTTPIngressPathArray{
										netwv1.HTTPIngressPathArgs{
											Path:     pulumi.String("/"),
											PathType: pulumi.String("Prefix"),
											Backend: netwv1.IngressBackendArgs{
												Service: netwv1.IngressServiceBackendArgs{
													Name: svc.Metadata.Name().Elem(),
													Port: netwv1.ServiceBackendPortArgs{
														Number: p.Port(),
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
				ings = pulumi.All(ings, p, ing).ApplyT(func(all []any) map[string]*netwv1.Ingress {
					ings := all[0].(map[string]*netwv1.Ingress)
					pb := all[1].(PortBinding)
					ing := all[2].(*netwv1.Ingress)

					prot := pb.Protocol
					if prot == "" {
						prot = "TCP"
					}

					ings[fmt.Sprintf("%d/%s", pb.Port, prot)] = ing
					return ings
				}).(netwv1.IngressMapOutput)
				ingSpecs = pulumi.All(ingSpecs, p, ing.Spec).ApplyT(func(all []any) map[string]netwv1.IngressSpec {
					ings := all[0].(map[string]netwv1.IngressSpec)
					pb := all[1].(PortBinding)
					ing := all[2].(netwv1.IngressSpec)

					prot := pb.Protocol
					if prot == "" {
						prot = "TCP"
					}

					ings[fmt.Sprintf("%d/%s", pb.Port, prot)] = ing
					return ings
				}).(IngressSpecMapOutput)

				ntp, err := netwv1.NewNetworkPolicy(ctx, fmt.Sprintf("emp-ntp-%s-%d", rawName, i), &netwv1.NetworkPolicyArgs{
					Metadata: metav1.ObjectMetaArgs{
						Labels: labels,
						Name: pulumi.All(args.Identity, args.Label, name, p.Port(), p.Protocol()).ApplyT(func(all []any) string {
							id := all[0].(string)
							name := all[2].(string)
							port := all[3].(int)
							prot := all[4].(string)
							if lbl, ok := all[1].(string); ok && lbl != "" {
								return fmt.Sprintf("emp-ntp-%s-%s-%s-%d-%s", lbl, id, name, port, prot)
							}
							return fmt.Sprintf("emp-ntp-%s-%s-%d-%s", id, name, port, prot)
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
										Port:     p.Port(),
										Protocol: p.Protocol(),
									},
								},
							},
						},
					},
				}, opts...)
				if err != nil {
					return err
				}
				emp.ntps = append(emp.ntps, ntp)
			}
		}
		emp.svcs = pulumi.All(emp.svcs, name, svcs).ApplyT(func(all []any) map[string]map[string]*corev1.Service {
			svcs := all[0].(map[string]map[string]*corev1.Service)
			svcs[all[1].(string)] = all[2].(map[string]*corev1.Service)
			return svcs
		}).(ServiceMapMapOutput)
		emp.svcSpecs = pulumi.All(emp.svcSpecs, name, svcSpecs).ApplyT(func(all []any) map[string]map[string]corev1.ServiceSpec {
			svcs := all[0].(map[string]map[string]corev1.ServiceSpec)
			svcs[all[1].(string)] = all[2].(map[string]corev1.ServiceSpec)
			return svcs
		}).(ServiceSpecMapMapOutput)
		emp.svcMetas = pulumi.All(emp.svcMetas, name, svcMetas).ApplyT(func(all []any) map[string]map[string]metav1.ObjectMeta {
			svcs := all[0].(map[string]map[string]metav1.ObjectMeta)
			svcs[all[1].(string)] = all[2].(map[string]metav1.ObjectMeta)
			return svcs
		}).(ObjectMetaMapMapOutput)
		emp.ings = pulumi.All(emp.ings, name, ings).ApplyT(func(all []any) map[string]map[string]*netwv1.Ingress {
			ings := all[0].(map[string]map[string]*netwv1.Ingress)
			ings[all[1].(string)] = all[2].(map[string]*netwv1.Ingress)
			return ings
		}).(IngressMapMapOutput)
		emp.ingSpecs = pulumi.All(emp.ingSpecs, name, ingSpecs).ApplyT(func(all []any) map[string]map[string]netwv1.IngressSpec {
			ings := all[0].(map[string]map[string]netwv1.IngressSpec)
			ings[all[1].(string)] = all[2].(map[string]netwv1.IngressSpec)
			return ings
		}).(IngressSpecMapMapOutput)
	}

	// Build all remaining networking resources depending on the exposure type
	var l int
	wg := sync.WaitGroup{}
	wg.Add(1)
	args.Rules.ToRuleArrayOutput().ApplyT(func(rules []Rule) error {
		l = len(rules)
		wg.Done()
		return nil
	})
	wg.Wait()

	// Uniquely identify the resources with labels
	labels := pulumi.StringMap{
		"chall-manager.ctfer.io/kind":     pulumi.String("exposed-multipod"),
		"chall-manager.ctfer.io/identity": args.Identity,
	}
	for i := 0; i < l; i++ {
		rule := args.Rules.ToRuleArrayOutput().Index(pulumi.Int(i))

		ntp, err := netwv1.NewNetworkPolicy(ctx, fmt.Sprintf("emp-ntp-rule-%d", i), &netwv1.NetworkPolicyArgs{
			Metadata: metav1.ObjectMetaArgs{
				Labels: labels,
				Name: pulumi.All(args.Identity, rule.From(), rule.To(), args.Label).ApplyT(func(all []any) string {
					id := all[0].(string)
					from := all[1].(string)
					to := all[2].(string)
					if lbl, ok := all[3].(string); ok && lbl != "" {
						return fmt.Sprintf("emp-ntp-%s-%s-to-%s-%s", lbl, from, to, id)
					}
					return fmt.Sprintf("emp-ntp-%s-to-%s-%s", from, to, id)
				}).(pulumi.StringOutput),
			},
			Spec: netwv1.NetworkPolicySpecArgs{
				PodSelector: metav1.LabelSelectorArgs{
					MatchLabels: emp.deps.MapIndex(rule.To()).Metadata().Labels(),
				},
				PolicyTypes: pulumi.ToStringArray([]string{
					"Ingress",
				}),
				Ingress: netwv1.NetworkPolicyIngressRuleArray{
					netwv1.NetworkPolicyIngressRuleArgs{
						From: netwv1.NetworkPolicyPeerArray{
							netwv1.NetworkPolicyPeerArgs{
								PodSelector: metav1.LabelSelectorArgs{
									MatchLabels: emp.deps.MapIndex(rule.From()).Metadata().Labels(),
								},
							},
						},
						Ports: netwv1.NetworkPolicyPortArray{
							netwv1.NetworkPolicyPortArgs{
								Port: rule.On(),
								Protocol: emp.deps.MapIndex(rule.To()).Spec().ApplyT(func(spec appsv1.DeploymentSpec) string {
									return *spec.Template.Spec.Containers[0].Ports[0].Protocol
								}).(pulumi.StringOutput),
							},
						},
					},
				},
			},
		}, opts...)
		if err != nil {
			return err
		}
		emp.ntps = append(emp.ntps, ntp)
	}

	return nil
}

func (emp *exposedMultipod) outputs(ctx *pulumi.Context, args *ExposedMultipodArgs) {
	keys := emp.svcs.ApplyT(func(svcs ServiceMapMap) []string {
		out := make([]string, 0, len(svcs))
		for k := range svcs {
			out = append(out, k)
		}
		return out
	}).(pulumi.StringArrayOutput)

	for i := 0; i < lenP(keys); i++ {
		name := keys.Index(pulumi.Int(i))

		// => Service Node Port
		svcUrls := pulumi.All(args.Hostname, emp.svcSpecs.MapIndex(name)).ApplyT(func(all []any) map[string]string {
			hostname := all[0].(string)
			specs := all[1].(map[string]corev1.ServiceSpec)

			urls := map[string]string{}
			for k, spec := range specs {
				np := spec.Ports[0].NodePort
				if np != nil {
					urls[k] = fmt.Sprintf("%s:%d", hostname, *np)
				}
			}
			return urls
		}).(pulumi.StringMapOutput)

		// => Ingresses
		ingUrls := emp.ingSpecs.MapIndex(name).ApplyT(func(specs map[string]netwv1.IngressSpec) map[string]string {
			urls := map[string]string{}
			for k, spec := range specs {
				h := spec.Rules[0].Host
				if h != nil {
					urls[k] = *h
				}
			}
			return urls
		}).(pulumi.StringMapOutput)

		emp.URLs = pulumi.All(emp.URLs, name, merge(svcUrls, ingUrls)).ApplyT(func(all []any) map[string]map[string]string {
			urls := all[0].(map[string]map[string]string)
			urls[all[1].(string)] = all[2].(map[string]string)
			return urls
		}).(pulumi.StringMapMapOutput)
	}
}
