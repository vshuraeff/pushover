//go:build !darwin

package launchagent

func Install(InstallOptions) error {
	return ErrNotImplemented
}

func Uninstall(UninstallOptions) error {
	return ErrNotImplemented
}

func Start(StartOptions) error {
	return ErrNotImplemented
}

func Stop(StopOptions) error {
	return ErrNotImplemented
}

func Restart(RestartOptions) error {
	return ErrNotImplemented
}

func Status(StatusOptions) (StatusResult, error) {
	return StatusResult{}, ErrNotImplemented
}
