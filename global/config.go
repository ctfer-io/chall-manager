package global

// Configuration holds the parameters that are shared across submodules.
type Configuration struct {
	Directory string
	Otel      struct {
		Tracing     bool
		ServiceName string
	}
	Lock struct {
		Kind string

		// For lock kind "etcd"

		EtcdEndpoints []string
		EtcdUsername  string
		EtcdPassword  string
	}
	OCI struct {
		RegistryURL *string
		Username    *string
		Password    *string
	}
}

var (
	Conf Configuration
)
