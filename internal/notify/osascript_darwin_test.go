//go:build darwin

package notify

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestAppleScriptNotifierPassesValuesAsArguments(t *testing.T) {
	originalRunner := runCommand
	t.Cleanup(func() { runCommand = originalRunner })

	var gotName string
	var gotArgs []string
	runCommand = func(_ context.Context, name string, args ...string) error {
		gotName = name
		gotArgs = append([]string(nil), args...)
		return nil
	}

	notification := Notification{
		Title:     `title " with quote`,
		Body:      "body with \n newline",
		SoundName: "Ping",
	}
	if err := NewAppleScriptNotifier().Notify(context.Background(), notification); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}

	if gotName != "osascript" {
		t.Fatalf("command name = %q, want osascript", gotName)
	}
	wantArgs := []string{"-e", notificationAppleScript, "--", notification.Title, notification.Body, notification.SoundName}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("command args = %#v, want %#v", gotArgs, wantArgs)
	}
	if strings.Contains(gotArgs[1], notification.Title) || strings.Contains(gotArgs[1], notification.Body) {
		t.Fatal("notification values were interpolated into the AppleScript")
	}
}

func TestAppleScriptNotifierSuppressesPassiveSound(t *testing.T) {
	originalRunner := runCommand
	t.Cleanup(func() { runCommand = originalRunner })

	var gotArgs []string
	runCommand = func(_ context.Context, _ string, args ...string) error {
		gotArgs = append([]string(nil), args...)
		return nil
	}
	if err := NewAppleScriptNotifier().Notify(context.Background(), Notification{Priority: -1, SoundName: "Ping"}); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if got := gotArgs[len(gotArgs)-1]; got != "" {
		t.Fatalf("sound argument = %q, want empty", got)
	}
}
