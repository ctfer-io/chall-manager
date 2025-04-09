package kubernetes

import (
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"go.uber.org/multierr"
)

type (
	ExposedMonopod struct {
		pulumi.ResourceState

		sub *exposedMultipod

		URLs pulumi.StringMapOutput
	}

	ExposedMonopodArgs struct {
		// Challenge instance attributes

		Identity pulumi.StringInput
		Label    pulumi.StringInput
		Hostname pulumi.StringInput

		// Kubernetes attributes

		Container ContainerInput

		// FromCIDR can be configured to specify an IP range that will
		// be able to access the pod, if ExposeType == "NodePort".
		// We cannot infer the proxy end user uses thus cannot support
		// ExposeType == "Ingress" and FromCIDR filtering.
		//
		// For Traefik, create a middleware with an ipAllowList and provide
		// IngressAnnotations for the ingress to adopt the CIDR filtering.
		//
		// You can supplement our SDK with your own, with specific knownledges
		// on the infrastructure, to natively adopt your ingress controller
		// requirements on IP filtering.
		FromCIDR pulumi.StringPtrInput

		// IngressAnnotations is a set of additional annotations to
		// put on the ingress, if the `ExposeType` is set to
		// `ExposeIngress`.
		IngressAnnotations pulumi.StringMapInput

		// IngressNamespace must be configured to the namespace in
		// which the ingress (e.g. nginx, traefik) is deployed.
		IngressNamespace pulumi.StringInput

		// IngressLabels must be configured to the labels of the ingress
		// pods (e.g. app=traefik, ...).
		IngressLabels pulumi.StringMapInput
	}
)

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

	sub, err := newExposedMultipod(ctx, &ExposedMultipodArgs{
		Identity: args.Identity,
		Label:    args.Label,
		Hostname: args.Hostname,
		Containers: ContainerMap{
			"one": args.Container,
		},
		Rules:              RuleArray{},
		FromCIDR:           args.FromCIDR,
		IngressAnnotations: args.IngressAnnotations,
		IngressNamespace:   args.IngressNamespace,
		IngressLabels:      args.IngressLabels,
	}, opts...)
	if err != nil {
		return nil, err
	}
	emp.sub = sub

	emp.URLs = emp.sub.URLs.MapIndex(pulumi.String("one"))
	if err := ctx.RegisterResourceOutputs(emp, pulumi.Map{
		"urls": emp.URLs,
	}); err != nil {
		return nil, err
	}

	return emp, nil
}

func (emp *ExposedMonopod) defaults(args *ExposedMonopodArgs) *ExposedMonopodArgs {
	if args == nil {
		args = &ExposedMonopodArgs{}
	}
	return args
}

func (emp *ExposedMonopod) check(args *ExposedMonopodArgs) error {
	// Smoke
	if args.Container == nil {
		return errors.New("container arguments required")
	}

	// In-depth checks
	wg := sync.WaitGroup{}
	checks := 1 // number of checks to perform
	wg.Add(checks)
	cerr := make(chan error, checks)

	args.Container.ToContainerOutput().ApplyT(func(c Container) error {
		defer wg.Done()

		// Following test contains subsets of container, thus skip if any first
		err := c.Check()
		if err != nil {
			cerr <- err
			return nil
		}

		// Then run the subset if no error yet
		for _, p := range c.Ports {
			if !slices.Contains([]ExposeType{ExposeNodePort, ExposeIngress}, p.ExposeType) {
				err = multierr.Append(err, fmt.Errorf("unsupported expose type %s", p.ExposeType))
			}
		}
		cerr <- err
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
