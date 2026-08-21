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
		root.AddCommand(newReceiptCommand())
	})
}

func newReceiptCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "receipt",
		Short: "Manage emergency message receipts",
	}
	cmd.AddCommand(newReceiptStatusCommand())
	cmd.AddCommand(newReceiptCancelCommand())
	cmd.AddCommand(newReceiptCancelByTagCommand())
	return cmd
}

func newReceiptStatusCommand() *cobra.Command {
	var wait bool
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "status <receipt-id>",
		Short: "Show an emergency message receipt",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			token, err := receiptToken(cmd)
			if err != nil {
				return err
			}
			client := api.New(config.APIURL(), Version)
			var receipt *api.Receipt
			if wait {
				receipt, err = client.PollReceipt(cmd.Context(), token, args[0], receiptIsComplete, nil)
			} else {
				receipt, err = client.ReceiptStatus(cmd.Context(), token, args[0])
			}
			if err != nil {
				return err
			}
			if jsonOutput {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(receipt)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "status: %d\nacknowledged: %d\nacknowledged_at: %d\nacknowledged_by: %s\nlast_delivered_at: %d\nexpired: %d\nexpires_at: %d\ncalled_back: %d\ncalled_back_at: %d\ncancelled: %t\ncancelled_at: %d\ncancelled_by_tag: %t\n",
				receipt.Status, receipt.Acknowledged, receipt.AcknowledgedAt, receipt.AcknowledgedBy,
				receipt.LastDeliveredAt, receipt.Expired, receipt.ExpiresAt, receipt.CalledBack,
				receipt.CalledBackAt, receipt.Cancelled, receipt.CancelledAt, receipt.CancelledByTag)
			return nil
		},
	}
	cmd.Flags().BoolVar(&wait, "wait", false, "wait for receipt completion")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "print JSON output")
	return cmd
}

func newReceiptCancelCommand() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "cancel <receipt-id>",
		Short: "Cancel an emergency message receipt",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			token, err := receiptToken(cmd)
			if err != nil {
				return err
			}
			if err := api.New(config.APIURL(), Version).CancelReceipt(cmd.Context(), token, args[0]); err != nil {
				return err
			}
			if jsonOutput {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
					Receipt   string `json:"receipt"`
					Cancelled bool   `json:"cancelled"`
				}{Receipt: args[0], Cancelled: true})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "receipt %s cancelled\n", args[0])
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "print JSON output")
	return cmd
}

func newReceiptCancelByTagCommand() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "cancel-by-tag <tag>",
		Short: "Cancel emergency message receipts by tag",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			token, err := receiptToken(cmd)
			if err != nil {
				return err
			}
			cancelled, err := api.New(config.APIURL(), Version).CancelReceiptsByTag(cmd.Context(), token, args[0])
			if err != nil {
				return err
			}
			if jsonOutput {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
					Tag       string `json:"tag"`
					Cancelled bool   `json:"cancelled"`
				}{Tag: args[0], Cancelled: true})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "cancelled: %d\n", cancelled)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "print JSON output")
	return cmd
}

func receiptToken(cmd *cobra.Command) (string, error) {
	envFlags, err := EnvFlagsFromCommand(cmd)
	if err != nil {
		return "", err
	}
	return config.Resolve(config.RoleAppToken, config.RoleAppToken, envFlags)
}

func receiptIsComplete(receipt *api.Receipt) bool {
	return receipt.Acknowledged != 0 || receipt.Expired != 0 || receipt.CalledBack != 0 || receipt.Cancelled
}
