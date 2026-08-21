package mockpushover

import (
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) handleOpenMessages(w http.ResponseWriter, r *http.Request) {
	deviceID := r.FormValue("device_id")
	secret := r.FormValue("secret")
	s.store.mu.RLock()
	device, ok := s.store.devices[deviceID]
	if ok && device.Secret == secret && secret != "" {
		messages := s.store.queues[deviceID]
		copied := make([]Message, len(messages))
		for i := range messages {
			copied[i] = cloneMessage(messages[i])
		}
		s.store.mu.RUnlock()
		s.writeSuccess(w, map[string]any{"messages": copied})
		return
	}
	s.store.mu.RUnlock()
	s.writeError(w, http.StatusBadRequest, "secret", "or device_id is invalid")
}

func (s *Server) handleDeviceRoutes(w http.ResponseWriter, r *http.Request) {
	remainder := strings.TrimPrefix(r.URL.Path, "/1/devices/")
	parts := strings.Split(remainder, "/")
	if len(parts) != 2 || parts[1] != "update_highest_message.json" {
		s.writeError(w, http.StatusNotFound, "request", "endpoint was not found")
		return
	}
	s.handleUpdateHighestMessage(w, r, parts[0])
}

func (s *Server) handleUpdateHighestMessage(w http.ResponseWriter, r *http.Request, deviceID string) {
	if !requireMethod(s, w, r, http.MethodPost) {
		return
	}
	highest, err := strconv.ParseInt(r.FormValue("message"), 10, 64)
	if err != nil || highest < 0 {
		s.writeError(w, http.StatusBadRequest, "message", "is invalid")
		return
	}
	secret := r.FormValue("secret")
	s.store.mu.Lock()
	device, ok := s.store.devices[deviceID]
	if !ok || device.Secret != secret || secret == "" {
		s.store.mu.Unlock()
		s.writeError(w, http.StatusBadRequest, "secret", "or device is invalid")
		return
	}
	if highest > s.store.highestDeleted[deviceID] {
		s.store.highestDeleted[deviceID] = highest
	}
	if highest > s.store.nextMessageID[deviceID] {
		s.store.nextMessageID[deviceID] = highest
	}
	messages := s.store.queues[deviceID]
	kept := messages[:0]
	for _, message := range messages {
		if message.ID > s.store.highestDeleted[deviceID] {
			kept = append(kept, message)
		}
	}
	s.store.queues[deviceID] = kept
	s.store.mu.Unlock()
	s.writeSuccess(w, nil)
}
