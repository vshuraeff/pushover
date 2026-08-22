//go:build darwin

package config

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestKeychainSessionStoreSaveLoad(t *testing.T) {
	want := State{
		DeviceID:               "test-device-id",
		DeviceName:             "test-device-name",
		Secret:                 "test-fake-session-secret-1234",
		LastProcessedMessageID: 42,
	}
	encoded, err := json.Marshal(&want)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	wantScript := "add-generic-password -a " + keychainAccount + " -s " + keychainServiceName + " -w " + base64.StdEncoding.EncodeToString(encoded) + " -U\n"

	var stored []byte
	store := &KeychainSessionStore{
		run: func(stdin string, args ...string) ([]byte, error) {
			switch args[0] {
			case "-iq":
				if len(args) != 1 || stdin != wantScript {
					t.Error("Save() invoked security with unexpected arguments")
					return nil, errors.New("unexpected security arguments")
				}
				stored = []byte(base64.StdEncoding.EncodeToString(encoded))
				return nil, nil
			case "find-generic-password":
				if stdin != "" || len(args) != 6 || args[1] != "-a" || args[2] != keychainAccount || args[3] != "-s" || args[4] != keychainServiceName || args[5] != "-w" {
					t.Error("Load() invoked security with unexpected arguments")
					return nil, errors.New("unexpected security arguments")
				}
				return stored, nil
			default:
				t.Errorf("unexpected security operation %q", args[0])
				return nil, errors.New("unexpected security operation")
			}
		},
	}

	if err := store.Save(&want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if string(stored) != base64.StdEncoding.EncodeToString(encoded) {
		t.Fatal("Save() did not store base64-encoded JSON")
	}

	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if *got != want {
		t.Error("Load() returned unexpected state")
	}
}

func TestKeychainSessionStoreLoadLegacyRawJSON(t *testing.T) {
	want := State{
		DeviceID:               "test-device-id",
		DeviceName:             "test-device-name",
		Secret:                 "test-fake-session-secret-1234",
		LastProcessedMessageID: 42,
	}
	data, err := json.Marshal(&want)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	store := &KeychainSessionStore{
		run: func(stdin string, args ...string) ([]byte, error) {
			if stdin != "" || len(args) != 6 || args[0] != "find-generic-password" || args[1] != "-a" || args[2] != keychainAccount || args[3] != "-s" || args[4] != keychainServiceName || args[5] != "-w" {
				t.Error("Load() invoked security with unexpected arguments")
				return nil, errors.New("unexpected security arguments")
			}
			return data, nil
		},
	}

	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if *got != want {
		t.Error("Load() returned unexpected legacy state")
	}
}

func TestKeychainSessionStoreLoadNotFound(t *testing.T) {
	store := &KeychainSessionStore{
		run: func(stdin string, args ...string) ([]byte, error) {
			return nil, errors.New("security item could not be found")
		},
	}

	_, err := store.Load()
	if !errors.Is(err, ErrKeychainEntryNotFound) {
		t.Errorf("Load() error = %v, want ErrKeychainEntryNotFound", err)
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Load() error = %v, want os.ErrNotExist", err)
	}
}

func TestKeychainSessionStoreRunnerErrorsDoNotLeakState(t *testing.T) {
	state := State{Secret: "test-fake-session-secret-1234"}
	encoded, err := json.Marshal(&state)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	tests := []struct {
		name string
		call func(*KeychainSessionStore) error
	}{
		{
			name: "save",
			call: func(store *KeychainSessionStore) error {
				return store.Save(&state)
			},
		},
		{
			name: "load",
			call: func(store *KeychainSessionStore) error {
				_, err := store.Load()
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runnerErr := errors.New("security command failed")
			store := &KeychainSessionStore{
				run: func(stdin string, args ...string) ([]byte, error) {
					return encoded, runnerErr
				},
			}

			err := tt.call(store)
			if err == nil {
				t.Fatal("operation error = nil, want runner error")
			}
			if !errors.Is(err, runnerErr) {
				t.Error("operation error did not wrap runner error")
			}
			if strings.Contains(err.Error(), state.Secret) || strings.Contains(err.Error(), string(encoded)) {
				t.Error("operation error leaked session state")
			}
		})
	}
}
