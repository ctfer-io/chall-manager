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
		Password string
	}

	OCI struct {
		Insecure bool
		Username string
		Password string
	}
}

var (
	Conf Configuration
)
