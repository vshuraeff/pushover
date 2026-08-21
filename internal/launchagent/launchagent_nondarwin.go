//go:build !darwin

package launchagent

func Install(InstallOptions) error {
	return ErrNotImplemented
}

func Uninstall(UninstallOptions) error {
	return ErrNotImplemented
}
