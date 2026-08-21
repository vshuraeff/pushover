package api

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestAPIErrorUnmarshalsErrorShapes(t *testing.T) {
	tests := []struct {
		name string
		body string
		want ErrorMessages
	}{
		{
			name: "array",
			body: `{"status":0,"request":"request-test-id","errors":["error one","error two"]}`,
			want: ErrorMessages{generalErrorsKey: {"error one", "error two"}},
		},
		{
			name: "object",
			body: `{"status":0,"request":"request-test-id","errors":{"field1":["bad value"],"field2":["required"]}}`,
			want: ErrorMessages{"field1": {"bad value"}, "field2": {"required"}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var apiError APIError
			if err := json.Unmarshal([]byte(test.body), &apiError); err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}
			if apiError.Request != "request-test-id" {
				t.Errorf("request = %q", apiError.Request)
			}
			if !reflect.DeepEqual(apiError.Errors, test.want) {
				t.Errorf("errors = %#v, want %#v", apiError.Errors, test.want)
			}
		})
	}
}

func TestQuotaErrorIncludesAPIMessageAndReset(t *testing.T) {
	err := (&QuotaError{
		Reset: "1900000123",
		APIError: &APIError{
			Errors: ErrorMessages{generalErrorsKey: {"monthly message limit exceeded"}},
		},
	}).Error()

	for _, want := range []string{"monthly message limit exceeded", "1900000123"} {
		if !strings.Contains(err, want) {
			t.Errorf("Error() = %q, want substring %q", err, want)
		}
	}
}
