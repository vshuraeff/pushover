package openclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vshuraeff/pushover/internal/api"
	"github.com/vshuraeff/pushover/internal/config"
)

type memoryStore struct {
	state *config.State
	err   error
}

func (s *memoryStore) Load() (*config.State, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.state == nil {
		return nil, os.ErrNotExist
	}
	copy := *s.state
	return &copy, nil
}

func (s *memoryStore) Save(state *config.State) error {
	if state == nil {
		return errors.New("state is nil")
	}
	copy := *state
	s.state = &copy
	return nil
}

func testClient(serverURL string) *api.Client {
	return api.New(serverURL, "test", api.WithSleep(func(time.Duration) {}))
}

func TestLoginStoresSecret(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/1/users/login.json" {
			t.Errorf("request = %s %s", request.Method, request.URL.Path)
		}
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}
		if request.Form.Get("email") != "test@example.com" || request.Form.Get("password") != "fake-password-123" {
			t.Error("login form did not contain the expected fake credentials")
		}
		_, _ = writer.Write([]byte(`{"status":1,"id":"test-user-id","secret":"fake-secret-123"}`))
	}))
	defer server.Close()

	store := &memoryStore{state: &config.State{DeviceID: "existing-device", DeviceName: "existing-name", LastProcessedMessageID: 9}}
	var output strings.Builder
	err := Login(context.Background(), testClient(server.URL), store, LoginOptions{
		Output: &output,
		TextReader: func(prompt string) (string, error) {
			if prompt != "Email: " {
				t.Errorf("prompt = %q", prompt)
			}
			return "test@example.com", nil
		},
		PasswordReader: func(prompt string) (string, error) {
			if prompt != "Password: " {
				t.Errorf("prompt = %q", prompt)
			}
			return "fake-password-123", nil
		},
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if store.state.Secret != "fake-secret-123" {
		t.Errorf("secret = %q", store.state.Secret)
	}
	if store.state.DeviceID != "existing-device" || store.state.DeviceName != "existing-name" || store.state.LastProcessedMessageID != 9 {
		t.Errorf("saved state did not preserve existing device fields: %#v", store.state)
	}
	if strings.Contains(output.String(), "fake-password-123") || strings.Contains(output.String(), "fake-secret-123") {
		t.Error("login output contained fake credentials")
	}
}

func TestLoginRetriesOnceWithTwoFactorCode(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}
		if got := requests.Add(1); got == 1 {
			if request.Form.Get("twofa") != "" {
				t.Error("first login request included a two-factor code")
			}
			writer.WriteHeader(http.StatusPreconditionFailed)
			_, _ = writer.Write([]byte(`{"status":0,"errors":["two-factor authentication required"]}`))
			return
		}
		if request.Form.Get("twofa") != "654321" {
			t.Error("retry did not include the expected two-factor code")
		}
		_, _ = writer.Write([]byte(`{"status":1,"secret":"fake-secret-123"}`))
	}))
	defer server.Close()

	store := &memoryStore{}
	var output strings.Builder
	textPrompts := make([]string, 0, 2)
	err := Login(context.Background(), testClient(server.URL), store, LoginOptions{
		Output: &output,
		TextReader: func(prompt string) (string, error) {
			textPrompts = append(textPrompts, prompt)
			switch prompt {
			case "Email: ":
				return "test@example.com", nil
			case "Two-factor authentication code: ":
				return "654321", nil
			default:
				return "", fmt.Errorf("unexpected prompt %q", prompt)
			}
		},
		PasswordReader: func(string) (string, error) { return "fake-password-123", nil },
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if got := requests.Load(); got != 2 {
		t.Errorf("requests = %d, want 2", got)
	}
	if got := strings.Join(textPrompts, "|"); got != "Email: |Two-factor authentication code: " {
		t.Errorf("text prompts = %q", got)
	}
	if store.state == nil || store.state.Secret != "fake-secret-123" {
		t.Errorf("saved state = %#v", store.state)
	}
	if strings.Contains(output.String(), "fake-password-123") || strings.Contains(output.String(), "654321") || strings.Contains(output.String(), "fake-secret-123") {
		t.Error("login output contained fake credentials")
	}
}

func TestRegisterValidatesAndStoresDevice(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.Method != http.MethodPost || request.URL.Path != "/1/devices.json" {
			t.Errorf("request = %s %s", request.Method, request.URL.Path)
		}
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}
		if request.Form.Get("secret") != "fake-secret-123" || request.Form.Get("name") != "desktop_1" || request.Form.Get("os") != "O" {
			t.Error("registration form did not contain the expected values")
		}
		_, _ = writer.Write([]byte(`{"status":1,"id":"fake-device-123"}`))
	}))
	defer server.Close()

	store := &memoryStore{state: &config.State{Secret: "fake-secret-123", LastProcessedMessageID: 11}}
	for _, name := range []string{strings.Repeat("a", 26), "bad name"} {
		if err := Register(context.Background(), testClient(server.URL), store, name); err == nil {
			t.Errorf("Register(%q) error = nil", name)
		}
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("registration calls after invalid names = %d, want 0", got)
	}
	if err := Register(context.Background(), testClient(server.URL), store, "desktop_1"); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("registration calls = %d, want 1", got)
	}
	if store.state.DeviceID != "fake-device-123" || store.state.DeviceName != "desktop_1" || store.state.Secret != "fake-secret-123" || store.state.LastProcessedMessageID != 11 {
		t.Errorf("saved state = %#v", store.state)
	}
}

func TestLoginAndRegisterPersistSecureState(t *testing.T) {
	t.Setenv("PUSHOVER_CONFIG_DIR", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/1/users/login.json":
			_, _ = writer.Write([]byte(`{"status":1,"secret":"fake-secret-123"}`))
		case "/1/devices.json":
			_, _ = writer.Write([]byte(`{"status":1,"id":"fake-device-123"}`))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	store, err := config.NewFileSessionStore()
	if err != nil {
		t.Fatalf("NewFileSessionStore() error = %v", err)
	}
	err = Login(context.Background(), testClient(server.URL), store, LoginOptions{
		Email:          "test@example.com",
		PasswordReader: func(string) (string, error) { return "fake-password-123", nil },
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if err := Register(context.Background(), testClient(server.URL), store, "desktop_1"); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	statePath, err := config.StateFilePath()
	if err != nil {
		t.Fatalf("StateFilePath() error = %v", err)
	}
	info, err := os.Stat(statePath)
	if err != nil {
		t.Fatalf("Stat(state file) error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("state file permissions = %o, want 600", got)
	}
	state, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if state.Secret != "fake-secret-123" || state.DeviceID != "fake-device-123" || state.DeviceName != "desktop_1" {
		t.Errorf("saved state = %#v", state)
	}
}

func TestLoadSessionRequiresCredentials(t *testing.T) {
	for _, test := range []struct {
		name          string
		store         config.SessionStore
		requireDevice bool
	}{
		{name: "missing file", store: &memoryStore{}, requireDevice: false},
		{name: "missing secret", store: &memoryStore{state: &config.State{}}, requireDevice: false},
		{name: "missing device", store: &memoryStore{state: &config.State{Secret: "fake-secret-123"}}, requireDevice: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := LoadSession(test.store, test.requireDevice)
			if err == nil || !strings.Contains(err.Error(), "povr client login") || !strings.Contains(err.Error(), "povr client register") {
				t.Errorf("LoadSession() error = %v", err)
			}
		})
	}
}

func TestLoginTwoFactorDoesNotRetryAfterSecondFailure(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		writer.WriteHeader(http.StatusPreconditionFailed)
		_, _ = writer.Write([]byte(`{"status":0,"errors":["two-factor authentication required"]}`))
	}))
	defer server.Close()

	err := Login(context.Background(), testClient(server.URL), &memoryStore{}, LoginOptions{
		Email:          "test@example.com",
		PasswordReader: func(string) (string, error) { return "fake-password-123", nil },
		TextReader:     func(string) (string, error) { return "654321", nil },
	})
	if err == nil {
		t.Fatal("Login() error = nil")
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("requests = %d, want 2", got)
	}
}

func TestRegisterUsesStoredSecret(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}
		expected := "name=device&os=O&secret=fake-secret-123"
		if request.Form.Encode() != expected {
			t.Error("unexpected registration form")
		}
		_, _ = writer.Write([]byte(`{"status":1,"id":"fake-device-123"}`))
	}))
	defer server.Close()

	store := &memoryStore{state: &config.State{Secret: "fake-secret-123"}}
	if err := Register(context.Background(), testClient(server.URL), store, "device"); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
}
