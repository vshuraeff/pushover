package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/vshuraeff/pushover/internal/api"
	"github.com/vshuraeff/pushover/internal/config"
	"github.com/vshuraeff/pushover/internal/openclient"
)

func init() {
	register(func(root *cobra.Command) {
		root.AddCommand(newClientCommand())
	})
}

func newClientCommand() *cobra.Command {
	clientCommand := &cobra.Command{
		Use:   "client",
		Short: "Manage a Pushover Open Client session",
	}
	clientCommand.AddCommand(newClientLoginCommand())
	clientCommand.AddCommand(newClientRegisterCommand())
	clientCommand.AddCommand(newClientFetchCommand())
	clientCommand.AddCommand(newClientAckCommand())
	return clientCommand
}

func newClientLoginCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Log in to a Pushover Open Client account",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := config.NewSessionStore()
			if err != nil {
				return fmt.Errorf("create session store: %w", err)
			}
			return openclient.Login(cmd.Context(), newOpenClient(), store, openclient.LoginOptions{
				Input:  cmd.InOrStdin(),
				Output: cmd.OutOrStdout(),
			})
		},
	}
}

func newClientRegisterCommand() *cobra.Command {
	var name string
	command := &cobra.Command{
		Use:   "register --name <name>",
		Short: "Register an Open Client device",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := config.NewSessionStore()
			if err != nil {
				return fmt.Errorf("create session store: %w", err)
			}
			return openclient.Register(cmd.Context(), newOpenClient(), store, name)
		},
	}
	command.Flags().StringVar(&name, "name", "", "device name")
	_ = command.MarkFlagRequired("name")
	return command
}

func newClientFetchCommand() *cobra.Command {
	var jsonOutput bool
	command := &cobra.Command{
		Use:   "fetch",
		Short: "Fetch queued Open Client messages",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runClientFetch(cmd.Context(), cmd, jsonOutput)
		},
	}
	command.Flags().BoolVar(&jsonOutput, "json", false, "write messages as JSON")
	return command
}

func newClientAckCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "ack <receipt>",
		Short: "Acknowledge an Open Client receipt",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := config.NewSessionStore()
			if err != nil {
				return fmt.Errorf("create session store: %w", err)
			}
			state, err := openclient.LoadSession(store, false)
			if err != nil {
				return err
			}
			return openclient.Acknowledge(cmd.Context(), newOpenClient(), state.Secret, args[0])
		},
	}
}

func runClientFetch(ctx context.Context, cmd *cobra.Command, jsonOutput bool) error {
	store, err := config.NewSessionStore()
	if err != nil {
		return fmt.Errorf("create session store: %w", err)
	}
	state, err := openclient.LoadSession(store, true)
	if err != nil {
		return err
	}
	messages, err := openclient.Fetch(ctx, newOpenClient(), state.Secret, state.DeviceID)
	if err != nil {
		return err
	}
	if err := writeFetchedMessages(cmd, messages, jsonOutput); err != nil {
		return err
	}
	if len(messages) == 0 {
		return nil
	}

	highest := messages[0].ID
	for _, message := range messages[1:] {
		if message.ID > highest {
			highest = message.ID
		}
	}
	return openclient.UpdateHighest(ctx, newOpenClient(), state.Secret, state.DeviceID, highest)
}

func writeFetchedMessages(cmd *cobra.Command, messages []openclient.Message, jsonOutput bool) error {
	output := cmd.OutOrStdout()
	if jsonOutput {
		encoded, err := json.Marshal(messages)
		if err != nil {
			return fmt.Errorf("encode messages: %w", err)
		}
		if _, err := fmt.Fprintln(output, string(encoded)); err != nil {
			return fmt.Errorf("write messages: %w", err)
		}
		return nil
	}

	writer := tabwriter.NewWriter(output, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "ID\tDATE\tPRIORITY\tAPP\tTITLE\tMESSAGE"); err != nil {
		return fmt.Errorf("write message table header: %w", err)
	}
	for _, message := range messages {
		date := time.Unix(message.Date, 0).Format(time.RFC3339)
		if _, err := fmt.Fprintf(writer, "%d\t%s\t%d\t%s\t%s\t%s\n", message.ID, date, message.Priority, message.App, sanitizeTableField(message.Title), sanitizeTableField(message.Message)); err != nil {
			return fmt.Errorf("write message table: %w", err)
		}
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("flush message table: %w", err)
	}
	return nil
}

func sanitizeTableField(value string) string {
	return strings.NewReplacer("\t", " ", "\n", " ", "\r", " ").Replace(value)
}

func newOpenClient() *api.Client {
	return api.New(config.APIURL(), Version)
}
