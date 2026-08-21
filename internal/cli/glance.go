package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vshuraeff/pushover/internal/api"
	"github.com/vshuraeff/pushover/internal/config"
)

func init() {
	register(func(root *cobra.Command) {
		command := &cobra.Command{
			Use:   "glance",
			Short: "Send a Glance update",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return runGlance(cmd)
			},
		}
		command.Flags().String("user", "", "user or group key")
		command.Flags().String("device", "", "device name")
		command.Flags().String("title", "", "glance title")
		command.Flags().String("text", "", "glance text")
		command.Flags().String("subtext", "", "glance subtext")
		command.Flags().Int("count", 0, "glance count")
		command.Flags().Int("percent", 0, "glance percent")
		root.AddCommand(command)
	})
}

func runGlance(cmd *cobra.Command) error {
	device, err := cmd.Flags().GetString("device")
	if err != nil {
		return fmt.Errorf("read --device flag: %w", err)
	}
	envFlags, err := EnvFlagsFromCommand(cmd)
	if err != nil {
		return err
	}
	token, err := config.Resolve(config.RoleAppToken, config.RoleAppToken, envFlags)
	if err != nil {
		return fmt.Errorf("resolve app token: %w", err)
	}
	user, err := cmd.Flags().GetString("user")
	if err != nil {
		return fmt.Errorf("read --user flag: %w", err)
	}
	if !cmd.Flags().Changed("user") {
		user, err = config.Resolve(config.RoleUserKey, config.RoleAppToken, envFlags)
		if err != nil {
			return err
		}
	}
	if user == "" {
		return fmt.Errorf("--user is required")
	}

	request := api.GlanceRequest{
		Token:  token,
		User:   user,
		Device: device,
	}
	if request.Title, err = changedStringFlag(cmd, "title"); err != nil {
		return err
	}
	if request.Text, err = changedStringFlag(cmd, "text"); err != nil {
		return err
	}
	if request.Subtext, err = changedStringFlag(cmd, "subtext"); err != nil {
		return err
	}
	if request.Count, err = changedIntFlag(cmd, "count"); err != nil {
		return err
	}
	if request.Percent, err = changedIntFlag(cmd, "percent"); err != nil {
		return err
	}

	service := api.NewGlancesService(api.New(config.APIURL(), Version))
	if _, err := service.Send(context.Background(), request); err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), "glance sent")
	return nil
}

func changedStringFlag(cmd *cobra.Command, name string) (*string, error) {
	if !cmd.Flags().Changed(name) {
		return nil, nil
	}
	value, err := cmd.Flags().GetString(name)
	if err != nil {
		return nil, fmt.Errorf("read --%s flag: %w", name, err)
	}
	return &value, nil
}

func changedIntFlag(cmd *cobra.Command, name string) (*int, error) {
	if !cmd.Flags().Changed(name) {
		return nil, nil
	}
	value, err := cmd.Flags().GetInt(name)
	if err != nil {
		return nil, fmt.Errorf("read --%s flag: %w", name, err)
	}
	return &value, nil
}
