package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/vshuraeff/pushover/internal/launchagent"
)

func TestDaemonCommandsRegistered(t *testing.T) {
	root := NewRootCommand()
	Register(root)

	for _, args := range [][]string{
		{"daemon", "install"},
		{"daemon", "uninstall"},
		{"daemon", "start"},
		{"daemon", "stop"},
		{"daemon", "restart"},
		{"daemon", "status"},
	} {
		command, remaining, err := root.Find(args)
		if err != nil {
			t.Fatalf("Find(%q) error = %v", args, err)
		}
		if command == nil || command.Name() != args[1] || len(remaining) != 0 {
			t.Fatalf("Find(%q) = command %#v, remaining %#v", args, command, remaining)
		}
	}

	daemon, _, err := root.Find([]string{"daemon"})
	if err != nil {
		t.Fatalf("Find(daemon) error = %v", err)
	}
	var commands []string
	for _, command := range daemon.Commands() {
		commands = append(commands, command.Name())
	}
	if got, want := commands, []string{"install", "restart", "start", "status", "stop", "uninstall"}; !equalDaemonStrings(got, want) {
		t.Errorf("daemon commands = %q, want %q", got, want)
	}
}

func equalDaemonStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func TestDaemonCommandHelp(t *testing.T) {
	for _, args := range [][]string{
		{"daemon", "--help"},
		{"daemon", "install", "--help"},
		{"daemon", "uninstall", "--help"},
		{"daemon", "start", "--help"},
		{"daemon", "stop", "--help"},
		{"daemon", "restart", "--help"},
		{"daemon", "status", "--help"},
	} {
		root, _, _ := clientRoot(args)
		if err := root.Execute(); err != nil {
			t.Errorf("povr %v error = %v", args, err)
		}
	}
}

func TestWriteDaemonStatus(t *testing.T) {
	status := launchagent.StatusResult{
		PlistInstalled:  true,
		Loaded:          true,
		PID:             123,
		LastExitCode:    7,
		HasLastExitCode: true,
		Detail:          "service is loaded",
	}
	for _, test := range []struct {
		name     string
		json     bool
		validate func(*testing.T, string)
	}{
		{
			name: "human",
			validate: func(t *testing.T, output string) {
				for _, want := range []string{"plist installed: true", "service loaded: true", "pid: 123", "last exit code: 7", "detail: service is loaded"} {
					if !strings.Contains(output, want) {
						t.Errorf("output = %q, want %q", output, want)
					}
				}
			},
		},
		{
			name: "json",
			json: true,
			validate: func(t *testing.T, output string) {
				var result launchagent.StatusResult
				if err := json.Unmarshal([]byte(output), &result); err != nil {
					t.Fatalf("Unmarshal() error = %v", err)
				}
				if result != status {
					t.Errorf("JSON result = %#v, want %#v", result, status)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := &cobra.Command{}
			command.Flags().Bool("json", test.json, "print JSON output")
			var output bytes.Buffer
			command.SetOut(&output)
			if err := writeDaemonStatus(command, status); err != nil {
				t.Fatalf("writeDaemonStatus() error = %v", err)
			}
			test.validate(t, output.String())
		})
	}
}
