package global

// Configuration holds the parameters that are shared across submodules.
type Configuration struct {
	StatesDir   string
	ScenarioDir string
	Salt        string
	Lock        struct {
		Kind string

		// For lock kind "etcd"

		EtcdEndpoints []string
		EtcdUsername  string
		EtcdPassword  string
	}
}

var (
	Conf Configuration
)
