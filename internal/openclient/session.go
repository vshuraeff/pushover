package openclient

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/vshuraeff/pushover/internal/api"
	"github.com/vshuraeff/pushover/internal/config"
)

const sessionSetupInstructions = "no session found: run `povr client login` and `povr client register` first"

// textreader reads one visible login input after displaying a prompt.
type TextReader func(prompt string) (string, error)

// passwordreader reads one hidden login input after displaying a prompt.
type PasswordReader func(prompt string) (string, error)

// loginoptions configures the interactive inputs used by login.
type LoginOptions struct {
	Email          string
	Input          io.Reader
	Output         io.Writer
	TextReader     TextReader
	PasswordReader PasswordReader
}

// login authenticates a pushover open client user and stores its session secret.
func Login(ctx context.Context, client *api.Client, store config.SessionStore, options LoginOptions) error {
	if client == nil {
		return errors.New("api client is required")
	}

	email, readText, readPassword := loginReaders(options)
	var err error
	if email == "" {
		email, err = readText("Email: ")
		if err != nil {
			return fmt.Errorf("read email: %w", err)
		}
	}

	password, err := readPassword("Password: ")
	if err != nil {
		return fmt.Errorf("read password: %w", err)
	}

	values := url.Values{"email": {email}, "password": {password}}
	response, err := client.PostForm(ctx, "/1/users/login.json", values, api.NoRetry())
	if response != nil && response.StatusCode == http.StatusPreconditionFailed {
		twoFactorCode, readErr := readText("Two-factor authentication code: ")
		if readErr != nil {
			return fmt.Errorf("read two-factor authentication code: %w", readErr)
		}
		values.Set("twofa", twoFactorCode)
		response, err = client.PostForm(ctx, "/1/users/login.json", values, api.NoRetry())
	}
	if err != nil {
		return fmt.Errorf("log in: %w", err)
	}

	var result struct {
		Secret string `json:"secret"`
	}
	if err := decodeSuccess(response, &result); err != nil {
		return fmt.Errorf("decode login response: %w", err)
	}
	if result.Secret == "" {
		return errors.New("decode login response: missing session secret")
	}

	state, err := loadOptionalState(store)
	if err != nil {
		return err
	}
	state.Secret = result.Secret
	if err := store.Save(state); err != nil {
		return fmt.Errorf("save session: %w", err)
	}
	return nil
}

// register creates a named open client device and stores its identifier.
func Register(ctx context.Context, client *api.Client, store config.SessionStore, name string) error {
	if client == nil {
		return errors.New("api client is required")
	}
	if err := validateDeviceName(name); err != nil {
		return err
	}

	state, err := LoadSession(store, false)
	if err != nil {
		return err
	}

	response, err := client.PostForm(ctx, "/1/devices.json", url.Values{
		"secret": {state.Secret},
		"name":   {name},
		"os":     {"O"},
	})
	if err != nil {
		return fmt.Errorf("register device: %w", err)
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := decodeSuccess(response, &result); err != nil {
		return fmt.Errorf("decode device registration response: %w", err)
	}
	if result.ID == "" {
		return errors.New("decode device registration response: missing device id")
	}

	state.DeviceID = result.ID
	state.DeviceName = name
	if err := store.Save(state); err != nil {
		return fmt.Errorf("save session: %w", err)
	}
	return nil
}

// loadsession loads a stored session and verifies its required credentials.
func LoadSession(store config.SessionStore, requireDeviceID bool) (*config.State, error) {
	if store == nil {
		return nil, errors.New(sessionSetupInstructions)
	}
	state, err := store.Load()
	if errors.Is(err, os.ErrNotExist) {
		return nil, errors.New(sessionSetupInstructions)
	}
	if err != nil {
		return nil, fmt.Errorf("load session: %w", err)
	}
	if state == nil || strings.TrimSpace(state.Secret) == "" || (requireDeviceID && strings.TrimSpace(state.DeviceID) == "") {
		return nil, errors.New(sessionSetupInstructions)
	}
	return state, nil
}

func loginReaders(options LoginOptions) (string, TextReader, PasswordReader) {
	input := options.Input
	if input == nil {
		input = os.Stdin
	}
	output := options.Output
	if output == nil {
		output = os.Stdout
	}

	textReader := options.TextReader
	if textReader == nil {
		reader := bufio.NewReader(input)
		textReader = func(prompt string) (string, error) {
			if _, err := fmt.Fprint(output, prompt); err != nil {
				return "", err
			}
			line, err := reader.ReadString('\n')
			if err != nil && !errors.Is(err, io.EOF) {
				return "", err
			}
			return strings.TrimSpace(line), nil
		}
	}

	passwordReader := options.PasswordReader
	if passwordReader == nil {
		passwordReader = func(prompt string) (string, error) {
			if _, err := fmt.Fprint(output, prompt); err != nil {
				return "", err
			}
			password, err := term.ReadPassword(int(os.Stdin.Fd()))
			if _, writeErr := fmt.Fprintln(output); err == nil && writeErr != nil {
				return "", writeErr
			}
			if err != nil {
				return "", err
			}
			return string(password), nil
		}
	}

	return strings.TrimSpace(options.Email), textReader, passwordReader
}

func loadOptionalState(store config.SessionStore) (*config.State, error) {
	if store == nil {
		return nil, errors.New("session store is required")
	}
	state, err := store.Load()
	if errors.Is(err, os.ErrNotExist) {
		return &config.State{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load session: %w", err)
	}
	if state == nil {
		return &config.State{}, nil
	}
	return state, nil
}

func validateDeviceName(name string) error {
	if name == "" || len(name) > 25 {
		return errors.New("device name must be 1 to 25 characters")
	}
	for _, character := range name {
		if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') && (character < '0' || character > '9') && character != '_' && character != '-' {
			return errors.New("device name may contain only letters, numbers, underscores, and hyphens")
		}
	}
	return nil
}

func decodeSuccess(response *api.Response, destination any) error {
	if response == nil {
		return errors.New("empty api response")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("unexpected http status %d", response.StatusCode)
	}
	if err := json.Unmarshal(response.Body, destination); err != nil {
		return err
	}
	return nil
}
