package global

// Configuration holds the parameters that are shared across submodules.
type Configuration struct {
	StatesDir   string
	ScenarioDir string
	Salt        string
}

var (
	Conf Configuration
)
