// package helperapp installs and inspects the povr macOS helper bundle.
package helperapp

import "errors"

// installoptions configures a helper bundle installation.
type InstallOptions struct {
	Source string
	Dest   string
}

// statusresult describes the installed helper bundle.
type StatusResult struct {
	Installed       bool   `json:"installed"`
	BundlePath      string `json:"bundle_path"`
	SignatureValid  bool   `json:"signature_valid"`
	HasEntitlements bool   `json:"has_entitlements"`
	Detail          string `json:"detail,omitempty"`
}

// errnotimplemented indicates that helper bundle management is unavailable on this platform.
var ErrNotImplemented = errors.New("helperapp: not implemented on this platform")
