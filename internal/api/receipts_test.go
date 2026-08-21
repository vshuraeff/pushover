package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestReceiptRoundTrips(t *testing.T) {
	const receiptID = "receipt / id"
	const tag = "release / blue"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case "/1/receipts/receipt%20%2F%20id.json":
			if r.Method != http.MethodGet {
				t.Errorf("status method = %s, want GET", r.Method)
			}
			if got := r.URL.Query().Get("token"); got != "test-token" {
				t.Error("status token was not sent")
			}
			_, _ = w.Write([]byte(`{"status":1,"acknowledged":1,"acknowledged_at":10,"acknowledged_by":"user","last_delivered_at":9,"expired":0,"expires_at":100,"called_back":0,"called_back_at":0,"cancelled":true,"cancelled_at":11,"cancelled_by_tag":true}`))
		case "/1/receipts/receipt%20%2F%20id/cancel.json":
			if r.Method != http.MethodPost {
				t.Errorf("cancel method = %s, want POST", r.Method)
			}
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error = %v", err)
			}
			if got := r.Form.Get("token"); got != "test-token" {
				t.Error("cancel token was not sent")
			}
			_, _ = w.Write([]byte(`{"status":1,"request":"cancel-request"}`))
		case "/1/receipts/cancel_by_tag/release%20%2F%20blue.json":
			if r.Method != http.MethodPost {
				t.Errorf("cancel by tag method = %s, want POST", r.Method)
			}
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error = %v", err)
			}
			if got := r.Form.Get("token"); got != "test-token" {
				t.Error("cancel by tag token was not sent")
			}
			_, _ = w.Write([]byte(`{"status":1,"request":"tag-request","cancelled":3}`))
		default:
			t.Errorf("unexpected path %q", r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := New(server.URL, "test")
	status, err := client.ReceiptStatus(context.Background(), "test-token", receiptID)
	if err != nil {
		t.Fatalf("ReceiptStatus() error = %v", err)
	}
	if status.Acknowledged != 1 || status.CancelledAt != 11 || !status.CancelledByTag {
		t.Errorf("ReceiptStatus() result = %+v", status)
	}
	if err := client.CancelReceipt(context.Background(), "test-token", receiptID); err != nil {
		t.Fatalf("CancelReceipt() error = %v", err)
	}
	cancelled, err := client.CancelReceiptsByTag(context.Background(), "test-token", tag)
	if err != nil {
		t.Fatalf("CancelReceiptsByTag() error = %v", err)
	}
	if cancelled != 3 {
		t.Errorf("CancelReceiptsByTag() = %d, want 3", cancelled)
	}
}

func TestPollReceiptUsesFiveSecondFloor(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if requests == 1 {
			_, _ = w.Write([]byte(`{"status":1,"acknowledged":0,"expired":0,"called_back":0,"cancelled":false}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":1,"acknowledged":1,"expired":0,"called_back":0,"cancelled":false}`))
	}))
	defer server.Close()

	var sleeps []time.Duration
	status, err := New(server.URL, "test").PollReceipt(
		context.Background(), "test-token", "receipt-id",
		func(receipt *Receipt) bool { return receipt.Acknowledged != 0 },
		func(duration time.Duration) { sleeps = append(sleeps, duration) },
	)
	if err != nil {
		t.Fatalf("PollReceipt() error = %v", err)
	}
	if status.Acknowledged != 1 {
		t.Errorf("PollReceipt() result = %+v", status)
	}
	if requests != 2 {
		t.Errorf("requests = %d, want 2", requests)
	}
	if len(sleeps) != 1 || sleeps[0] < 5*time.Second {
		t.Errorf("sleeps = %v, want one duration of at least 5s", sleeps)
	}
}

func TestPollReceiptWaitRespectsContextCancellation(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write([]byte(`{"status":1,"acknowledged":0,"expired":0,"called_back":0,"cancelled":false}`))
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	result, err := New(server.URL, "test").PollReceipt(
		ctx, "test-token", "receipt-id",
		func(*Receipt) bool { return false }, nil,
	)
	if err != context.DeadlineExceeded {
		t.Errorf("PollReceipt() error = %v, want deadline exceeded", err)
	}
	if result != nil {
		t.Errorf("PollReceipt() result = %+v, want nil", result)
	}
	if requests != 1 {
		t.Errorf("requests = %d, want 1", requests)
	}
	if elapsed := time.Since(started); elapsed >= time.Second {
		t.Errorf("PollReceipt() waited %s, want prompt cancellation", elapsed)
	}
}

func TestPollReceiptRejectsNilStop(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer server.Close()
	result, err := New(server.URL, "test").PollReceipt(context.Background(), "test-token", "receipt-id", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "stop") {
		t.Errorf("PollReceipt() error = %v, want stop function error", err)
	}
	if result != nil {
		t.Errorf("PollReceipt() result = %+v, want nil", result)
	}
}
