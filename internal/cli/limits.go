package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vshuraeff/pushover/internal/api"
	"github.com/vshuraeff/pushover/internal/config"
)

func init() {
	register(func(root *cobra.Command) {
		root.AddCommand(newLimitsCommand())
	})
}

func newLimitsCommand() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "limits",
		Short: "Show application message limits",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			envFlags, err := EnvFlagsFromCommand(cmd)
			if err != nil {
				return err
			}
			token, err := config.Resolve(config.RoleAppToken, config.RoleAppToken, envFlags)
			if err != nil {
				return err
			}
			limits, err := api.New(config.APIURL(), Version).AppLimits(cmd.Context(), token)
			if err != nil {
				return err
			}
			if jsonOutput {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(limits)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "limit: %d\nremaining: %d\nreset: %d\nheader_limit: %s\nheader_remaining: %s\nheader_reset: %s\n", limits.Limit, limits.Remaining, limits.Reset, limits.Header.Limit, limits.Header.Remaining, limits.Header.Reset)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "print JSON output")
	return cmd
}
