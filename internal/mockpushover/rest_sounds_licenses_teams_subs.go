package mockpushover

import (
	"net/http"
	"strings"
)

var builtInSounds = map[string]string{
	"pushover":     "Pushover (default)",
	"bike":         "Bike",
	"bugle":        "Bugle",
	"cashregister": "Cash Register",
	"classical":    "Classical",
	"cosmic":       "Cosmic",
	"falling":      "Falling",
	"gamelan":      "Gamelan",
	"incoming":     "Incoming",
	"intermission": "Intermission",
	"magic":        "Magic",
	"mechanical":   "Mechanical",
	"pianobar":     "Piano Bar",
	"siren":        "Siren",
	"spacealarm":   "Space Alarm",
	"tugboat":      "Tug Boat",
	"alien":        "Alien Alarm (long)",
	"climb":        "Climb (long)",
	"persistent":   "Persistent (long)",
	"echo":         "Pushover Echo (long)",
	"updown":       "Up Down (long)",
	"vibrate":      "Vibrate Only",
	"none":         "None (silent)",
}

func (s *Server) handleSounds(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(s, w, r, http.MethodGet) {
		return
	}
	if _, ok := s.requireApp(w, r.FormValue("token")); !ok {
		return
	}
	sounds := make(map[string]string, len(builtInSounds))
	for key, label := range builtInSounds {
		sounds[key] = label
	}
	s.store.mu.RLock()
	for key, label := range s.store.customSounds {
		sounds[key] = label
	}
	s.store.mu.RUnlock()
	s.writeSuccess(w, map[string]any{"sounds": sounds})
}

func (s *Server) handleAssignLicense(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(s, w, r, http.MethodPost) {
		return
	}
	token := r.FormValue("token")
	if _, ok := s.requireApp(w, token); !ok {
		return
	}
	userKey := r.FormValue("user")
	email := r.FormValue("email")
	if (userKey == "") == (email == "") {
		s.writeError(w, http.StatusBadRequest, "user", "or email must be supplied, but not both")
		return
	}
	os := r.FormValue("os")
	if os != "" && os != "Android" && os != "iOS" && os != "Desktop" {
		s.writeError(w, http.StatusBadRequest, "os", "is invalid")
		return
	}
	s.store.mu.Lock()
	if userKey != "" {
		if _, ok := s.store.users[userKey]; !ok {
			s.store.mu.Unlock()
			s.writeError(w, http.StatusBadRequest, "user", "identifier is invalid")
			return
		}
	}
	pool := s.store.licenses[token]
	if pool.Credits <= 0 {
		s.store.mu.Unlock()
		s.writeError(w, http.StatusBadRequest, "token", "is out of available license credits")
		return
	}
	pool.Credits--
	pool.Assigned = append(pool.Assigned, LicenseAssignment{User: userKey, Email: email, OS: os})
	s.store.licenses[token] = pool
	credits := pool.Credits
	s.store.mu.Unlock()
	s.writeSuccess(w, map[string]any{"credits": credits})
}

func (s *Server) handleLicenses(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(s, w, r, http.MethodGet) {
		return
	}
	token := r.FormValue("token")
	if _, ok := s.requireApp(w, token); !ok {
		return
	}
	s.store.mu.RLock()
	pool := s.store.licenses[token]
	assigned := make([]LicenseAssignment, len(pool.Assigned))
	copy(assigned, pool.Assigned)
	s.store.mu.RUnlock()
	s.writeSuccess(w, map[string]any{"credits": pool.Credits, "assigned": assigned})
}

func (s *Server) handleTeams(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(s, w, r, http.MethodGet) {
		return
	}
	team, ok := s.team(r.FormValue("token"))
	if !ok {
		s.writeError(w, http.StatusBadRequest, "token", "team token is invalid")
		return
	}
	s.writeSuccess(w, map[string]any{"name": team.Name, "users": team.Users})
}

func (s *Server) handleTeamAddUser(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(s, w, r, http.MethodPost) {
		return
	}
	token := r.FormValue("token")
	email := strings.TrimSpace(r.FormValue("email"))
	if email == "" {
		s.writeError(w, http.StatusBadRequest, "email", "is required")
		return
	}
	s.store.mu.Lock()
	team, ok := s.store.teams[token]
	if !ok {
		s.store.mu.Unlock()
		s.writeError(w, http.StatusBadRequest, "token", "team token is invalid")
		return
	}
	member := TeamMember{
		ID: randomID(), Name: r.FormValue("name"), Email: email,
		Administrator: r.FormValue("admin") == "true", Invited: true,
	}
	team.Users = append(team.Users, member)
	s.store.teams[token] = team
	s.store.mu.Unlock()
	s.writeSuccess(w, map[string]any{"id": member.ID})
}

func (s *Server) handleTeamRevokeInvitation(w http.ResponseWriter, r *http.Request) {
	s.handleTeamDeleteUser(w, r, true)
}

func (s *Server) handleTeamRemoveUser(w http.ResponseWriter, r *http.Request) {
	s.handleTeamDeleteUser(w, r, false)
}

func (s *Server) handleTeamDeleteUser(w http.ResponseWriter, r *http.Request, invitationOnly bool) {
	if !requireMethod(s, w, r, http.MethodPost) {
		return
	}
	token := r.FormValue("token")
	email := r.FormValue("email")
	if email == "" {
		s.writeError(w, http.StatusBadRequest, "email", "is required")
		return
	}
	s.store.mu.Lock()
	team, ok := s.store.teams[token]
	if !ok {
		s.store.mu.Unlock()
		s.writeError(w, http.StatusBadRequest, "token", "team token is invalid")
		return
	}
	kept := team.Users[:0]
	for _, member := range team.Users {
		matches := strings.EqualFold(member.Email, email) && (!invitationOnly || member.Invited)
		if !matches {
			kept = append(kept, member)
		}
	}
	team.Users = kept
	s.store.teams[token] = team
	s.store.mu.Unlock()
	s.writeSuccess(w, nil)
}

func (s *Server) team(token string) (Team, bool) {
	s.store.mu.RLock()
	defer s.store.mu.RUnlock()
	team, ok := s.store.teams[token]
	team.Users = cloneTeamMembers(team.Users)
	return team, ok
}

func (s *Server) handleSubscriptionMigrate(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(s, w, r, http.MethodPost) {
		return
	}
	if _, ok := s.requireApp(w, r.FormValue("token")); !ok {
		return
	}
	if r.FormValue("subscription") == "" {
		s.writeError(w, http.StatusBadRequest, "subscription", "is required")
		return
	}
	userKey := r.FormValue("user")
	s.store.mu.RLock()
	_, ok := s.store.users[userKey]
	s.store.mu.RUnlock()
	if !ok {
		s.writeError(w, http.StatusBadRequest, "user", "identifier is invalid")
		return
	}
	s.writeSuccess(w, map[string]any{"subscribed_user_key": randomKey(30)})
}

func (s *Server) handleAppLimits(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(s, w, r, http.MethodGet) {
		return
	}
	app, ok := s.requireApp(w, r.FormValue("token"))
	if !ok {
		return
	}
	setQuotaHeaders(w, app.Quota)
	s.writeSuccess(w, map[string]any{
		"limit": app.Quota.Limit, "remaining": app.Quota.Remaining, "reset": app.Quota.Reset,
	})
}
