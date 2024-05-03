package instance

func NewManager() *Manager {
	return &Manager{}
}

type Manager struct {
	UnimplementedInstanceManagerServer
}
