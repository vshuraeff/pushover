package cli

import "github.com/spf13/cobra"

type commandRegistrar func(*cobra.Command)

var commandRegistrars []commandRegistrar

func register(registrar commandRegistrar) {
	commandRegistrars = append(commandRegistrars, registrar)
}

func NewRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "povr",
		Short: "Pushover command-line client",
	}

	flags := cmd.PersistentFlags()
	flags.String("key-env", "", "environment variable name for the primary credential")
	flags.String("token-env", "", "environment variable name for the app token")
	flags.String("user-env", "", "environment variable name for the user key")
	flags.String("team-env", "", "environment variable name for the team token")
	flags.String("encryption-key-env", "", "environment variable name for the message encryption key")

	return cmd
}

func Register(cmd *cobra.Command) {
	for _, registrar := range commandRegistrars {
		registrar(cmd)
	}
}
