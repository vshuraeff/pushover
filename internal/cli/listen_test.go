package cli

import (
	"strings"
	"testing"
)

func TestClientListenCommandRegistered(t *testing.T) {
	root := NewRootCommand()
	Register(root)
	command, remaining, err := root.Find([]string{"client", "listen"})
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if command == nil || command.Name() != "listen" || len(remaining) != 0 {
		t.Fatalf("command = %#v, remaining = %#v", command, remaining)
	}
}

func TestClientListenRejectsShortEmergencyInterval(t *testing.T) {
	root, _, _ := clientRoot([]string{"client", "listen", "--emergency-realert", "29"})
	err := root.Execute()
	if err == nil || err.Error() != "--emergency-realert must be at least 30 seconds" {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestClientListenFailsClosedWhenNotifierSelectionFails(t *testing.T) {
	t.Setenv("PUSHOVER_CONFIG_DIR", t.TempDir())
	t.Setenv("PUSHOVER_NOTIFY_BACKEND", "definitely-not-a-backend")
	root, _, _ := clientRoot([]string{"client", "listen"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown backend") {
		t.Fatalf("Execute() error = %v", err)
	}
}
