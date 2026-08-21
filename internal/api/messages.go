package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"
)

const maxAttachmentBytes = 5_242_880

type SendMessageRequest struct {
	Token            string
	User             []string
	Message          string
	Device           string
	Title            string
	URL              string
	URLTitle         string
	Priority         *int
	Retry            int
	Expire           int
	Callback         string
	Tags             string
	Sound            string
	Timestamp        int64
	TTL              int
	HTML             bool
	Monospace        bool
	Encrypted        bool
	Attachment       *FilePart
	AttachmentBase64 []byte
	AttachmentType   string
}

type SendMessageResponse struct {
	Status  int    `json:"status"`
	Request string `json:"request"`
	Receipt string `json:"receipt,omitempty"`
}

func (c *Client) SendMessage(ctx context.Context, req SendMessageRequest, options ...RequestOption) (*SendMessageResponse, error) {
	if err := validateSendMessage(req); err != nil {
		return nil, err
	}

	fields := url.Values{
		"token":   {req.Token},
		"user":    {strings.Join(req.User, ",")},
		"message": {req.Message},
	}
	setStringField(fields, "device", req.Device)
	setStringField(fields, "title", req.Title)
	setStringField(fields, "url", req.URL)
	setStringField(fields, "url_title", req.URLTitle)
	if req.Priority != nil {
		fields.Set("priority", strconv.Itoa(*req.Priority))
	}
	setIntField(fields, "retry", req.Retry)
	setIntField(fields, "expire", req.Expire)
	setStringField(fields, "callback", req.Callback)
	setStringField(fields, "tags", req.Tags)
	setStringField(fields, "sound", req.Sound)
	if req.Timestamp != 0 {
		fields.Set("timestamp", strconv.FormatInt(req.Timestamp, 10))
	}
	setIntField(fields, "ttl", req.TTL)
	if req.HTML {
		fields.Set("html", "1")
	}
	if req.Monospace {
		fields.Set("monospace", "1")
	}
	if req.Encrypted {
		fields.Set("encrypted", "1")
	}
	if req.AttachmentBase64 != nil {
		fields.Set("attachment_base64", base64.StdEncoding.EncodeToString(req.AttachmentBase64))
		attachmentType := req.AttachmentType
		if attachmentType == "" {
			attachmentType = http.DetectContentType(req.AttachmentBase64)
		}
		fields.Set("attachment_type", attachmentType)
	}

	var (
		response *Response
		err      error
	)
	if req.Attachment != nil {
		fileName := req.Attachment.FileName
		if fileName != "" {
			fileName = filepath.Base(fileName)
		}
		response, err = c.PostMultipart(ctx, "/1/messages.json", fields, FilePart{
			FieldName: "attachment",
			FileName:  fileName,
			Reader:    req.Attachment.Reader,
		}, options...)
	} else {
		response, err = c.PostForm(ctx, "/1/messages.json", fields, options...)
	}
	if err != nil {
		return nil, err
	}

	var result SendMessageResponse
	if err := json.Unmarshal(response.Body, &result); err != nil {
		return nil, fmt.Errorf("decode send message response: %w", err)
	}
	return &result, nil
}

func validateSendMessage(req SendMessageRequest) error {
	if req.Message == "" {
		return fmt.Errorf("message is required")
	}
	if utf8.RuneCountInString(req.Message) > 1024 {
		return fmt.Errorf("message exceeds 1024 runes")
	}
	for _, field := range []struct {
		name  string
		value string
		limit int
	}{
		{name: "title", value: req.Title, limit: 250},
		{name: "url", value: req.URL, limit: 512},
		{name: "url_title", value: req.URLTitle, limit: 100},
	} {
		if utf8.RuneCountInString(field.value) > field.limit {
			return fmt.Errorf("%s exceeds %d runes", field.name, field.limit)
		}
	}
	if len(req.User) == 0 || len(req.User) > 50 {
		return fmt.Errorf("user must contain between 1 and 50 recipients")
	}
	if req.Priority != nil {
		if *req.Priority < -2 || *req.Priority > 2 {
			return fmt.Errorf("priority must be between -2 and 2")
		}
		if *req.Priority == 2 {
			if req.Retry < 30 {
				return fmt.Errorf("retry must be at least 30 for emergency priority")
			}
			if req.Expire <= 0 || req.Expire > 10_800 {
				return fmt.Errorf("expire must be between 1 and 10800 for emergency priority")
			}
		}
	}
	if req.Attachment != nil && req.AttachmentBase64 != nil {
		return fmt.Errorf("only one attachment may be set")
	}
	if len(req.AttachmentBase64) > maxAttachmentBytes {
		return fmt.Errorf("attachment exceeds %d bytes", maxAttachmentBytes)
	}
	if req.Attachment != nil {
		if err := validateAttachment(req.Attachment); err != nil {
			return err
		}
	}
	return nil
}

func validateAttachment(attachment *FilePart) error {
	if attachment.Reader == nil {
		return fmt.Errorf("attachment reader is required")
	}
	if _, err := attachment.Reader.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("seek attachment to start: %w", err)
	}
	data, readErr := io.ReadAll(io.LimitReader(attachment.Reader, maxAttachmentBytes+1))
	if _, err := attachment.Reader.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("reset attachment to start: %w", err)
	}
	if readErr != nil {
		return fmt.Errorf("read attachment: %w", readErr)
	}
	if len(data) > maxAttachmentBytes {
		return fmt.Errorf("attachment exceeds %d bytes", maxAttachmentBytes)
	}
	return nil
}

func setStringField(fields url.Values, name, value string) {
	if value != "" {
		fields.Set(name, value)
	}
}

func setIntField(fields url.Values, name string, value int) {
	if value != 0 {
		fields.Set(name, strconv.Itoa(value))
	}
}
