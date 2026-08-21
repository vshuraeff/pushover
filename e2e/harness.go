//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/vshuraeff/pushover/internal/config"
	"github.com/vshuraeff/pushover/internal/mockpushover"
)

const (
	fixtureAppToken   = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	fixtureUserKey    = "uuuuuuuuuuuuuuuuuuuuuuuuuuuuuu"
	fixtureUserKey2   = "vvvvvvvvvvvvvvvvvvvvvvvvvvvvvv"
	fixtureInvalidKey = "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
	fixtureTeamToken  = "fixture-team-token"
	fixtureDeviceID   = "fixture-device-one"
	fixtureDeviceID2  = "fixture-device-two"
	fixtureSecret     = "fixture-session-secret-one"
	fixtureSecret2    = "fixture-session-secret-two"
	fixtureEmail      = "one@example.test"
	fixturePassword   = "fixture-password-one"
	fixtureTwoFAEmail = "two@example.test"
	fixtureTwoFAPass  = "fixture-password-two"
	fixtureTwoFACode  = "123456"
)

type harness struct {
	server           *mockpushover.Server
	control          *mockpushover.ControlClient
	env              []string
	configDir        string
	notificationPath string
}

type commandResult struct {
	stdout   string
	stderr   string
	err      error
	exitCode int
}

func defaultSeed() mockpushover.SeedState {
	return mockpushover.SeedState{
		Tokens: []mockpushover.AppToken{{
			Token: fixtureAppToken,
			Name:  "E2E App",
			ID:    42,
			Icon:  "fixture-icon",
			Quota: mockpushover.AppQuota{Limit: 100, Remaining: 100, Reset: 1_900_000_000},
		}},
		Users: []mockpushover.User{
			{Key: fixtureUserKey, Email: fixtureEmail, Password: fixturePassword, Secret: fixtureSecret},
			{Key: fixtureUserKey2, Secret: fixtureSecret2},
		},
		TwoFAAccounts: []mockpushover.TwoFAAccount{{
			Email: fixtureTwoFAEmail, Password: fixtureTwoFAPass, User: fixtureUserKey2,
			Secret: fixtureSecret2, Code: fixtureTwoFACode,
		}},
		Devices: []mockpushover.Device{
			{ID: fixtureDeviceID, User: fixtureUserKey, Name: "phone", OS: "O", Secret: fixtureSecret, Enabled: true},
			{ID: fixtureDeviceID2, User: fixtureUserKey2, Name: "tablet", OS: "O", Secret: fixtureSecret2, Enabled: true},
		},
		LicensePools: map[string]mockpushover.LicensePool{
			fixtureAppToken: {Credits: 3},
		},
		Teams: []mockpushover.Team{{
			Token: fixtureTeamToken,
			Name:  "E2E Team",
			Users: []mockpushover.TeamMember{{
				ID: "member-one", Name: "Member One", Email: "member@example.test", Administrator: true,
				Devices: []mockpushover.Device{{ID: "team-device", Name: "phone"}},
			}},
		}},
		CustomSounds: map[string]string{"e2e-custom": "E2E Custom Sound"},
	}
}

func newHarness(t *testing.T, seed mockpushover.SeedState) *harness {
	t.Helper()
	server, err := mockpushover.Start(mockpushover.Config{Addr: "127.0.0.1:0", InitialState: &seed})
	if err != nil {
		t.Fatalf("start mock pushover server: %v", err)
	}
	t.Cleanup(func() {
		if err := server.Close(); err != nil {
			t.Errorf("close mock pushover server: %v", err)
		}
	})

	parsed, err := url.Parse(server.URL())
	if err != nil {
		t.Fatalf("parse mock server url: %v", err)
	}
	if parsed.Scheme != "http" || parsed.Hostname() != "127.0.0.1" {
		t.Fatalf("mock server is not an http localhost endpoint")
	}
	parsed.Scheme = "ws"
	parsed.Path = "/push"
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""

	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	homeDir := filepath.Join(root, "home")
	cacheDir := filepath.Join(root, "cache")
	for _, dir := range []string{configDir, homeDir, cacheDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("create child directory: %v", err)
		}
	}
	notificationPath := filepath.Join(root, "notifications.jsonl")
	env := mergeEnv(os.Environ(), map[string]string{
		"HOME":                    homeDir,
		"XDG_CACHE_HOME":          cacheDir,
		"PUSHOVER_API_URL":        server.URL(),
		"PUSHOVER_WS_URL":         parsed.String(),
		"PUSHOVER_CONFIG_DIR":     configDir,
		"PUSHOVER_NOTIFY_BACKEND": "file:" + notificationPath,
		"PUSHOVER_APP_TOKEN":      fixtureAppToken,
		"PUSHOVER_USER_KEY":       fixtureUserKey,
		"PUSHOVER_TEAM_TOKEN":     fixtureTeamToken,
		"PUSHOVER_KEY":            "",
	})
	return &harness{
		server: server, control: server.Control(), env: env,
		configDir: configDir, notificationPath: notificationPath,
	}
}

func mergeEnv(base []string, overrides map[string]string) []string {
	result := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		name, _, ok := strings.Cut(entry, "=")
		_, replaced := overrides[name]
		if replaced || !ok {
			continue
		}
		result = append(result, entry)
	}
	for name, value := range overrides {
		result = append(result, name+"="+value)
	}
	return result
}

func (h *harness) withEnv(overrides map[string]string) []string {
	return mergeEnv(h.env, overrides)
}

func (h *harness) run(t *testing.T, args ...string) commandResult {
	t.Helper()
	return runProgram(t, h.env, "", binaryPath, args...)
}

func (h *harness) runWithInput(t *testing.T, input string, args ...string) commandResult {
	t.Helper()
	return runProgram(t, h.env, input, binaryPath, args...)
}

func runProgram(t *testing.T, env []string, input, program string, args ...string) commandResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := processCommand(ctx, env, program, args...)
	if input != "" {
		cmd.Stdin = strings.NewReader(input)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatalf("child command timed out")
	}
	exitCode := 0
	if err != nil {
		exitCode = -1
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			exitCode = exitError.ExitCode()
		}
	}
	return commandResult{stdout: stdout.String(), stderr: stderr.String(), err: err, exitCode: exitCode}
}

func processCommand(ctx context.Context, env []string, program string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, program, args...)
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		return signalProcessGroup(cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 2 * time.Second
	return cmd
}

func signalProcessGroup(pid int, signal syscall.Signal) error {
	err := syscall.Kill(-pid, signal)
	if errors.Is(err, syscall.ESRCH) {
		return os.ErrProcessDone
	}
	return err
}

func requireSuccess(t *testing.T, result commandResult) {
	t.Helper()
	if result.err != nil {
		t.Fatalf("command failed with exit code %d: %v\nstdout: %s\nstderr: %s", result.exitCode, result.err, result.stdout, result.stderr)
	}
}

func requireFailure(t *testing.T, result commandResult, message string) {
	t.Helper()
	if result.err == nil || result.exitCode == 0 {
		t.Fatalf("command unexpectedly succeeded: stdout: %s stderr: %s", result.stdout, result.stderr)
	}
	if !strings.Contains(strings.ToLower(result.stderr), strings.ToLower(message)) {
		t.Fatalf("stderr did not contain %q: %s", message, result.stderr)
	}
}

func (h *harness) requests(t *testing.T) []mockpushover.JournaledRequest {
	t.Helper()
	requests, err := h.control.Requests()
	if err != nil {
		t.Fatalf("read mock request journal: %v", err)
	}
	return requests
}

func requireField(t *testing.T, request mockpushover.JournaledRequest, name, want string) {
	t.Helper()
	values, ok := request.Fields[name]
	if !ok || len(values) != 1 || values[0] != want {
		t.Fatalf("request field %q did not match the expected value", name)
	}
}

func writeState(t *testing.T, configDir string, state config.State) {
	t.Helper()
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("encode session state: %v", err)
	}
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("create session state directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "state.json"), append(data, '\n'), 0o600); err != nil {
		t.Fatalf("write session state: %v", err)
	}
}

func readState(t *testing.T, configDir string) config.State {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(configDir, "state.json"))
	if err != nil {
		t.Fatalf("read session state: %v", err)
	}
	var state config.State
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("decode session state: %v", err)
	}
	return state
}

func runViaPTY(t *testing.T, env []string, args, stdinLines []string) commandResult {
	t.Helper()
	const scriptPath = "/usr/bin/script"
	if runtime.GOOS != "darwin" {
		t.Skip("BSD-style /usr/bin/script PTY invocation is only supported on macOS (darwin)")
	}
	if info, err := os.Stat(scriptPath); err != nil || info.IsDir() {
		t.Skip("/usr/bin/script is required for interactive login e2e tests")
	}
	prompts := []string{"Email: ", "Password: "}
	if len(stdinLines) == 3 {
		prompts = append(prompts, "Two-factor authentication code: ")
	}
	if len(stdinLines) != len(prompts) {
		t.Fatalf("pty input line count = %d, want %d", len(stdinLines), len(prompts))
	}

	scriptArgs := append([]string{"-q", "/dev/null", binaryPath}, args...)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := processCommand(ctx, env, scriptPath, scriptArgs...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("open pty stdin: %v", err)
	}
	output := newPromptBuffer()
	cmd.Stdout = output
	cmd.Stderr = output
	if err := cmd.Start(); err != nil {
		stdin.Close()
		t.Fatalf("start pty command: %v", err)
	}
	for i, prompt := range prompts {
		if !output.waitContains(ctx, prompt) {
			cancel()
			stdin.Close()
			_ = cmd.Wait()
			t.Fatalf("pty command did not emit prompt %q", prompt)
		}
		if _, err := fmt.Fprintln(stdin, stdinLines[i]); err != nil {
			cancel()
			stdin.Close()
			_ = cmd.Wait()
			t.Fatalf("write pty input line %d: %v", i+1, err)
		}
	}
	err = cmd.Wait()
	_ = stdin.Close()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatalf("pty command timed out")
	}
	exitCode := 0
	if err != nil {
		exitCode = -1
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			exitCode = exitError.ExitCode()
		}
	}
	return commandResult{stdout: output.String(), err: err, exitCode: exitCode}
}

type promptBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
	change chan struct{}
}

func newPromptBuffer() *promptBuffer {
	return &promptBuffer{change: make(chan struct{}, 1)}
}

func (b *promptBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	n, err := b.buffer.Write(data)
	b.mu.Unlock()
	select {
	case b.change <- struct{}{}:
	default:
	}
	return n, err
}

func (b *promptBuffer) waitContains(ctx context.Context, target string) bool {
	for {
		b.mu.Lock()
		found := strings.Contains(b.buffer.String(), target)
		b.mu.Unlock()
		if found {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-b.change:
		}
	}
}

func (b *promptBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

func eventually(t *testing.T, timeout, interval time.Duration, check func() (bool, string)) {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	last := "no observation"
	for {
		if ok, observation := check(); ok {
			return
		} else {
			last = observation
		}
		select {
		case <-deadline.C:
			t.Fatalf("condition was not met within %s; last observation: %s", timeout, last)
		case <-ticker.C:
		}
	}
}

func consistently(t *testing.T, duration, interval time.Duration, check func() (bool, string)) {
	t.Helper()
	deadline := time.NewTimer(duration)
	defer deadline.Stop()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if ok, observation := check(); !ok {
			t.Fatalf("condition changed during %s: %s", duration, observation)
		}
		select {
		case <-deadline.C:
			return
		case <-ticker.C:
		}
	}
}

type backgroundProcess struct {
	cmd    *exec.Cmd
	cancel context.CancelFunc
	done   chan struct{}
	mu     sync.Mutex
	err    error
	stdout bytes.Buffer
	stderr bytes.Buffer
}

func (h *harness) start(t *testing.T, args ...string) *backgroundProcess {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	process := &backgroundProcess{cancel: cancel, done: make(chan struct{})}
	process.cmd = processCommand(ctx, h.env, binaryPath, args...)
	process.cmd.Stdout = &process.stdout
	process.cmd.Stderr = &process.stderr
	if err := process.cmd.Start(); err != nil {
		cancel()
		t.Fatalf("start child command: %v", err)
	}
	go func() {
		err := process.cmd.Wait()
		process.mu.Lock()
		process.err = err
		process.mu.Unlock()
		close(process.done)
	}()
	t.Cleanup(process.stop)
	return process
}

func (p *backgroundProcess) stop() {
	select {
	case <-p.done:
		p.cancel()
		return
	default:
	}
	_ = signalProcessGroup(p.cmd.Process.Pid, syscall.SIGTERM)
	p.cancel()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case <-p.done:
		return
	case <-timer.C:
		_ = signalProcessGroup(p.cmd.Process.Pid, syscall.SIGKILL)
		<-p.done
	}
}

func (p *backgroundProcess) wait(t *testing.T, timeout time.Duration) error {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-p.done:
		p.mu.Lock()
		defer p.mu.Unlock()
		return p.err
	case <-timer.C:
		t.Fatalf("child process did not exit within %s", timeout)
		return nil
	}
}

func (p *backgroundProcess) output() (string, string) {
	<-p.done
	return p.stdout.String(), p.stderr.String()
}

func describeWSStatus(status mockpushover.WSStatus, err error) string {
	if err != nil {
		return fmt.Sprintf("ws status error: %v", err)
	}
	return fmt.Sprintf("connected=%t connect_count=%d login_ok=%t", status.Connected, status.ConnectCount, status.LoginOK)
}
