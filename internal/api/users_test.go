package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUsersValidate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %s, want %s", request.Method, http.MethodPost)
		}
		if request.URL.Path != "/1/users/validate.json" {
			t.Errorf("path = %s, want /1/users/validate.json", request.URL.Path)
		}
		if err := request.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		if got := request.PostForm.Get("token"); got != "test-app-token-not-real" {
			t.Errorf("token = %q", got)
		}
		if got := request.PostForm.Get("user"); got != "test-user-key-not-real" {
			t.Errorf("user = %q", got)
		}
		if got := request.PostForm.Get("device"); got != "phone" {
			t.Errorf("device = %q", got)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"status":1,"request":"test-request","devices":["phone"],"licenses":["test-license"]}`))
	}))
	defer server.Close()

	service := NewUsersService(New(server.URL, "test-version"))
	response, err := service.Validate(context.Background(), ValidateRequest{
		Token:  "test-app-token-not-real",
		User:   "test-user-key-not-real",
		Device: "phone",
	})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if response.Status != 1 {
		t.Fatalf("status = %d, want 1", response.Status)
	}
	if len(response.Devices) != 1 || response.Devices[0] != "phone" {
		t.Fatalf("devices = %#v", response.Devices)
	}
	if len(response.Licenses) != 1 || response.Licenses[0] != "test-license" {
		t.Fatalf("licenses = %#v", response.Licenses)
	}
}

func TestUsersValidateReturnsInvalidResponseAndAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"status":0,"request":"test-request","errors":{"user":["is invalid"]}}`))
	}))
	defer server.Close()

	service := NewUsersService(New(server.URL, "test-version"))
	response, err := service.Validate(context.Background(), ValidateRequest{
		Token: "test-app-token-not-real",
		User:  "test-user-key-not-real",
	})
	if response == nil || response.Status != 0 {
		t.Fatalf("response = %#v, want status 0", response)
	}
	var apiError *APIError
	if !errors.As(err, &apiError) {
		t.Fatalf("error = %v, want APIError", err)
	}
	if got := response.Errors["user"]; len(got) != 1 || got[0] != "is invalid" {
		t.Fatalf("errors = %#v", response.Errors)
	}
}
