package kubernetes

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"sync"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"go.uber.org/multierr"
)

// PortBinding represents the exposure of a [Container] on a <port,protocol>.
type PortBinding struct {
	// Port the [Container] listens on.
	Port int `pulumi:"port"`

	// Protocol to comunicate with the port.
	// Defaults to TCP.
	Protocol string `pulumi:"protocol"`

	// ExposeType is the [ExposeType] strategy to expose the port.
	ExposeType ExposeType `pulumi:"exposeType"`

	// Annotations is an optional k=v map of annotations to set on
	// the exposing resource (i.e. service or ingress) that exposes the container
	// on this port.
	//
	// For instance, if the ExposeType=ExposeIngress, then the Annotations are
	// passed to the Ingress resource.
	// That could be a place to define pulumi.com/skipAwait=true if needed
	// References:
	// - https://www.pulumi.com/blog/improving-kubernetes-management-with-pulumis-await-logic/
	// - https://github.com/pulumi/pulumi-kubernetes/issues/1812
	Annotations map[string]string `pulumi:"annotations"`
}

type PortBindingInput interface {
	pulumi.Input

	ToPortBindingOutput() PortBindingOutput
	ToPortBindingOutputWithContext(context.Context) PortBindingOutput
}

// PortBindingArgs is the input of [PortBinding], i.e. represents the exposure
// of a [Container] on a <port,protocol>.
type PortBindingArgs struct {
	// Port the [Container] listens on.
	Port pulumi.IntInput `pulumi:"port"`

	// Protocol to comunicate with the port.
	// Defaults to TCP.
	Protocol pulumi.StringInput `pulumi:"protocol"`

	// ExposeType is the [ExposeType] strategy to expose the port.
	ExposeType ExposeTypeInput `pulumi:"exposeType"`

	// Annotations is an optional k=v map of annotations to set on
	// the exposing resource (i.e. service or ingress) that exposes the container
	// on this port.
	//
	// For instance, if the ExposeType=ExposeIngress, then the Annotations are
	// passed to the Ingress resource.
	// That could be a place to define pulumi.com/skipAwait=true if needed
	// References:
	// - https://www.pulumi.com/blog/improving-kubernetes-management-with-pulumis-await-logic/
	// - https://github.com/pulumi/pulumi-kubernetes/issues/1812
	Annotations pulumi.StringMapInput `pulumi:"annotations"`
}

func (PortBindingArgs) ElementType() reflect.Type {
	return reflect.TypeOf((*PortBinding)(nil)).Elem()
}

func (i PortBindingArgs) ToPortBindingOutput() PortBindingOutput {
	return i.ToPortBindingOutputWithContext(context.Background())
}

func (i PortBindingArgs) ToPortBindingOutputWithContext(ctx context.Context) PortBindingOutput {
	return pulumi.ToOutputWithContext(ctx, i).(PortBindingOutput)
}

type PortBindingOutput struct{ *pulumi.OutputState }

func (PortBindingOutput) ElementType() reflect.Type {
	return reflect.TypeOf((*PortBinding)(nil)).Elem()
}

func (o PortBindingOutput) ToPortBindingOutput() PortBindingOutput {
	return o
}

func (o PortBindingOutput) ToPortBindingOutputWithContext(_ context.Context) PortBindingOutput {
	return o
}

// Port the [Container] listens on.
func (o PortBindingOutput) Port() pulumi.IntOutput {
	return o.ApplyT(func(v PortBinding) int {
		return v.Port
	}).(pulumi.IntOutput)
}

// Protocol to comunicate with the port.
// Defaults to TCP.
func (o PortBindingOutput) Protocol() pulumi.StringOutput {
	return o.ApplyT(func(v PortBinding) string {
		// default value is TCP
		if v.Protocol == "" {
			return "TCP"
		}
		return v.Protocol
	}).(pulumi.StringOutput)
}

// ExposeType is the [ExposeType] strategy to expose the port.
func (o PortBindingOutput) ExposeType() ExposeTypeOutput {
	return o.ApplyT(func(v PortBinding) ExposeType {
		// default value is Internal (not exposed -> security by default)
		if v.ExposeType == "" {
			return ExposeInternal
		}
		return v.ExposeType
	}).(ExposeTypeOutput)
}

func (o PortBindingOutput) Annotations() pulumi.StringMapOutput {
	return o.ApplyT(func(v PortBinding) map[string]string {
		return v.Annotations
	}).(pulumi.StringMapOutput)
}

type PortBindingArrayInput interface {
	pulumi.Input

	ToPortBindingArrayOutput() PortBindingArrayOutput
	ToPortBindingArrayOutputWithContext(context.Context) PortBindingArrayOutput
}

type PortBindingArray []PortBindingInput

func (PortBindingArray) ElementType() reflect.Type {
	return reflect.TypeOf((*[]PortBinding)(nil)).Elem()
}

func (i PortBindingArray) ToPortBindingArrayOutput() PortBindingArrayOutput {
	return i.ToPortBindingArrayOutputWithContext(context.Background())
}

func (i PortBindingArray) ToPortBindingArrayOutputWithContext(ctx context.Context) PortBindingArrayOutput {
	return pulumi.ToOutputWithContext(ctx, i).(PortBindingArrayOutput)
}

type PortBindingArrayOutput struct{ *pulumi.OutputState }

func (PortBindingArrayOutput) ElementType() reflect.Type {
	return reflect.TypeOf((*[]PortBinding)(nil)).Elem()
}

func (o PortBindingArrayOutput) ToPortBindingArrayOutput() PortBindingArrayOutput {
	return o
}

func (o PortBindingArrayOutput) ToPortBindingArrayOutputWithContext(_ context.Context) PortBindingArrayOutput {
	return o
}

// Len is a synchronous helper that returns the length of the array.
func (o PortBindingArrayOutput) Len() int {
	var l int
	wg := sync.WaitGroup{}
	wg.Add(1)
	o.ApplyT(func(v []PortBinding) error {
		l = len(v)
		wg.Done()
		return nil
	})
	wg.Wait()
	return l
}

func (o PortBindingArrayOutput) Index(i pulumi.IntInput) PortBindingOutput {
	return pulumi.All(o, i).ApplyT(func(all []any) PortBinding {
		return all[0].([]PortBinding)[all[1].(int)]
	}).(PortBindingOutput)
}

type PortBindingMapArrayInput interface {
	pulumi.Input

	ToPortBindingMapArrayOutput() PortBindingMapArrayOutput
	ToPortBindingMapArrayOutputWithContext(context.Context) PortBindingMapArrayOutput
}

type PortBindingMapArray map[string]PortBindingArray

func (PortBindingMapArray) ElementType() reflect.Type {
	return reflect.TypeOf((*map[string][]PortBinding)(nil)).Elem()
}

func (i PortBindingMapArray) ToPortBindingMapArrayOutput() PortBindingMapArrayOutput {
	return i.ToPortBindingMapArrayOutputWithContext(context.Background())
}

func (i PortBindingMapArray) ToPortBindingMapArrayOutputWithContext(ctx context.Context) PortBindingMapArrayOutput {
	return pulumi.ToOutputWithContext(ctx, i).(PortBindingMapArrayOutput)
}

type PortBindingMapArrayOutput struct{ *pulumi.OutputState }

func (PortBindingMapArrayOutput) ElementType() reflect.Type {
	return reflect.TypeOf((*map[string][]PortBinding)(nil)).Elem()
}

func (o PortBindingMapArrayOutput) ToPortBindingMapArrayOutput() PortBindingMapArrayOutput {
	return o
}

func (o PortBindingMapArrayOutput) ToPortBindingMapArrayOutputWithContext(_ context.Context) PortBindingMapArrayOutput {
	return o
}

func (o PortBindingMapArrayOutput) MapIndex(k pulumi.StringInput) PortBindingArrayOutput {
	return pulumi.All(o, k).ApplyT(func(all []any) []PortBinding {
		return all[0].(map[string][]PortBinding)[all[1].(string)]
	}).(PortBindingArrayOutput)
}

func init() {
	pulumi.RegisterInputType(reflect.TypeOf((*PortBindingInput)(nil)).Elem(), PortBindingArgs{})
	pulumi.RegisterInputType(reflect.TypeOf((*PortBindingArrayInput)(nil)).Elem(), PortBindingArray{})
	pulumi.RegisterInputType(reflect.TypeOf((*PortBindingMapArrayInput)(nil)).Elem(), PortBindingMapArray{})
	pulumi.RegisterOutputType(PortBindingOutput{})
	pulumi.RegisterOutputType(PortBindingArrayOutput{})
	pulumi.RegisterOutputType(PortBindingMapArrayOutput{})
}

// Check ensures the [PortBinding] configuration is good.
func (pb PortBinding) Check() (err error) {
	if pb.Port < 1 || pb.Port > 65535 {
		err = multierr.Append(err, fmt.Errorf("port %d out of bounds [1;65535]", pb.Port))
	}

	if pb.Protocol == "" {
		pb.Protocol = "TCP"
	}
	if !slices.Contains([]string{
		"TCP",
		"UDP",
	}, pb.Protocol) {
		err = multierr.Append(err, fmt.Errorf("unsupported protocol %s", pb.Protocol))
	}

	if !slices.Contains([]ExposeType{
		ExposeInternal,
		ExposeNodePort,
		ExposeIngress,
	}, ExposeType(pb.ExposeType)) {
		err = multierr.Append(err, fmt.Errorf("unsupported expose type %s", pb.ExposeType))
	}
	return err
}
