package launchagent

import "errors"

const Label = "net.vshuraeff.povr.listen"

type InstallOptions struct {
	Executable string
	Dest       string
}

type UninstallOptions struct {
	Dest string
}

var ErrNotImplemented = errors.New("launchagent: not implemented on this platform")
