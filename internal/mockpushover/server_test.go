package mockpushover

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

const (
	testToken     = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testUser      = "uuuuuuuuuuuuuuuuuuuuuuuuuuuuuu"
	testUser2     = "vvvvvvvvvvvvvvvvvvvvvvvvvvvvvv"
	testGroup     = "gggggggggggggggggggggggggggggg"
	testDeviceID  = "device-id-1"
	testDeviceID2 = "device-id-2"
	testBinding   = "session-binding-one"
	testBinding2  = "session-binding-two"
	testTeamToken = "team-token"
)

func testSeed() SeedState {
	return SeedState{
		Tokens: []AppToken{{
			Token: testToken, Name: "Test App", ID: 42, Icon: "test-icon",
			Quota: AppQuota{Limit: 5, Remaining: 5, Reset: 1_800_000_000}, Credits: 2,
		}},
		Users: []User{
			{Key: testUser, Email: "one@example.com", Password: "password-one", Secret: testBinding},
			{Key: testUser2, Email: "two@example.com", Password: "password-two", Secret: testBinding2},
		},
		TwoFAAccounts: []TwoFAAccount{{
			Email: "two@example.com", Password: "password-two", User: testUser2, Secret: testBinding2, Code: "123456",
		}},
		Devices: []Device{
			{ID: testDeviceID, User: testUser, Name: "phone", OS: "O", Secret: testBinding, Enabled: true},
			{ID: testDeviceID2, User: testUser2, Name: "tablet", OS: "O", Secret: testBinding2, Enabled: true},
		},
		Groups: []Group{{
			Key: testGroup, Name: "Operators",
			Members: []GroupMember{{User: testUser, Device: "phone", Memo: "primary"}},
		}},
		CustomSounds: map[string]string{"custom": "Custom Sound"},
		Teams: []Team{{Token: testTeamToken, Name: "Test Team", Users: []TeamMember{{
			ID: "member-1", Name: "Member One", Email: "member@example.com",
		}}}},
	}
}

func startTestServer(t *testing.T) *Server {
	t.Helper()
	seed := testSeed()
	server, err := Start(Config{InitialState: &seed})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() {
		if err := server.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	return server
}

func doForm(t *testing.T, server *Server, method, endpoint string, values url.Values) (*http.Response, []byte) {
	t.Helper()
	var body io.Reader
	if method != http.MethodGet {
		body = strings.NewReader(values.Encode())
	} else if len(values) > 0 {
		endpoint += "?" + values.Encode()
	}
	request, err := http.NewRequest(method, server.URL()+endpoint, body)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	if method != http.MethodGet {
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	response, err := server.client.Do(request)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	return response, responseBody
}

func doMultipart(t *testing.T, server *Server, endpoint string, fields map[string]string, filename string, attachment []byte) (*http.Response, []byte) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("WriteField() error = %v", err)
		}
	}
	if filename != "" {
		part, err := writer.CreateFormFile("attachment", filename)
		if err != nil {
			t.Fatalf("CreateFormFile() error = %v", err)
		}
		if _, err := part.Write(attachment); err != nil {
			t.Fatalf("attachment Write() error = %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("multipart Close() error = %v", err)
	}
	request, err := http.NewRequest(http.MethodPost, server.URL()+endpoint, &body)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := server.client.Do(request)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	return response, responseBody
}

func decodeObject(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", body, err)
	}
	return payload
}

func requireStatus(t *testing.T, response *http.Response, want int, body []byte) {
	t.Helper()
	if response.StatusCode != want {
		t.Fatalf("status = %d, want %d, body = %s", response.StatusCode, want, body)
	}
}

func validMessageForm() url.Values {
	return url.Values{"token": {testToken}, "user": {testUser}, "message": {"hello"}}
}
