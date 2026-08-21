package cli

import "testing"

func TestClientDaemonCommandsRegistered(t *testing.T) {
	root := NewRootCommand()
	Register(root)
	for _, args := range [][]string{{"client", "install-daemon"}, {"client", "uninstall-daemon"}} {
		command, remaining, err := root.Find(args)
		if err != nil {
			t.Fatalf("Find(%q) error = %v", args, err)
		}
		if command == nil || command.Name() != args[1] || len(remaining) != 0 {
			t.Fatalf("Find(%q) = command %#v, remaining %#v", args, command, remaining)
		}
	}
}

func TestClientDaemonCommandHelp(t *testing.T) {
	for _, args := range [][]string{{"client", "install-daemon", "--help"}, {"client", "uninstall-daemon", "--help"}} {
		root, _, _ := clientRoot(args)
		if err := root.Execute(); err != nil {
			t.Errorf("povr %v error = %v", args, err)
		}
	}
}
