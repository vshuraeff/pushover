package cli

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/vshuraeff/pushover/internal/config"
	"github.com/vshuraeff/pushover/internal/listen"
	"github.com/vshuraeff/pushover/internal/notify"
)

func init() {
	register(func(root *cobra.Command) {
		for _, command := range root.Commands() {
			if command.Name() == "client" {
				command.AddCommand(newClientListenCommand())
				return
			}
		}
	})
}

func newClientListenCommand() *cobra.Command {
	var emergencyRealertSeconds int
	command := &cobra.Command{
		Use:   "listen",
		Short: "Listen for Open Client messages",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if emergencyRealertSeconds < 30 {
				return fmt.Errorf("--emergency-realert must be at least 30 seconds")
			}
			store, err := config.NewFileSessionStore()
			if err != nil {
				return fmt.Errorf("create session store: %w", err)
			}
			notifier, err := notify.Select(cmd.Context())
			if err != nil {
				return err
			}
			logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
			return listen.Run(cmd.Context(), &listen.Daemon{
				Store:            store,
				APIClient:        newOpenClient(),
				Notifier:         notifier,
				Logger:           logger,
				WSURL:            config.WSURL(),
				EmergencyRealert: time.Duration(emergencyRealertSeconds) * time.Second,
			})
		},
	}
	command.Flags().IntVar(&emergencyRealertSeconds, "emergency-realert", 60, "seconds between unacknowledged emergency alerts")
	return command
}
