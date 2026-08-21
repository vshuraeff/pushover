package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var Version = "dev"

func init() {
	register(func(root *cobra.Command) {
		root.AddCommand(&cobra.Command{
			Use:   "version",
			Short: "Print the povr version",
			Run: func(cmd *cobra.Command, args []string) {
				fmt.Fprintf(cmd.OutOrStdout(), "povr version %s\n", Version)
			},
		})
	})
}
