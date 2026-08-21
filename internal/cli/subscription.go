package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vshuraeff/pushover/internal/config"
)

func init() {
	register(func(root *cobra.Command) {
		root.AddCommand(newSubscriptionCommand())
	})
}

func newSubscriptionCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "subscription",
		Short: "Manage Pushover subscriptions",
	}
	command.AddCommand(newSubscriptionMigrateCommand())
	return command
}

func newSubscriptionMigrateCommand() *cobra.Command {
	var subscription string
	var user string
	var deviceName string
	var sound string

	command := &cobra.Command{
		Use:          "migrate",
		Short:        "Migrate a subscription",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if subscription == "" {
				return fmt.Errorf("--subscription is required")
			}
			if user == "" {
				return fmt.Errorf("--user is required")
			}
			token, err := resolveCredential(cmd, config.RoleAppToken)
			if err != nil {
				return fmt.Errorf("resolve application token: %w", err)
			}
			subscribedUserKey, err := newAPIClient().MigrateSubscription(commandContext(cmd), token, subscription, user, deviceName, sound)
			if err != nil {
				return fmt.Errorf("migrate subscription: %w", err)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "subscribed user key: %s\n", subscribedUserKey)
			return nil
		},
	}
	command.Flags().StringVar(&subscription, "subscription", "", "subscription identifier to migrate")
	command.Flags().StringVar(&user, "user", "", "user key for the subscription")
	command.Flags().StringVar(&deviceName, "device-name", "", "device name for the migrated subscription")
	command.Flags().StringVar(&sound, "sound", "", "sound for the migrated subscription")
	return command
}
