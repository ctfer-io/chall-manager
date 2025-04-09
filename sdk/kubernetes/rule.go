package kubernetes

import (
	"context"
	"reflect"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Rule represents a networking rule between [Container].
// It is transformed into a Kubernetes NetworkPolicy.
type Rule struct {
	// From which [Container].
	From string `pulumi:"from"`

	// To which [Container].
	To string `pulumi:"to"`

	// On which port.
	On int `pulumi:"on"`

	// Protocol to communicate with.
	// Defaults to TCP.
	Protocol string `pulumi:"protocol"`
}

type RuleInput interface {
	pulumi.Input

	ToRuleOutput() RuleOutput
	ToRuleOutputWithContext(context.Context) RuleOutput
}

// RuleArgs is the input of [Rule], i.e. represents a networking
// rule between [Container].
// It is transformed into a Kubernetes NetworkPolicy.
type RuleArgs struct {
	// From which [Container].
	From pulumi.StringInput `pulumi:"from"`

	// To which [Container].
	To pulumi.StringInput `pulumi:"to"`

	// On which port.
	On pulumi.IntInput `pulumi:"on"`

	// Protocol to communicate with.
	// Defaults to TCP.
	Protocol pulumi.StringInput `pulumi:"protocol"`
}

func (RuleArgs) ElementType() reflect.Type {
	return reflect.TypeOf((*Rule)(nil)).Elem()
}

func (i RuleArgs) ToRuleOutput() RuleOutput {
	return i.ToRuleOutputWithContext(context.Background())
}

func (i RuleArgs) ToRuleOutputWithContext(ctx context.Context) RuleOutput {
	return pulumi.ToOutputWithContext(ctx, i).(RuleOutput)
}

type RuleOutput struct{ *pulumi.OutputState }

func (o RuleOutput) ElementType() reflect.Type {
	return reflect.TypeOf((*Rule)(nil)).Elem()
}

func (o RuleOutput) ToContainerOutput() RuleOutput {
	return o
}

func (o RuleOutput) ToContainerOutputWithContext(_ context.Context) RuleOutput {
	return o
}

// From which [Container].
func (o RuleOutput) From() pulumi.StringOutput {
	return o.ApplyT(func(v Rule) string {
		return v.From
	}).(pulumi.StringOutput)
}

// To which [Container].
func (o RuleOutput) To() pulumi.StringOutput {
	return o.ApplyT(func(v Rule) string {
		return v.To
	}).(pulumi.StringOutput)
}

// On which port.
func (o RuleOutput) On() pulumi.IntOutput {
	return o.ApplyT(func(v Rule) int {
		return v.On
	}).(pulumi.IntOutput)
}

// Protocol to communicate with.
// Defaults to TCP.
func (o RuleOutput) Protocol() pulumi.StringOutput {
	return o.ApplyT(func(v Rule) string {
		return v.Protocol
	}).(pulumi.StringOutput)
}

type RuleArrayInput interface {
	pulumi.Input

	ToRuleArrayOutput() RuleArrayOutput
	ToRuleArrayOutputWithContext(context.Context) RuleArrayOutput
}

type RuleArray []RuleInput

func (RuleArray) ElementType() reflect.Type {
	return reflect.TypeOf((*[]Rule)(nil)).Elem()
}

func (i RuleArray) ToRuleArrayOutput() RuleArrayOutput {
	return i.ToRuleArrayOutputWithContext(context.Background())
}

func (i RuleArray) ToRuleArrayOutputWithContext(ctx context.Context) RuleArrayOutput {
	return pulumi.ToOutputWithContext(ctx, i).(RuleArrayOutput)
}

type RuleArrayOutput struct{ *pulumi.OutputState }

func (RuleArrayOutput) ElementType() reflect.Type {
	return reflect.TypeOf((*[]Rule)(nil)).Elem()
}

func (o RuleArrayOutput) ToRuleArrayOutput() RuleArrayOutput {
	return o
}

func (o RuleArrayOutput) ToRuleArrayOutputWithContext(_ context.Context) RuleArrayOutput {
	return o
}

func (o RuleArrayOutput) Index(i pulumi.IntInput) RuleOutput {
	return pulumi.All(o, i).ApplyT(func(all []any) Rule {
		return all[0].([]Rule)[all[1].(int)]
	}).(RuleOutput)
}

func init() {
	pulumi.RegisterInputType(reflect.TypeOf((*RuleInput)(nil)).Elem(), RuleArgs{})
	pulumi.RegisterInputType(reflect.TypeOf((*RuleArrayInput)(nil)).Elem(), RuleArray{})
	pulumi.RegisterOutputType(RuleOutput{})
	pulumi.RegisterOutputType(RuleArrayOutput{})
}
