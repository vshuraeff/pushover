package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vshuraeff/pushover/internal/launchagent"
)

func init() {
	register(func(root *cobra.Command) {
		root.AddCommand(newDaemonCommand())
	})
}

func newDaemonCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "daemon",
		Short: "Manage the launchd agent for client listen",
	}
	command.AddCommand(
		newDaemonInstallCommand(),
		newDaemonUninstallCommand(),
		newDaemonStartCommand(),
		newDaemonStopCommand(),
		newDaemonRestartCommand(),
		newDaemonStatusCommand(),
	)
	return command
}

func newDaemonInstallCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Install and load the launchd agent for client listen",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := launchagent.Install(launchagent.InstallOptions{}); err != nil {
				return err
			}
			_, err := fmt.Fprintln(cmd.OutOrStdout(), "installed launchd agent for povr client listen")
			return err
		},
	}
}

func newDaemonUninstallCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Unload and remove the launchd agent for client listen",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := launchagent.Uninstall(launchagent.UninstallOptions{}); err != nil {
				return err
			}
			_, err := fmt.Fprintln(cmd.OutOrStdout(), "removed launchd agent for povr client listen")
			return err
		},
	}
}

func newDaemonStartCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Load the installed launchd agent",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := launchagent.Start(launchagent.StartOptions{}); err != nil {
				return err
			}
			_, err := fmt.Fprintln(cmd.OutOrStdout(), "started launchd agent for povr client listen")
			return err
		},
	}
}

func newDaemonStopCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Unload the launchd agent",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := launchagent.Stop(launchagent.StopOptions{}); err != nil {
				return err
			}
			_, err := fmt.Fprintln(cmd.OutOrStdout(), "stopped launchd agent for povr client listen")
			return err
		},
	}
}

func newDaemonRestartCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "restart",
		Short: "Restart the launchd agent",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := launchagent.Restart(launchagent.RestartOptions{}); err != nil {
				return err
			}
			_, err := fmt.Fprintln(cmd.OutOrStdout(), "restarted launchd agent for povr client listen")
			return err
		},
	}
}

func newDaemonStatusCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "status",
		Short: "Show launchd agent status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			status, err := launchagent.Status(launchagent.StatusOptions{})
			if err != nil {
				return err
			}
			return writeDaemonStatus(cmd, status)
		},
	}
	command.Flags().Bool("json", false, "print JSON output")
	return command
}

func writeDaemonStatus(cmd *cobra.Command, status launchagent.StatusResult) error {
	jsonOutput, err := cmd.Flags().GetBool("json")
	if err != nil {
		return fmt.Errorf("read --json: %w", err)
	}
	if jsonOutput {
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(status); err != nil {
			return fmt.Errorf("write JSON output: %w", err)
		}
		return nil
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "plist installed: %t\nservice loaded: %t\n", status.PlistInstalled, status.Loaded); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	if status.PID != 0 {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "pid: %d\n", status.PID); err != nil {
			return fmt.Errorf("write output: %w", err)
		}
	}
	if status.HasLastExitCode {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "last exit code: %d\n", status.LastExitCode); err != nil {
			return fmt.Errorf("write output: %w", err)
		}
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "detail: %s\n", status.Detail); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}
