//go:build darwin

package helperapp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type commandInvocation struct {
	name string
	args []string
}

func TestInstallSignsStagedBundleWithoutEntitlements(t *testing.T) {
	source := testBundle(t, "source")
	dest := filepath.Join(t.TempDir(), "Povr.app")

	originalExecCommand := execCommand
	var commands []commandInvocation
	execCommand = func(_ context.Context, name string, args ...string) ([]byte, error) {
		commands = append(commands, commandInvocation{name: name, args: append([]string(nil), args...)})
		return nil, nil
	}
	t.Cleanup(func() {
		execCommand = originalExecCommand
	})

	if err := Install(InstallOptions{Source: source, Dest: dest}); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	var signed bool
	for _, command := range commands {
		if command.name != "codesign" {
			continue
		}
		signed = true
		if !containsArgs(command.args, "--force") {
			t.Errorf("codesign args = %q, want --force", command.args)
		}
		if !containsArgPair(command.args, "--sign", "-") {
			t.Errorf("codesign args = %q, want --sign -", command.args)
		}
		if containsArgs(command.args, "--entitlements") {
			t.Errorf("codesign args = %q, must not include --entitlements", command.args)
		}
	}
	if !signed {
		t.Error("codesign command was not invoked")
	}
	if !containsCommand(commands, lsregisterPath, "-f", dest) {
		t.Errorf("lsregister invocation not found in %q", commands)
	}
	if _, err := os.Stat(filepath.Join(dest, "Contents", "MacOS", "povr-helper")); err != nil {
		t.Errorf("installed helper file: %v", err)
	}
}

func TestInstallIgnoresLaunchServicesRegistrationFailure(t *testing.T) {
	source := testBundle(t, "source")
	dest := filepath.Join(t.TempDir(), "Povr.app")

	originalExecCommand := execCommand
	var commands []commandInvocation
	execCommand = func(_ context.Context, name string, args ...string) ([]byte, error) {
		commands = append(commands, commandInvocation{name: name, args: append([]string(nil), args...)})
		if name == lsregisterPath {
			return nil, errors.New("forced lsregister failure")
		}
		return nil, nil
	}
	t.Cleanup(func() {
		execCommand = originalExecCommand
	})

	if err := Install(InstallOptions{Source: source, Dest: dest}); err != nil {
		t.Fatalf("Install() error = %v, want nil when lsregister fails", err)
	}
	if !containsCommand(commands, "codesign") {
		t.Error("codesign command was not invoked")
	}
	if !containsCommand(commands, lsregisterPath, "-f", dest) {
		t.Errorf("lsregister invocation not found in %q", commands)
	}
}

func TestInstallRestoresPreviousBundleWhenSwapFails(t *testing.T) {
	parent := t.TempDir()
	source := testBundle(t, "source")
	dest := filepath.Join(parent, "Povr.app")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	oldFile := filepath.Join(dest, "previous")
	if err := os.WriteFile(oldFile, []byte("old bundle"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	originalExecCommand := execCommand
	execCommand = func(context.Context, string, ...string) ([]byte, error) {
		return nil, nil
	}
	t.Cleanup(func() {
		execCommand = originalExecCommand
	})
	originalRenamePath := renamePath
	renamePath = func(oldPath, newPath string) error {
		if newPath == dest && filepath.Base(oldPath) == filepath.Base(dest) && strings.Contains(filepath.Dir(oldPath), ".Povr.app.new.") {
			return errors.New("forced staged bundle move failure")
		}
		return os.Rename(oldPath, newPath)
	}
	t.Cleanup(func() {
		renamePath = originalRenamePath
	})

	err := Install(InstallOptions{Source: source, Dest: dest})
	if err == nil {
		t.Fatal("Install() error = nil, want swap failure")
	}
	contents, readErr := os.ReadFile(oldFile)
	if readErr != nil {
		t.Fatalf("ReadFile(previous install) error = %v", readErr)
	}
	if got := string(contents); got != "old bundle" {
		t.Errorf("previous install contents = %q, want old bundle", got)
	}
	stagingPaths, globErr := filepath.Glob(filepath.Join(parent, ".Povr.app.new.*"))
	if globErr != nil {
		t.Fatalf("Glob() error = %v", globErr)
	}
	if len(stagingPaths) != 0 {
		t.Errorf("staging paths = %q, want none", stagingPaths)
	}
}

func TestStatus(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "Povr.app")

	notInstalled, err := Status(dest)
	if err != nil {
		t.Fatalf("Status(not installed) error = %v", err)
	}
	if notInstalled.Installed || notInstalled.BundlePath != dest {
		t.Errorf("Status(not installed) = %+v", notInstalled)
	}

	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	originalExecCommand := execCommand
	entitlements := []byte("<plist><dict><key>com.example.helper</key><true/></dict></plist>")
	var verifyErr error
	execCommand = func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name != "codesign" {
			t.Fatalf("command = %q, want codesign", name)
		}
		switch {
		case containsArgs(args, "--verify"):
			return nil, verifyErr
		case containsArgs(args, "--entitlements"):
			return entitlements, nil
		default:
			t.Fatalf("unexpected codesign args %q", args)
			return nil, nil
		}
	}
	t.Cleanup(func() {
		execCommand = originalExecCommand
	})

	withEntitlements, err := Status(dest)
	if err != nil {
		t.Fatalf("Status(with entitlements) error = %v", err)
	}
	if !withEntitlements.Installed || !withEntitlements.SignatureValid || !withEntitlements.HasEntitlements {
		t.Errorf("Status(with entitlements) = %+v", withEntitlements)
	}

	entitlements = []byte("<plist><dict/></plist>")
	withoutEntitlements, err := Status(dest)
	if err != nil {
		t.Fatalf("Status(without entitlements) error = %v", err)
	}
	if !withoutEntitlements.Installed || !withoutEntitlements.SignatureValid || withoutEntitlements.HasEntitlements {
		t.Errorf("Status(without entitlements) = %+v", withoutEntitlements)
	}

	verifyErr = errors.New("invalid signature")
	invalidSignature, err := Status(dest)
	if err != nil {
		t.Fatalf("Status(invalid signature) error = %v", err)
	}
	if invalidSignature.SignatureValid {
		t.Errorf("Status(invalid signature) = %+v, want SignatureValid false", invalidSignature)
	}
}

func testBundle(t *testing.T, name string) string {
	t.Helper()
	bundle := filepath.Join(t.TempDir(), name, "Povr.app")
	helper := filepath.Join(bundle, "Contents", "MacOS", "povr-helper")
	if err := os.MkdirAll(filepath.Dir(helper), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(helper, []byte("helper"), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return bundle
}

func containsArgs(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func containsArgPair(args []string, first, second string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == first && args[index+1] == second {
			return true
		}
	}
	return false
}

func containsCommand(commands []commandInvocation, wantName string, wantArgs ...string) bool {
	for _, command := range commands {
		if command.name != wantName {
			continue
		}
		if len(wantArgs) == 0 {
			return true
		}
		if len(command.args) != len(wantArgs) {
			continue
		}
		for index, wantArg := range wantArgs {
			if command.args[index] != wantArg {
				break
			}
			if index == len(wantArgs)-1 {
				return true
			}
		}
	}
	return false
}
