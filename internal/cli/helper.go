package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/vshuraeff/pushover/internal/helperapp"
)

// helperbundlepath is the helper bundle location set at build time.
var HelperBundlePath string

func init() {
	register(func(root *cobra.Command) {
		root.AddCommand(newHelperCommand())
	})
}

func newHelperCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "helper",
		Short: "Install and inspect the Povr notification helper",
	}
	cmd.AddCommand(newHelperInstallCommand())
	cmd.AddCommand(newHelperStatusCommand())
	return cmd
}

func newHelperInstallCommand() *cobra.Command {
	var source string
	var dest string
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install the Povr notification helper",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			resolvedSource := source
			if !cmd.Flags().Changed("from") && resolvedSource == "" {
				resolvedSource = HelperBundlePath
			}
			if resolvedSource == "" {
				return fmt.Errorf("helper: source bundle is required: set --from or use a build with a baked-in helper bundle path")
			}
			if err := helperapp.Install(helperapp.InstallOptions{Source: resolvedSource, Dest: dest}); err != nil {
				return err
			}
			installedPath, err := helperInstallPath(dest)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "installed Povr.app at %s\n", installedPath)
			return nil
		},
	}
	cmd.Flags().StringVar(&source, "from", "", "source Povr.app bundle")
	cmd.Flags().StringVar(&dest, "dest", "", "destination Povr.app bundle")
	if err := cmd.Flags().MarkHidden("dest"); err != nil {
		panic(err)
	}
	return cmd
}

func newHelperStatusCommand() *cobra.Command {
	var dest string
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show Povr notification helper status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := helperapp.Status(dest)
			if err != nil {
				return err
			}
			if jsonOutput {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "installed: %t\nbundle path: %s\nsignature valid: %t\nhas entitlements: %t\n", result.Installed, result.BundlePath, result.SignatureValid, result.HasEntitlements)
			if result.Detail != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "detail: %s\n", result.Detail)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dest, "dest", "", "destination Povr.app bundle")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "print JSON output")
	if err := cmd.Flags().MarkHidden("dest"); err != nil {
		panic(err)
	}
	return cmd
}

func helperInstallPath(dest string) (string, error) {
	if dest != "" {
		return dest, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("helper: locate home directory: %w", err)
	}
	return filepath.Join(home, "Library", "Application Support", "povr", "Povr.app"), nil
}
