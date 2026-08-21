//go:build darwin

package notify

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrHelperBundlePathEmpty = errors.New("notify: helper bundle path is empty")
	ErrHelperNotInstalled    = errors.New("notify: helper binary not found in bundle")
	ErrHelperClosed          = errors.New("notify: helper notifier is closed")
)

// helperrequest is a single line-delimited json request sent to the helper.
type HelperRequest struct {
	ID         string       `json:"id"`
	Title      string       `json:"title"`
	Subtitle   string       `json:"subtitle,omitempty"`
	Body       string       `json:"body"`
	Icon       string       `json:"icon,omitempty"`
	Attachment string       `json:"attachment,omitempty"`
	Sound      *HelperSound `json:"sound,omitempty"`
	Level      string       `json:"level"`
	Thread     string       `json:"thread,omitempty"`
	URL        string       `json:"url,omitempty"`
}

// helpersound is either a named system sound or a sound file path.
type HelperSound struct {
	Name string `json:"name,omitempty"`
	File string `json:"file,omitempty"`
}

// helperevent is a single line-delimited json event received from the helper.
type HelperEvent struct {
	Event string `json:"event"`
	ID    string `json:"id,omitempty"`
	Error string `json:"error,omitempty"`
	URL   string `json:"url,omitempty"`
}

type helperCommand func() *exec.Cmd

// helpernotifier delivers notifications through the installed povr helper app.
type HelperNotifier struct {
	command helperCommand

	mu          sync.Mutex
	cmd         *exec.Cmd
	stdin       io.WriteCloser
	pending     map[string]chan HelperEvent
	processDone chan error
	closed      bool

	writeMu   sync.Mutex
	closeOnce sync.Once
	closeErr  error
	sequence  atomic.Uint64
}

// newhelpernotifier creates a notifier for the helper bundle.
func NewHelperNotifier(bundlePath string) (*HelperNotifier, error) {
	if bundlePath == "" {
		return nil, ErrHelperBundlePathEmpty
	}

	helperPath := filepath.Join(bundlePath, "Contents", "MacOS", "povr-helper")
	info, err := os.Stat(helperPath)
	if err != nil || info.IsDir() {
		if err == nil {
			err = errors.New("path is a directory")
		}
		return nil, fmt.Errorf("%w: %s: %v", ErrHelperNotInstalled, helperPath, err)
	}

	return newHelperNotifier(func() *exec.Cmd {
		return exec.Command(helperPath)
	}), nil
}

func newHelperNotifier(command helperCommand) *HelperNotifier {
	return &HelperNotifier{
		command: command,
		pending: make(map[string]chan HelperEvent),
	}
}

// notify sends one request and waits for the matching delivery result.
func (h *HelperNotifier) Notify(ctx context.Context, n Notification) error {
	request := h.request(n)
	if request.ID == "" {
		request.ID = fmt.Sprintf("povr-%d-%d", os.Getpid(), h.sequence.Add(1))
	}
	data, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("notify: encode helper request: %w", err)
	}
	data = append(data, '\n')

	if err := h.start(); err != nil {
		return err
	}

	result := make(chan HelperEvent, 1)
	h.writeMu.Lock()
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		h.writeMu.Unlock()
		return ErrHelperClosed
	}
	if h.cmd == nil || h.stdin == nil {
		h.mu.Unlock()
		h.writeMu.Unlock()
		return errors.New("notify: helper process is not running")
	}
	if _, exists := h.pending[request.ID]; exists {
		h.mu.Unlock()
		h.writeMu.Unlock()
		return fmt.Errorf("notify: helper request id %q is already pending", request.ID)
	}
	h.pending[request.ID] = result
	stdin := h.stdin
	h.mu.Unlock()

	_, writeErr := stdin.Write(data)
	h.writeMu.Unlock()
	if writeErr != nil {
		h.removePending(request.ID, result)
		return fmt.Errorf("notify: write helper request: %w", writeErr)
	}

	select {
	case event := <-result:
		if event.Event == "error" {
			helperErr := errors.New(event.Error)
			return fmt.Errorf("notify: helper delivery %q: %w", request.ID, helperErr)
		}
		return nil
	case <-ctx.Done():
		h.removePending(request.ID, result)
		return fmt.Errorf("notify: wait for helper delivery %q: %w", request.ID, ctx.Err())
	}
}

// caps reports helper backend capabilities.
func (*HelperNotifier) Caps() Caps {
	return Caps{Images: true, Sound: true, Levels: true, Actions: false}
}

// close stops the helper process and waits for it to exit.
func (h *HelperNotifier) Close() error {
	h.closeOnce.Do(func() {
		h.closeErr = h.closeProcess()
	})
	return h.closeErr
}

func (h *HelperNotifier) start() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closed {
		return ErrHelperClosed
	}
	if h.cmd != nil {
		return nil
	}
	if h.command == nil {
		return errors.New("notify: helper command is not configured")
	}

	cmd := h.command()
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("notify: open helper stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return fmt.Errorf("notify: open helper stdout: %w", err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return fmt.Errorf("notify: start helper: %w", err)
	}

	done := make(chan error, 1)
	h.cmd = cmd
	h.stdin = stdin
	h.processDone = done
	go h.readAndWait(cmd, stdout, done)
	return nil
}

func (h *HelperNotifier) readAndWait(cmd *exec.Cmd, stdout io.ReadCloser, done chan<- error) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		var event HelperEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}
		if event.Event != "delivered" && event.Event != "error" {
			continue
		}

		h.mu.Lock()
		result, ok := h.pending[event.ID]
		if ok {
			delete(h.pending, event.ID)
		}
		h.mu.Unlock()
		if ok {
			result <- event
		}
	}

	readErr := scanner.Err()
	waitErr := cmd.Wait()
	processErr := waitErr
	if readErr != nil {
		processErr = fmt.Errorf("read helper events: %w", readErr)
	}

	h.mu.Lock()
	if h.cmd == cmd {
		h.cmd = nil
		h.stdin = nil
		h.processDone = nil
		message := "helper process exited"
		if processErr != nil {
			message = processErr.Error()
		}
		for id, result := range h.pending {
			delete(h.pending, id)
			result <- HelperEvent{Event: "error", ID: id, Error: message}
		}
	}
	h.mu.Unlock()

	done <- processErr
	close(done)
}

func (h *HelperNotifier) removePending(id string, result chan HelperEvent) {
	h.mu.Lock()
	if h.pending[id] == result {
		delete(h.pending, id)
	}
	h.mu.Unlock()
}

func (h *HelperNotifier) closeProcess() error {
	h.mu.Lock()
	h.closed = true
	stdin := h.stdin
	cmd := h.cmd
	done := h.processDone
	h.mu.Unlock()
	if cmd == nil {
		return nil
	}

	h.writeMu.Lock()
	closeErr := stdin.Close()
	h.writeMu.Unlock()
	var resultErr error
	if closeErr != nil && !errors.Is(closeErr, os.ErrClosed) {
		resultErr = fmt.Errorf("notify: close helper stdin: %w", closeErr)
	}

	select {
	case err := <-done:
		if err != nil {
			return errors.Join(resultErr, fmt.Errorf("notify: wait for helper exit: %w", err))
		}
		return resultErr
	case <-time.After(5 * time.Second):
	}

	var signalErr error
	if err := cmd.Process.Signal(os.Interrupt); err != nil && !errors.Is(err, os.ErrProcessDone) {
		signalErr = fmt.Errorf("notify: interrupt helper: %w", err)
	}
	select {
	case err := <-done:
		if err != nil {
			return errors.Join(resultErr, signalErr, fmt.Errorf("notify: wait for interrupted helper: %w", err))
		}
		return errors.Join(resultErr, signalErr)
	case <-time.After(2 * time.Second):
	}

	if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return errors.Join(resultErr, signalErr, fmt.Errorf("notify: kill helper: %w", err))
	}
	if err := <-done; err != nil {
		return errors.Join(resultErr, signalErr, fmt.Errorf("notify: wait for killed helper: %w", err))
	}
	return errors.Join(resultErr, signalErr)
}

func (h *HelperNotifier) request(n Notification) HelperRequest {
	level := PriorityToLevel(n.Priority)
	request := HelperRequest{
		ID:         n.ID,
		Title:      n.Title,
		Subtitle:   n.AppName,
		Body:       n.Body,
		Icon:       n.IconPath,
		Attachment: n.AttachmentPath,
		Level:      string(level),
		Thread:     n.ThreadID,
		URL:        n.URL,
	}
	if !SuppressSoundForLevel(level) {
		switch {
		case n.SoundName != "":
			request.Sound = &HelperSound{Name: n.SoundName}
		case n.SoundPath != "":
			request.Sound = &HelperSound{File: n.SoundPath}
		}
	}
	return request
}
