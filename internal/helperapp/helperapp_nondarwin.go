//go:build !darwin

package helperapp

// install reports that helper bundle management is unavailable on this platform.
func Install(InstallOptions) error {
	return ErrNotImplemented
}

// status reports that helper bundle management is unavailable on this platform.
func Status(string) (StatusResult, error) {
	return StatusResult{}, ErrNotImplemented
}
