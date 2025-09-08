package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sync"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"go.uber.org/multierr"
)

type (
	ExposedMonopod struct {
		pulumi.ResourceState

		sub *exposedMultipod

		// URLs exposed by the container.
		URLs pulumi.StringMapOutput
	}
)

func NewExposedMonopod(ctx *pulumi.Context, name string, args *ExposedMonopodArgs, opts ...pulumi.ResourceOption) (*ExposedMonopod, error) {
	emp := &ExposedMonopod{}
	if args == nil {
		return nil, errors.New("nil args")
	}
	argsOut := args.ToExposedMonopodArgsOutput()
	if err := emp.check(argsOut); err != nil {
		return nil, err
	}

	if err := ctx.RegisterComponentResource("ctfer-io:chall-manager/sdk:kubernetes.ExposedMonopod", name, emp, opts...); err != nil {
		return nil, err
	}
	opts = append(opts, pulumi.Parent(emp))

	sub, err := newExposedMultipod(ctx, ExposedMultipodArgs{
		Identity: argsOut.Identity(),
		Label:    argsOut.Label(),
		Hostname: argsOut.Hostname(),
		Containers: ContainerMap{
			"one": argsOut.Container(),
		},
		Rules:            RuleArray{},
		FromCIDR:         argsOut.FromCIDR(),
		IngressNamespace: argsOut.IngressNamespace(),
		IngressLabels:    argsOut.IngressLabels(),
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

func (emp *ExposedMonopod) check(args ExposedMonopodArgsOutput) error {
	// In-depth checks
	wg := sync.WaitGroup{}
	checks := 1 // number of checks to perform
	wg.Add(checks)
	cerr := make(chan error, checks)

	args.Container().ApplyT(func(c Container) error {
		defer wg.Done()

		// Following test contains subsets of container, thus skip if any first
		err := c.Check()
		if err != nil {
			cerr <- err
			return nil
		}

		// Then run the subset if no error yet
		for _, p := range c.Ports {
			if !slices.Contains([]ExposeType{ExposeNodePort, ExposeIngress, ExposeLoadBalancer}, p.ExposeType) {
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

type ExposedMonopodArgsRaw struct {
	Identity         string            `pulumi:"identity"`
	Label            *string           `pulumi:"label"`
	Hostname         string            `pulumi:"hostname"`
	Container        Container         `pulumi:"container"`
	FromCIDR         string            `pulumi:"fromCIDR"`
	IngressNamespace string            `pulumi:"ingressNamespace"`
	IngressLabels    map[string]string `pulumi:"ingressLabels"`
}

type ExposedMonopodArgsInput interface {
	pulumi.Input

	ToExposedMonopodArgsOutput() ExposedMonopodArgsOutput
	ToExposedMonopodArgsOutputWithContext(context.Context) ExposedMonopodArgsOutput
}

type ExposedMonopodArgs struct {
	Identity         pulumi.StringInput    `pulumi:"identity"`
	Label            pulumi.StringPtrInput `pulumi:"label"`
	Hostname         pulumi.StringInput    `pulumi:"hostname"`
	Container        ContainerInput        `pulumi:"container"`
	FromCIDR         pulumi.StringInput    `pulumi:"fromCIDR"`
	IngressNamespace pulumi.StringInput    `pulumi:"ingressNamespace"`
	IngressLabels    pulumi.StringMapInput `pulumi:"ingressLabels"`
}

func (ExposedMonopodArgs) ElementType() reflect.Type {
	return reflect.TypeOf((*ExposedMonopodArgsRaw)(nil)).Elem()
}

func (i ExposedMonopodArgs) ToExposedMonopodArgsOutput() ExposedMonopodArgsOutput {
	return i.ToExposedMonopodArgsOutputWithContext(context.Background())
}

func (i ExposedMonopodArgs) ToExposedMonopodArgsOutputWithContext(ctx context.Context) ExposedMonopodArgsOutput {
	return pulumi.ToOutputWithContext(ctx, i).(ExposedMonopodArgsOutput)
}

type ExposedMonopodArgsOutput struct{ *pulumi.OutputState }

func (ExposedMonopodArgsOutput) ElementType() reflect.Type {
	return reflect.TypeOf((*ExposedMonopodArgsRaw)(nil)).Elem()
}

func (o ExposedMonopodArgsOutput) ToExposedMonopodArgsOutput() ExposedMonopodArgsOutput {
	return o
}

func (o ExposedMonopodArgsOutput) ToExposedMonopodArgsOutputWithContext(_ context.Context) ExposedMonopodArgsOutput {
	return o
}

func (o ExposedMonopodArgsOutput) Identity() pulumi.StringOutput {
	return o.ApplyT(func(args ExposedMonopodArgsRaw) string {
		return args.Identity
	}).(pulumi.StringOutput)
}

func (o ExposedMonopodArgsOutput) Label() pulumi.StringPtrOutput {
	return o.ApplyT(func(args ExposedMonopodArgsRaw) *string {
		return args.Label
	}).(pulumi.StringPtrOutput)
}

func (o ExposedMonopodArgsOutput) Hostname() pulumi.StringOutput {
	return o.ApplyT(func(args ExposedMonopodArgsRaw) string {
		return args.Hostname
	}).(pulumi.StringOutput)
}

func (o ExposedMonopodArgsOutput) Container() ContainerOutput {
	return o.ApplyT(func(args ExposedMonopodArgsRaw) Container {
		return args.Container
	}).(ContainerOutput)
}

func (o ExposedMonopodArgsOutput) FromCIDR() pulumi.StringOutput {
	return o.ApplyT(func(args ExposedMonopodArgsRaw) string {
		return args.FromCIDR
	}).(pulumi.StringOutput)
}

func (o ExposedMonopodArgsOutput) IngressNamespace() pulumi.StringOutput {
	return o.ApplyT(func(args ExposedMonopodArgsRaw) string {
		return args.IngressNamespace
	}).(pulumi.StringOutput)
}

func (o ExposedMonopodArgsOutput) IngressLabels() pulumi.StringMapOutput {
	return o.ApplyT(func(args ExposedMonopodArgsRaw) map[string]string {
		return args.IngressLabels
	}).(pulumi.StringMapOutput)
}

func init() {
	pulumi.RegisterInputType(reflect.TypeOf((*ExposedMonopodArgsInput)(nil)).Elem(), ExposedMonopodArgs{})
	pulumi.RegisterOutputType(ExposedMonopodArgsOutput{})
}
