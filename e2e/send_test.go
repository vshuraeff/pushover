//go:build e2e

package e2e

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pushovercrypto "github.com/vshuraeff/pushover/internal/crypto"
	"github.com/vshuraeff/pushover/internal/mockpushover"
)

func TestSendEchoesEveryParameter(t *testing.T) {
	h := newHarness(t, defaultSeed())
	// client-side encryption inflates payloads; pushover's public docs do not specify plaintext or ciphertext limits, so this test omits url_title.
	result := h.run(t,
		"send", "full parameter message",
		"--user", fixtureUserKey,
		"--device", "phone",
		"--title", "deployment",
		"--url", "https://example.test/run/42",
		"--priority", "1",
		"--retry", "31",
		"--expire", "90",
		"--callback", "https://example.test/callback",
		"--tags", "deploy,e2e",
		"--sound", "e2e-custom",
		"--timestamp", "1700000000",
		"--ttl", "600",
		"--html",
		"--monospace",
		"--encrypted",
	)
	requireSuccess(t, result)

	requests := h.requests(t)
	if len(requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(requests))
	}
	request := requests[0]
	if request.Method != http.MethodPost || request.Path != "/1/messages.json" {
		t.Fatalf("request = %s %s, want POST /1/messages.json", request.Method, request.Path)
	}
	wants := map[string]string{
		"user": fixtureUserKey, "device": "phone",
		"priority": "1", "retry": "31", "expire": "90", "callback": "https://example.test/callback",
		"tags": "deploy,e2e", "sound": "e2e-custom", "timestamp": "1700000000", "ttl": "600",
		"html": "1", "monospace": "1", "encrypted": "1",
	}
	for field, want := range wants {
		requireField(t, request, field, want)
	}
	key, err := pushovercrypto.ParseKey(fixtureEncryptionKey)
	if err != nil {
		t.Fatalf("parse fixture encryption key: %v", err)
	}
	for field, want := range map[string]string{
		"message": "full parameter message",
		"title":   "deployment",
		"url":     "https://example.test/run/42",
	} {
		values, ok := request.Fields[field]
		if !ok || len(values) != 1 {
			t.Fatalf("request field %q did not contain one encrypted value", field)
		}
		if values[0] == want {
			t.Fatalf("request field %q was sent as plaintext", field)
		}
		plaintext, err := pushovercrypto.DecryptField(values[0], key)
		if err != nil {
			t.Fatalf("decrypt request field %q: %v", field, err)
		}
		if plaintext != want {
			t.Fatalf("decrypted request field %q did not match the expected value", field)
		}
	}
	if _, ok := request.Fields["url_title"]; ok {
		t.Fatal("request unexpectedly contained url_title")
	}
}

func TestSendMultipartAttachment(t *testing.T) {
	h := newHarness(t, defaultSeed())
	attachment := []byte("small raw attachment fixture\n")
	path := filepath.Join(t.TempDir(), "fixture.bin")
	if err := os.WriteFile(path, attachment, 0o600); err != nil {
		t.Fatalf("write attachment fixture: %v", err)
	}

	result := h.run(t, "send", "multipart attachment", "--attachment", path)
	requireSuccess(t, result)
	requests := h.requests(t)
	if len(requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(requests))
	}
	requireField(t, requests[0], "attachment", "fixture.bin")
	if contentType := requests[0].Headers.Get("Content-Type"); !strings.HasPrefix(contentType, "multipart/form-data;") {
		t.Fatalf("content type = %q, want multipart/form-data", contentType)
	}
	// the public mock control journal exposes the filename but not stored raw bytes.
}

func TestSendBase64Attachment(t *testing.T) {
	h := newHarness(t, defaultSeed())
	attachment := []byte("base64 attachment fixture")
	path := filepath.Join(t.TempDir(), "fixture.dat")
	if err := os.WriteFile(path, attachment, 0o600); err != nil {
		t.Fatalf("write attachment fixture: %v", err)
	}

	result := h.run(t, "send", "base64 attachment", "--attachment-base64", path, "--attachment-type", "application/e2e")
	requireSuccess(t, result)
	requests := h.requests(t)
	if len(requests) != 1 {
		t.Fatalf("request count = %d, want 1", len(requests))
	}
	requireField(t, requests[0], "attachment_base64", base64.StdEncoding.EncodeToString(attachment))
	requireField(t, requests[0], "attachment_type", "application/e2e")
}

func TestSendRejectsOversizedAttachmentsBeforeRequest(t *testing.T) {
	for _, test := range []struct {
		name string
		flag string
	}{
		{name: "multipart", flag: "--attachment"},
		{name: "base64", flag: "--attachment-base64"},
	} {
		t.Run(test.name, func(t *testing.T) {
			h := newHarness(t, defaultSeed())
			path := filepath.Join(t.TempDir(), "oversized.bin")
			file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
			if err != nil {
				t.Fatalf("create oversized fixture: %v", err)
			}
			if err := file.Truncate(5_242_881); err != nil {
				file.Close()
				t.Fatalf("size oversized fixture: %v", err)
			}
			if err := file.Close(); err != nil {
				t.Fatalf("close oversized fixture: %v", err)
			}

			result := h.run(t, "send", "too large", test.flag, path)
			requireFailure(t, result, "attachment exceeds 5242880 bytes")
			if requests := h.requests(t); len(requests) != 0 {
				t.Fatalf("request count = %d, want 0", len(requests))
			}
		})
	}
}

func TestEmergencySendAndReceiptLifecycle(t *testing.T) {
	h := newHarness(t, defaultSeed())
	send := h.run(t, "send", "emergency", "--priority", "2", "--retry", "30", "--expire", "300", "--tags", "incident-e2e", "--json")
	requireSuccess(t, send)
	var response struct {
		Status  int    `json:"status"`
		Receipt string `json:"receipt"`
	}
	if err := json.Unmarshal([]byte(send.stdout), &response); err != nil {
		t.Fatalf("decode emergency send output: %v", err)
	}
	if response.Status != 1 || response.Receipt == "" {
		t.Fatalf("emergency send did not return a receipt")
	}

	status := h.run(t, "receipt", "status", response.Receipt, "--json")
	requireSuccess(t, status)
	var receipt struct {
		Status       int  `json:"status"`
		Acknowledged int  `json:"acknowledged"`
		Cancelled    bool `json:"cancelled"`
	}
	if err := json.Unmarshal([]byte(status.stdout), &receipt); err != nil {
		t.Fatalf("decode receipt status output: %v", err)
	}
	if receipt.Status != 1 || receipt.Acknowledged != 0 || receipt.Cancelled {
		t.Fatalf("unexpected initial receipt state")
	}

	cancel := h.run(t, "receipt", "cancel", response.Receipt)
	requireSuccess(t, cancel)
	if !strings.Contains(cancel.stdout, "cancelled") {
		t.Fatalf("cancel output did not confirm cancellation: %s", cancel.stdout)
	}

	second := h.run(t, "send", "second emergency", "--priority", "2", "--retry", "30", "--expire", "300", "--tags", "incident-e2e", "--json")
	requireSuccess(t, second)
	cancelByTag := h.run(t, "receipt", "cancel-by-tag", "incident-e2e", "--json")
	requireSuccess(t, cancelByTag)
	var tagged struct {
		Tag       string `json:"tag"`
		Cancelled bool   `json:"cancelled"`
	}
	if err := json.Unmarshal([]byte(cancelByTag.stdout), &tagged); err != nil {
		t.Fatalf("decode cancel-by-tag output: %v", err)
	}
	if tagged.Tag != "incident-e2e" || !tagged.Cancelled {
		t.Fatalf("cancel-by-tag output did not confirm the tag")
	}
}

func TestSendQuotaExhaustionReportsResetContext(t *testing.T) {
	seed := defaultSeed()
	seed.Tokens[0].Quota = mockpushover.AppQuota{Limit: 100, Remaining: 0, Reset: 1_900_000_123}
	h := newHarness(t, seed)

	result := h.run(t, "send", "quota exhausted")
	if result.err == nil || result.exitCode == 0 {
		t.Fatal("quota-exhausted send unexpectedly succeeded")
	}
	stderr := strings.ToLower(result.stderr)
	if !strings.Contains(stderr, "quota") {
		t.Fatalf("stderr did not describe quota exhaustion: %s", result.stderr)
	}
	if !strings.Contains(result.stderr, "1900000123") {
		t.Fatalf("stderr did not report quota reset time: %s", result.stderr)
	}
	requests := h.requests(t)
	if len(requests) != 1 || requests[0].Path != "/1/messages.json" {
		t.Fatalf("quota request count/path did not match the expected 429 flow")
	}
}
