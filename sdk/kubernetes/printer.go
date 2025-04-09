package kubernetes

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Printer is a triggerable formatter.
// By defining services as the containers name and port binding (if many
// ports are defined, else optional) such that the service name to
// reach will be dynamically provided.
type Printer struct {
	// Fmt is the format of the string to output.
	Fmt string `pulumi:"fmt"`

	// Services are formatted as <service>[:<port>[/<protocol>]].
	// It defaults protocol to TCP, and if there is only 1 port,
	// defaults to it.
	Services []string `pulumi:"services"`
}

type PrinterInput interface {
	pulumi.Input

	ToPrinterOutput() PrinterOutput
	ToPrinterOutputWithContext(context.Context) PrinterOutput
}

// PrinterArgs is the input of [Printer], e.g. is a triggerable formatter.
// By defining services as the containers name and port binding (if many
// ports are defined, else optional) such that the service name to
// reach will be dynamically provided.
type PrinterArgs struct {
	// Fmt is the format of the string to output.
	Fmt pulumi.StringInput `pulumi:"fmt"`

	// Services are formatted as <service>[:<port>[/<protocol>]].
	// It defaults protocol to TCP, and if there is only 1 port,
	// defaults to it.
	Services pulumi.StringArrayInput `pulumi:"services"`
}

func (PrinterArgs) ElementType() reflect.Type {
	return reflect.TypeOf((*Printer)(nil)).Elem()
}

func (i PrinterArgs) ToPrinterOutput() PrinterOutput {
	return i.ToPrinterOutputWithContext(context.Background())
}

func (i PrinterArgs) ToPrinterOutputWithContext(ctx context.Context) PrinterOutput {
	return pulumi.ToOutputWithContext(ctx, i).(PrinterOutput)
}

type PrinterOutput struct{ *pulumi.OutputState }

func (PrinterOutput) ElementType() reflect.Type {
	return reflect.TypeOf((*Printer)(nil)).Elem()
}

func (o PrinterOutput) ToPrinterOutput() PrinterOutput {
	return o
}

func (o PrinterOutput) ToPrinterOutputWithContext(_ context.Context) PrinterOutput {
	return o
}

// Fmt is the format of the string to output.
func (o PrinterOutput) Fmt() pulumi.StringOutput {
	return o.ApplyT(func(v Printer) string {
		return v.Fmt
	}).(pulumi.StringOutput)
}

// Services are formatted as <service>[:<port>[/<protocol>]].
// It defaults protocol to TCP, and if there is only 1 port,
// defaults to it.
func (o PrinterOutput) Services() pulumi.StringArrayOutput {
	return o.ApplyT(func(v Printer) []string {
		return v.Services
	}).(pulumi.StringArrayOutput)
}

// ToPrinter is a helper that transforms a [pulumi.StringInput] to
// a [PrinterOutput], by using the string as the formatter and no
// services.
// Example:
//
//	ToPrinter(pulumi.String("..."))
func ToPrinter(in pulumi.StringInput) PrinterOutput {
	return PrinterArgs{Fmt: in}.ToPrinterOutput()
}

type PrinterMapInput interface {
	pulumi.Input

	ToPrinterMapOutput() PrinterMapOutput
	ToPrinterMapOutputWithContext(context.Context) PrinterMapOutput
}

type PrinterMap map[string]PrinterInput

func (PrinterMap) ElementType() reflect.Type {
	return reflect.TypeOf((*map[string]Printer)(nil)).Elem()
}

func (i PrinterMap) ToPrinterMapOutput() PrinterMapOutput {
	return i.ToPrinterMapOutputWithContext(context.Background())
}

func (i PrinterMap) ToPrinterMapOutputWithContext(ctx context.Context) PrinterMapOutput {
	return pulumi.ToOutputWithContext(ctx, i).(PrinterMapOutput)
}

type PrinterMapOutput struct{ *pulumi.OutputState }

func (PrinterMapOutput) ElementType() reflect.Type {
	return reflect.TypeOf((*map[string]Printer)(nil)).Elem()
}

func (o PrinterMapOutput) ToPrinterMapOutput() PrinterMapOutput {
	return o
}

func (o PrinterMapOutput) ToPrinterMapOutputWithContext(_ context.Context) PrinterMapOutput {
	return o
}

func (o PrinterMapOutput) MapIndex(k pulumi.StringInput) PrinterOutput {
	return pulumi.All(o, k).ApplyT(func(all []any) Printer {
		return all[0].(map[string]Printer)[all[1].(string)]
	}).(PrinterOutput)
}

// Print triggers the formatters using the service object metas to
// resolve dependencies.
func (o PrinterMapOutput) Print(deps ObjectMetaMapMapInput) pulumi.StringMapOutput {
	return pulumi.All(o, deps).ApplyT(func(all []any) map[string]string {
		mp := all[0].(map[string]Printer)
		deps := all[1].(map[string]map[string]metav1.ObjectMeta)

		out := map[string]string{}
		for k, v := range mp {
			metas := make([]metav1.ObjectMeta, len(v.Services))
			for i, name := range v.Services {
				// Get service by name
				svcName, pb, hasPb := strings.Cut(name, ":")
				svc := deps[svcName]

				// Look for a corresponding meta
				var meta metav1.ObjectMeta
				if hasPb {
					// Infer port and protocol
					port, prot, hasProt := strings.Cut(pb, "/")
					if !hasProt {
						prot = "TCP"
					}
					pb = fmt.Sprintf("%s/%s", port, prot)
					meta = svc[pb]
				} else {
					// Get first value (should have no more than 1
					// thanks to pre-flight checks).
					for _, v := range svc {
						meta = v
						break
					}
				}

				// Record spec in same order than services
				metas[i] = meta
			}

			names := []any{}
			for _, meta := range metas {
				names = append(names, *meta.Name)
			}
			out[k] = fmt.Sprintf(v.Fmt, names...)
		}
		return out
	}).(pulumi.StringMapOutput)
}

func init() {
	pulumi.RegisterInputType(reflect.TypeOf((*PrinterInput)(nil)).Elem(), PrinterArgs{})
	pulumi.RegisterInputType(reflect.TypeOf((*PrinterMapInput)(nil)).Elem(), PrinterMap{})
	pulumi.RegisterOutputType(PrinterOutput{})
	pulumi.RegisterOutputType(PrinterMapOutput{})
}

// NewPrinter is a helper that transforms native strings to pulumi
// strings, easing the writing of a scenario.
// Example:
//
//	NewPrinter("http://%s", "a")
func NewPrinter(fmt string, services ...string) PrinterArgs {
	return PrinterArgs{
		Fmt:      pulumi.String(fmt),
		Services: pulumi.ToStringArray(services),
	}
}
