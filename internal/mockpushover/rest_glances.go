package mockpushover

import (
	"net/http"
	"strconv"
	"unicode/utf8"
)

func (s *Server) handleGlances(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(s, w, r, http.MethodPost) {
		return
	}
	if _, ok := s.requireApp(w, r.FormValue("token")); !ok {
		return
	}
	userKey := r.FormValue("user")
	deviceName := r.FormValue("device")
	for _, field := range []string{"title", "text", "subtext"} {
		if utf8.RuneCountInString(r.FormValue(field)) > 100 {
			s.writeError(w, http.StatusBadRequest, field, "is too long")
			return
		}
	}
	count := 0
	if r.Form.Has("count") {
		parsed, err := strconv.Atoi(r.FormValue("count"))
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "count", "must be an integer")
			return
		}
		count = parsed
	}
	percent := 0
	if r.Form.Has("percent") {
		parsed, err := strconv.Atoi(r.FormValue("percent"))
		if err != nil || parsed < 0 || parsed > 100 {
			s.writeError(w, http.StatusBadRequest, "percent", "must be between 0 and 100")
			return
		}
		percent = parsed
	}

	s.store.mu.Lock()
	_, userExists := s.store.users[userKey]
	deviceExists := deviceName == ""
	if deviceName != "" {
		for _, device := range s.store.devices {
			if device.User == userKey && device.Name == deviceName && device.Enabled {
				deviceExists = true
				break
			}
		}
	}
	if !userExists || !deviceExists {
		s.store.mu.Unlock()
		if !userExists {
			s.writeError(w, http.StatusBadRequest, "user", "identifier is invalid")
		} else {
			s.writeError(w, http.StatusBadRequest, "device", "is invalid")
		}
		return
	}
	key := glanceKey(userKey, deviceName)
	glance := s.store.glances[key]
	glance.User = userKey
	glance.Device = deviceName
	if r.Form.Has("title") {
		glance.Title = r.FormValue("title")
	}
	if r.Form.Has("text") {
		glance.Text = r.FormValue("text")
	}
	if r.Form.Has("subtext") {
		glance.Subtext = r.FormValue("subtext")
	}
	if r.Form.Has("count") {
		glance.Count = count
	}
	if r.Form.Has("percent") {
		glance.Percent = percent
	}
	s.store.glances[key] = glance
	s.store.mu.Unlock()
	s.writeSuccess(w, nil)
}
