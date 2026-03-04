package global

var (
	Version = ""
)

// Configuration holds the parameters that are shared across submodules.
type Configuration struct {
	Directory string
	Cache     string
	LogLevel  string

	Otel struct {
		Tracing     bool
		ServiceName string
	}

	Etcd struct {
		Endpoint string
		Username string
		Password string //nolint:gosec //#gosec G117 -- FP, we don't marshal this object into JSON
	}

	OCI struct {
		Insecure bool
		Username string
		Password string //nolint:gosec //#gosec G117 -- FP, we don't marshal this object into JSON
	}
}

var (
	Conf Configuration
)
