package openclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestFetchDecodesMessageAndDoesNotAcknowledge(t *testing.T) {
	var acknowledgementCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/1/messages.json" {
			t.Errorf("request = %s %s", request.Method, request.URL.Path)
		}
		if request.URL.Query().Get("secret") != "fake-secret-123" || request.URL.Query().Get("device_id") != "fake-device-123" {
			t.Error("fetch query did not contain expected credentials")
		}
		if strings.Contains(request.URL.Path, "acknowledge") {
			acknowledgementCalls.Add(1)
		}
		_, _ = writer.Write([]byte(`{
			"status":1,
			"messages":[{
				"id":123,"id_str":"123","umid":456,"umid_str":"456","title":"Title","message":"Body","app":"Example","aid":789,"aid_str":"789","icon":"icon","date":1700000000,"queued_date":1700000001,"dispatched_date":1700000002,"priority":2,"sound":"bike","url":"https://example.invalid/path","url_title":"Example URL","acked":1,"receipt":"fake-receipt-123","html":1,"unknown_extra":"ignored"
			}],
			"mfailure":0
		}`))
	}))
	defer server.Close()

	messages, err := Fetch(context.Background(), testClient(server.URL), "fake-secret-123", "fake-device-123")
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("messages length = %d, want 1", len(messages))
	}
	want := Message{
		ID: 123, IDStr: "123", UMID: 456, UMIDStr: "456", Title: "Title", Message: "Body", App: "Example", AID: 789, AIDStr: "789", Icon: "icon",
		Date: 1700000000, QueuedDate: 1700000001, DispatchedDate: 1700000002, Priority: 2, Sound: "bike", URL: "https://example.invalid/path", URLTitle: "Example URL", Acked: 1, Receipt: "fake-receipt-123", HTML: 1,
	}
	if messages[0] != want {
		t.Errorf("message = %#v, want %#v", messages[0], want)
	}
	if got := acknowledgementCalls.Load(); got != 0 {
		t.Errorf("implicit acknowledgement calls = %d, want 0", got)
	}
}

func TestUpdateHighestSendsExpectedRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/1/devices/fake-device-123/update_highest_message.json" {
			t.Errorf("request = %s %s", request.Method, request.URL.Path)
		}
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}
		if request.Form.Get("secret") != "fake-secret-123" || request.Form.Get("message") != "987" {
			t.Error("update form did not contain expected fields")
		}
		_, _ = writer.Write([]byte(`{"status":1}`))
	}))
	defer server.Close()

	if err := UpdateHighest(context.Background(), testClient(server.URL), "fake-secret-123", "fake-device-123", 987); err != nil {
		t.Fatalf("UpdateHighest() error = %v", err)
	}
}
