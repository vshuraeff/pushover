package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vshuraeff/pushover/internal/api"
	"github.com/vshuraeff/pushover/internal/config"
)

func init() {
	register(func(root *cobra.Command) {
		root.AddCommand(newLicenseCommand())
	})
}

func newLicenseCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "license",
		Short: "Manage Pushover licenses",
	}
	command.AddCommand(newLicenseCreditsCommand(), newLicenseAssignCommand())
	return command
}

func newLicenseCreditsCommand() *cobra.Command {
	return &cobra.Command{
		Use:          "credits",
		Short:        "Show remaining license credits",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			token, err := resolveCredential(cmd, config.RoleAppToken)
			if err != nil {
				return fmt.Errorf("resolve application token: %w", err)
			}
			credits, err := newAPIClient().LicenseCredits(commandContext(cmd), token)
			if err != nil {
				return fmt.Errorf("get license credits: %w", err)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "remaining license credits: %d\n", credits)
			return nil
		},
	}
}

func newLicenseAssignCommand() *cobra.Command {
	var user string
	var email string
	var operatingSystem string
	var yes bool

	command := &cobra.Command{
		Use:          "assign",
		Short:        "Assign an irreversible license",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if (user == "") == (email == "") {
				return errors.New("exactly one of --user or --email is required")
			}

			token, err := resolveCredential(cmd, config.RoleAppToken)
			if err != nil {
				return fmt.Errorf("resolve application token: %w", err)
			}

			client := newAPIClient()
			if !yes {
				if err := requireTerminalStdin(); err != nil {
					return err
				}
				credits, err := client.LicenseCredits(commandContext(cmd), token)
				if err != nil {
					return fmt.Errorf("get license credits: %w", err)
				}
				if err := confirmLicenseCredits(cmd, credits, user, email); err != nil {
					return err
				}
			}

			credits, err := client.AssignLicense(commandContext(cmd), token, user, email, operatingSystem)
			if err != nil {
				return fmt.Errorf("assign license: %w", err)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "remaining license credits: %d\n", credits)
			return nil
		},
	}
	command.Flags().StringVar(&user, "user", "", "user key to assign the license to")
	command.Flags().StringVar(&email, "email", "", "email address to assign the license to")
	command.Flags().StringVar(&operatingSystem, "os", "", "target operating system: Android, iOS, or Desktop")
	command.Flags().BoolVar(&yes, "yes", false, "assign without confirmation")
	return command
}

func requireTerminalStdin() error {
	info, err := os.Stdin.Stat()
	if err != nil {
		return fmt.Errorf("inspect stdin: %w", err)
	}
	if info.Mode()&os.ModeCharDevice == 0 {
		return errors.New("license assignment requires --yes when stdin is not a terminal")
	}
	return nil
}

func confirmLicenseCredits(cmd *cobra.Command, credits int, user, email string) error {
	target := fmt.Sprintf("user %q", user)
	if email != "" {
		target = fmt.Sprintf("email %q", email)
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "remaining license credits: %d\n", credits)
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Assigning a license to %s spends one credit and is irreversible. Continue? [y/N]: ", target)

	answer, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if err != nil && strings.TrimSpace(answer) == "" {
		return fmt.Errorf("read license assignment confirmation: %w", err)
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return nil
	default:
		return errors.New("license assignment aborted")
	}
}

func newAPIClient() *api.Client {
	return api.New(config.APIURL(), Version)
}

func resolveCredential(cmd *cobra.Command, role config.Role) (string, error) {
	flags, err := EnvFlagsFromCommand(cmd)
	if err != nil {
		return "", err
	}
	return config.Resolve(role, role, flags)
}

func commandContext(cmd *cobra.Command) context.Context {
	if ctx := cmd.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}
