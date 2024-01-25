package launch

func NewLauncherServer() *launcherServer {
	return &launcherServer{}
}

type launcherServer struct {
	*UnimplementedLauncherServer // comment it to make sure there is only the issue of "must implement unimplemented ... server"
}
