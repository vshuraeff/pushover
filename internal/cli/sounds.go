package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/vshuraeff/pushover/internal/api"
	"github.com/vshuraeff/pushover/internal/config"
)

func init() {
	register(func(root *cobra.Command) {
		command := &cobra.Command{
			Use:   "sounds",
			Short: "List available sounds",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return runSounds(cmd)
			},
		}
		command.Flags().Bool("json", false, "print sounds as JSON")
		root.AddCommand(command)
	})
}

func runSounds(cmd *cobra.Command) error {
	jsonOutput, err := cmd.Flags().GetBool("json")
	if err != nil {
		return fmt.Errorf("read --json flag: %w", err)
	}
	envFlags, err := EnvFlagsFromCommand(cmd)
	if err != nil {
		return err
	}
	token, err := config.Resolve(config.RoleAppToken, config.RoleAppToken, envFlags)
	if err != nil {
		return fmt.Errorf("resolve app token: %w", err)
	}

	service := api.NewSoundsService(api.New(config.APIURL(), Version))
	response, err := service.List(context.Background(), token)
	if err != nil {
		return err
	}
	if jsonOutput {
		encoded, err := json.Marshal(response.Sounds)
		if err != nil {
			return fmt.Errorf("encode sounds: %w", err)
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(encoded))
		return nil
	}

	names := make([]string, 0, len(response.Sounds))
	for name := range response.Sounds {
		names = append(names, name)
	}
	sort.Strings(names)

	writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 8, 2, ' ', 0)
	for _, name := range names {
		fmt.Fprintf(writer, "%s:\t%s\n", name, response.Sounds[name])
	}
	return writer.Flush()
}
