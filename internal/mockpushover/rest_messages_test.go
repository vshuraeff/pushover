package mockpushover

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func TestMessagesAndOpenClient(t *testing.T) {
	t.Run("send download and advance cursor", func(t *testing.T) {
		server := startTestServer(t)
		form := validMessageForm()
		form.Set("title", "Greeting")
		form.Set("url", "https://example.com")
		form.Set("url_title", "Example")
		form.Set("sound", "bike")
		form.Set("html", "1")
		response, body := doForm(t, server, http.MethodPost, "/1/messages.json", form)
		requireStatus(t, response, http.StatusOK, body)
		if response.Header.Get("X-Limit-App-Limit") != "5" || response.Header.Get("X-Limit-App-Remaining") != "4" {
			t.Fatalf("quota headers = %#v", response.Header)
		}

		response, body = doForm(t, server, http.MethodGet, "/1/messages.json", url.Values{
			"secret": {testBinding}, "device_id": {testDeviceID},
		})
		requireStatus(t, response, http.StatusOK, body)
		var downloaded struct {
			Status   int       `json:"status"`
			Messages []Message `json:"messages"`
		}
		if err := json.Unmarshal(body, &downloaded); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		if len(downloaded.Messages) != 1 {
			t.Fatalf("messages = %#v", downloaded.Messages)
		}
		message := downloaded.Messages[0]
		if message.ID != 1 || message.IDString != "1" || message.UMIDString == "" || message.Title != "Greeting" || message.HTML != 1 {
			t.Fatalf("message = %#v", message)
		}
		response, body = doForm(t, server, http.MethodPost, "/1/devices/"+testDeviceID+"/update_highest_message.json", url.Values{
			"secret": {testBinding}, "message": {strconv.FormatInt(message.ID, 10)},
		})
		requireStatus(t, response, http.StatusOK, body)
		response, body = doForm(t, server, http.MethodGet, "/1/messages.json", url.Values{
			"secret": {testBinding}, "device_id": {testDeviceID},
		})
		requireStatus(t, response, http.StatusOK, body)
		if messages := decodeObject(t, body)["messages"].([]any); len(messages) != 0 {
			t.Fatalf("messages after cursor = %#v", messages)
		}
		response, body = doForm(t, server, http.MethodPost, "/1/devices/"+testDeviceID+"/update_highest_message.json", url.Values{
			"secret": {testBinding}, "message": {"100"},
		})
		requireStatus(t, response, http.StatusOK, body)
		response, body = doForm(t, server, http.MethodPost, "/1/messages.json", validMessageForm())
		requireStatus(t, response, http.StatusOK, body)
		response, body = doForm(t, server, http.MethodGet, "/1/messages.json", url.Values{
			"secret": {testBinding}, "device_id": {testDeviceID},
		})
		requireStatus(t, response, http.StatusOK, body)
		messages := decodeObject(t, body)["messages"].([]any)
		if len(messages) != 1 || messages[0].(map[string]any)["id"] != float64(101) {
			t.Fatalf("messages after advanced cursor = %s", body)
		}
	})

	t.Run("multipart attachment is stored and journaled", func(t *testing.T) {
		server := startTestServer(t)
		attachment := []byte("fixture-attachment")
		response, body := doMultipart(t, server, "/1/messages.json", map[string]string{
			"token": testToken, "user": testUser, "message": "with attachment",
		}, "fixture.png", attachment)
		requireStatus(t, response, http.StatusOK, body)
		server.store.mu.RLock()
		stored := append([]byte(nil), server.store.queues[testDeviceID][0].Attachment...)
		server.store.mu.RUnlock()
		if !bytes.Equal(stored, attachment) {
			t.Fatalf("attachment = %q, want %q", stored, attachment)
		}
		requests, err := server.Control().Requests()
		if err != nil {
			t.Fatalf("Requests() error = %v", err)
		}
		if got := requests[0].Fields["attachment"]; len(got) != 1 || got[0] != "fixture.png" {
			t.Fatalf("journal attachment field = %#v", got)
		}
	})

	t.Run("base64 attachment is stored", func(t *testing.T) {
		server := startTestServer(t)
		attachment := []byte("base64 fixture")
		form := validMessageForm()
		form.Set("attachment_base64", base64.StdEncoding.EncodeToString(attachment))
		response, body := doForm(t, server, http.MethodPost, "/1/messages.json", form)
		requireStatus(t, response, http.StatusOK, body)
		server.store.mu.RLock()
		stored := append([]byte(nil), server.store.queues[testDeviceID][0].Attachment...)
		server.store.mu.RUnlock()
		if !bytes.Equal(stored, attachment) {
			t.Fatalf("attachment = %q, want %q", stored, attachment)
		}
	})

	t.Run("accepted delivery with no devices decrements quota", func(t *testing.T) {
		seed := testSeed()
		seed.Devices = nil
		server, err := Start(Config{InitialState: &seed})
		if err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		t.Cleanup(func() { _ = server.Close() })
		response, body := doForm(t, server, http.MethodPost, "/1/messages.json", validMessageForm())
		requireStatus(t, response, http.StatusOK, body)
		if got := response.Header.Get("X-Limit-App-Remaining"); got != "4" {
			t.Fatalf("remaining = %q, want 4", got)
		}
	})
}

func TestMessageValidation(t *testing.T) {
	tooManyUsers := make([]string, 51)
	for i := range tooManyUsers {
		tooManyUsers[i] = testUser
	}
	tests := []struct {
		name  string
		field string
		value string
		want  int
	}{
		{name: "message required", field: "message", value: "", want: http.StatusBadRequest},
		{name: "message too long", field: "message", value: strings.Repeat("m", 1025), want: http.StatusBadRequest},
		{name: "title too long", field: "title", value: strings.Repeat("t", 251), want: http.StatusBadRequest},
		{name: "url too long", field: "url", value: strings.Repeat("u", 513), want: http.StatusBadRequest},
		{name: "url title too long", field: "url_title", value: strings.Repeat("u", 101), want: http.StatusBadRequest},
		{name: "invalid user", field: "user", value: "short", want: http.StatusBadRequest},
		{name: "too many users", field: "user", value: strings.Join(tooManyUsers, ","), want: http.StatusBadRequest},
		{name: "priority range", field: "priority", value: "3", want: http.StatusBadRequest},
		{name: "html range", field: "html", value: "2", want: http.StatusBadRequest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := startTestServer(t)
			form := validMessageForm()
			form.Set(test.field, test.value)
			response, body := doForm(t, server, http.MethodPost, "/1/messages.json", form)
			requireStatus(t, response, test.want, body)
			if decodeObject(t, body)["status"] != float64(0) {
				t.Fatalf("body = %s", body)
			}
		})
	}

	emergencyTests := []struct {
		name   string
		retry  string
		expire string
	}{
		{name: "retry too small", retry: "29", expire: "60"},
		{name: "expire missing", retry: "30", expire: ""},
		{name: "expire too large", retry: "30", expire: "10801"},
	}
	for _, test := range emergencyTests {
		t.Run(test.name, func(t *testing.T) {
			server := startTestServer(t)
			form := validMessageForm()
			form.Set("priority", "2")
			form.Set("retry", test.retry)
			form.Set("expire", test.expire)
			response, body := doForm(t, server, http.MethodPost, "/1/messages.json", form)
			requireStatus(t, response, http.StatusBadRequest, body)
		})
	}
}

func TestEmergencyReceiptFlow(t *testing.T) {
	server := startTestServer(t)
	form := validMessageForm()
	form.Set("priority", "2")
	form.Set("retry", "30")
	form.Set("expire", "300")
	form.Set("tags", "incident-7")
	response, body := doForm(t, server, http.MethodPost, "/1/messages.json", form)
	requireStatus(t, response, http.StatusOK, body)
	receiptID, ok := decodeObject(t, body)["receipt"].(string)
	if !ok || receiptID == "" {
		t.Fatalf("receipt response = %s", body)
	}

	response, body = doForm(t, server, http.MethodGet, "/1/receipts/"+receiptID+".json", url.Values{"token": {testToken}})
	requireStatus(t, response, http.StatusOK, body)
	if decodeObject(t, body)["acknowledged"] != float64(0) {
		t.Fatalf("receipt body = %s", body)
	}
	response, body = doForm(t, server, http.MethodPost, "/1/receipts/"+receiptID+"/acknowledge.json", url.Values{"secret": {"wrong"}})
	requireStatus(t, response, http.StatusBadRequest, body)
	response, body = doForm(t, server, http.MethodPost, "/1/receipts/"+receiptID+"/acknowledge.json", url.Values{"secret": {testBinding}})
	requireStatus(t, response, http.StatusOK, body)
	response, body = doForm(t, server, http.MethodGet, "/1/receipts/"+receiptID+".json", url.Values{"token": {testToken}})
	requireStatus(t, response, http.StatusOK, body)
	receipt := decodeObject(t, body)
	if receipt["acknowledged"] != float64(1) || receipt["acknowledged_by"] != testUser {
		t.Fatalf("acknowledged receipt = %s", body)
	}
	response, body = doForm(t, server, http.MethodPost, "/1/receipts/"+receiptID+"/cancel.json", url.Values{"token": {testToken}})
	requireStatus(t, response, http.StatusOK, body)

	second := validMessageForm()
	second.Set("priority", "2")
	second.Set("retry", "30")
	second.Set("expire", "300")
	second.Set("tags", "incident-7")
	response, body = doForm(t, server, http.MethodPost, "/1/messages.json", second)
	requireStatus(t, response, http.StatusOK, body)
	secondReceipt := decodeObject(t, body)["receipt"].(string)
	response, body = doForm(t, server, http.MethodPost, "/1/receipts/cancel_by_tag/incident-7.json", url.Values{"token": {testToken}})
	requireStatus(t, response, http.StatusOK, body)
	response, body = doForm(t, server, http.MethodGet, "/1/receipts/"+secondReceipt+".json", url.Values{"token": {testToken}})
	requireStatus(t, response, http.StatusOK, body)
	if cancelled, _ := decodeObject(t, body)["cancelled_by_tag"].(bool); !cancelled {
		t.Fatalf("cancel by tag receipt = %s", body)
	}
}

func TestAssets(t *testing.T) {
	server := startTestServer(t)
	tests := []struct {
		path        string
		contentType string
		prefix      []byte
	}{
		{path: "/icons/test-icon.png", contentType: "image/png", prefix: []byte{0x89, 'P', 'N', 'G'}},
		{path: "/sounds/bike.mp3", contentType: "audio/mpeg", prefix: []byte("ID3")},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			response, err := server.client.Get(server.URL() + test.path)
			if err != nil {
				t.Fatalf("Get() error = %v", err)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatalf("ReadAll() error = %v", err)
			}
			if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != test.contentType || !bytes.HasPrefix(body, test.prefix) {
				t.Fatalf("status = %d, content-type = %q, body = %x", response.StatusCode, response.Header.Get("Content-Type"), body)
			}
		})
	}
}
