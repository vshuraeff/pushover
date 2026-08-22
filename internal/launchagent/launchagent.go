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

type StartOptions struct {
	Dest string
}

type StopOptions struct {
	Dest string
}

type RestartOptions struct {
	Dest string
}

type StatusOptions struct {
	Dest string
}

type StatusResult struct {
	PlistInstalled  bool   `json:"plist_installed"`
	Loaded          bool   `json:"loaded"`
	PID             int    `json:"pid"`
	LastExitCode    int    `json:"last_exit_code"`
	HasLastExitCode bool   `json:"has_last_exit_code"`
	Detail          string `json:"detail"`
}

var ErrNotImplemented = errors.New("launchagent: not implemented on this platform")
