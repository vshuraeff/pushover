package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	pushovercrypto "github.com/vshuraeff/pushover/internal/crypto"
)

func TestSendMessageFullParameters(t *testing.T) {
	encryptionKey := bytes.Repeat([]byte{0x42}, 32)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/1/messages.json" {
			t.Errorf("path = %s, want /1/messages.json", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}
		for name, want := range map[string]string{
			"token": "test-token", "user": "user-one,user-two", "device": "phone",
			"priority": "2", "retry": "30", "expire": "3600",
			"callback": "https://example.test/callback", "tags": "deploy,urgent", "sound": "siren",
			"timestamp": "1700000000", "ttl": "120", "html": "1", "monospace": "1", "encrypted": "1",
		} {
			if got := r.Form.Get(name); got != want {
				t.Errorf("form %q = %q, want %q", name, got, want)
			}
		}
		for name, want := range map[string]string{
			"message": "message text", "title": "title", "url": "https://example.test/link",
			"url_title": "link title",
		} {
			got := r.Form.Get(name)
			if got == want {
				t.Errorf("form %q was sent as plaintext", name)
			}
			plaintext, err := pushovercrypto.DecryptField(got, encryptionKey)
			if err != nil {
				t.Errorf("decrypt form %q: %v", name, err)
				continue
			}
			if plaintext != want {
				t.Errorf("decrypted form %q = %q, want %q", name, plaintext, want)
			}
		}
		_, _ = w.Write([]byte(`{"status":1,"request":"request-id","receipt":"receipt-id"}`))
	}))
	defer server.Close()

	priority := 2
	result, err := New(server.URL, "test").SendMessage(context.Background(), SendMessageRequest{
		Token: "test-token", User: []string{"user-one", "user-two"}, Message: "message text",
		Device: "phone", Title: "title", URL: "https://example.test/link", URLTitle: "link title",
		Priority: &priority, Retry: 30, Expire: 3600, Callback: "https://example.test/callback",
		Tags: "deploy,urgent", Sound: "siren", Timestamp: 1_700_000_000, TTL: 120,
		HTML: true, Monospace: true, Encrypted: true, EncryptionKey: encryptionKey,
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if result.Status != 1 || result.Request != "request-id" || result.Receipt != "receipt-id" {
		t.Errorf("SendMessage() result = %+v", result)
	}
}

func TestSendMessageMultipartAttachment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("ParseMultipartForm() error = %v", err)
		}
		if got := r.FormValue("message"); got != "with file" {
			t.Errorf("message = %q, want with file", got)
		}
		file, header, err := r.FormFile("attachment")
		if err != nil {
			t.Fatalf("FormFile() error = %v", err)
		}
		defer file.Close()
		data := make([]byte, 64)
		n, err := file.Read(data)
		if err != nil && n == 0 {
			t.Fatalf("attachment read error = %v", err)
		}
		if header.Filename != "note.txt" {
			t.Errorf("filename = %q, want note.txt", header.Filename)
		}
		if got := string(data[:n]); got != "attachment data" {
			t.Errorf("attachment = %q, want attachment data", got)
		}
		_, _ = w.Write([]byte(`{"status":1,"request":"request-id"}`))
	}))
	defer server.Close()

	result, err := New(server.URL, "test").SendMessage(context.Background(), SendMessageRequest{
		Token: "test-token", User: []string{"user-one"}, Message: "with file",
		Attachment: &FilePart{FileName: "nested/note.txt", Reader: bytes.NewReader([]byte("attachment data"))},
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if result.Request != "request-id" {
		t.Errorf("request = %q, want request-id", result.Request)
	}
}

func TestSendMessageBase64Attachment(t *testing.T) {
	tests := []struct {
		name           string
		attachmentType string
		wantType       string
	}{
		{name: "auto type", wantType: "text/plain; charset=utf-8"},
		{name: "explicit type", attachmentType: "application/x-test", wantType: "application/x-test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := r.ParseForm(); err != nil {
					t.Fatalf("ParseForm() error = %v", err)
				}
				if got := r.Form.Get("attachment_base64"); got != "aGVsbG8=" {
					t.Errorf("attachment_base64 = %q, want aGVsbG8=", got)
				}
				if got := r.Form.Get("attachment_type"); got != tt.wantType {
					t.Errorf("attachment_type = %q, want %q", got, tt.wantType)
				}
				_, _ = w.Write([]byte(`{"status":1,"request":"request-id"}`))
			}))
			defer server.Close()

			_, err := New(server.URL, "test").SendMessage(context.Background(), SendMessageRequest{
				Token: "test-token", User: []string{"user-one"}, Message: "base64 attachment",
				AttachmentBase64: []byte("hello"), AttachmentType: tt.attachmentType,
			})
			if err != nil {
				t.Fatalf("SendMessage() error = %v", err)
			}
		})
	}
}

func TestSendMessageValidation(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	priorityTooHigh := 3
	emergency := 2
	overLimit := make([]byte, MaxAttachmentBytes+1)
	valid := func() SendMessageRequest {
		return SendMessageRequest{Token: "test-token", User: []string{"user-one"}, Message: "message"}
	}
	tests := []struct {
		name string
		req  SendMessageRequest
		want string
	}{
		{name: "empty message", req: SendMessageRequest{Token: "test-token", User: []string{"user-one"}}, want: "message"},
		{name: "long message", req: SendMessageRequest{Token: "test-token", User: []string{"user-one"}, Message: strings.Repeat("é", 1025)}, want: "message"},
		{name: "long title", req: func() SendMessageRequest { req := valid(); req.Title = strings.Repeat("é", 251); return req }(), want: "title"},
		{name: "long URL", req: func() SendMessageRequest { req := valid(); req.URL = strings.Repeat("é", 513); return req }(), want: "url"},
		{name: "long URL title", req: func() SendMessageRequest { req := valid(); req.URLTitle = strings.Repeat("é", 101); return req }(), want: "url_title"},
		{name: "encrypted with invalid key", req: func() SendMessageRequest {
			req := valid()
			req.Encrypted = true
			req.EncryptionKey = []byte("short")
			return req
		}(), want: "encryption key must be 32 bytes"},
		{name: "no users", req: func() SendMessageRequest { req := valid(); req.User = nil; return req }(), want: "user"},
		{name: "too many users", req: func() SendMessageRequest { req := valid(); req.User = make([]string, 51); return req }(), want: "user"},
		{name: "priority out of range", req: func() SendMessageRequest { req := valid(); req.Priority = &priorityTooHigh; return req }(), want: "priority"},
		{name: "emergency retry missing", req: func() SendMessageRequest { req := valid(); req.Priority = &emergency; req.Expire = 60; return req }(), want: "retry"},
		{name: "emergency expire missing", req: func() SendMessageRequest { req := valid(); req.Priority = &emergency; req.Retry = 30; return req }(), want: "expire"},
		{name: "oversized base64 attachment", req: func() SendMessageRequest { req := valid(); req.AttachmentBase64 = overLimit; return req }(), want: "attachment"},
		{name: "oversized multipart attachment", req: func() SendMessageRequest {
			req := valid()
			req.Attachment = &FilePart{FileName: "large.bin", Reader: bytes.NewReader(overLimit)}
			return req
		}(), want: "attachment"},
		{name: "both attachment kinds", req: func() SendMessageRequest {
			req := valid()
			req.Attachment = &FilePart{FileName: "file.txt", Reader: bytes.NewReader([]byte("file"))}
			req.AttachmentBase64 = []byte("base64")
			return req
		}(), want: "only one"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := New(server.URL, "test").SendMessage(context.Background(), tt.req)
			if err == nil {
				t.Fatal("SendMessage() error = nil, want validation error")
			}
			if result != nil {
				t.Errorf("SendMessage() result = %+v, want nil", result)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("SendMessage() error = %q, want it to contain %q", err, tt.want)
			}
		})
	}
	if got := requests.Load(); got != 0 {
		t.Errorf("requests = %d, want 0", got)
	}
}

func TestSendMessageResponseJSON(t *testing.T) {
	response := SendMessageResponse{Status: 1, Request: "request-id", Receipt: "receipt-id"}
	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if !strings.Contains(string(data), "receipt-id") {
		t.Errorf("response JSON = %s", data)
	}
}
