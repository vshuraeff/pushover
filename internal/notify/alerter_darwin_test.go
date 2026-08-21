//go:build darwin

package notify

import (
	"context"
	"reflect"
	"testing"
)

func TestAlerterNotifierBuildsExpectedArguments(t *testing.T) {
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
		Title:          "title",
		Body:           "body",
		IconPath:       "icon.png",
		AttachmentPath: "attachment.png",
		SoundName:      "Ping",
	}
	notifier := &AlerterNotifier{path: "alerter"}
	if err := notifier.Notify(context.Background(), notification); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}

	if gotName != "alerter" {
		t.Fatalf("command name = %q, want alerter", gotName)
	}
	wantArgs := []string{"-title", "title", "-message", "body", "-appIcon", "icon.png", "-contentImage", "attachment.png", "-sound", "Ping"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("command args = %#v, want %#v", gotArgs, wantArgs)
	}
}

func TestAlerterNotifierSuppressesPassiveSound(t *testing.T) {
	originalRunner := runCommand
	t.Cleanup(func() { runCommand = originalRunner })

	var gotArgs []string
	runCommand = func(_ context.Context, _ string, args ...string) error {
		gotArgs = append([]string(nil), args...)
		return nil
	}
	notifier := &AlerterNotifier{path: "alerter"}
	if err := notifier.Notify(context.Background(), Notification{Priority: -2, SoundName: "Ping"}); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	for _, arg := range gotArgs {
		if arg == "-sound" {
			t.Fatal("passive notification included a sound flag")
		}
	}
}
