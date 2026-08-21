package openclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAcknowledgeSendsExpectedRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/1/receipts/fake-receipt-123/acknowledge.json" {
			t.Errorf("request = %s %s", request.Method, request.URL.Path)
		}
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}
		if request.Form.Get("secret") != "fake-secret-123" {
			t.Error("acknowledgement form did not contain the expected secret")
		}
		_, _ = writer.Write([]byte(`{"status":1}`))
	}))
	defer server.Close()

	if err := Acknowledge(context.Background(), testClient(server.URL), "fake-secret-123", "fake-receipt-123"); err != nil {
		t.Fatalf("Acknowledge() error = %v", err)
	}
}
