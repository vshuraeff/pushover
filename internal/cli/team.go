package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vshuraeff/pushover/internal/api"
	"github.com/vshuraeff/pushover/internal/config"
)

func init() {
	register(func(root *cobra.Command) {
		root.AddCommand(newTeamCommand())
	})
}

func newTeamCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "team",
		Short: "Manage a Pushover team",
	}
	command.AddCommand(
		newTeamListCommand(),
		newTeamAddUserCommand(),
		newTeamRevokeInvitationCommand(),
		newTeamRemoveUserCommand(),
	)
	return command
}

func newTeamListCommand() *cobra.Command {
	return &cobra.Command{
		Use:          "list",
		Short:        "List team members",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			token, err := resolveCredential(cmd, config.RoleTeamToken)
			if err != nil {
				return fmt.Errorf("resolve team token: %w", err)
			}
			name, users, err := newAPIClient().ListTeam(commandContext(cmd), token)
			if err != nil {
				return fmt.Errorf("list team: %w", err)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "team: %s\n", name)
			for _, user := range users {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\tadministrator=%t\tdevices=%d\n", user.ID, user.Name, user.Email, user.Administrator, len(user.Devices))
			}
			return nil
		},
	}
}

func newTeamAddUserCommand() *cobra.Command {
	var email string
	var name string
	var password string
	var instant bool
	var administrator bool
	var group string

	command := &cobra.Command{
		Use:          "add-user",
		Short:        "Invite a user to the team",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if email == "" {
				return fmt.Errorf("--email is required")
			}
			token, err := resolveCredential(cmd, config.RoleTeamToken)
			if err != nil {
				return fmt.Errorf("resolve team token: %w", err)
			}
			err = newAPIClient().AddTeamUser(commandContext(cmd), token, email, api.TeamUserOptions{
				Name:          name,
				Password:      password,
				Instant:       instant,
				Administrator: administrator,
				Group:         group,
			})
			if err != nil {
				return fmt.Errorf("add team user: %w", err)
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "team user invitation sent")
			return nil
		},
	}
	command.Flags().StringVar(&email, "email", "", "email address to invite")
	command.Flags().StringVar(&name, "name", "", "name for the invited user")
	command.Flags().StringVar(&password, "password", "", "password for the invited user")
	command.Flags().BoolVar(&instant, "instant", false, "send the invitation immediately")
	command.Flags().BoolVar(&administrator, "admin", false, "make the invited user an administrator")
	command.Flags().StringVar(&group, "group", "", "group key to assign")
	return command
}

func newTeamRevokeInvitationCommand() *cobra.Command {
	var email string

	command := &cobra.Command{
		Use:          "revoke-invitation",
		Short:        "Revoke a team invitation",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if email == "" {
				return fmt.Errorf("--email is required")
			}
			token, err := resolveCredential(cmd, config.RoleTeamToken)
			if err != nil {
				return fmt.Errorf("resolve team token: %w", err)
			}
			if err := newAPIClient().RevokeTeamInvitation(commandContext(cmd), token, email); err != nil {
				return fmt.Errorf("revoke team invitation: %w", err)
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "team invitation revoked")
			return nil
		},
	}
	command.Flags().StringVar(&email, "email", "", "email address of the invitation")
	return command
}

func newTeamRemoveUserCommand() *cobra.Command {
	var email string

	command := &cobra.Command{
		Use:          "remove-user",
		Short:        "Remove a user from the team",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if email == "" {
				return fmt.Errorf("--email is required")
			}
			token, err := resolveCredential(cmd, config.RoleTeamToken)
			if err != nil {
				return fmt.Errorf("resolve team token: %w", err)
			}
			if err := newAPIClient().RemoveTeamUser(commandContext(cmd), token, email); err != nil {
				return fmt.Errorf("remove team user: %w", err)
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "team user removed")
			return nil
		},
	}
	command.Flags().StringVar(&email, "email", "", "email address of the team user")
	return command
}
