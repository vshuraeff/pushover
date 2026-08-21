package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vshuraeff/pushover/internal/api"
)

func TestLimitsCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/1/apps/limits.json" {
			t.Errorf("path = %s, want /1/apps/limits.json", r.URL.Path)
		}
		if r.URL.Query().Get("token") == "" {
			t.Error("app token was not sent")
		}
		w.Header().Set("X-Limit-App-Limit", "10000")
		w.Header().Set("X-Limit-App-Remaining", "9999")
		w.Header().Set("X-Limit-App-Reset", "1700000000")
		_, _ = w.Write([]byte(`{"status":1,"request":"request-id","limit":10000,"remaining":9999,"reset":1700000000}`))
	}))
	defer server.Close()
	configureCLITestEnv(t, server.URL)

	output, err := executeCLI(t, nil, "limits", "--json")
	if err != nil {
		t.Fatalf("povr limits error = %v", err)
	}
	var limits api.AppLimits
	if err := json.Unmarshal([]byte(output), &limits); err != nil {
		t.Fatalf("limits output is not JSON: %v", err)
	}
	if limits.Limit != 10000 || limits.Remaining != 9999 || limits.Reset != 1_700_000_000 {
		t.Errorf("limits = %+v", limits)
	}
	if limits.Header != (api.RateLimits{Limit: "10000", Remaining: "9999", Reset: "1700000000"}) {
		t.Errorf("limits header = %+v", limits.Header)
	}

	humanOutput, err := executeCLI(t, nil, "limits")
	if err != nil {
		t.Fatalf("povr limits error = %v", err)
	}
	for _, want := range []string{"header_limit: 10000", "header_remaining: 9999", "header_reset: 1700000000"} {
		if !strings.Contains(humanOutput, want) {
			t.Errorf("limits output = %q, want it to contain %q", humanOutput, want)
		}
	}
}
