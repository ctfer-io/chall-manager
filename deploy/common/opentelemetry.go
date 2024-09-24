package common

import "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

type (
	OtelArgs struct {
		// The OpenTelemetry service name to export signals with.
		ServiceName pulumi.StringPtrInput

		// The OpenTelemetry Collector (OTLP through gRPC) endpoint to send signals to.
		Endpoint pulumi.StringInput

		// Set to true if the endpoint is insecure (i.e. no TLS).
		Insecure bool
	}
)
