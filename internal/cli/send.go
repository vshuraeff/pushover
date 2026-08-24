package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vshuraeff/pushover/internal/api"
	"github.com/vshuraeff/pushover/internal/config"
	pushovercrypto "github.com/vshuraeff/pushover/internal/crypto"
)

func init() {
	register(func(root *cobra.Command) {
		root.AddCommand(newSendCommand())
	})
}

func newSendCommand() *cobra.Command {
	var (
		recipientFlag        string
		messageFlag          string
		device               string
		title                string
		messageURL           string
		urlTitle             string
		priority             int
		retry                int
		expire               int
		callback             string
		tags                 string
		sound                string
		timestamp            int64
		ttl                  int
		html                 bool
		monospace            bool
		encrypted            bool
		attachmentPath       string
		attachmentBase64Path string
		attachmentType       string
		jsonOutput           bool
	)

	cmd := &cobra.Command{
		Use:   "send [message]",
		Short: "Send a Pushover message",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			message, err := sendMessageText(cmd, args, messageFlag)
			if err != nil {
				return err
			}
			if attachmentPath != "" && attachmentBase64Path != "" {
				return fmt.Errorf("only one attachment may be set")
			}

			envFlags, err := EnvFlagsFromCommand(cmd)
			if err != nil {
				return err
			}
			token, err := config.Resolve(config.RoleAppToken, config.RoleAppToken, envFlags)
			if err != nil {
				return err
			}
			userSet := cmd.Flags().Changed("user")
			var recipients []string
			if !userSet {
				user, err := config.Resolve(config.RoleUserKey, config.RoleAppToken, envFlags)
				if err != nil {
					return err
				}
				recipients = []string{user}
			} else {
				recipients = recipientKeys(recipientFlag)
			}
			var encryptionKey []byte
			if encrypted {
				encodedKey, err := config.Resolve(config.RoleEncryptionKey, config.RoleAppToken, envFlags)
				if err != nil {
					return err
				}
				encryptionKey, err = pushovercrypto.ParseKey(encodedKey)
				if err != nil {
					return err
				}
			}

			req := api.SendMessageRequest{
				Token: token, User: recipients, Message: message, Device: device,
				Title: title, URL: messageURL, URLTitle: urlTitle, Retry: retry, Expire: expire,
				Callback: callback, Tags: tags, Sound: sound, Timestamp: timestamp, TTL: ttl,
				HTML: html, Monospace: monospace, Encrypted: encrypted, EncryptionKey: encryptionKey,
			}
			if cmd.Flags().Changed("priority") {
				req.Priority = &priority
			}

			if attachmentPath != "" {
				file, err := os.Open(attachmentPath)
				if err != nil {
					return fmt.Errorf("open attachment: %w", err)
				}
				defer file.Close()
				info, err := file.Stat()
				if err != nil {
					return fmt.Errorf("stat attachment: %w", err)
				}
				if info.Size() > api.MaxAttachmentBytes {
					return fmt.Errorf("attachment exceeds %d bytes", api.MaxAttachmentBytes)
				}
				req.Attachment = &api.FilePart{FileName: filepath.Base(attachmentPath), Reader: file}
			}
			if attachmentBase64Path != "" {
				data, err := os.ReadFile(attachmentBase64Path)
				if err != nil {
					return fmt.Errorf("read base64 attachment: %w", err)
				}
				if len(data) > api.MaxAttachmentBytes {
					return fmt.Errorf("attachment exceeds %d bytes", api.MaxAttachmentBytes)
				}
				req.AttachmentBase64 = data
				req.AttachmentType = attachmentType
				if req.AttachmentType == "" {
					req.AttachmentType = http.DetectContentType(data)
				}
			}

			response, err := api.New(config.APIURL(), Version).SendMessage(cmd.Context(), req)
			if err != nil {
				return err
			}
			if encrypted {
				fmt.Fprintln(cmd.ErrOrStderr(), "warning: encrypted messages are delivered only to the official iOS/Android apps; Open Client devices (including povr client listen) will not receive them")
			}
			if jsonOutput {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(response)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "request: %s\n", response.Request)
			if response.Receipt != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "receipt: %s\n", response.Receipt)
			}
			return nil
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&recipientFlag, "user", "", "recipient user or group key")
	flags.StringVar(&messageFlag, "message", "", "message text")
	flags.StringVar(&device, "device", "", "target device name")
	flags.StringVar(&title, "title", "", "message title")
	flags.StringVar(&messageURL, "url", "", "supplementary URL")
	flags.StringVar(&urlTitle, "url-title", "", "supplementary URL title")
	flags.IntVar(&priority, "priority", 0, "message priority")
	flags.IntVar(&retry, "retry", 0, "emergency retry interval in seconds")
	flags.IntVar(&expire, "expire", 0, "emergency expiry in seconds")
	flags.StringVar(&callback, "callback", "", "emergency callback URL")
	flags.StringVar(&tags, "tags", "", "message tags")
	flags.StringVar(&sound, "sound", "", "notification sound")
	flags.Int64Var(&timestamp, "timestamp", 0, "message timestamp as unix seconds")
	flags.IntVar(&ttl, "ttl", 0, "message time to live in seconds")
	flags.BoolVar(&html, "html", false, "enable HTML formatting")
	flags.BoolVar(&monospace, "monospace", false, "enable monospace formatting")
	flags.BoolVar(&encrypted, "encrypted", false, "encrypt message fields")
	flags.StringVar(&attachmentPath, "attachment", "", "attachment file path")
	flags.StringVar(&attachmentBase64Path, "attachment-base64", "", "base64 attachment file path")
	flags.StringVar(&attachmentType, "attachment-type", "", "base64 attachment MIME type")
	flags.BoolVar(&jsonOutput, "json", false, "print JSON output")
	return cmd
}

func sendMessageText(cmd *cobra.Command, args []string, messageFlag string) (string, error) {
	if len(args) == 1 {
		if args[0] != "-" {
			return args[0], nil
		}
		data, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return "", fmt.Errorf("read message from stdin: %w", err)
		}
		return string(data), nil
	}
	if messageFlag == "" {
		return "", fmt.Errorf("message is required")
	}
	return messageFlag, nil
}

func recipientKeys(value string) []string {
	entries := strings.Split(value, ",")
	recipients := make([]string, 0, len(entries))
	for _, entry := range entries {
		if recipient := strings.TrimSpace(entry); recipient != "" {
			recipients = append(recipients, recipient)
		}
	}
	return recipients
}
