package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vshuraeff/pushover/internal/config"
)

func EnvFlagsFromCommand(cmd *cobra.Command) (config.EnvFlags, error) {
	keyEnv, err := cmd.Flags().GetString("key-env")
	if err != nil {
		return config.EnvFlags{}, fmt.Errorf("read --key-env flag: %w", err)
	}
	tokenEnv, err := cmd.Flags().GetString("token-env")
	if err != nil {
		return config.EnvFlags{}, fmt.Errorf("read --token-env flag: %w", err)
	}
	userEnv, err := cmd.Flags().GetString("user-env")
	if err != nil {
		return config.EnvFlags{}, fmt.Errorf("read --user-env flag: %w", err)
	}
	teamEnv, err := cmd.Flags().GetString("team-env")
	if err != nil {
		return config.EnvFlags{}, fmt.Errorf("read --team-env flag: %w", err)
	}
	encryptionKeyEnv, err := cmd.Flags().GetString("encryption-key-env")
	if err != nil {
		return config.EnvFlags{}, fmt.Errorf("read --encryption-key-env flag: %w", err)
	}

	return config.EnvFlags{
		KeyEnv:           keyEnv,
		TokenEnv:         tokenEnv,
		UserEnv:          userEnv,
		TeamEnv:          teamEnv,
		EncryptionKeyEnv: encryptionKeyEnv,
	}, nil
}
