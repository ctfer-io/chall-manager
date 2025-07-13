package global

var (
	Version = ""
)

// Configuration holds the parameters that are shared across submodules.
type Configuration struct {
	Directory string
	LogLevel  string

	Otel struct {
		Tracing     bool
		ServiceName string
	}

	Lock struct {
		Kind string

		// For lock kind "etcd"

		EtcdEndpoint string
		EtcdUsername string
		EtcdPassword string
	}

	OCI struct {
		Insecure bool
		Username *string
		Password *string
	}
}

var (
	Conf Configuration
)
