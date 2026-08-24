//go:build darwin

package launchagent

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const plistFilename = Label + ".plist"

const plistDocType = `<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">`

const bootstrapAttempts = 3

const bootstrapRetryDelay = 200 * time.Millisecond

type execRunner func(ctx context.Context, name string, args ...string) (stdout []byte, err error)

var execCommand execRunner = func(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

var osExecutable = os.Executable

func Install(opts InstallOptions) error {
	executable, err := executablePath(opts.Executable)
	if err != nil {
		return err
	}
	plistPath, err := launchAgentPath(opts.Dest)
	if err != nil {
		return err
	}
	logDirectory, err := launchAgentLogDirectory()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(logDirectory, 0o755); err != nil {
		return fmt.Errorf("launchagent: create log directory %q: %w", logDirectory, err)
	}
	if err := writePlist(plistPath, executable, logDirectory); err != nil {
		return err
	}
	bootoutErr := bootout()
	if isServiceNotLoaded(bootoutErr) {
		bootoutErr = nil
	} else if bootoutErr != nil {
		bootoutErr = launchctlError("bootout", domainLabel(), bootoutErr)
	}
	if err := bootstrapWithRetry(plistPath, bootoutErr); err != nil {
		if bootoutErr != nil {
			return fmt.Errorf("%v; %w", bootoutErr, err)
		}
		return err
	}
	return nil
}

func Uninstall(opts UninstallOptions) error {
	plistPath, err := launchAgentPath(opts.Dest)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(plistPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("launchagent: inspect plist %q: %w", plistPath, err)
	}

	bootoutErr := bootout()
	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("launchagent: remove plist %q: %w", plistPath, err)
	}
	if isServiceNotLoaded(bootoutErr) {
		return nil
	}
	if bootoutErr != nil {
		return launchctlError("bootout", domainLabel(), bootoutErr)
	}
	return nil
}

func Start(opts StartOptions) error {
	plistPath, err := launchAgentPath(opts.Dest)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(plistPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("launchagent: no plist installed at %q; run 'povr daemon install' first", plistPath)
		}
		return fmt.Errorf("launchagent: inspect plist %q: %w", plistPath, err)
	}
	loaded, _ := serviceStatus()
	if loaded {
		return nil
	}
	return bootstrap(plistPath)
}

func Stop(opts StopOptions) error {
	if _, err := launchAgentPath(opts.Dest); err != nil {
		return err
	}
	if err := bootout(); err != nil {
		if isServiceNotLoaded(err) {
			return nil
		}
		return launchctlError("bootout", domainLabel(), err)
	}
	return nil
}

func Restart(opts RestartOptions) error {
	loaded, _ := serviceStatus()
	if !loaded {
		return Start(StartOptions{Dest: opts.Dest})
	}
	if _, err := execCommand(context.Background(), "launchctl", "kickstart", "-k", domainLabel()); err != nil {
		return launchctlError("kickstart", domainLabel(), err)
	}
	return nil
}

func Status(opts StatusOptions) (StatusResult, error) {
	plistPath, err := launchAgentPath(opts.Dest)
	if err != nil {
		return StatusResult{}, err
	}
	result := StatusResult{}
	if _, err := os.Lstat(plistPath); err != nil {
		if !os.IsNotExist(err) {
			return StatusResult{}, fmt.Errorf("launchagent: inspect plist %q: %w", plistPath, err)
		}
	} else {
		result.PlistInstalled = true
	}

	loaded, output := serviceStatus()
	if !loaded {
		if !isServiceNotFound(output.err) {
			return StatusResult{}, launchctlError("print", domainLabel(), output.err)
		}
		result.Detail = launchctlError("print", domainLabel(), output.err).Error()
		return result, nil
	}
	result.Loaded = true
	result.Detail = "service is loaded"
	parseStatusOutput(&result, output.stdout)
	return result, nil
}

type serviceStatusResult struct {
	stdout []byte
	err    error
}

func serviceStatus() (bool, serviceStatusResult) {
	stdout, err := execCommand(context.Background(), "launchctl", "print", domainLabel())
	return err == nil, serviceStatusResult{stdout: stdout, err: err}
}

func parseStatusOutput(result *StatusResult, output []byte) {
	for _, line := range strings.Split(string(output), "\n") {
		separator := strings.Index(line, "=")
		if separator < 0 {
			continue
		}
		key, value := line[:separator], line[separator+1:]
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			continue
		}
		switch strings.TrimSpace(key) {
		case "pid":
			result.PID = parsed
		case "last exit code":
			result.LastExitCode = parsed
			result.HasLastExitCode = true
		}
	}
}

func bootstrap(plistPath string) error {
	if _, err := execCommand(context.Background(), "launchctl", "bootstrap", guiDomain(), plistPath); err != nil {
		return launchctlError("bootstrap", plistPath, err)
	}
	return nil
}

func bootstrapWithRetry(plistPath string, bootoutErr error) error {
	err := bootstrap(plistPath)
	for attempt := 1; attempt < bootstrapAttempts && isBootstrapRetryable(bootoutErr, err); attempt++ {
		time.Sleep(bootstrapRetryDelay)
		err = bootstrap(plistPath)
	}
	return err
}

func bootout() error {
	_, err := execCommand(context.Background(), "launchctl", "bootout", domainLabel())
	return err
}

// launchctl bootout exits 3 (ESRCH) when the service is not loaded; confirmed empirically on macOS.
func isServiceNotLoaded(err error) bool {
	return hasLaunchctlExitCode(err, 3)
}

func isServiceNotFound(err error) bool {
	return hasLaunchctlExitCode(err, 113) || hasLaunchctlExitCode(err, 3)
}

func isBootstrapRetryable(bootoutErr, bootstrapErr error) bool {
	return hasLaunchctlExitCode(bootoutErr, 36) && hasLaunchctlExitCode(bootstrapErr, 5)
}

func hasLaunchctlExitCode(err error, code int) bool {
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) {
		return false
	}
	return exitError.ExitCode() == code
}

func domainLabel() string {
	return guiDomain() + "/" + Label
}

func launchctlError(action, target string, err error) error {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if stderr := strings.TrimSpace(string(exitErr.Stderr)); stderr != "" {
			return fmt.Errorf("launchagent: %s %q: %s: %w (inspect with: launchctl print %s; or: log show --predicate 'eventMessage contains %q' --last 5m)", action, target, stderr, err, domainLabel(), Label)
		}
	}
	return fmt.Errorf("launchagent: %s %q: %w (inspect with: launchctl print %s; or: log show --predicate 'eventMessage contains %q' --last 5m)", action, target, err, domainLabel(), Label)
}

func executablePath(path string) (string, error) {
	if path == "" {
		var err error
		path, err = osExecutable()
		if err != nil {
			return "", fmt.Errorf("launchagent: locate executable: %w", err)
		}
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("launchagent: inspect executable %q: %w", path, err)
		}
		return path, nil
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

func launchAgentLogDirectory() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("launchagent: locate home directory: %w", err)
	}
	return filepath.Join(home, "Library", "Logs", "povr"), nil
}

func guiDomain() string {
	return fmt.Sprintf("gui/%d", os.Getuid())
}

func writePlist(path, executable, logDirectory string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("launchagent: create plist directory %q: %w", filepath.Dir(path), err)
	}

	var contents bytes.Buffer
	contents.WriteString(xml.Header)
	contents.WriteString(plistDocType)
	contents.WriteByte('\n')
	encoder := xml.NewEncoder(&contents)
	encoder.Indent("", "\t")
	if err := encoder.Encode(launchdPlist(executable, logDirectory)); err != nil {
		return fmt.Errorf("launchagent: encode plist: %w", err)
	}
	if err := encoder.Flush(); err != nil {
		return fmt.Errorf("launchagent: flush plist: %w", err)
	}
	encoded := bytes.ReplaceAll(contents.Bytes(), []byte("<true></true>"), []byte("<true/>"))
	encoded = bytes.ReplaceAll(encoded, []byte("<false></false>"), []byte("<false/>"))
	temp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return fmt.Errorf("launchagent: create temporary plist %q: %w", path, err)
	}
	tempPath := temp.Name()
	renamed := false
	defer func() {
		if !renamed {
			_ = os.Remove(tempPath)
		}
	}()
	if _, err := temp.Write(encoded); err != nil {
		_ = temp.Close()
		return fmt.Errorf("launchagent: write temporary plist %q: %w", tempPath, err)
	}
	if err := temp.Chmod(0o644); err != nil {
		_ = temp.Close()
		return fmt.Errorf("launchagent: set plist permissions %q: %w", tempPath, err)
	}
	// sync the temporary plist before rename for durability against power loss during install.
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("launchagent: sync temporary plist %q: %w", tempPath, err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("launchagent: close temporary plist %q: %w", tempPath, err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("launchagent: replace plist %q: %w", path, err)
	}
	renamed = true
	return nil
}

func launchdPlist(executable, logDirectory string) plistDocument {
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
					stringElement("key", "StandardOutPath"),
					stringElement("string", filepath.Join(logDirectory, "daemon.out.log")),
					stringElement("key", "StandardErrorPath"),
					stringElement("string", filepath.Join(logDirectory, "daemon.err.log")),
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
