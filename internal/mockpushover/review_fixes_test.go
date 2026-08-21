package mockpushover

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestQuotaExhaustedIncludesHeaders(t *testing.T) {
	seed := testSeed()
	seed.Tokens[0].Quota = AppQuota{Limit: 5, Remaining: 0, Reset: 1_800_000_000}
	server, err := Start(Config{InitialState: &seed})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })

	response, body := doForm(t, server, http.MethodPost, "/1/messages.json", validMessageForm())
	requireStatus(t, response, http.StatusTooManyRequests, body)
	wantHeaders := map[string]string{
		"X-Limit-App-Limit":     "5",
		"X-Limit-App-Remaining": "0",
		"X-Limit-App-Reset":     "1800000000",
	}
	for name, want := range wantHeaders {
		if got := response.Header.Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestAPIErrorEnvelope(t *testing.T) {
	t.Run("request body too large", func(t *testing.T) {
		server := startTestServer(t)
		response, body := doForm(t, server, http.MethodPost, "/1/messages.json", url.Values{
			"payload": {strings.Repeat("x", (8<<20)+1)},
		})
		requireStatus(t, response, http.StatusRequestEntityTooLarge, body)
		requireStandardErrorEnvelope(t, response, body)
	})

	t.Run("unknown route", func(t *testing.T) {
		server := startTestServer(t)
		response, body := doForm(t, server, http.MethodGet, "/not-registered", nil)
		requireStatus(t, response, http.StatusNotFound, body)
		requireStandardErrorEnvelope(t, response, body)
	})
}

func TestFaultContentTypeMatchesBody(t *testing.T) {
	server := startTestServer(t)
	faults := []FaultRule{
		{
			Method: http.MethodGet, PathGlob: "/1/sounds.json", Status: http.StatusServiceUnavailable,
			Body: nil, Times: 1,
		},
		{
			Method: http.MethodGet, PathGlob: "/1/sounds.json", Status: http.StatusBadGateway,
			Body: map[string]any{"fault": "json"}, Times: 1,
		},
	}
	for _, fault := range faults {
		if err := server.Control().Fault(fault); err != nil {
			t.Fatalf("Fault() error = %v", err)
		}
	}

	response, body := doForm(t, server, http.MethodGet, "/1/sounds.json", url.Values{"token": {testToken}})
	requireStatus(t, response, http.StatusServiceUnavailable, body)
	if contentType := response.Header.Get("Content-Type"); strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("Content-Type = %q for non-JSON body %q", contentType, body)
	}

	response, body = doForm(t, server, http.MethodGet, "/1/sounds.json", url.Values{"token": {testToken}})
	requireStatus(t, response, http.StatusBadGateway, body)
	if contentType := response.Header.Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}
	if got := decodeObject(t, body)["fault"]; got != "json" {
		t.Fatalf("fault body = %s", body)
	}
}

func TestSeedPreservesErrorShape(t *testing.T) {
	server := startTestServer(t)
	seed := testSeed()
	seed.ErrorShape = ErrorShapeObject
	if err := server.Control().Seed(seed); err != nil {
		t.Fatalf("Seed(object) error = %v", err)
	}
	seed.ErrorShape = ""
	if err := server.Control().Seed(seed); err != nil {
		t.Fatalf("Seed(unset) error = %v", err)
	}

	response, body := doForm(t, server, http.MethodPost, "/1/users/validate.json", url.Values{
		"token": {testToken}, "user": {strings.Repeat("x", 30)},
	})
	requireStatus(t, response, http.StatusBadRequest, body)
	payload := requireStandardErrorEnvelope(t, response, body)
	rawErrors, err := json.Marshal(payload["errors"])
	if err != nil {
		t.Fatalf("json.Marshal(errors) error = %v", err)
	}
	var errors map[string][]string
	if err := json.Unmarshal(rawErrors, &errors); err != nil {
		t.Fatalf("errors = %s, want object: %v", rawErrors, err)
	}
	if len(errors["user"]) != 1 {
		t.Fatalf("errors = %#v", errors)
	}
}

func TestDisabledDeviceOpenMessages(t *testing.T) {
	server := startTestServer(t)
	seed := testSeed()
	seed.Devices[0].Enabled = false
	if err := server.Control().Seed(seed); err != nil {
		t.Fatalf("Seed() error = %v", err)
	}

	response, body := doForm(t, server, http.MethodGet, "/1/messages.json", url.Values{
		"device_id": {testDeviceID}, "secret": {testBinding},
	})
	requireStatus(t, response, http.StatusBadRequest, body)
	payload := requireStandardErrorEnvelope(t, response, body)
	errors, ok := payload["errors"].([]any)
	if !ok || len(errors) != 1 || errors[0] != "secret or device_id is invalid" {
		t.Fatalf("errors = %#v", payload["errors"])
	}
}

func requireStandardErrorEnvelope(t *testing.T, response *http.Response, body []byte) map[string]any {
	t.Helper()
	if contentType := response.Header.Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}
	payload := decodeObject(t, body)
	if payload["status"] != float64(0) {
		t.Fatalf("status payload = %#v", payload)
	}
	if _, ok := payload["errors"]; !ok {
		t.Fatalf("errors missing from payload = %#v", payload)
	}
	if request, ok := payload["request"].(string); !ok || request == "" {
		t.Fatalf("request missing from payload = %#v", payload)
	}
	return payload
}
