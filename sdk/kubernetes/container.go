package kubernetes

import (
	"context"
	"errors"
	"reflect"
	"sync"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"go.uber.org/multierr"
)

// Container represents a computing unit that is deployed on Kubernetes
// (e.g. a Docker container by default).
type Container struct {
	// Image to run (e.g. ctferio/chall-manager:latest).
	Image string `pulumi:"image"`

	// Ports to listen on.
	Ports []PortBinding `pulumi:"ports"`

	// Envs is a map of environment variable that is passed to the container.
	// Leveraging the printers, these can be provisioned with an outer
	// service name.
	Envs map[string]Printer `pulumi:"envs"`

	// Files is a map of absolute paths to a file and its content.
	Files map[string]string `pulumi:"files"`

	// LimitCPU is an optional (yet recommended) resource limit of CPU usage.
	LimitCPU *string `pulumi:"limitCpu"`

	// LimitMemory is an optional (yet recommended) resource limit of memory
	// (RAM) usage.
	LimitMemory *string `pulumi:"limitMemory"`
}

type ContainerInput interface {
	pulumi.Input

	ToContainerOutput() ContainerOutput
	ToContainerOutputWithContext(context.Context) ContainerOutput
}

// ContainerArgs is the input of [Container], i.e. a computing unit that is
// deployed on Kubernetes (e.g. a Docker container by default).
type ContainerArgs struct {
	// Image to run (e.g. ctferio/chall-manager:latest).
	Image pulumi.StringInput `pulumi:"image"`

	// Ports to listen on.
	Ports PortBindingArrayInput `pulumi:"ports"`

	// Envs is a map of environment variable that is passed to the container.
	// Leveraging the printers, these can be provisioned with an outer
	// service name.
	Envs PrinterMapInput `pulumi:"envs"`

	// Files is a map of absolute paths to a file and its content.
	Files pulumi.StringMapInput `pulumi:"files"`

	// LimitCPU is an optional (yet recommended) resource limit of CPU usage.
	LimitCPU pulumi.StringPtrInput `pulumi:"limitCpu"`

	// LimitMemory is an optional (yet recommended) resource limit of memory
	// (RAM) usage.
	LimitMemory pulumi.StringPtrInput `pulumi:"limitMemory"`
}

func (ContainerArgs) ElementType() reflect.Type {
	return reflect.TypeOf((*Container)(nil)).Elem()
}

func (i ContainerArgs) ToContainerOutput() ContainerOutput {
	return i.ToContainerOutputWithContext(context.Background())
}

func (i ContainerArgs) ToContainerOutputWithContext(ctx context.Context) ContainerOutput {
	return pulumi.ToOutputWithContext(ctx, i).(ContainerOutput)
}

type ContainerOutput struct{ *pulumi.OutputState }

func (ContainerOutput) ElementType() reflect.Type {
	return reflect.TypeOf((*Container)(nil)).Elem()
}

func (o ContainerOutput) ToContainerOutput() ContainerOutput {
	return o
}

func (o ContainerOutput) ToContainerOutputWithContext(_ context.Context) ContainerOutput {
	return o
}

// Image to run (e.g. ctferio/chall-manager:latest).
func (o ContainerOutput) Image() pulumi.StringOutput {
	return o.ApplyT(func(v Container) string {
		return v.Image
	}).(pulumi.StringOutput)
}

// Ports to listen on.
func (o ContainerOutput) Ports() PortBindingArrayOutput {
	return o.ApplyT(func(v Container) []PortBinding {
		return v.Ports
	}).(PortBindingArrayOutput)
}

// Envs is a map of environment variable that is passed to the container.
// Leveraging the printers, these can be provisioned with an outer
// service name.
func (o ContainerOutput) Envs() PrinterMapOutput {
	return o.ApplyT(func(v Container) map[string]Printer {
		return v.Envs
	}).(PrinterMapOutput)
}

// Files is a map of absolute paths to a file and its content.
func (o ContainerOutput) Files() pulumi.StringMapOutput {
	return o.ApplyT(func(v Container) map[string]string {
		return v.Files
	}).(pulumi.StringMapOutput)
}

// HasFiles is a helper that returns whether there are files known.
// It can be used to determine whether to create ConfigMaps and mount
// volumes to containers (else it would cost time and resources).
func (o ContainerOutput) HasFiles() bool {
	var b bool
	wg := sync.WaitGroup{}
	wg.Add(1)
	o.ApplyT(func(v Container) error {
		b = len(v.Files) != 0
		wg.Done()
		return nil
	})
	wg.Wait()
	return b
}

// LimitCPU is an optional (yet recommended) resource limit of CPU usage.
func (o ContainerOutput) LimitCPU() pulumi.StringPtrOutput {
	return o.ApplyT(func(v Container) *string {
		return v.LimitCPU
	}).(pulumi.StringPtrOutput)
}

// LimitMemory is an optional (yet recommended) resource limit of memory
// (RAM) usage.
func (o ContainerOutput) LimitMemory() pulumi.StringPtrOutput {
	return o.ApplyT(func(v Container) *string {
		return v.LimitMemory
	}).(pulumi.StringPtrOutput)
}

type ContainerMapInput interface {
	pulumi.Input

	ToContainerMapOutput() ContainerMapOutput
	ToContainerMapOutputWithContext(context.Context) ContainerMapOutput
}

type ContainerMap map[string]ContainerInput

func (ContainerMap) ElementType() reflect.Type {
	return reflect.TypeOf((*map[string]Container)(nil)).Elem()
}

func (i ContainerMap) ToContainerMapOutput() ContainerMapOutput {
	return i.ToContainerMapOutputWithContext(context.Background())
}

func (i ContainerMap) ToContainerMapOutputWithContext(ctx context.Context) ContainerMapOutput {
	return pulumi.ToOutputWithContext(ctx, i).(ContainerMapOutput)
}

type ContainerMapOutput struct{ *pulumi.OutputState }

func (ContainerMapOutput) ElementType() reflect.Type {
	return reflect.TypeOf((*map[string]Container)(nil)).Elem()
}

func (o ContainerMapOutput) ToContainerMapOutput() ContainerMapOutput {
	return o
}

func (o ContainerMapOutput) ToContainerMapOutputWithContext(_ context.Context) ContainerMapOutput {
	return o
}

func (o ContainerMapOutput) MapIndex(k pulumi.StringInput) ContainerOutput {
	return pulumi.All(o, k).ApplyT(func(all []any) Container {
		return all[0].(map[string]Container)[all[1].(string)]
	}).(ContainerOutput)
}

func init() {
	pulumi.RegisterInputType(reflect.TypeOf((*ContainerInput)(nil)).Elem(), ContainerArgs{})
	pulumi.RegisterInputType(reflect.TypeOf((*ContainerMapInput)(nil)).Elem(), ContainerMap{})
	pulumi.RegisterOutputType(ContainerOutput{})
	pulumi.RegisterOutputType(ContainerMapOutput{})
}

func (c Container) Check() (err error) {
	if c.Image == "" {
		err = multierr.Append(err, errors.New("image could not be empty"))
	}
	if len(c.Ports) == 0 {
		err = multierr.Append(err, errors.New("ports could not be empty"))
	}
	for _, port := range c.Ports {
		err = multierr.Append(err, port.Check())
	}
	return
}
