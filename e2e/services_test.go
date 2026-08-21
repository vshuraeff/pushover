//go:build e2e

package e2e

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/vshuraeff/pushover/internal/mockpushover"
)

func TestGroupFullLifecycle(t *testing.T) {
	h := newHarness(t, defaultSeed())
	baseURL := h.server.URL() + "/1"
	runGroup := func(args ...string) commandResult {
		t.Helper()
		prefix := []string{"group", "--api-base-url", baseURL, "--json"}
		return h.run(t, append(prefix, args...)...)
	}

	created := runGroup("create", "Incident Responders")
	requireSuccess(t, created)
	var createOutput struct {
		Group string `json:"group"`
	}
	if err := json.Unmarshal([]byte(created.stdout), &createOutput); err != nil {
		t.Fatalf("decode group create output: %v", err)
	}
	if len(createOutput.Group) != 30 {
		t.Fatalf("created group key length = %d, want 30", len(createOutput.Group))
	}
	groupKey := createOutput.Group

	listed := runGroup("list")
	requireSuccess(t, listed)
	if !strings.Contains(listed.stdout, groupKey) || !strings.Contains(listed.stdout, "Incident Responders") {
		t.Fatalf("created group was absent from list output: %s", listed.stdout)
	}

	added := runGroup("add-user", groupKey, fixtureUserKey2, "--device", "tablet", "--memo", "secondary")
	requireSuccess(t, added)
	shown := runGroup("show", groupKey)
	requireSuccess(t, shown)
	if !strings.Contains(shown.stdout, fixtureUserKey2) || !strings.Contains(shown.stdout, "secondary") {
		t.Fatalf("added member was absent from group output: %s", shown.stdout)
	}

	disabled := runGroup("disable-user", groupKey, fixtureUserKey2, "--device", "tablet")
	requireSuccess(t, disabled)
	shown = runGroup("show", groupKey)
	requireSuccess(t, shown)
	if !strings.Contains(shown.stdout, `"disabled": true`) {
		t.Fatalf("disabled membership was not reflected by show: %s", shown.stdout)
	}

	enabled := runGroup("enable-user", groupKey, fixtureUserKey2, "--device", "tablet")
	requireSuccess(t, enabled)
	shown = runGroup("show", groupKey)
	requireSuccess(t, shown)
	if !strings.Contains(shown.stdout, `"disabled": false`) {
		t.Fatalf("enabled membership was not reflected by show: %s", shown.stdout)
	}

	renamed := runGroup("rename", groupKey, "Primary Responders")
	requireSuccess(t, renamed)
	listed = runGroup("list")
	requireSuccess(t, listed)
	if !strings.Contains(listed.stdout, "Primary Responders") {
		t.Fatalf("renamed group was not reflected by list: %s", listed.stdout)
	}

	removed := runGroup("remove-user", groupKey, fixtureUserKey2, "--device", "tablet")
	requireSuccess(t, removed)
	shown = runGroup("show", groupKey)
	requireSuccess(t, shown)
	if strings.Contains(shown.stdout, fixtureUserKey2) {
		t.Fatalf("removed member remained in group output: %s", shown.stdout)
	}
}

func TestGlanceSetsFieldsAndExplicitlyClearsTitle(t *testing.T) {
	h := newHarness(t, defaultSeed())
	set := h.run(t, "glance", "--user", fixtureUserKey, "--device", "phone", "--title", "deployment", "--text", "healthy", "--subtext", "production", "--count", "7", "--percent", "95")
	requireSuccess(t, set)
	clear := h.run(t, "glance", "--user", fixtureUserKey, "--device", "phone", "--title", "")
	requireSuccess(t, clear)

	requests := h.requests(t)
	if len(requests) != 2 {
		t.Fatalf("glance request count = %d, want 2", len(requests))
	}
	for field, want := range map[string]string{
		"title": "deployment", "text": "healthy", "subtext": "production", "count": "7", "percent": "95",
	} {
		requireField(t, requests[0], field, want)
	}
	values, ok := requests[1].Fields["title"]
	if !ok || len(values) != 1 || values[0] != "" {
		t.Fatal("clear request did not carry an explicit empty title field")
	}
	// the successful second request exercises the mock's clear branch; no public state read endpoint exists.
}

func TestSoundsAndLimitsReflectSeededValues(t *testing.T) {
	seed := defaultSeed()
	seed.Tokens[0].Quota = mockpushover.AppQuota{Limit: 500, Remaining: 321, Reset: 1_900_000_456}
	h := newHarness(t, seed)

	sounds := h.run(t, "sounds", "--json")
	requireSuccess(t, sounds)
	var soundMap map[string]string
	if err := json.Unmarshal([]byte(sounds.stdout), &soundMap); err != nil {
		t.Fatalf("decode sounds output: %v", err)
	}
	if soundMap["e2e-custom"] != "E2E Custom Sound" || soundMap["pushover"] == "" {
		t.Fatalf("sounds output did not contain seeded and built-in sounds")
	}

	limits := h.run(t, "limits", "--json")
	requireSuccess(t, limits)
	var quota struct {
		Limit     int   `json:"limit"`
		Remaining int   `json:"remaining"`
		Reset     int64 `json:"reset"`
		Header    struct {
			Limit     string `json:"limit"`
			Remaining string `json:"remaining"`
			Reset     string `json:"reset"`
		} `json:"header"`
	}
	if err := json.Unmarshal([]byte(limits.stdout), &quota); err != nil {
		t.Fatalf("decode limits output: %v", err)
	}
	if quota.Limit != 500 || quota.Remaining != 321 || quota.Reset != 1_900_000_456 {
		t.Fatalf("limits output did not match seeded quota")
	}
	if quota.Header.Limit != "500" || quota.Header.Remaining != "321" || quota.Header.Reset != "1900000456" {
		t.Fatalf("limits output did not include matching rate-limit headers")
	}
}

func TestValidateValidAndInvalidUserDeviceCombinations(t *testing.T) {
	h := newHarness(t, defaultSeed())
	valid := h.run(t, "validate", "--user", fixtureUserKey, "--device", "phone")
	requireSuccess(t, valid)
	if !strings.Contains(valid.stdout, "valid") || !strings.Contains(valid.stdout, "devices: phone") {
		t.Fatalf("valid user/device output = %q", valid.stdout)
	}

	invalid := h.run(t, "validate", "--user", fixtureUserKey, "--device", "missing")
	requireSuccess(t, invalid)
	if !strings.Contains(invalid.stdout, "invalid") || !strings.Contains(invalid.stdout, "device") {
		t.Fatalf("invalid user/device output = %q", invalid.stdout)
	}
}

func TestValidatePrintsArrayAndObjectErrorShapes(t *testing.T) {
	outputs := make(map[mockpushover.ErrorShape]string)
	for _, shape := range []mockpushover.ErrorShape{mockpushover.ErrorShapeArray, mockpushover.ErrorShapeObject} {
		t.Run(string(shape), func(t *testing.T) {
			seed := defaultSeed()
			seed.ErrorShape = shape
			h := newHarness(t, seed)
			result := h.run(t, "validate", "--user", fixtureInvalidKey)
			requireSuccess(t, result)
			if !strings.Contains(result.stdout, "invalid: pushover api error") || !strings.Contains(result.stdout, "invalid") {
				t.Fatalf("validation error shape was not readable: %s", result.stdout)
			}
			outputs[shape] = result.stdout
		})
	}
	if outputs[mockpushover.ErrorShapeArray] == outputs[mockpushover.ErrorShapeObject] {
		t.Fatal("array and object error shapes produced indistinguishable output")
	}
}

func TestLicenseCreditsAssignmentAndNoRetryFault(t *testing.T) {
	h := newHarness(t, defaultSeed())
	credits := h.run(t, "license", "credits")
	requireSuccess(t, credits)
	if !strings.Contains(credits.stdout, "remaining license credits: 3") {
		t.Fatalf("initial credits output = %q", credits.stdout)
	}

	assigned := h.run(t, "license", "assign", "--yes", "--user", fixtureUserKey, "--os", "Desktop")
	requireSuccess(t, assigned)
	if !strings.Contains(assigned.stdout, "remaining license credits: 2") {
		t.Fatalf("assignment output = %q", assigned.stdout)
	}
	credits = h.run(t, "license", "credits")
	requireSuccess(t, credits)
	if !strings.Contains(credits.stdout, "remaining license credits: 2") {
		t.Fatalf("credits were not decremented: %q", credits.stdout)
	}

	baseline := len(h.requests(t))
	if err := h.control.Fault(mockpushover.FaultRule{
		Method: http.MethodPost, PathGlob: "/1/licenses/assign.json", Status: http.StatusInternalServerError,
		Body: map[string]any{"status": 0, "errors": []string{"injected assignment failure"}}, Times: 1,
	}); err != nil {
		t.Fatalf("inject license assignment fault: %v", err)
	}
	failed := h.run(t, "license", "assign", "--yes", "--email", "fault@example.test", "--os", "iOS")
	requireFailure(t, failed, "injected assignment failure")
	requests := h.requests(t)
	if len(requests)-baseline != 1 {
		t.Fatalf("faulted assignment request count = %d, want exactly 1", len(requests)-baseline)
	}
	if requests[baseline].Path != "/1/licenses/assign.json" {
		t.Fatalf("faulted request path = %q", requests[baseline].Path)
	}
}

func TestTeamListUsesCustomTeamTokenEnvironmentName(t *testing.T) {
	h := newHarness(t, defaultSeed())
	env := h.withEnv(map[string]string{
		"MY_TEAM_TOKEN":       fixtureTeamToken,
		"PUSHOVER_TEAM_TOKEN": "wrong-default-team-token",
	})
	result := runProgram(t, env, "", binaryPath, "--team-env", "MY_TEAM_TOKEN", "team", "list")
	requireSuccess(t, result)
	for _, want := range []string{"team: E2E Team", "member-one", "devices=1"} {
		if !strings.Contains(result.stdout, want) {
			t.Fatalf("team output = %q, want it to contain %q", result.stdout, want)
		}
	}
	requests := h.requests(t)
	if len(requests) != 1 || requests[0].Path != "/1/teams.json" {
		t.Fatalf("team request did not reach the mock teams endpoint")
	}
}

func TestSubscriptionMigrate(t *testing.T) {
	h := newHarness(t, defaultSeed())
	result := h.run(t, "subscription", "migrate", "--subscription", "subscription-e2e", "--user", fixtureUserKey, "--device-name", "E2E Phone", "--sound", "e2e-custom")
	requireSuccess(t, result)
	if !strings.Contains(result.stdout, "subscribed user key:") {
		t.Fatalf("migration output = %q", result.stdout)
	}
	requests := h.requests(t)
	if len(requests) != 1 || requests[0].Path != "/1/subscriptions/migrate.json" {
		t.Fatalf("subscription migrate request did not reach the mock")
	}
	for field, want := range map[string]string{
		"subscription": "subscription-e2e", "user": fixtureUserKey, "device_name": "E2E Phone", "sound": "e2e-custom",
	} {
		requireField(t, requests[0], field, want)
	}
}
