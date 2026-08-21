package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestAssignLicenseSendsFormAndParsesCredits(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %s, want %s", request.Method, http.MethodPost)
		}
		if request.URL.Path != "/1/licenses/assign.json" {
			t.Errorf("path = %s", request.URL.Path)
		}
		if got := request.FormValue("token"); got != "test-fake-app-token-1234" {
			t.Errorf("token = %q", got)
		}
		if got := request.FormValue("user"); got != "test-fake-user-key-1234" {
			t.Errorf("user = %q", got)
		}
		if got := request.FormValue("email"); got != "" {
			t.Errorf("email = %q, want empty", got)
		}
		if got := request.FormValue("os"); got != "iOS" {
			t.Errorf("os = %q", got)
		}
		_, _ = writer.Write([]byte(`{"status":1,"credits":4}`))
	}))
	defer server.Close()

	credits, err := newTestClient(server.URL).AssignLicense(context.Background(), "test-fake-app-token-1234", "test-fake-user-key-1234", "", "iOS")
	if err != nil {
		t.Fatalf("AssignLicense() error = %v", err)
	}
	if credits != 4 {
		t.Errorf("credits = %d, want 4", credits)
	}
}

func TestLicenseCreditsSendsTokenAndParsesResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			t.Errorf("method = %s, want %s", request.Method, http.MethodGet)
		}
		if request.URL.Path != "/1/licenses.json" {
			t.Errorf("path = %s", request.URL.Path)
		}
		if got := request.URL.Query().Get("token"); got != "test-fake-app-token-1234" {
			t.Errorf("token = %q", got)
		}
		_, _ = writer.Write([]byte(`{"status":1,"credits":7}`))
	}))
	defer server.Close()

	credits, err := newTestClient(server.URL).LicenseCredits(context.Background(), "test-fake-app-token-1234")
	if err != nil {
		t.Fatalf("LicenseCredits() error = %v", err)
	}
	if credits != 7 {
		t.Errorf("credits = %d, want 7", credits)
	}
}

func TestAssignLicenseDoesNotRetryServerErrors(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		writer.WriteHeader(http.StatusInternalServerError)
		_, _ = writer.Write([]byte(`{"status":0,"errors":["server error"]}`))
	}))
	defer server.Close()

	_, err := newTestClient(server.URL).AssignLicense(context.Background(), "test-fake-app-token-1234", "test-fake-user-key-1234", "", "")
	if err == nil {
		t.Fatal("AssignLicense() error = nil, want error")
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("requests = %d, want 1", got)
	}
}

func TestLicenseCreditsPropagatesAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = writer.Write([]byte(`{"status":0,"errors":["invalid token"]}`))
	}))
	defer server.Close()

	_, err := newTestClient(server.URL).LicenseCredits(context.Background(), "test-fake-app-token-1234")
	if err == nil {
		t.Fatal("LicenseCredits() error = nil, want error")
	}
}

func TestAssignLicenseValidatesInputsBeforeRequest(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()

	_, err := newTestClient(server.URL).AssignLicense(context.Background(), "test-fake-app-token-1234", "", "", "Desktop")
	if err == nil {
		t.Fatal("AssignLicense() error = nil, want error")
	}
	_, err = newTestClient(server.URL).AssignLicense(context.Background(), "test-fake-app-token-1234", "test-fake-user-key-1234", "", "Windows")
	if err == nil {
		t.Fatal("AssignLicense() error = nil, want error")
	}
	if got := requests.Load(); got != 0 {
		t.Errorf("requests = %d, want 0", got)
	}
}
