package mockpushover

import (
	"encoding/base64"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const maxAttachmentBytes = 5_242_880

var userKeyPattern = regexp.MustCompile(`^[A-Za-z0-9]{30}$`)

func (s *Server) handleMessagesRoute(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.handleSendMessage(w, r)
	case http.MethodGet:
		s.handleOpenMessages(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "request", "method is not allowed")
	}
}

func (s *Server) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	token := r.FormValue("token")
	if _, ok := s.requireApp(w, token); !ok {
		return
	}
	messageText := r.FormValue("message")
	if messageText == "" {
		s.writeError(w, http.StatusBadRequest, "message", "is required")
		return
	}
	if utf8.RuneCountInString(messageText) > 1024 {
		s.writeError(w, http.StatusBadRequest, "message", "is too long")
		return
	}
	for _, field := range []struct {
		name  string
		limit int
	}{
		{name: "title", limit: 250},
		{name: "url", limit: 512},
		{name: "url_title", limit: 100},
	} {
		if utf8.RuneCountInString(r.FormValue(field.name)) > field.limit {
			s.writeError(w, http.StatusBadRequest, field.name, "is too long")
			return
		}
	}
	targets := strings.Split(r.FormValue("user"), ",")
	if len(targets) == 0 || len(targets) > 50 {
		s.writeError(w, http.StatusBadRequest, "user", "identifier is invalid")
		return
	}
	for _, target := range targets {
		if !userKeyPattern.MatchString(target) {
			s.writeError(w, http.StatusBadRequest, "user", "identifier is invalid")
			return
		}
	}
	priority, err := optionalInt(r.FormValue("priority"), 0)
	if err != nil || priority < -2 || priority > 2 {
		s.writeError(w, http.StatusBadRequest, "priority", "must be between -2 and 2")
		return
	}
	retry, retryErr := optionalInt(r.FormValue("retry"), 0)
	expire, expireErr := optionalInt(r.FormValue("expire"), 0)
	if priority == 2 && (retryErr != nil || retry < 30) {
		s.writeError(w, http.StatusBadRequest, "retry", "must be at least 30")
		return
	}
	if priority == 2 && (expireErr != nil || expire <= 0 || expire > 10_800) {
		s.writeError(w, http.StatusBadRequest, "expire", "must be between 1 and 10800")
		return
	}
	attachment, err := readAttachment(r)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "attachment", err.Error())
		return
	}
	html, err := optionalInt(r.FormValue("html"), 0)
	if err != nil || (html != 0 && html != 1) {
		s.writeError(w, http.StatusBadRequest, "html", "must be 0 or 1")
		return
	}

	now := time.Now().Unix()
	date := now
	if timestamp := r.FormValue("timestamp"); timestamp != "" {
		parsed, parseErr := strconv.ParseInt(timestamp, 10, 64)
		if parseErr != nil {
			s.writeError(w, http.StatusBadRequest, "timestamp", "is invalid")
			return
		}
		date = parsed
	}

	s.store.mu.Lock()
	app := s.store.apps[token]
	deviceIDs, validTargets := s.resolveTargetsLocked(targets, r.FormValue("device"))
	if !validTargets {
		s.store.mu.Unlock()
		s.writeError(w, http.StatusBadRequest, "user", "identifier is invalid")
		return
	}
	if app.Quota.Remaining <= 0 {
		quota := app.Quota
		s.store.mu.Unlock()
		setQuotaHeaders(w, quota)
		s.writeError(w, http.StatusTooManyRequests, "token", "has no remaining message quota")
		return
	}

	receiptID := ""
	if priority == 2 {
		receiptID = randomID()
		s.store.receipts[receiptID] = Receipt{
			ID: receiptID, Token: token, Tag: r.FormValue("tags"),
			LastDeliveredAt: now, ExpiresAt: now + int64(expire), DeviceIDs: append([]string(nil), deviceIDs...),
		}
	}
	umid := s.store.nextUMID
	s.store.nextUMID++
	for _, deviceID := range deviceIDs {
		s.store.nextMessageID[deviceID]++
		id := s.store.nextMessageID[deviceID]
		message := Message{
			ID: id, IDString: strconv.FormatInt(id, 10),
			UMID: umid, UMIDString: strconv.FormatInt(umid, 10),
			Title: r.FormValue("title"), Message: messageText,
			App: app.Name, AID: app.ID, AIDString: strconv.FormatInt(app.ID, 10), Icon: app.Icon,
			Date: date, QueuedDate: now, DispatchedDate: now, Priority: priority,
			Sound: r.FormValue("sound"), URL: r.FormValue("url"), URLTitle: r.FormValue("url_title"),
			Receipt: receiptID, HTML: html, Attachment: append([]byte(nil), attachment...),
		}
		s.store.queues[deviceID] = append(s.store.queues[deviceID], message)
	}
	app.Quota.Remaining--
	s.store.apps[token] = app
	quota := app.Quota
	s.store.mu.Unlock()

	setQuotaHeaders(w, quota)
	extra := map[string]any{}
	if receiptID != "" {
		extra["receipt"] = receiptID
	}
	s.writeSuccess(w, extra)
}

func (s *Server) resolveTargetsLocked(targets []string, requestedDevice string) ([]string, bool) {
	seen := make(map[string]bool)
	var deviceIDs []string
	for _, target := range targets {
		if _, ok := s.store.users[target]; ok {
			matching := s.userDevicesLocked(target, requestedDevice, len(targets) == 1)
			for _, deviceID := range matching {
				if !seen[deviceID] {
					seen[deviceID] = true
					deviceIDs = append(deviceIDs, deviceID)
				}
			}
			continue
		}
		group, ok := s.store.groups[target]
		if !ok {
			return nil, false
		}
		for _, member := range group.Members {
			if member.Disabled {
				continue
			}
			for _, deviceID := range s.userDevicesLocked(member.User, member.Device, true) {
				if !seen[deviceID] {
					seen[deviceID] = true
					deviceIDs = append(deviceIDs, deviceID)
				}
			}
		}
	}
	return deviceIDs, true
}

func (s *Server) userDevicesLocked(user, deviceName string, useFilter bool) []string {
	var all []string
	var filtered []string
	for id, device := range s.store.devices {
		if device.User != user || !device.Enabled {
			continue
		}
		all = append(all, id)
		if deviceName != "" && device.Name == deviceName {
			filtered = append(filtered, id)
		}
	}
	if useFilter && deviceName != "" && len(filtered) > 0 {
		return filtered
	}
	return all
}

func readAttachment(r *http.Request) ([]byte, error) {
	if encoded := r.FormValue("attachment_base64"); encoded != "" {
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, err
		}
		if len(decoded) > maxAttachmentBytes {
			return nil, io.ErrShortBuffer
		}
		return decoded, nil
	}
	if r.MultipartForm == nil || len(r.MultipartForm.File["attachment"]) == 0 {
		return nil, nil
	}
	file, err := r.MultipartForm.File["attachment"][0].Open()
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxAttachmentBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxAttachmentBytes {
		return nil, io.ErrShortBuffer
	}
	return data, nil
}

func optionalInt(value string, fallback int) (int, error) {
	if value == "" {
		return fallback, nil
	}
	return strconv.Atoi(value)
}

func setQuotaHeaders(w http.ResponseWriter, quota AppQuota) {
	w.Header().Set("X-Limit-App-Limit", strconv.Itoa(quota.Limit))
	w.Header().Set("X-Limit-App-Remaining", strconv.Itoa(quota.Remaining))
	w.Header().Set("X-Limit-App-Reset", strconv.FormatInt(quota.Reset, 10))
}
