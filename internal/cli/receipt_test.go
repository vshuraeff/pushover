package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vshuraeff/pushover/internal/api"
)

func TestReceiptCommands(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/1/receipts/receipt-id.json":
			if r.Method != http.MethodGet {
				t.Errorf("status method = %s, want GET", r.Method)
			}
			if r.URL.Query().Get("token") == "" {
				t.Error("status token was not sent")
			}
			_, _ = w.Write([]byte(`{"status":1,"acknowledged":1,"acknowledged_at":10,"acknowledged_by":"user","last_delivered_at":9,"expired":0,"expires_at":100,"called_back":0,"called_back_at":0,"cancelled":false}`))
		case "/1/receipts/receipt-id/cancel.json":
			if r.Method != http.MethodPost {
				t.Errorf("cancel method = %s, want POST", r.Method)
			}
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error = %v", err)
			}
			if r.Form.Get("token") == "" {
				t.Error("cancel token was not sent")
			}
			_, _ = w.Write([]byte(`{"status":1,"request":"request-id"}`))
		case "/1/receipts/cancel_by_tag/release.json":
			if r.Method != http.MethodPost {
				t.Errorf("cancel by tag method = %s, want POST", r.Method)
			}
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error = %v", err)
			}
			if r.Form.Get("token") == "" {
				t.Error("cancel by tag token was not sent")
			}
			_, _ = w.Write([]byte(`{"status":1,"request":"request-id","cancelled":2}`))
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	configureCLITestEnv(t, server.URL)

	statusOutput, err := executeCLI(t, nil, "receipt", "status", "--json", "receipt-id")
	if err != nil {
		t.Fatalf("povr receipt status error = %v", err)
	}
	var receipt api.Receipt
	if err := json.Unmarshal([]byte(statusOutput), &receipt); err != nil {
		t.Fatalf("receipt status output is not JSON: %v", err)
	}
	if receipt.Acknowledged != 1 || receipt.AcknowledgedBy != "user" {
		t.Errorf("receipt status = %+v", receipt)
	}

	cancelOutput, err := executeCLI(t, nil, "receipt", "cancel", "receipt-id")
	if err != nil {
		t.Fatalf("povr receipt cancel error = %v", err)
	}
	if !strings.Contains(cancelOutput, "receipt receipt-id cancelled") {
		t.Errorf("receipt cancel output = %q", cancelOutput)
	}
	cancelJSONOutput, err := executeCLI(t, nil, "receipt", "cancel", "--json", "receipt-id")
	if err != nil {
		t.Fatalf("povr receipt cancel --json error = %v", err)
	}
	var cancelJSON struct {
		Receipt   string `json:"receipt"`
		Cancelled bool   `json:"cancelled"`
	}
	if err := json.Unmarshal([]byte(cancelJSONOutput), &cancelJSON); err != nil {
		t.Fatalf("receipt cancel JSON output is not JSON: %v", err)
	}
	if cancelJSON.Receipt != "receipt-id" || !cancelJSON.Cancelled {
		t.Errorf("receipt cancel JSON = %+v", cancelJSON)
	}

	byTagOutput, err := executeCLI(t, nil, "receipt", "cancel-by-tag", "release")
	if err != nil {
		t.Fatalf("povr receipt cancel-by-tag error = %v", err)
	}
	if !strings.Contains(byTagOutput, "cancelled: 2") {
		t.Errorf("receipt cancel-by-tag output = %q", byTagOutput)
	}
	byTagJSONOutput, err := executeCLI(t, nil, "receipt", "cancel-by-tag", "--json", "release")
	if err != nil {
		t.Fatalf("povr receipt cancel-by-tag --json error = %v", err)
	}
	var byTagJSON struct {
		Tag       string `json:"tag"`
		Cancelled bool   `json:"cancelled"`
	}
	if err := json.Unmarshal([]byte(byTagJSONOutput), &byTagJSON); err != nil {
		t.Fatalf("receipt cancel-by-tag JSON output is not JSON: %v", err)
	}
	if byTagJSON.Tag != "release" || !byTagJSON.Cancelled {
		t.Errorf("receipt cancel-by-tag JSON = %+v", byTagJSON)
	}
}
