//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/vshuraeff/pushover/internal/config"
	"github.com/vshuraeff/pushover/internal/mockpushover"
)

func TestClientListenSyncsAndHandlesReconnectAndTerminalFrames(t *testing.T) {
	seed := defaultSeed()
	seed.Queues = map[string][]mockpushover.Message{}
	h := newHarness(t, seed)
	writeState(t, h.configDir, config.State{DeviceID: fixtureDeviceID, DeviceName: "phone", Secret: fixtureSecret})
	process := h.start(t, "client", "listen", "--emergency-realert", "30")

	eventually(t, 5*time.Second, 20*time.Millisecond, func() (bool, string) {
		status, err := h.control.WSStatus()
		return err == nil && status.Connected && status.LoginOK && status.ConnectCount >= 1, describeWSStatus(status, err)
	})

	queued := defaultSeed()
	queued.Queues = map[string][]mockpushover.Message{
		fixtureDeviceID: {
			{
				ID: 1, IDString: "1", UMID: 1, UMIDString: "1", Title: "queued title",
				Message: "queued notification body", App: "E2E App", AID: 42, AIDString: "42",
				Icon: "fixture-icon", Date: 1_700_000_000, Priority: 1, Sound: "e2e-custom",
			},
		},
	}
	if err := h.control.Seed(queued); err != nil {
		t.Fatalf("seed queued listen message: %v", err)
	}
	if err := h.control.WSSend("!"); err != nil {
		t.Fatalf("send websocket sync frame: %v", err)
	}

	type notificationRecord struct {
		ID       string `json:"id"`
		Title    string `json:"title"`
		Body     string `json:"body"`
		AppName  string `json:"appName"`
		IconPath string `json:"iconPath"`
		Priority int    `json:"priority"`
	}
	var notification notificationRecord
	eventually(t, 5*time.Second, 20*time.Millisecond, func() (bool, string) {
		data, err := os.ReadFile(h.notificationPath)
		if err != nil {
			return false, "notification file is not readable yet"
		}
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		if len(lines) == 0 || lines[0] == "" {
			return false, "notification file is empty"
		}
		var candidate notificationRecord
		if err := json.Unmarshal([]byte(lines[len(lines)-1]), &candidate); err != nil {
			return false, "notification record is not complete json yet"
		}
		if candidate.Body != "queued notification body" {
			return false, "queued notification has not been recorded yet"
		}
		notification = candidate
		return true, "queued notification recorded"
	})
	if notification.ID != "1" || notification.Title != "queued title" || notification.AppName != "E2E App" || notification.Priority != 1 {
		t.Fatalf("notification payload did not match the queued message")
	}
	if filepath.Base(notification.IconPath) != "fixture-icon.png" {
		t.Fatalf("notification icon path did not point at the cached fixture icon")
	}
	if _, err := os.Stat(notification.IconPath); !os.IsNotExist(err) {
		t.Fatalf("delivered notification icon still exists: %v", err)
	}
	response, err := http.Get(h.server.URL() + "/icons/fixture-icon.png")
	if err != nil {
		t.Fatalf("fetch fixture icon: %v", err)
	}
	defer response.Body.Close()
	fixtureIcon, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read fixture icon: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("fixture icon status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	cachedIcon, err := os.ReadFile(h.iconCachePath("fixture-icon"))
	if err != nil {
		t.Fatalf("read durable icon cache: %v", err)
	}
	if !bytes.Equal(cachedIcon, fixtureIcon) {
		t.Fatal("durable icon cache did not match the fixture icon")
	}

	status, err := h.control.WSStatus()
	if err != nil {
		t.Fatalf("read websocket status before reconnect: %v", err)
	}
	beforeReconnect := status.ConnectCount
	if err := h.control.WSSend("R"); err != nil {
		t.Fatalf("send websocket reconnect frame: %v", err)
	}
	eventually(t, 5*time.Second, 20*time.Millisecond, func() (bool, string) {
		current, currentErr := h.control.WSStatus()
		return currentErr == nil && current.Connected && current.ConnectCount > beforeReconnect, describeWSStatus(current, currentErr)
	})

	status, err = h.control.WSStatus()
	if err != nil {
		t.Fatalf("read websocket status before forced close: %v", err)
	}
	beforeClose := status.ConnectCount
	if err := h.control.WSClose(); err != nil {
		t.Fatalf("force websocket close: %v", err)
	}
	eventually(t, 5*time.Second, 20*time.Millisecond, func() (bool, string) {
		current, currentErr := h.control.WSStatus()
		return currentErr == nil && current.Connected && current.ConnectCount > beforeClose, describeWSStatus(current, currentErr)
	})

	status, err = h.control.WSStatus()
	if err != nil {
		t.Fatalf("read websocket status before terminal frame: %v", err)
	}
	beforeTerminal := status.ConnectCount
	if err := h.control.WSSend("E"); err != nil {
		t.Fatalf("send websocket terminal frame: %v", err)
	}
	if err := process.wait(t, 5*time.Second); err == nil {
		t.Fatal("listen process exited successfully after a permanent websocket error")
	}
	_, stderr := process.output()
	if !strings.Contains(stderr, "permanently rejected") {
		t.Fatalf("listen stderr did not describe the permanent websocket error: %s", stderr)
	}

	consistently(t, 3*time.Second, 50*time.Millisecond, func() (bool, string) {
		current, currentErr := h.control.WSStatus()
		if currentErr != nil {
			return false, describeWSStatus(current, currentErr)
		}
		return current.ConnectCount == beforeTerminal, describeWSStatus(current, nil)
	})
}

func TestClientListenDecryptsExplicitlyEncryptedMessage(t *testing.T) {
	// this mock server delivers encrypted messages to Open Client devices; the real Pushover server does not.
	seed := defaultSeed()
	seed.Queues = map[string][]mockpushover.Message{}
	h := newHarness(t, seed)
	writeState(t, h.configDir, config.State{DeviceID: fixtureDeviceID, DeviceName: "phone", Secret: fixtureSecret})
	h.start(t, "client", "listen")

	eventually(t, 5*time.Second, 20*time.Millisecond, func() (bool, string) {
		status, err := h.control.WSStatus()
		return err == nil && status.Connected && status.LoginOK && status.ConnectCount >= 1, describeWSStatus(status, err)
	})

	result := h.run(t,
		"send", "explicitly encrypted notification body",
		"--user", fixtureUserKey,
		"--device", "phone",
		"--title", "explicitly encrypted notification title",
		"--url", "https://example.test/explicit-encryption",
		"--encrypted",
	)
	requireSuccess(t, result)

	var request mockpushover.JournaledRequest
	for _, candidate := range h.requests(t) {
		if candidate.Method == http.MethodPost && candidate.Path == "/1/messages.json" {
			request = candidate
			break
		}
	}
	if request.Path == "" {
		t.Fatal("send request was not recorded")
	}
	requireField(t, request, "encrypted", "1")
	for field, plaintext := range map[string]string{
		"message": "explicitly encrypted notification body",
		"title":   "explicitly encrypted notification title",
		"url":     "https://example.test/explicit-encryption",
	} {
		values, ok := request.Fields[field]
		if !ok || len(values) != 1 {
			t.Fatalf("request field %q did not contain one encrypted value", field)
		}
		if values[0] == plaintext {
			t.Fatalf("request field %q was sent as plaintext", field)
		}
	}

	if err := h.control.WSSend("!"); err != nil {
		t.Fatalf("send websocket sync frame: %v", err)
	}

	type notificationRecord struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	var notification notificationRecord
	eventually(t, 5*time.Second, 20*time.Millisecond, func() (bool, string) {
		data, err := os.ReadFile(h.notificationPath)
		if err != nil {
			return false, "notification file is not readable yet"
		}
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		if len(lines) == 0 || lines[0] == "" {
			return false, "notification file is empty"
		}
		var candidate notificationRecord
		if err := json.Unmarshal([]byte(lines[len(lines)-1]), &candidate); err != nil {
			return false, "notification record is not complete json yet"
		}
		if candidate.Body != "explicitly encrypted notification body" {
			return false, "encrypted notification has not been decrypted yet"
		}
		notification = candidate
		return true, "decrypted notification recorded"
	})
	if notification.Title != "explicitly encrypted notification title" || notification.Body != "explicitly encrypted notification body" {
		t.Fatalf("notification payload did not match the decrypted message")
	}
}

func TestClientListenRendersHTMLMessageAsPlainText(t *testing.T) {
	seed := defaultSeed()
	seed.Queues = map[string][]mockpushover.Message{}
	h := newHarness(t, seed)
	writeState(t, h.configDir, config.State{DeviceID: fixtureDeviceID, DeviceName: "phone", Secret: fixtureSecret})
	h.start(t, "client", "listen")

	eventually(t, 5*time.Second, 20*time.Millisecond, func() (bool, string) {
		status, err := h.control.WSStatus()
		return err == nil && status.Connected && status.LoginOK && status.ConnectCount >= 1, describeWSStatus(status, err)
	})

	result := h.run(t,
		"send", `<b>disk</b> at <font color="#ff0000">95%</font><br><a href="https://example.test/disk">details</a> &amp; more`,
		"--user", fixtureUserKey,
		"--device", "phone",
		"--title", "html notification",
		"--html",
	)
	requireSuccess(t, result)

	if err := h.control.WSSend("!"); err != nil {
		t.Fatalf("send websocket sync frame: %v", err)
	}

	const want = "disk at 95%\ndetails & more"
	eventually(t, 5*time.Second, 20*time.Millisecond, func() (bool, string) {
		data, err := os.ReadFile(h.notificationPath)
		if err != nil {
			return false, "notification file is not readable yet"
		}
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		var candidate struct {
			Title string `json:"title"`
			Body  string `json:"body"`
			URL   string `json:"url"`
		}
		if err := json.Unmarshal([]byte(lines[len(lines)-1]), &candidate); err != nil {
			return false, "notification record is not complete json yet"
		}
		if candidate.Title != "html notification" || candidate.Body != want || candidate.URL != "https://example.test/disk" {
			return false, "html notification body = " + strconv.Quote(candidate.Body) + " url = " + strconv.Quote(candidate.URL)
		}
		return true, "plain text notification recorded"
	})
}

func TestClientListenWarnsWhenExplicitEncryptionKeyEnvIsUnavailable(t *testing.T) {
	seed := defaultSeed()
	seed.Queues = map[string][]mockpushover.Message{}
	h := newHarness(t, seed)
	writeState(t, h.configDir, config.State{DeviceID: fixtureDeviceID, DeviceName: "phone", Secret: fixtureSecret})
	process := h.start(t, "client", "listen", "--encryption-key-env", "SOME_UNSET_ENCRYPTION_KEY_VAR_E2E")

	eventually(t, 5*time.Second, 20*time.Millisecond, func() (bool, string) {
		status, err := h.control.WSStatus()
		stderr := process.Stderr()
		return err == nil && status.Connected && status.LoginOK &&
				strings.Contains(stderr, "level=WARN") &&
				strings.Contains(stderr, "message decryption disabled"),
			describeWSStatus(status, err) + "; listen stderr: " + stderr
	})
}

func TestClientListenDoesNotWarnWhenEncryptionKeyIsUnconfigured(t *testing.T) {
	seed := defaultSeed()
	seed.Queues = map[string][]mockpushover.Message{}
	h := newHarness(t, seed)
	h.env = h.withEnv(map[string]string{"PUSHOVER_ENCRYPTION_KEY": ""})
	writeState(t, h.configDir, config.State{DeviceID: fixtureDeviceID, DeviceName: "phone", Secret: fixtureSecret})
	process := h.start(t, "client", "listen")

	eventually(t, 5*time.Second, 20*time.Millisecond, func() (bool, string) {
		status, err := h.control.WSStatus()
		stderr := process.Stderr()
		return err == nil && status.Connected && status.LoginOK && !strings.Contains(stderr, "level=WARN"),
			describeWSStatus(status, err) + "; listen stderr: " + stderr
	})
}

func TestClientListenSyncsQueuedMessageAfterReconnectWithoutSyncFrame(t *testing.T) {
	seed := defaultSeed()
	seed.Queues = map[string][]mockpushover.Message{}
	h := newHarness(t, seed)
	writeState(t, h.configDir, config.State{DeviceID: fixtureDeviceID, DeviceName: "phone", Secret: fixtureSecret})
	h.start(t, "client", "listen")

	eventually(t, 5*time.Second, 20*time.Millisecond, func() (bool, string) {
		status, err := h.control.WSStatus()
		return err == nil && status.Connected && status.LoginOK && status.ConnectCount >= 1, describeWSStatus(status, err)
	})

	queued := defaultSeed()
	queued.Queues = map[string][]mockpushover.Message{
		fixtureDeviceID: {
			{
				ID: 1, IDString: "1", UMID: 1, UMIDString: "1", Title: "reconnected queued title",
				Message: "reconnected queued notification body", App: "E2E App", AID: 42, AIDString: "42",
				Icon: "fixture-icon", Date: 1_700_000_000, Priority: 1,
			},
		},
	}
	if err := h.control.Seed(queued); err != nil {
		t.Fatalf("seed queued listen message: %v", err)
	}

	status, err := h.control.WSStatus()
	if err != nil {
		t.Fatalf("read websocket status before forced close: %v", err)
	}
	beforeClose := status.ConnectCount
	data, err := os.ReadFile(h.notificationPath)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read notification file before forced close: %v", err)
	}
	if len(strings.TrimSpace(string(data))) != 0 {
		t.Fatal("queued message was delivered before the forced close; reconnect sync path was not exercised")
	}
	if err := h.control.WSClose(); err != nil {
		t.Fatalf("force websocket close: %v", err)
	}
	eventually(t, 5*time.Second, 20*time.Millisecond, func() (bool, string) {
		current, currentErr := h.control.WSStatus()
		return currentErr == nil && current.Connected && current.LoginOK && current.ConnectCount > beforeClose, describeWSStatus(current, currentErr)
	})

	type notificationRecord struct {
		ID    string `json:"id"`
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	var notification notificationRecord
	eventually(t, 5*time.Second, 20*time.Millisecond, func() (bool, string) {
		data, err := os.ReadFile(h.notificationPath)
		if err != nil {
			return false, "notification file is not readable yet"
		}
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		if len(lines) == 0 || lines[0] == "" {
			return false, "notification file is empty"
		}
		var candidate notificationRecord
		if err := json.Unmarshal([]byte(lines[len(lines)-1]), &candidate); err != nil {
			return false, "notification record is not complete json yet"
		}
		if candidate.Body != "reconnected queued notification body" {
			return false, "queued notification has not been recorded yet"
		}
		notification = candidate
		return true, "queued notification recorded"
	})
	if notification.ID != "1" || notification.Title != "reconnected queued title" {
		t.Fatalf("notification payload did not match the queued message")
	}
}

func TestClientListenDeliversSharedIconBatchWhenBackendConsumesFiles(t *testing.T) {
	seed := defaultSeed()
	seed.Queues = map[string][]mockpushover.Message{
		fixtureDeviceID: {
			{
				ID: 1, IDString: "1", UMID: 1, UMIDString: "1", Title: "first shared icon",
				Message: "first shared icon body", App: "E2E App", AID: 42, AIDString: "42",
				Icon: "fixture-icon", Date: 1_700_000_000,
			},
			{
				ID: 2, IDString: "2", UMID: 2, UMIDString: "2", Title: "second shared icon",
				Message: "second shared icon body", App: "E2E App", AID: 42, AIDString: "42",
				Icon: "fixture-icon", Date: 1_700_000_001,
			},
		},
	}
	h := newHarness(t, seed)
	h.env = h.withEnv(map[string]string{"PUSHOVER_FILE_NOTIFIER_CONSUME_FILES": "1"})
	writeState(t, h.configDir, config.State{DeviceID: fixtureDeviceID, DeviceName: "phone", Secret: fixtureSecret})
	h.start(t, "client", "listen")

	eventually(t, 5*time.Second, 20*time.Millisecond, func() (bool, string) {
		state := readState(t, h.configDir)
		return state.LastProcessedMessageID == 2, "shared-icon batch has not finished"
	})
	data, err := os.ReadFile(h.notificationPath)
	if err != nil {
		t.Fatalf("read notification records: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("notification record count = %d, want 2", len(lines))
	}
	type notificationRecord struct {
		ID       string `json:"id"`
		Title    string `json:"title"`
		Body     string `json:"body"`
		IconPath string `json:"iconPath"`
	}
	records := make([]notificationRecord, 0, len(lines))
	for _, line := range lines {
		var record notificationRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("decode notification record: %v", err)
		}
		records = append(records, record)
	}
	if records[0].ID != "1" || records[0].Title != "first shared icon" || records[0].Body != "first shared icon body" {
		t.Fatalf("first notification record = %#v", records[0])
	}
	if records[1].ID != "2" || records[1].Title != "second shared icon" || records[1].Body != "second shared icon body" {
		t.Fatalf("second notification record = %#v", records[1])
	}
	if records[0].IconPath == records[1].IconPath {
		t.Fatalf("notification icon paths are shared: %q", records[0].IconPath)
	}
	deadLetterPath := filepath.Join(h.configDir, "deadletter.jsonl")
	deadLetterData, err := os.ReadFile(deadLetterPath)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read dead-letter records: %v", err)
	}
	if len(deadLetterData) != 0 {
		t.Fatalf("dead-letter bytes = %d, want 0", len(deadLetterData))
	}
}

func TestClientListenAfterLoginAndRegistrationSyncsMessage(t *testing.T) {
	const deviceName = "e2e-listener"

	h := newHarness(t, defaultSeed())
	login := runViaPTY(t, h.env, []string{"client", "login"}, []string{fixtureEmail, fixturePassword})
	requireSuccess(t, login)
	registration := h.run(t, "client", "register", "--name", deviceName)
	requireSuccess(t, registration)

	state := readState(t, h.configDir)
	if state.DeviceID == "" {
		t.Fatal("registration state did not contain a device id")
	}
	if state.DeviceName != deviceName {
		t.Fatalf("registration state device name = %q, want %q", state.DeviceName, deviceName)
	}

	process := h.start(t, "client", "listen")
	eventually(t, 5*time.Second, 20*time.Millisecond, func() (bool, string) {
		status, err := h.control.WSStatus()
		return err == nil && status.Connected && status.LoginOK && status.LoginDeviceID == state.DeviceID && status.ConnectCount >= 1,
			describeWSStatus(status, err) + "; listen stderr: " + process.Stderr()
	})

	queued := defaultSeed()
	// the mock preserves the registration session secret on the created device.
	queued.Devices = append(queued.Devices, mockpushover.Device{
		ID: state.DeviceID, User: fixtureUserKey, Name: state.DeviceName, OS: "O", Secret: state.Secret, Enabled: true,
	})
	queued.Queues = map[string][]mockpushover.Message{
		state.DeviceID: {
			{
				ID: 1, IDString: "1", UMID: 1, UMIDString: "1", Title: "registered queued title",
				Message: "registered queued notification body", App: "E2E App", AID: 42, AIDString: "42",
				Icon: "fixture-icon", Date: 1_700_000_000,
			},
		},
	}
	if err := h.control.Seed(queued); err != nil {
		t.Fatalf("seed queued listen message: %v", err)
	}
	if err := h.control.WSSend("!"); err != nil {
		t.Fatalf("send websocket sync frame: %v", err)
	}

	type notificationRecord struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	var notification notificationRecord
	eventually(t, 5*time.Second, 20*time.Millisecond, func() (bool, string) {
		data, err := os.ReadFile(h.notificationPath)
		if err != nil {
			return false, "notification file is not readable yet"
		}
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		if len(lines) == 0 || lines[0] == "" {
			return false, "notification file is empty"
		}
		var candidate notificationRecord
		if err := json.Unmarshal([]byte(lines[len(lines)-1]), &candidate); err != nil {
			return false, "notification record is not complete json yet"
		}
		if candidate.Body != "registered queued notification body" {
			return false, "queued notification has not been recorded yet"
		}
		notification = candidate
		return true, "queued notification recorded"
	})
	if notification.Title != "registered queued title" || notification.Body != "registered queued notification body" {
		t.Fatalf("notification payload did not match the queued message")
	}
}

func TestClientListenExitsOnReplacementWithoutReconnect(t *testing.T) {
	seed := defaultSeed()
	seed.Queues = map[string][]mockpushover.Message{}
	h := newHarness(t, seed)
	writeState(t, h.configDir, config.State{DeviceID: fixtureDeviceID, DeviceName: "phone", Secret: fixtureSecret})
	process := h.start(t, "client", "listen")

	eventually(t, 5*time.Second, 20*time.Millisecond, func() (bool, string) {
		status, err := h.control.WSStatus()
		return err == nil && status.Connected && status.LoginOK && status.ConnectCount >= 1, describeWSStatus(status, err)
	})

	status, err := h.control.WSStatus()
	if err != nil {
		t.Fatalf("read websocket status before replacement frame: %v", err)
	}
	beforeReplacement := status.ConnectCount
	if err := h.control.WSSend("A"); err != nil {
		t.Fatalf("send websocket replacement frame: %v", err)
	}
	if err := process.wait(t, 5*time.Second); err == nil {
		t.Fatal("listen process exited successfully after a websocket replacement")
	}
	_, stderr := process.output()
	if !strings.Contains(stderr, "websocket connection replaced") {
		t.Fatalf("listen stderr did not describe the websocket replacement: %s", stderr)
	}

	consistently(t, 3*time.Second, 50*time.Millisecond, func() (bool, string) {
		current, currentErr := h.control.WSStatus()
		if currentErr != nil {
			return false, describeWSStatus(current, currentErr)
		}
		return current.ConnectCount == beforeReplacement, describeWSStatus(current, nil)
	})
}
