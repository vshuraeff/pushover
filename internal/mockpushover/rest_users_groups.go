package mockpushover

import (
	"net/http"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

var deviceNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,25}$`)

func (s *Server) handleValidateUser(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(s, w, r, http.MethodPost) {
		return
	}
	if _, ok := s.requireApp(w, r.FormValue("token")); !ok {
		return
	}
	userKey := r.FormValue("user")
	deviceName := r.FormValue("device")
	s.store.mu.RLock()
	_, userExists := s.store.users[userKey]
	_, groupExists := s.store.groups[userKey]
	var deviceNames []string
	if userExists {
		for _, device := range s.store.devices {
			if device.User == userKey && device.Enabled {
				deviceNames = append(deviceNames, device.Name)
			}
		}
	}
	s.store.mu.RUnlock()
	if !userExists && !groupExists {
		s.writeError(w, http.StatusBadRequest, "user", "identifier is invalid")
		return
	}
	if deviceName != "" {
		found := false
		for _, name := range deviceNames {
			if name == deviceName {
				found = true
				break
			}
		}
		if !found {
			s.writeError(w, http.StatusBadRequest, "device", "is invalid")
			return
		}
	}
	sort.Strings(deviceNames)
	extra := map[string]any{}
	if userExists {
		extra["devices"] = deviceNames
	}
	s.writeSuccess(w, extra)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(s, w, r, http.MethodPost) {
		return
	}
	email := r.FormValue("email")
	s.store.mu.RLock()
	account, ok := s.store.accounts[email]
	s.store.mu.RUnlock()
	if !ok || account.Password != r.FormValue("password") {
		s.writeError(w, http.StatusBadRequest, "email", "or password is invalid")
		return
	}
	if account.Requires2FA && account.TwoFACode != r.FormValue("twofa") {
		writeJSON(w, http.StatusPreconditionFailed, map[string]any{"status": 0})
		return
	}
	s.writeSuccess(w, map[string]any{"id": account.User, "secret": account.Secret})
}

func (s *Server) handleCreateDevice(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(s, w, r, http.MethodPost) {
		return
	}
	secret := r.FormValue("secret")
	name := r.FormValue("name")
	if !deviceNamePattern.MatchString(name) {
		s.writeFieldErrors(w, http.StatusBadRequest, map[string][]string{"name": {"is invalid"}})
		return
	}
	if r.FormValue("os") != "O" {
		s.writeError(w, http.StatusBadRequest, "os", "must be O")
		return
	}
	s.store.mu.Lock()
	userKey := ""
	for key, user := range s.store.users {
		if user.Secret == secret && secret != "" {
			userKey = key
			break
		}
	}
	if userKey == "" {
		s.store.mu.Unlock()
		s.writeError(w, http.StatusBadRequest, "secret", "is invalid")
		return
	}
	for _, device := range s.store.devices {
		if device.User == userKey && device.Name == name {
			s.store.mu.Unlock()
			s.writeFieldErrors(w, http.StatusBadRequest, map[string][]string{"name": {"has already been taken"}})
			return
		}
	}
	deviceID := randomKey(40)
	// the new device's secret matches the session secret used to register it.
	// e2e/listen_test.go's login/register/listen chain depends on this re-seed invariant.
	s.store.devices[deviceID] = Device{
		ID: deviceID, User: userKey, Name: name, OS: "O", Secret: secret, Enabled: true,
	}
	s.store.mu.Unlock()
	s.writeSuccess(w, map[string]any{"id": deviceID})
}

func (s *Server) handleGroups(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.handleCreateGroup(w, r)
	case http.MethodGet:
		s.handleListGroups(w, r)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "request", "method is not allowed")
	}
}

func (s *Server) handleCreateGroup(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireApp(w, r.FormValue("token")); !ok {
		return
	}
	name := r.FormValue("name")
	if name == "" {
		s.writeError(w, http.StatusBadRequest, "name", "is required")
		return
	}
	key := randomKey(30)
	s.store.mu.Lock()
	s.store.groups[key] = Group{Key: key, Name: name}
	s.store.mu.Unlock()
	s.writeSuccess(w, map[string]any{"group": key})
}

func (s *Server) handleListGroups(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireApp(w, r.FormValue("token")); !ok {
		return
	}
	s.store.mu.RLock()
	groups := make([]map[string]string, 0, len(s.store.groups))
	for _, group := range s.store.groups {
		groups = append(groups, map[string]string{"group": group.Key, "name": group.Name})
	}
	s.store.mu.RUnlock()
	sort.Slice(groups, func(i, j int) bool { return groups[i]["group"] < groups[j]["group"] })
	s.writeSuccess(w, map[string]any{"groups": groups})
}

func (s *Server) handleGroup(w http.ResponseWriter, r *http.Request) {
	remainder := strings.TrimPrefix(r.URL.Path, "/1/groups/")
	parts := strings.Split(remainder, "/")
	if len(parts) == 1 && strings.HasSuffix(parts[0], ".json") {
		s.handleGroupDetail(w, r, strings.TrimSuffix(parts[0], ".json"))
		return
	}
	if len(parts) != 2 {
		s.writeError(w, http.StatusNotFound, "request", "endpoint was not found")
		return
	}
	key := parts[0]
	switch parts[1] {
	case "add_user.json":
		s.handleGroupAddUser(w, r, key)
	case "remove_user.json":
		s.handleGroupRemoveUser(w, r, key)
	case "disable_user.json":
		s.handleGroupToggleUser(w, r, key, true)
	case "enable_user.json":
		s.handleGroupToggleUser(w, r, key, false)
	case "rename.json":
		s.handleGroupRename(w, r, key)
	default:
		s.writeError(w, http.StatusNotFound, "request", "endpoint was not found")
	}
}

func (s *Server) authorizeGroupRequest(w http.ResponseWriter, r *http.Request, key string) (Group, bool) {
	if _, ok := s.requireApp(w, r.FormValue("token")); !ok {
		return Group{}, false
	}
	s.store.mu.RLock()
	group, ok := s.store.groups[key]
	group.Members = append([]GroupMember(nil), group.Members...)
	s.store.mu.RUnlock()
	if !ok {
		s.writeError(w, http.StatusBadRequest, "group", "is invalid")
	}
	return group, ok
}

func (s *Server) handleGroupDetail(w http.ResponseWriter, r *http.Request, key string) {
	if !requireMethod(s, w, r, http.MethodGet) {
		return
	}
	group, ok := s.authorizeGroupRequest(w, r, key)
	if !ok {
		return
	}
	users := make([]map[string]any, 0, len(group.Members))
	for _, member := range group.Members {
		var device any
		if member.Device != "" {
			device = member.Device
		}
		users = append(users, map[string]any{
			"user": member.User, "device": device, "memo": member.Memo, "disabled": member.Disabled,
		})
	}
	s.writeSuccess(w, map[string]any{"name": group.Name, "users": users})
}

func (s *Server) handleGroupAddUser(w http.ResponseWriter, r *http.Request, key string) {
	if !requireMethod(s, w, r, http.MethodPost) {
		return
	}
	if _, ok := s.authorizeGroupRequest(w, r, key); !ok {
		return
	}
	userKey := r.FormValue("user")
	memo := r.FormValue("memo")
	if utf8.RuneCountInString(memo) > 200 {
		s.writeError(w, http.StatusBadRequest, "memo", "is too long")
		return
	}
	s.store.mu.Lock()
	_, userExists := s.store.users[userKey]
	if !userExists {
		s.store.mu.Unlock()
		s.writeError(w, http.StatusBadRequest, "user", "identifier is invalid")
		return
	}
	group := s.store.groups[key]
	group.Members = append(group.Members, GroupMember{
		User: userKey, Device: r.FormValue("device"), Memo: memo,
	})
	s.store.groups[key] = group
	s.store.mu.Unlock()
	s.writeSuccess(w, nil)
}

func (s *Server) handleGroupRemoveUser(w http.ResponseWriter, r *http.Request, key string) {
	if !requireMethod(s, w, r, http.MethodPost) {
		return
	}
	if _, ok := s.authorizeGroupRequest(w, r, key); !ok {
		return
	}
	userKey := r.FormValue("user")
	deviceName := r.FormValue("device")
	s.store.mu.Lock()
	group := s.store.groups[key]
	kept := group.Members[:0]
	for _, member := range group.Members {
		matches := member.User == userKey && (deviceName == "" || member.Device == deviceName)
		if !matches {
			kept = append(kept, member)
		}
	}
	group.Members = kept
	s.store.groups[key] = group
	s.store.mu.Unlock()
	s.writeSuccess(w, nil)
}

func (s *Server) handleGroupToggleUser(w http.ResponseWriter, r *http.Request, key string, disabled bool) {
	if !requireMethod(s, w, r, http.MethodPost) {
		return
	}
	if _, ok := s.authorizeGroupRequest(w, r, key); !ok {
		return
	}
	userKey := r.FormValue("user")
	deviceName := r.FormValue("device")
	s.store.mu.Lock()
	group := s.store.groups[key]
	for i := range group.Members {
		if group.Members[i].User == userKey && (deviceName == "" || group.Members[i].Device == deviceName) {
			group.Members[i].Disabled = disabled
		}
	}
	s.store.groups[key] = group
	s.store.mu.Unlock()
	s.writeSuccess(w, nil)
}

func (s *Server) handleGroupRename(w http.ResponseWriter, r *http.Request, key string) {
	if !requireMethod(s, w, r, http.MethodPost) {
		return
	}
	if _, ok := s.authorizeGroupRequest(w, r, key); !ok {
		return
	}
	name := r.FormValue("name")
	if name == "" {
		s.writeError(w, http.StatusBadRequest, "name", "is required")
		return
	}
	s.store.mu.Lock()
	group := s.store.groups[key]
	group.Name = name
	s.store.groups[key] = group
	s.store.mu.Unlock()
	s.writeSuccess(w, nil)
}
