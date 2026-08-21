package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vshuraeff/pushover/internal/api"
	"github.com/vshuraeff/pushover/internal/config"
)

func init() {
	register(func(root *cobra.Command) {
		root.AddCommand(newGroupCommand())
	})
}

func newGroupCommand() *cobra.Command {
	group := &cobra.Command{
		Use:   "group",
		Short: "Manage Pushover groups",
	}
	group.PersistentFlags().Bool("json", false, "print JSON output")
	group.AddCommand(
		newGroupCreateCommand(),
		newGroupListCommand(),
		newGroupShowCommand(),
		newGroupAddUserCommand(),
		newGroupRemoveUserCommand(),
		newGroupDisableUserCommand(),
		newGroupEnableUserCommand(),
		newGroupRenameCommand(),
	)
	return group
}

func newGroupCreateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "create <name>",
		Short: "Create a group",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, appToken, err := groupClient(cmd)
			if err != nil {
				return err
			}
			groupKey, err := client.CreateGroup(groupContext(cmd), appToken, args[0])
			if err != nil {
				return err
			}
			return writeGroupOutput(cmd, struct {
				Group string `json:"group"`
			}{Group: groupKey}, groupKey+"\n")
		},
	}
}

func newGroupListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List groups",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, appToken, err := groupClient(cmd)
			if err != nil {
				return err
			}
			groups, err := client.ListGroups(groupContext(cmd), appToken)
			if err != nil {
				return err
			}
			var output strings.Builder
			for _, group := range groups {
				fmt.Fprintf(&output, "%s  %s\n", group.Key, group.Name)
			}
			return writeGroupOutput(cmd, groups, output.String())
		},
	}
}

func newGroupShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show <group-key>",
		Short: "Show a group and its users",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, appToken, err := groupClient(cmd)
			if err != nil {
				return err
			}
			detail, err := client.GetGroup(groupContext(cmd), appToken, args[0])
			if err != nil {
				return err
			}
			var output strings.Builder
			fmt.Fprintf(&output, "name: %s\n", detail.Name)
			for _, user := range detail.Users {
				fmt.Fprintf(&output, "user: %s  device: %s  memo: %s  disabled: %t\n", user.User, user.Device, user.Memo, user.Disabled)
			}
			return writeGroupOutput(cmd, detail, output.String())
		},
	}
}

func newGroupAddUserCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "add-user <group-key> <user-key>",
		Short: "Add a user to a group",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			device, err := groupStringFlag(cmd, "device")
			if err != nil {
				return err
			}
			memo, err := groupStringFlag(cmd, "memo")
			if err != nil {
				return err
			}
			client, appToken, err := groupClient(cmd)
			if err != nil {
				return err
			}
			if err := client.AddGroupUser(groupContext(cmd), appToken, args[0], args[1], device, memo); err != nil {
				return err
			}
			return writeGroupConfirmation(cmd, "added user "+args[1]+" to group "+args[0])
		},
	}
	command.Flags().String("device", "", "device name")
	command.Flags().String("memo", "", "membership memo")
	return command
}

func newGroupRemoveUserCommand() *cobra.Command {
	command := newGroupUserCommand("remove-user <group-key> <user-key>", "Remove a user from a group", func(client *api.Client, ctx context.Context, appToken, groupKey, user, device string) error {
		return client.RemoveGroupUser(ctx, appToken, groupKey, user, device)
	}, "removed")
	return command
}

func newGroupDisableUserCommand() *cobra.Command {
	command := newGroupUserCommand("disable-user <group-key> <user-key>", "Disable a user's group membership", func(client *api.Client, ctx context.Context, appToken, groupKey, user, device string) error {
		return client.DisableGroupUser(ctx, appToken, groupKey, user, device)
	}, "disabled")
	return command
}

func newGroupEnableUserCommand() *cobra.Command {
	command := newGroupUserCommand("enable-user <group-key> <user-key>", "Enable a user's group membership", func(client *api.Client, ctx context.Context, appToken, groupKey, user, device string) error {
		return client.EnableGroupUser(ctx, appToken, groupKey, user, device)
	}, "enabled")
	return command
}

func newGroupUserCommand(use, short string, action func(*api.Client, context.Context, string, string, string, string) error, confirmation string) *cobra.Command {
	command := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			device, err := groupStringFlag(cmd, "device")
			if err != nil {
				return err
			}
			client, appToken, err := groupClient(cmd)
			if err != nil {
				return err
			}
			if err := action(client, groupContext(cmd), appToken, args[0], args[1], device); err != nil {
				return err
			}
			return writeGroupConfirmation(cmd, confirmation+" user "+args[1]+" in group "+args[0])
		},
	}
	command.Flags().String("device", "", "device name")
	return command
}

func newGroupRenameCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "rename <group-key> <new-name>",
		Short: "Rename a group",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, appToken, err := groupClient(cmd)
			if err != nil {
				return err
			}
			if err := client.RenameGroup(groupContext(cmd), appToken, args[0], args[1]); err != nil {
				return err
			}
			return writeGroupConfirmation(cmd, "renamed group "+args[0])
		},
	}
}

func groupClient(cmd *cobra.Command) (*api.Client, string, error) {
	flags, err := EnvFlagsFromCommand(cmd)
	if err != nil {
		return nil, "", err
	}
	appToken, err := config.Resolve(config.RoleAppToken, config.RoleAppToken, flags)
	if err != nil {
		return nil, "", err
	}
	baseURL := strings.TrimRight(config.APIURL(), "/") + "/1"
	return api.New(baseURL, Version), appToken, nil
}

func groupContext(cmd *cobra.Command) context.Context {
	if ctx := cmd.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}

func groupStringFlag(cmd *cobra.Command, name string) (string, error) {
	value, err := cmd.Flags().GetString(name)
	if err != nil {
		return "", fmt.Errorf("read --%s: %w", name, err)
	}
	return value, nil
}

func writeGroupConfirmation(cmd *cobra.Command, message string) error {
	return writeGroupOutput(cmd, struct {
		Status string `json:"status"`
	}{Status: "ok"}, message+"\n")
}

func writeGroupOutput(cmd *cobra.Command, value any, human string) error {
	jsonOutput, err := cmd.Flags().GetBool("json")
	if err != nil {
		return fmt.Errorf("read --json: %w", err)
	}
	if jsonOutput {
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(value); err != nil {
			return fmt.Errorf("write JSON output: %w", err)
		}
		return nil
	}
	if _, err := fmt.Fprint(cmd.OutOrStdout(), human); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}
