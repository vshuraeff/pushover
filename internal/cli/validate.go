package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vshuraeff/pushover/internal/api"
	"github.com/vshuraeff/pushover/internal/config"
)

func init() {
	register(func(root *cobra.Command) {
		command := &cobra.Command{
			Use:   "validate",
			Short: "Validate a user or group key",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return runValidate(cmd)
			},
		}
		command.Flags().String("user", "", "user or group key to validate")
		command.Flags().String("device", "", "device name to validate")
		root.AddCommand(command)
	})
}

func runValidate(cmd *cobra.Command) error {
	user, err := cmd.Flags().GetString("user")
	if err != nil {
		return fmt.Errorf("read --user flag: %w", err)
	}
	if user == "" {
		return fmt.Errorf("--user is required")
	}
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

	service := api.NewUsersService(api.New(config.APIURL(), Version))
	response, err := service.Validate(context.Background(), api.ValidateRequest{
		Token:  token,
		User:   user,
		Device: device,
	})
	if err != nil {
		var quotaError *api.QuotaError
		if errors.As(err, &quotaError) {
			return err
		}
		var apiError *api.APIError
		if !errors.As(err, &apiError) || apiError.Status != 0 {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "invalid: %v\n", err)
		writeValidateDetails(cmd, response)
		return nil
	}

	if response.Status != 1 {
		fmt.Fprintln(cmd.OutOrStdout(), "invalid")
		writeValidateDetails(cmd, response)
		return nil
	}
	fmt.Fprintln(cmd.OutOrStdout(), "valid")
	writeValidateDetails(cmd, response)
	return nil
}

func writeValidateDetails(cmd *cobra.Command, response *api.ValidateResponse) {
	if response == nil {
		return
	}
	if len(response.Devices) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "devices: %s\n", strings.Join(response.Devices, ", "))
	}
	if len(response.Licenses) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "licenses: %s\n", strings.Join(response.Licenses, ", "))
	}
}
