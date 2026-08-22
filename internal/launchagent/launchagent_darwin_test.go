//go:build darwin

package launchagent

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type commandInvocation struct {
	name string
	args []string
}

func TestInstallWritesPlistAndBootstrapsAgent(t *testing.T) {
	dest := t.TempDir()
	realExecutable := filepath.Join(dest, "povr")
	if err := os.WriteFile(realExecutable, []byte("povr"), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	executableLink := filepath.Join(dest, "povr-link")
	if err := os.Symlink(realExecutable, executableLink); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	invocations := swapExecCommand(t, nil)
	if err := Install(InstallOptions{Executable: executableLink, Dest: dest}); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	plistPath := filepath.Join(dest, plistFilename)
	contents, err := os.ReadFile(plistPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	info, err := os.Stat(plistPath)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o644); got != want {
		t.Errorf("plist permissions = %o, want %o", got, want)
	}
	values := decodePlistValues(t, contents)
	if got := values["Label"].Text; got != Label {
		t.Errorf("Label = %q, want %q", got, Label)
	}
	arguments := values["ProgramArguments"]
	if got, want := elementTexts(arguments.Elements), []string{realExecutable, "client", "listen"}; !equalStrings(got, want) {
		t.Errorf("ProgramArguments = %q, want %q", got, want)
	}
	if got := values["RunAtLoad"].XMLName.Local; got != "true" {
		t.Errorf("RunAtLoad = <%s>, want <true>", got)
	}
	if got := values["KeepAlive"].XMLName.Local; got != "true" {
		t.Errorf("KeepAlive = <%s>, want <true>", got)
	}

	if got, want := *invocations, []commandInvocation{
		{name: "launchctl", args: []string{"bootout", domainLabel()}},
		{name: "launchctl", args: []string{"bootstrap", guiDomain(), plistPath}},
	}; !equalInvocations(got, want) {
		t.Errorf("launchctl invocations = %#v, want %#v", got, want)
	}
}

func TestInstallKeepsSelfDiscoveredExecutableSymlink(t *testing.T) {
	dest := t.TempDir()
	realExecutable := filepath.Join(dest, "Cellar", "povr", "0.2.1", "bin", "povr")
	if err := os.MkdirAll(filepath.Dir(realExecutable), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(realExecutable, []byte("povr"), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	stableExecutable := filepath.Join(dest, "bin", "povr")
	if err := os.MkdirAll(filepath.Dir(stableExecutable), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.Symlink(realExecutable, stableExecutable); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	swapOSExecutable(t, stableExecutable)
	swapExecCommand(t, nil)
	if err := Install(InstallOptions{Dest: dest}); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	contents, err := os.ReadFile(filepath.Join(dest, plistFilename))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	arguments := decodePlistValues(t, contents)["ProgramArguments"]
	if got, want := elementTexts(arguments.Elements), []string{stableExecutable, "client", "listen"}; !equalStrings(got, want) {
		t.Errorf("ProgramArguments = %q, want %q", got, want)
	}
}

func TestInstallIgnoresUnloadedService(t *testing.T) {
	dest := t.TempDir()
	executable := testExecutable(t, dest)
	invocations := swapExecCommandRunner(t, func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[0] == "bootout" {
			return nil, launchctlExitError(t, 3)
		}
		return nil, nil
	})

	if err := Install(InstallOptions{Executable: executable, Dest: dest}); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if got, want := *invocations, []commandInvocation{
		{name: "launchctl", args: []string{"bootout", domainLabel()}},
		{name: "launchctl", args: []string{"bootstrap", guiDomain(), filepath.Join(dest, plistFilename)}},
	}; !equalInvocations(got, want) {
		t.Errorf("launchctl invocations = %#v, want %#v", got, want)
	}
}

func TestInstallSucceedsWhenBootoutFailsButBootstrapSucceeds(t *testing.T) {
	dest := t.TempDir()
	executable := testExecutable(t, dest)
	invocations := swapExecCommandRunner(t, func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[0] == "bootout" {
			return nil, errors.New("permission denied")
		}
		return nil, nil
	})

	err := Install(InstallOptions{Executable: executable, Dest: dest})
	if err != nil {
		t.Fatalf("Install() error = %v, want nil", err)
	}
	if got, want := *invocations, []commandInvocation{
		{name: "launchctl", args: []string{"bootout", domainLabel()}},
		{name: "launchctl", args: []string{"bootstrap", guiDomain(), filepath.Join(dest, plistFilename)}},
	}; !equalInvocations(got, want) {
		t.Errorf("launchctl invocations = %#v, want %#v", got, want)
	}
}

func TestInstallCombinesBootoutAndBootstrapFailures(t *testing.T) {
	dest := t.TempDir()
	executable := testExecutable(t, dest)
	swapExecCommandRunner(t, func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[0] == "bootout" {
			return nil, errors.New("permission denied")
		}
		return nil, errors.New("bootstrap denied")
	})

	err := Install(InstallOptions{Executable: executable, Dest: dest})
	if err == nil {
		t.Fatal("Install() error = nil, want error")
	}
	for _, want := range []string{"bootout", "bootstrap"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Install() error = %q, want %q", err, want)
		}
	}
}

func TestInstallRetriesBootstrapAfterAsyncBootout(t *testing.T) {
	dest := t.TempDir()
	executable := testExecutable(t, dest)
	bootstrapCalls := 0
	invocations := swapExecCommandRunner(t, func(_ context.Context, _ string, args ...string) ([]byte, error) {
		switch args[0] {
		case "bootout":
			return nil, launchctlExitError(t, 36)
		case "bootstrap":
			bootstrapCalls++
			if bootstrapCalls == 1 {
				return nil, launchctlExitError(t, 5)
			}
		}
		return nil, nil
	})

	if err := Install(InstallOptions{Executable: executable, Dest: dest}); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if bootstrapCalls <= 1 {
		t.Errorf("bootstrap calls = %d, want retry", bootstrapCalls)
	}
	if got, want := len(*invocations), 3; got != want {
		t.Errorf("launchctl invocations = %d, want %d", got, want)
	}
}

func TestInstallFailsAfterAsyncBootoutBootstrapRetriesExhausted(t *testing.T) {
	dest := t.TempDir()
	executable := testExecutable(t, dest)
	bootstrapCalls := 0
	swapExecCommandRunner(t, func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[0] == "bootout" {
			return nil, launchctlExitError(t, 36)
		}
		bootstrapCalls++
		return nil, launchctlExitError(t, 5)
	})

	if err := Install(InstallOptions{Executable: executable, Dest: dest}); err == nil {
		t.Fatal("Install() error = nil, want error")
	}
	if got, want := bootstrapCalls, bootstrapAttempts; got != want {
		t.Errorf("bootstrap calls = %d, want %d", got, want)
	}
}

func TestInstallWrapsBootstrapFailure(t *testing.T) {
	dest := t.TempDir()
	executable := testExecutable(t, dest)
	swapExecCommandRunner(t, func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[0] == "bootstrap" {
			return nil, errors.New("exit status 5")
		}
		return nil, nil
	})

	err := Install(InstallOptions{Executable: executable, Dest: dest})
	if err == nil {
		t.Fatal("Install() error = nil, want error")
	}
	for _, want := range []string{"bootstrap", domainLabel(), "inspect with: launchctl print", "log show"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Install() error = %q, want %q", err, want)
		}
	}
}

func TestLaunchctlErrorIncludesStderr(t *testing.T) {
	_, err := exec.Command("sh", "-c", "echo boom >&2; exit 5").Output()
	if err == nil {
		t.Fatal("shell error = nil, want error")
	}

	got := launchctlError("bootstrap", "/tmp/povr.plist", err).Error()
	for _, want := range []string{"boom", "inspect with: launchctl print", "log show"} {
		if !strings.Contains(got, want) {
			t.Errorf("launchctlError() = %q, want %q", got, want)
		}
	}
}

func TestUninstallBootsOutAgentAndRemovesPlist(t *testing.T) {
	dest := t.TempDir()
	plistPath := filepath.Join(dest, plistFilename)
	if err := os.WriteFile(plistPath, []byte("plist"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	invocations := swapExecCommand(t, nil)
	if err := Uninstall(UninstallOptions{Dest: dest}); err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	if _, err := os.Stat(plistPath); !os.IsNotExist(err) {
		t.Errorf("Stat(installed plist) error = %v, want not exist", err)
	}
	if got, want := *invocations, []commandInvocation{{name: "launchctl", args: []string{"bootout", guiDomain() + "/" + Label}}}; !equalInvocations(got, want) {
		t.Errorf("launchctl invocations = %#v, want %#v", got, want)
	}
}

func TestUninstallMissingPlistIsIdempotent(t *testing.T) {
	dest := t.TempDir()
	invocations := swapExecCommand(t, launchctlExitError(t, 3))
	if err := Uninstall(UninstallOptions{Dest: dest}); err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	if got, want := *invocations, []commandInvocation{{name: "launchctl", args: []string{"bootout", guiDomain() + "/" + Label}}}; !equalInvocations(got, want) {
		t.Errorf("launchctl invocations = %#v, want %#v", got, want)
	}
}

func TestStartRequiresInstalledPlist(t *testing.T) {
	dest := t.TempDir()
	invocations := swapExecCommand(t, nil)

	err := Start(StartOptions{Dest: dest})
	if err == nil || !strings.Contains(err.Error(), "povr daemon install") {
		t.Fatalf("Start() error = %v, want install guidance", err)
	}
	if len(*invocations) != 0 {
		t.Errorf("launchctl invocations = %#v, want none", *invocations)
	}
}

func TestStartIsIdempotentWhenLoaded(t *testing.T) {
	dest := t.TempDir()
	plistPath := writeTestPlist(t, dest)
	invocations := swapExecCommand(t, nil)

	if err := Start(StartOptions{Dest: dest}); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if got, want := *invocations, []commandInvocation{{name: "launchctl", args: []string{"print", domainLabel()}}}; !equalInvocations(got, want) {
		t.Errorf("launchctl invocations = %#v, want %#v", got, want)
	}
	if _, err := os.Stat(plistPath); err != nil {
		t.Errorf("Stat(plist) error = %v", err)
	}
}

func TestStartBootstrapsUnloadedService(t *testing.T) {
	dest := t.TempDir()
	plistPath := writeTestPlist(t, dest)
	invocations := swapExecCommandRunner(t, func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[0] == "print" {
			return nil, launchctlExitError(t, 3)
		}
		return nil, nil
	})

	if err := Start(StartOptions{Dest: dest}); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if got, want := *invocations, []commandInvocation{
		{name: "launchctl", args: []string{"print", domainLabel()}},
		{name: "launchctl", args: []string{"bootstrap", guiDomain(), plistPath}},
	}; !equalInvocations(got, want) {
		t.Errorf("launchctl invocations = %#v, want %#v", got, want)
	}
}

func TestStopIgnoresUnloadedService(t *testing.T) {
	dest := t.TempDir()
	invocations := swapExecCommand(t, launchctlExitError(t, 3))

	if err := Stop(StopOptions{Dest: dest}); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if got, want := *invocations, []commandInvocation{{name: "launchctl", args: []string{"bootout", domainLabel()}}}; !equalInvocations(got, want) {
		t.Errorf("launchctl invocations = %#v, want %#v", got, want)
	}
}

func TestRestartKickstartsLoadedService(t *testing.T) {
	dest := t.TempDir()
	invocations := swapExecCommand(t, nil)

	if err := Restart(RestartOptions{Dest: dest}); err != nil {
		t.Fatalf("Restart() error = %v", err)
	}
	if got, want := *invocations, []commandInvocation{
		{name: "launchctl", args: []string{"print", domainLabel()}},
		{name: "launchctl", args: []string{"kickstart", "-k", domainLabel()}},
	}; !equalInvocations(got, want) {
		t.Errorf("launchctl invocations = %#v, want %#v", got, want)
	}
}

func TestStatusReportsPlistAndServiceDetails(t *testing.T) {
	dest := t.TempDir()
	writeTestPlist(t, dest)
	invocations := swapExecCommandRunner(t, func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return []byte("\tpid = 123\n\tlast exit code = 7\n"), nil
	})

	status, err := Status(StatusOptions{Dest: dest})
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if !status.PlistInstalled || !status.Loaded || status.PID != 123 || status.LastExitCode != 7 || !status.HasLastExitCode {
		t.Errorf("Status() = %#v", status)
	}
	if got, want := *invocations, []commandInvocation{{name: "launchctl", args: []string{"print", domainLabel()}}}; !equalInvocations(got, want) {
		t.Errorf("launchctl invocations = %#v, want %#v", got, want)
	}
}

func TestStatusReportsUnloadedServiceWithoutFailing(t *testing.T) {
	dest := t.TempDir()
	invocations := swapExecCommand(t, launchctlExitError(t, 113))

	status, err := Status(StatusOptions{Dest: dest})
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.PlistInstalled || status.Loaded {
		t.Errorf("Status() = %#v", status)
	}
	for _, want := range []string{"print", domainLabel(), "inspect with: launchctl print"} {
		if !strings.Contains(status.Detail, want) {
			t.Errorf("status detail = %q, want %q", status.Detail, want)
		}
	}
	if got, want := *invocations, []commandInvocation{{name: "launchctl", args: []string{"print", domainLabel()}}}; !equalInvocations(got, want) {
		t.Errorf("launchctl invocations = %#v, want %#v", got, want)
	}
}

func TestStatusTreatsESRCHAsUnloadedService(t *testing.T) {
	dest := t.TempDir()
	swapExecCommand(t, launchctlExitError(t, 3))

	status, err := Status(StatusOptions{Dest: dest})
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.PlistInstalled || status.Loaded {
		t.Errorf("Status() = %#v", status)
	}
}

func TestStatusReturnsErrorWhenLaunchctlPrintFails(t *testing.T) {
	dest := t.TempDir()
	swapExecCommand(t, launchctlExitError(t, 1))

	status, err := Status(StatusOptions{Dest: dest})
	if err == nil {
		t.Fatal("Status() error = nil, want error")
	}
	if status != (StatusResult{}) {
		t.Errorf("Status() = %#v, want zero value", status)
	}
	for _, want := range []string{"print", domainLabel()} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Status() error = %q, want %q", err, want)
		}
	}
}

func TestWritePlistRemovesTemporaryFileAfterRenameFailure(t *testing.T) {
	dest := t.TempDir()
	path := filepath.Join(dest, plistFilename)
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}

	err := writePlist(path, "/tmp/povr")
	if err == nil {
		t.Fatal("writePlist() error = nil, want error")
	}
	files, err := filepath.Glob(filepath.Join(dest, "."+plistFilename+".*"))
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	if len(files) != 0 {
		t.Errorf("temporary plist files = %q, want none", files)
	}
}

func TestWritePlistUsesSelfClosingBooleanTags(t *testing.T) {
	path := filepath.Join(t.TempDir(), plistFilename)
	if err := writePlist(path, "/tmp/povr"); err != nil {
		t.Fatalf("writePlist() error = %v", err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if strings.Contains(string(contents), "<true></true>") {
		t.Error("plist contains paired true tags")
	}
	if !strings.Contains(string(contents), "<true/>") {
		t.Error("plist does not contain self-closing true tag")
	}

	plutil, err := exec.LookPath("plutil")
	if err != nil {
		t.Skip("plutil is not available")
	}
	if output, err := exec.Command(plutil, "-lint", path).CombinedOutput(); err != nil {
		t.Fatalf("plutil -lint error = %v, output = %s", err, output)
	}
}

func swapExecCommand(t *testing.T, result error) *[]commandInvocation {
	t.Helper()
	return swapExecCommandRunner(t, func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return nil, result
	})
}

func swapOSExecutable(t *testing.T, executable string) {
	t.Helper()
	originalOSExecutable := osExecutable
	osExecutable = func() (string, error) {
		return executable, nil
	}
	t.Cleanup(func() {
		osExecutable = originalOSExecutable
	})
}

func swapExecCommandRunner(t *testing.T, runner execRunner) *[]commandInvocation {
	t.Helper()
	originalExecCommand := execCommand
	invocations := &[]commandInvocation{}
	execCommand = func(_ context.Context, name string, args ...string) ([]byte, error) {
		*invocations = append(*invocations, commandInvocation{name: name, args: append([]string(nil), args...)})
		return runner(context.Background(), name, args...)
	}
	t.Cleanup(func() {
		execCommand = originalExecCommand
	})
	return invocations
}

func launchctlExitError(t *testing.T, code int) error {
	t.Helper()
	_, err := exec.Command("sh", "-c", "exit "+strconv.Itoa(code)).Output()
	if err == nil {
		t.Fatalf("shell exit %d error = nil", code)
	}
	return err
}

func testExecutable(t *testing.T, dest string) string {
	t.Helper()
	path := filepath.Join(dest, "povr")
	if err := os.WriteFile(path, []byte("povr"), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func writeTestPlist(t *testing.T, dest string) string {
	t.Helper()
	path := filepath.Join(dest, plistFilename)
	if err := os.WriteFile(path, []byte("plist"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func decodePlistValues(t *testing.T, contents []byte) map[string]plistElement {
	t.Helper()
	var document plistDocument
	if err := xml.NewDecoder(bytes.NewReader(contents)).Decode(&document); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if len(document.Elements) != 1 || document.Elements[0].XMLName.Local != "dict" {
		t.Fatalf("plist elements = %#v, want one dict", document.Elements)
	}
	dict := document.Elements[0].Elements
	if len(dict)%2 != 0 {
		t.Fatalf("dict elements = %#v, want key-value pairs", dict)
	}
	values := make(map[string]plistElement, len(dict)/2)
	for index := 0; index < len(dict); index += 2 {
		if dict[index].XMLName.Local != "key" {
			t.Fatalf("dict element %d = <%s>, want <key>", index, dict[index].XMLName.Local)
		}
		values[dict[index].Text] = dict[index+1]
	}
	return values
}

func elementTexts(elements []plistElement) []string {
	values := make([]string, len(elements))
	for index, element := range elements {
		values[index] = element.Text
	}
	return values
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func equalInvocations(left, right []commandInvocation) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].name != right[index].name || !equalStrings(left[index].args, right[index].args) {
			return false
		}
	}
	return true
}
