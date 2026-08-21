package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vshuraeff/pushover/internal/launchagent"
)

func init() {
	register(func(root *cobra.Command) {
		for _, command := range root.Commands() {
			if command.Name() == "client" {
				command.AddCommand(newClientInstallDaemonCommand())
				command.AddCommand(newClientUninstallDaemonCommand())
				return
			}
		}
	})
}

func newClientInstallDaemonCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "install-daemon",
		Short: "Install a launchd agent so client listen starts on login",
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

func newClientUninstallDaemonCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall-daemon",
		Short: "Remove the launchd agent for client listen",
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
