//go:build e2e

package e2e

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vshuraeff/pushover/internal/config"
	"github.com/vshuraeff/pushover/internal/mockpushover"
)

func TestClientLoginHappyPathViaPTY(t *testing.T) {
	h := newHarness(t, defaultSeed())
	result := runViaPTY(t, h.env, []string{"client", "login"}, []string{fixtureEmail, fixturePassword})
	combined := strings.ReplaceAll(result.stdout+result.stderr, "\r\n", "\n")
	if !strings.Contains(combined, "Email:") || !strings.Contains(combined, "Password:") {
		t.Fatalf("interactive login output did not contain both prompts: %s", redactedPTYOutput(combined))
	}
	if _, err := os.Stat(filepath.Join(h.configDir, "state.json")); err != nil {
		t.Fatalf("interactive login did not create session state: %v; output: %s", err, redactedPTYOutput(combined))
	}
	state := readState(t, h.configDir)
	if state.Secret != fixtureSecret {
		t.Fatal("login state did not contain the expected session secret")
	}
	requests := h.requests(t)
	if len(requests) != 1 || requests[0].Method != http.MethodPost || requests[0].Path != "/1/users/login.json" {
		t.Fatalf("login journal did not contain exactly one login post")
	}
	if len(requests[0].Fields["password"]) != 1 {
		t.Fatal("login request did not contain a password field")
	}
}

func TestClientLoginTwoFactorChallengeViaPTY(t *testing.T) {
	h := newHarness(t, defaultSeed())
	result := runViaPTY(t, h.env, []string{"client", "login"}, []string{fixtureTwoFAEmail, fixtureTwoFAPass, fixtureTwoFACode})
	combined := strings.ReplaceAll(result.stdout+result.stderr, "\r\n", "\n")
	if !strings.Contains(combined, "Two-factor authentication code:") {
		t.Fatalf("interactive login output did not contain the two-factor prompt: %s", redactedPTYOutput(combined))
	}
	state := readState(t, h.configDir)
	if state.Secret != fixtureSecret2 {
		t.Fatal("two-factor login state did not contain the expected session secret")
	}
	requests := h.requests(t)
	if len(requests) != 2 {
		t.Fatalf("two-factor login request count = %d, want 2", len(requests))
	}
	for i, request := range requests {
		if request.Method != http.MethodPost || request.Path != "/1/users/login.json" {
			t.Fatalf("login request %d = %s %s", i, request.Method, request.Path)
		}
	}
	if _, present := requests[0].Fields["twofa"]; present {
		t.Fatal("initial login request unexpectedly contained twofa")
	}
	requireField(t, requests[1], "twofa", fixtureTwoFACode)
}

func TestClientRegisterRejectsInvalidNameBeforeRequest(t *testing.T) {
	h := newHarness(t, defaultSeed())
	writeState(t, h.configDir, config.State{Secret: fixtureSecret})
	result := h.run(t, "client", "register", "--name", "bad device name")
	requireFailure(t, result, "device name may contain only")
	if requests := h.requests(t); len(requests) != 0 {
		t.Fatalf("invalid registration request count = %d, want 0", len(requests))
	}
}

func TestClientFetchPrintsMessagesThenUpdatesHighest(t *testing.T) {
	seed := defaultSeed()
	seed.Queues = map[string][]mockpushover.Message{
		fixtureDeviceID: {
			{ID: 3, IDString: "3", UMID: 10, UMIDString: "10", Title: "first", Message: "first body", App: "E2E App", Icon: "fixture-icon", Date: 1_700_000_000},
			{ID: 7, IDString: "7", UMID: 11, UMIDString: "11", Title: "second", Message: "second body", App: "E2E App", Icon: "fixture-icon", Date: 1_700_000_100},
		},
	}
	h := newHarness(t, seed)
	writeState(t, h.configDir, config.State{DeviceID: fixtureDeviceID, DeviceName: "phone", Secret: fixtureSecret})

	result := h.run(t, "client", "fetch", "--json")
	requireSuccess(t, result)
	var messages []struct {
		ID      int64  `json:"id"`
		Title   string `json:"title"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &messages); err != nil {
		t.Fatalf("decode client fetch output: %v", err)
	}
	if len(messages) != 2 || messages[0].ID != 3 || messages[1].ID != 7 || messages[1].Message != "second body" {
		t.Fatalf("fetch output did not contain both queued messages in order")
	}

	requests := h.requests(t)
	if len(requests) != 2 {
		t.Fatalf("fetch request count = %d, want 2", len(requests))
	}
	if requests[0].Method != http.MethodGet || requests[0].Path != "/1/messages.json" {
		t.Fatalf("first fetch request = %s %s", requests[0].Method, requests[0].Path)
	}
	if requests[1].Method != http.MethodPost || requests[1].Path != "/1/devices/"+fixtureDeviceID+"/update_highest_message.json" {
		t.Fatalf("second fetch request = %s %s", requests[1].Method, requests[1].Path)
	}
	requireField(t, requests[1], "message", "7")
}

func TestClientAcknowledgeReceipt(t *testing.T) {
	h := newHarness(t, defaultSeed())
	writeState(t, h.configDir, config.State{DeviceID: fixtureDeviceID, DeviceName: "phone", Secret: fixtureSecret})
	send := h.run(t, "send", "acknowledge me", "--priority", "2", "--retry", "30", "--expire", "300", "--json")
	requireSuccess(t, send)
	var sent struct {
		Receipt string `json:"receipt"`
	}
	if err := json.Unmarshal([]byte(send.stdout), &sent); err != nil || sent.Receipt == "" {
		t.Fatalf("emergency send did not return a receipt")
	}
	baseline := len(h.requests(t))

	result := h.run(t, "client", "ack", sent.Receipt)
	requireSuccess(t, result)
	requests := h.requests(t)
	if len(requests) != baseline+1 || requests[baseline].Method != http.MethodPost || requests[baseline].Path != "/1/receipts/"+sent.Receipt+"/acknowledge.json" {
		t.Fatalf("acknowledgement request did not reach the expected mock endpoint")
	}
	if len(requests[baseline].Fields["secret"]) != 1 {
		t.Fatal("acknowledgement request did not contain the stored session secret")
	}
}

func redactedPTYOutput(output string) string {
	for _, value := range []string{fixtureEmail, fixturePassword, fixtureTwoFAEmail, fixtureTwoFAPass, fixtureTwoFACode} {
		output = strings.ReplaceAll(output, value, "[redacted]")
	}
	return output
}
