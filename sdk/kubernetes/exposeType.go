package kubernetes

import (
	"context"
	"reflect"
	"sync"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// ExposeType represents an exposure of a [PortBinding].
type ExposeType string

const (
	// ExposeInternal defines a non-exposed [PortBinding].
	ExposeInternal ExposeType = ""

	// ExposeNodePort defines a [PortBinding] that is exposed through
	// a Kubernetes Service typed "NodePort".
	ExposeNodePort ExposeType = "NodePort"

	// ExposeIngress defines a [PortBinding] that is exposed through
	// a Kubernetes Ingress.
	ExposeIngress ExposeType = "Ingress"

	// ExposeLoadBalancer defines a [PortBinding] that is exposed through
	// a Kubernetes Service typed "LoadBalancer".
	ExposeLoadBalancer ExposeType = "LoadBalancer"
)

type ExposeTypeInput interface {
	pulumi.Input

	ToExposeTypeOutput() ExposeTypeOutput
	ToExposeTypeOutputWithContext(context.Context) ExposeTypeOutput
}

func (ExposeType) ElementType() reflect.Type {
	return reflect.TypeOf((*ExposeType)(nil)).Elem()
}

func (i ExposeType) ToExposeTypeOutput() ExposeTypeOutput {
	return i.ToExposeTypeOutputWithContext(context.Background())
}

func (i ExposeType) ToExposeTypeOutputWithContext(ctx context.Context) ExposeTypeOutput {
	return pulumi.ToOutputWithContext(ctx, i).(ExposeTypeOutput)
}

type ExposeTypeOutput struct{ *pulumi.OutputState }

func (o ExposeTypeOutput) ElementType() reflect.Type {
	return reflect.TypeOf((*ExposeType)(nil)).Elem()
}

func (o ExposeTypeOutput) ToExposeTypeOutput() ExposeTypeOutput {
	return o
}

func (o ExposeTypeOutput) ToExposeTypeOutputWithContext(_ context.Context) ExposeTypeOutput {
	return o
}

// Raw is a synchronous helper that returns the string value of
// the [ExposeTypeOutput], such that it could be manipulated as a
// [ExposeType] directly (e.g. switch statements).
func (o ExposeTypeOutput) Raw() ExposeType {
	var et ExposeType
	wg := sync.WaitGroup{}
	wg.Add(1)
	o.ApplyT(func(v ExposeType) error {
		et = v
		wg.Done()
		return nil
	})
	wg.Wait()
	return et
}

func init() {
	pulumi.RegisterInputType(reflect.TypeOf((*ExposeTypeInput)(nil)).Elem(), ExposeType(""))
	pulumi.RegisterOutputType(ExposeTypeOutput{})
}
