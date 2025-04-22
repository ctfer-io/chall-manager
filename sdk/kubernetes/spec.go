package kubernetes

import (
	"context"
	"reflect"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	netwv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/networking/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// region ObjectMetaArray

type ObjectMetaArrayInput interface {
	pulumi.Input

	ToObjectMetaArrayOutput() ObjectMetaArrayOutput
	ToObjectMetaArrayOutputWithContext(context.Context) ObjectMetaArrayOutput
}

type ObjectMetaArray map[string]metav1.ObjectMetaInput

func (ObjectMetaArray) ElementType() reflect.Type {
	return reflect.TypeOf((*[]metav1.ObjectMeta)(nil)).Elem()
}

func (i ObjectMetaArray) ToObjectMetaArrayOutput() ObjectMetaArrayOutput {
	return i.ToObjectMetaArrayOutputWithContext(context.Background())
}

func (i ObjectMetaArray) ToObjectMetaArrayOutputWithContext(ctx context.Context) ObjectMetaArrayOutput {
	return pulumi.ToOutputWithContext(ctx, i).(ObjectMetaArrayOutput)
}

type ObjectMetaArrayOutput struct{ *pulumi.OutputState }

func (ObjectMetaArrayOutput) ElementType() reflect.Type {
	return reflect.TypeOf((*[]metav1.ObjectMeta)(nil)).Elem()
}

func (o ObjectMetaArrayOutput) ToObjectMetaArrayOutput() ObjectMetaArrayOutput {
	return o
}

func (o ObjectMetaArrayOutput) ToObjectMetaArrayOutputWithContext(_ context.Context) ObjectMetaArrayOutput {
	return o
}

func (o ObjectMetaArrayOutput) Index(i pulumi.IntInput) metav1.ObjectMetaOutput {
	return pulumi.All(o, i).ApplyT(func(all []any) metav1.ObjectMeta {
		return all[0].([]metav1.ObjectMeta)[all[1].(int)]
	}).(metav1.ObjectMetaOutput)
}

// region ObjectMetaMap

type ObjectMetaMapInput interface {
	pulumi.Input

	ToObjectMetaMapOutput() ObjectMetaMapOutput
	ToObjectMetaMapOutputWithContext(context.Context) ObjectMetaMapOutput
}

type ObjectMetaMap map[string]metav1.ObjectMetaInput

func (ObjectMetaMap) ElementType() reflect.Type {
	return reflect.TypeOf((*map[string]metav1.ObjectMeta)(nil)).Elem()
}

func (i ObjectMetaMap) ToObjectMetaMapOutput() ObjectMetaMapOutput {
	return i.ToObjectMetaMapOutputWithContext(context.Background())
}

func (i ObjectMetaMap) ToObjectMetaMapOutputWithContext(ctx context.Context) ObjectMetaMapOutput {
	return pulumi.ToOutputWithContext(ctx, i).(ObjectMetaMapOutput)
}

type ObjectMetaMapOutput struct{ *pulumi.OutputState }

func (ObjectMetaMapOutput) ElementType() reflect.Type {
	return reflect.TypeOf((*map[string]metav1.ObjectMeta)(nil)).Elem()
}

func (o ObjectMetaMapOutput) ToObjectMetaMapOutput() ObjectMetaMapOutput {
	return o
}

func (o ObjectMetaMapOutput) ToObjectMetaMapOutputWithContext(_ context.Context) ObjectMetaMapOutput {
	return o
}

func (o ObjectMetaMapOutput) MapIndex(k pulumi.StringInput) metav1.ObjectMetaOutput {
	return pulumi.All(o, k).ApplyT(func(all []any) metav1.ObjectMeta {
		return all[0].(map[string]metav1.ObjectMeta)[all[1].(string)]
	}).(metav1.ObjectMetaOutput)
}

// region IngressSpecMap

type IngressSpecMapInput interface {
	pulumi.Input

	ToIngressSpecMapOutput() IngressSpecMapOutput
	ToIngressSpecMapOutputWithContext(context.Context) IngressSpecMapOutput
}

type IngressSpecMap map[string]netwv1.IngressSpecInput

func (IngressSpecMap) ElementType() reflect.Type {
	return reflect.TypeOf((*map[string]netwv1.IngressSpec)(nil)).Elem()
}

func (i IngressSpecMap) ToIngressSpecMapOutput() IngressSpecMapOutput {
	return i.ToIngressSpecMapOutputWithContext(context.Background())
}

func (i IngressSpecMap) ToIngressSpecMapOutputWithContext(ctx context.Context) IngressSpecMapOutput {
	return pulumi.ToOutputWithContext(ctx, i).(IngressSpecMapOutput)
}

type IngressSpecMapOutput struct{ *pulumi.OutputState }

func (IngressSpecMapOutput) ElementType() reflect.Type {
	return reflect.TypeOf((*map[string]netwv1.IngressSpec)(nil)).Elem()
}

func (o IngressSpecMapOutput) ToIngressSpecMapOutput() IngressSpecMapOutput {
	return o
}

func (o IngressSpecMapOutput) ToIngressSpecMapOutputWithContext(_ context.Context) IngressSpecMapOutput {
	return o
}

func (o IngressSpecMapOutput) MapIndex(k pulumi.StringInput) netwv1.IngressSpecOutput {
	return pulumi.All(o, k).ApplyT(func(all []any) netwv1.IngressSpec {
		return all[0].(map[string]netwv1.IngressSpec)[all[1].(string)]
	}).(netwv1.IngressSpecOutput)
}

// region ServiceSpecMap

type ServiceSpecMapInput interface {
	pulumi.Input

	ToServiceSpecMapOutput() ServiceSpecMapOutput
	ToServiceSpecMapOutputWithContext(context.Context) ServiceSpecMapOutput
}

type ServiceSpecMap map[string]corev1.ServiceSpecInput

func (ServiceSpecMap) ElementType() reflect.Type {
	return reflect.TypeOf((*map[string]corev1.ServiceSpec)(nil)).Elem()
}

func (i ServiceSpecMap) ToServiceSpecMapOutput() ServiceSpecMapOutput {
	return i.ToServiceSpecMapOutputWithContext(context.Background())
}

func (i ServiceSpecMap) ToServiceSpecMapOutputWithContext(ctx context.Context) ServiceSpecMapOutput {
	return pulumi.ToOutputWithContext(ctx, i).(ServiceSpecMapOutput)
}

type ServiceSpecMapOutput struct{ *pulumi.OutputState }

func (ServiceSpecMapOutput) ElementType() reflect.Type {
	return reflect.TypeOf((*map[string]corev1.ServiceSpec)(nil)).Elem()
}

func (o ServiceSpecMapOutput) ToServiceSpecMapOutput() ServiceSpecMapOutput {
	return o
}

func (o ServiceSpecMapOutput) ToServiceSpecMapOutputWithContext(_ context.Context) ServiceSpecMapOutput {
	return o
}

func (o ServiceSpecMapOutput) MapIndex(k pulumi.StringInput) corev1.ServiceSpecOutput {
	return pulumi.All(o, k).ApplyT(func(all []any) corev1.ServiceSpec {
		return all[0].(map[string]corev1.ServiceSpec)[all[1].(string)]
	}).(corev1.ServiceSpecOutput)
}

// region ServiceSpecMapMap

type ServiceMapMapInput interface {
	pulumi.Input

	ToServiceMapMapOutput() ServiceMapMapOutput
	ToServiceMapMapOutputWithContext(context.Context) ServiceMapMapOutput
}

type ServiceMapMap map[string]map[string]*corev1.Service

func (ServiceMapMap) ElementType() reflect.Type {
	return reflect.TypeOf((*map[string]map[string]*corev1.Service)(nil)).Elem()
}

func (i ServiceMapMap) ToServiceMapMapOutput() ServiceMapMapOutput {
	return i.ToServiceMapMapOutputWithContext(context.Background())
}

func (i ServiceMapMap) ToServiceMapMapOutputWithContext(ctx context.Context) ServiceMapMapOutput {
	return pulumi.ToOutputWithContext(ctx, i).(ServiceMapMapOutput)
}

type ServiceMapMapOutput struct{ *pulumi.OutputState }

func (ServiceMapMapOutput) ElementType() reflect.Type {
	return reflect.TypeOf((*map[string]map[string]*corev1.Service)(nil)).Elem()
}

func (o ServiceMapMapOutput) ToServiceMapMapOutput() ServiceMapMapOutput {
	return o
}

func (o ServiceMapMapOutput) ToServiceMapMapOutputWithContext(_ context.Context) ServiceMapMapOutput {
	return o
}

func (o ServiceMapMapOutput) MapIndex(k pulumi.StringInput) corev1.ServiceMapOutput {
	return pulumi.All(o, k).ApplyT(func(all []any) map[string]*corev1.Service {
		return all[0].(map[string]map[string]*corev1.Service)[all[1].(string)]
	}).(corev1.ServiceMapOutput)
}

// region ObjectMetaMapMap

type ObjectMetaMapMapInput interface {
	pulumi.Input

	ToObjectMetaMapMapOutput() ObjectMetaMapMapOutput
	ToObjectMetaMapMapOutputWithContext(context.Context) ObjectMetaMapMapOutput
}

type ObjectMetaMapMap map[string]map[string]metav1.ObjectMeta

func (ObjectMetaMapMap) ElementType() reflect.Type {
	return reflect.TypeOf((*map[string]map[string]metav1.ObjectMeta)(nil)).Elem()
}

func (i ObjectMetaMapMap) ToObjectMetaMapMapOutput() ObjectMetaMapMapOutput {
	return i.ToObjectMetaMapMapOutputWithContext(context.Background())
}

func (i ObjectMetaMapMap) ToObjectMetaMapMapOutputWithContext(ctx context.Context) ObjectMetaMapMapOutput {
	return pulumi.ToOutputWithContext(ctx, i).(ObjectMetaMapMapOutput)
}

type ObjectMetaMapMapOutput struct{ *pulumi.OutputState }

func (ObjectMetaMapMapOutput) ElementType() reflect.Type {
	return reflect.TypeOf((*map[string]map[string]metav1.ObjectMeta)(nil)).Elem()
}

func (o ObjectMetaMapMapOutput) ToObjectMetaMapMapOutput() ObjectMetaMapMapOutput {
	return o
}

func (o ObjectMetaMapMapOutput) ToObjectMetaMapMapOutputWithContext(_ context.Context) ObjectMetaMapMapOutput {
	return o
}

func (o ObjectMetaMapMapOutput) MapIndex(k pulumi.StringInput) ObjectMetaMapOutput {
	return pulumi.All(o, k).ApplyT(func(all []any) map[string]metav1.ObjectMeta {
		return all[0].(map[string]map[string]metav1.ObjectMeta)[all[1].(string)]
	}).(ObjectMetaMapOutput)
}

// region IngressMapMap

type IngressMapMapInput interface {
	pulumi.Input

	ToIngressMapMapOutput() IngressMapMapOutput
	ToIngressMapMapOutputWithContext(context.Context) IngressMapMapOutput
}

type IngressMapMap map[string]map[string]*netwv1.Ingress

func (IngressMapMap) ElementType() reflect.Type {
	return reflect.TypeOf((*map[string]map[string]*netwv1.Ingress)(nil)).Elem()
}

func (i IngressMapMap) ToIngressMapMapOutput() IngressMapMapOutput {
	return i.ToIngressMapMapOutputWithContext(context.Background())
}

func (i IngressMapMap) ToIngressMapMapOutputWithContext(ctx context.Context) IngressMapMapOutput {
	return pulumi.ToOutputWithContext(ctx, i).(IngressMapMapOutput)
}

type IngressMapMapOutput struct{ *pulumi.OutputState }

func (IngressMapMapOutput) ElementType() reflect.Type {
	return reflect.TypeOf((*map[string]map[string]*netwv1.Ingress)(nil)).Elem()
}

func (o IngressMapMapOutput) ToIngressMapMapOutput() IngressMapMapOutput {
	return o
}

func (o IngressMapMapOutput) ToIngressMapMapOutputWithContext(_ context.Context) IngressMapMapOutput {
	return o
}

func (o IngressMapMapOutput) MapIndex(k pulumi.StringInput) netwv1.IngressMapOutput {
	return pulumi.All(o, k).ApplyT(func(all []any) map[string]netwv1.IngressSpec {
		return all[0].(map[string]map[string]netwv1.IngressSpec)[all[1].(string)]
	}).(netwv1.IngressMapOutput)
}

// region IngressSpecMapMap

type IngressSpecMapMapInput interface {
	pulumi.Input

	ToIngressSpecMapMapOutput() IngressSpecMapMapOutput
	ToIngressSpecMapMapOutputWithContext(context.Context) IngressSpecMapMapOutput
}

type IngressSpecMapMap map[string]map[string]netwv1.IngressSpec

func (IngressSpecMapMap) ElementType() reflect.Type {
	return reflect.TypeOf((*map[string]map[string]netwv1.IngressSpec)(nil)).Elem()
}

func (i IngressSpecMapMap) ToIngressSpecMapMapOutput() IngressSpecMapMapOutput {
	return i.ToIngressSpecMapMapOutputWithContext(context.Background())
}

func (i IngressSpecMapMap) ToIngressSpecMapMapOutputWithContext(ctx context.Context) IngressSpecMapMapOutput {
	return pulumi.ToOutputWithContext(ctx, i).(IngressSpecMapMapOutput)
}

type IngressSpecMapMapOutput struct{ *pulumi.OutputState }

func (IngressSpecMapMapOutput) ElementType() reflect.Type {
	return reflect.TypeOf((*map[string]map[string]netwv1.IngressSpec)(nil)).Elem()
}

func (o IngressSpecMapMapOutput) ToIngressSpecMapMapOutput() IngressSpecMapMapOutput {
	return o
}

func (o IngressSpecMapMapOutput) ToIngressSpecMapMapOutputWithContext(_ context.Context) IngressSpecMapMapOutput {
	return o
}

func (o IngressSpecMapMapOutput) MapIndex(k pulumi.StringInput) IngressSpecMapOutput {
	return pulumi.All(o, k).ApplyT(func(all []any) map[string]netwv1.IngressSpec {
		return all[0].(map[string]map[string]netwv1.IngressSpec)[all[1].(string)]
	}).(IngressSpecMapOutput)
}

// region ServiceSpecMapMap

type ServiceSpecMapMapInput interface {
	pulumi.Input

	ToServiceSpecMapMapOutput() ServiceSpecMapMapOutput
	ToServiceSpecMapMapOutputWithContext(context.Context) ServiceSpecMapMapOutput
}

type ServiceSpecMapMap map[string]map[string]corev1.ServiceSpec

func (ServiceSpecMapMap) ElementType() reflect.Type {
	return reflect.TypeOf((*map[string]map[string]corev1.ServiceSpec)(nil)).Elem()
}

func (i ServiceSpecMapMap) ToServiceSpecMapMapOutput() ServiceSpecMapMapOutput {
	return i.ToServiceSpecMapMapOutputWithContext(context.Background())
}

func (i ServiceSpecMapMap) ToServiceSpecMapMapOutputWithContext(ctx context.Context) ServiceSpecMapMapOutput {
	return pulumi.ToOutputWithContext(ctx, i).(ServiceSpecMapMapOutput)
}

type ServiceSpecMapMapOutput struct{ *pulumi.OutputState }

func (ServiceSpecMapMapOutput) ElementType() reflect.Type {
	return reflect.TypeOf((*map[string]map[string]corev1.ServiceSpec)(nil)).Elem()
}

func (o ServiceSpecMapMapOutput) ToServiceSpecMapMapOutput() ServiceSpecMapMapOutput {
	return o
}

func (o ServiceSpecMapMapOutput) ToServiceSpecMapMapOutputWithContext(_ context.Context) ServiceSpecMapMapOutput {
	return o
}

func (o ServiceSpecMapMapOutput) MapIndex(k pulumi.StringInput) ServiceSpecMapOutput {
	return pulumi.All(o, k).ApplyT(func(all []any) map[string]corev1.ServiceSpec {
		return all[0].(map[string]map[string]corev1.ServiceSpec)[all[1].(string)]
	}).(ServiceSpecMapOutput)
}

func init() {
	pulumi.RegisterInputType(reflect.TypeOf((*ObjectMetaArrayInput)(nil)).Elem(), ObjectMetaArray{})
	pulumi.RegisterInputType(reflect.TypeOf((*ObjectMetaMapInput)(nil)).Elem(), ObjectMetaMap{})
	pulumi.RegisterInputType(reflect.TypeOf((*IngressSpecMapInput)(nil)).Elem(), IngressSpecMap{})
	pulumi.RegisterInputType(reflect.TypeOf((*ServiceSpecMapInput)(nil)).Elem(), ServiceSpecMap{})
	pulumi.RegisterInputType(reflect.TypeOf((*ServiceMapMapInput)(nil)).Elem(), ServiceMapMap{})
	pulumi.RegisterInputType(reflect.TypeOf((*ObjectMetaMapMapInput)(nil)).Elem(), ObjectMetaMapMap{})
	pulumi.RegisterInputType(reflect.TypeOf((*IngressMapMapInput)(nil)).Elem(), IngressMapMap{})
	pulumi.RegisterInputType(reflect.TypeOf((*IngressSpecMapMapInput)(nil)).Elem(), IngressSpecMapMap{})
	pulumi.RegisterInputType(reflect.TypeOf((*ServiceSpecMapMapInput)(nil)).Elem(), ServiceSpecMapMap{})
	pulumi.RegisterOutputType(ObjectMetaArrayOutput{})
	pulumi.RegisterOutputType(ObjectMetaMapOutput{})
	pulumi.RegisterOutputType(IngressSpecMapOutput{})
	pulumi.RegisterOutputType(ServiceSpecMapOutput{})
	pulumi.RegisterOutputType(ServiceMapMapOutput{})
	pulumi.RegisterOutputType(ObjectMetaMapMapOutput{})
	pulumi.RegisterOutputType(IngressMapMapOutput{})
	pulumi.RegisterOutputType(IngressSpecMapMapOutput{})
	pulumi.RegisterOutputType(ServiceSpecMapMapOutput{})
}
