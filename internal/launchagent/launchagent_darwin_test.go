//go:build darwin

package launchagent

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"os"
	"path/filepath"
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

	if got, want := *invocations, []commandInvocation{{name: "launchctl", args: []string{"bootstrap", guiDomain(), plistPath}}}; !equalInvocations(got, want) {
		t.Errorf("launchctl invocations = %#v, want %#v", got, want)
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
	invocations := swapExecCommand(t, errors.New("service not loaded"))
	if err := Uninstall(UninstallOptions{Dest: dest}); err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	if got, want := *invocations, []commandInvocation{{name: "launchctl", args: []string{"bootout", guiDomain() + "/" + Label}}}; !equalInvocations(got, want) {
		t.Errorf("launchctl invocations = %#v, want %#v", got, want)
	}
}

func swapExecCommand(t *testing.T, result error) *[]commandInvocation {
	t.Helper()
	originalExecCommand := execCommand
	invocations := &[]commandInvocation{}
	execCommand = func(_ context.Context, name string, args ...string) ([]byte, error) {
		*invocations = append(*invocations, commandInvocation{name: name, args: append([]string(nil), args...)})
		return nil, result
	}
	t.Cleanup(func() {
		execCommand = originalExecCommand
	})
	return invocations
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
