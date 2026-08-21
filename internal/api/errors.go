package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const generalErrorsKey = "general"

// error messages contains api validation messages grouped by field.
type ErrorMessages map[string][]string

// unmarshal json accepts both error formats returned by the pushover api.
func (m *ErrorMessages) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		*m = nil
		return nil
	}

	var messages []string
	if err := json.Unmarshal(data, &messages); err == nil {
		*m = ErrorMessages{generalErrorsKey: messages}
		return nil
	}

	var fields map[string][]string
	if err := json.Unmarshal(data, &fields); err != nil {
		return fmt.Errorf("decode pushover errors: %w", err)
	}
	*m = ErrorMessages(fields)
	return nil
}

// api error is an error reported in a pushover api response body.
type APIError struct {
	Status  int           `json:"status"`
	Request string        `json:"request"`
	Errors  ErrorMessages `json:"errors"`
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	message := e.message()
	if e.Request != "" {
		message += " (request " + e.Request + ")"
	}
	return message
}

func (e *APIError) message() string {
	keys := make([]string, 0, len(e.Errors))
	for key := range e.Errors {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	messages := make([]string, 0, len(keys))
	for _, key := range keys {
		prefix := ""
		if key != generalErrorsKey {
			prefix = key + ": "
		}
		messages = append(messages, prefix+strings.Join(e.Errors[key], ", "))
	}

	message := "pushover api error"
	if len(messages) > 0 {
		message += ": " + strings.Join(messages, "; ")
	}
	return message
}

// quota error reports that the application has exceeded a pushover quota.
type QuotaError struct {
	Reset string
	*APIError
}

func (e *QuotaError) Error() string {
	if e == nil {
		return ""
	}
	if e.APIError != nil {
		message := e.APIError.message()
		if e.Reset != "" {
			message += " (resets at " + e.Reset + ")"
		}
		if e.APIError.Request != "" {
			message += " (request " + e.APIError.Request + ")"
		}
		return message
	}
	if e.Reset != "" {
		return "pushover quota exceeded (resets at " + e.Reset + ")"
	}
	return "pushover quota exceeded"
}

// unwrap exposes an api error returned with a quota response, when present.
func (e *QuotaError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.APIError
}

// http error is an http response not otherwise described by api error.
type HTTPError struct {
	StatusCode int
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("pushover api returned http status %d", e.StatusCode)
}

func apiErrorFromBody(body []byte) *APIError {
	var status struct {
		Status *int `json:"status"`
	}
	if err := json.Unmarshal(body, &status); err != nil || status.Status == nil || *status.Status == 1 {
		return nil
	}

	var apiError APIError
	if err := json.Unmarshal(body, &apiError); err != nil {
		return &APIError{Status: *status.Status}
	}
	return &apiError
}
