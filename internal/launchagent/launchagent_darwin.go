//go:build darwin

package launchagent

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const plistFilename = Label + ".plist"

const plistDocType = `<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">`

type execRunner func(ctx context.Context, name string, args ...string) (stdout []byte, err error)

var execCommand execRunner = func(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

func Install(opts InstallOptions) error {
	executable, err := executablePath(opts.Executable)
	if err != nil {
		return err
	}
	plistPath, err := launchAgentPath(opts.Dest)
	if err != nil {
		return err
	}
	if err := writePlist(plistPath, executable); err != nil {
		return err
	}
	if _, err := execCommand(context.Background(), "launchctl", "bootstrap", guiDomain(), plistPath); err != nil {
		return fmt.Errorf("launchagent: bootstrap %q: %w", plistPath, err)
	}
	return nil
}

func Uninstall(opts UninstallOptions) error {
	plistPath, err := launchAgentPath(opts.Dest)
	if err != nil {
		return err
	}
	_, statErr := os.Lstat(plistPath)
	if statErr != nil && !os.IsNotExist(statErr) {
		return fmt.Errorf("launchagent: inspect plist %q: %w", plistPath, statErr)
	}
	wasAbsent := os.IsNotExist(statErr)

	_, bootoutErr := execCommand(context.Background(), "launchctl", "bootout", guiDomain()+"/"+Label)
	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("launchagent: remove plist %q: %w", plistPath, err)
	}
	if wasAbsent {
		// a missing service makes bootout fail; preserve idempotent uninstall.
		return nil
	}
	if bootoutErr != nil {
		return fmt.Errorf("launchagent: bootout %q: %w", Label, bootoutErr)
	}
	return nil
}

func executablePath(path string) (string, error) {
	if path == "" {
		var err error
		path, err = os.Executable()
		if err != nil {
			return "", fmt.Errorf("launchagent: locate executable: %w", err)
		}
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("launchagent: resolve executable %q: %w", path, err)
	}
	return resolved, nil
}

func launchAgentPath(dest string) (string, error) {
	if dest == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("launchagent: locate home directory: %w", err)
		}
		dest = filepath.Join(home, "Library", "LaunchAgents")
	}
	return filepath.Join(dest, plistFilename), nil
}

func guiDomain() string {
	return fmt.Sprintf("gui/%d", os.Getuid())
}

func writePlist(path, executable string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("launchagent: create plist directory %q: %w", filepath.Dir(path), err)
	}

	var contents bytes.Buffer
	contents.WriteString(xml.Header)
	contents.WriteString(plistDocType)
	contents.WriteByte('\n')
	encoder := xml.NewEncoder(&contents)
	encoder.Indent("", "\t")
	if err := encoder.Encode(launchdPlist(executable)); err != nil {
		return fmt.Errorf("launchagent: encode plist: %w", err)
	}
	if err := encoder.Flush(); err != nil {
		return fmt.Errorf("launchagent: flush plist: %w", err)
	}
	if err := os.WriteFile(path, contents.Bytes(), 0o644); err != nil {
		return fmt.Errorf("launchagent: write plist %q: %w", path, err)
	}
	return nil
}

func launchdPlist(executable string) plistDocument {
	return plistDocument{
		XMLName: xml.Name{Local: "plist"},
		Version: "1.0",
		Elements: []plistElement{
			{
				XMLName: xml.Name{Local: "dict"},
				Elements: []plistElement{
					stringElement("key", "Label"),
					stringElement("string", Label),
					stringElement("key", "ProgramArguments"),
					{
						XMLName: xml.Name{Local: "array"},
						Elements: []plistElement{
							stringElement("string", executable),
							stringElement("string", "client"),
							stringElement("string", "listen"),
						},
					},
					stringElement("key", "RunAtLoad"),
					{XMLName: xml.Name{Local: "true"}},
					stringElement("key", "KeepAlive"),
					{XMLName: xml.Name{Local: "true"}},
				},
			},
		},
	}
}

func stringElement(name, value string) plistElement {
	return plistElement{XMLName: xml.Name{Local: name}, Text: value}
}

type plistDocument struct {
	XMLName  xml.Name       `xml:"plist"`
	Version  string         `xml:"version,attr"`
	Elements []plistElement `xml:",any"`
}

type plistElement struct {
	XMLName  xml.Name
	Text     string         `xml:",chardata"`
	Elements []plistElement `xml:",any"`
}
