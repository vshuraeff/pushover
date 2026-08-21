package mockpushover

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	pathpkg "path"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Logger       *log.Logger
	InitialState *SeedState
	ErrorShape   ErrorShape
}

type Server struct {
	url         string
	listener    net.Listener
	httpServer  *http.Server
	client      *http.Client
	store       *store
	defaultSeed SeedState
	controlMux  *http.ServeMux
}

func Start(cfg Config) (*Server, error) {
	seed := SeedState{ErrorShape: cfg.ErrorShape}
	if cfg.InitialState != nil {
		var err error
		seed, err = cloneSeed(*cfg.InitialState)
		if err != nil {
			return nil, err
		}
		if cfg.ErrorShape != "" {
			seed.ErrorShape = cfg.ErrorShape
		}
	}
	defaultSeed, err := cloneSeed(seed)
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	s := &Server{
		url:         "http://" + listener.Addr().String(),
		listener:    listener,
		client:      &http.Client{Timeout: 5 * time.Second},
		store:       newStore(seed),
		defaultSeed: defaultSeed,
		controlMux:  http.NewServeMux(),
	}
	rootMux := http.NewServeMux()
	apiMux := http.NewServeMux()
	s.registerControlRoutes()
	s.registerRESTRoutes(apiMux)
	rootMux.Handle("/_control/", s.controlMux)
	rootMux.Handle("/", s.apiMiddleware(apiMux))
	s.httpServer = &http.Server{
		Handler:           rootMux,
		ReadHeaderTimeout: 5 * time.Second,
		ErrorLog:          cfg.Logger,
	}
	go func() {
		if serveErr := s.httpServer.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) && cfg.Logger != nil {
			cfg.Logger.Printf("mock pushover server: %v", serveErr)
		}
	}()
	return s, nil
}

func (s *Server) URL() string {
	return s.url
}

func (s *Server) Control() *ControlClient {
	return &ControlClient{baseURL: s.url, client: s.client}
}

// registercontrolroute is the additive extension point for future control-plane transports.
func (s *Server) RegisterControlRoute(pattern string, handler http.Handler) {
	s.controlMux.Handle(pattern, handler)
}

func (s *Server) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := s.httpServer.Shutdown(ctx)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) registerRESTRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/1/messages.json", s.handleMessagesRoute)
	mux.HandleFunc("/1/receipts/", s.handleReceipts)
	mux.HandleFunc("/1/users/validate.json", s.handleValidateUser)
	mux.HandleFunc("/1/users/login.json", s.handleLogin)
	mux.HandleFunc("/1/devices.json", s.handleCreateDevice)
	mux.HandleFunc("/1/devices/", s.handleDeviceRoutes)
	mux.HandleFunc("/1/groups.json", s.handleGroups)
	mux.HandleFunc("/1/groups/", s.handleGroup)
	mux.HandleFunc("/1/glances.json", s.handleGlances)
	mux.HandleFunc("/1/sounds.json", s.handleSounds)
	mux.HandleFunc("/1/licenses/assign.json", s.handleAssignLicense)
	mux.HandleFunc("/1/licenses.json", s.handleLicenses)
	mux.HandleFunc("/1/teams.json", s.handleTeams)
	mux.HandleFunc("/1/teams/add_user.json", s.handleTeamAddUser)
	mux.HandleFunc("/1/teams/revoke_invitation.json", s.handleTeamRevokeInvitation)
	mux.HandleFunc("/1/teams/remove_user.json", s.handleTeamRemoveUser)
	mux.HandleFunc("/1/subscriptions/migrate.json", s.handleSubscriptionMigrate)
	mux.HandleFunc("/1/apps/limits.json", s.handleAppLimits)
	mux.HandleFunc("/icons/", s.handleIcon)
	mux.HandleFunc("/sounds/", s.handleSoundAsset)
}

func (s *Server) apiMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
		parseErr := parseRequestFields(r)
		if r.MultipartForm != nil {
			defer r.MultipartForm.RemoveAll()
		}
		s.recordRequest(r)
		if rule, ok := s.takeFault(r.Method, r.URL.Path); ok {
			writeFault(w, rule)
			return
		}
		if parseErr != nil {
			s.writeError(w, http.StatusBadRequest, "request", "request body is invalid")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func parseRequestFields(r *http.Request) error {
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		return r.ParseMultipartForm(6 << 20)
	}
	return r.ParseForm()
}

func (s *Server) recordRequest(r *http.Request) {
	fields := make(map[string][]string, len(r.Form))
	for key, values := range r.Form {
		fields[key] = append([]string(nil), values...)
	}
	if r.MultipartForm != nil {
		for key, files := range r.MultipartForm.File {
			for _, file := range files {
				fields[key] = append(fields[key], file.Filename)
			}
		}
	}
	entry := JournaledRequest{
		Method: r.Method, Path: r.URL.Path, Fields: fields,
		Headers: r.Header.Clone(), Timestamp: time.Now().UTC(),
	}
	s.store.mu.Lock()
	s.store.journal = append(s.store.journal, entry)
	s.store.mu.Unlock()
}

func (s *Server) takeFault(method, requestPath string) (FaultRule, bool) {
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	for i := range s.store.faults {
		rule := &s.store.faults[i]
		methodMatches := rule.Method == "" || strings.EqualFold(rule.Method, method)
		pathMatches := matchesPathGlob(rule.PathGlob, requestPath)
		if !methodMatches || !pathMatches || rule.Times <= 0 {
			continue
		}
		rule.Times--
		return *rule, true
	}
	return FaultRule{}, false
}

func matchesPathGlob(pattern, requestPath string) bool {
	if matched, err := pathpkg.Match(pattern, requestPath); err == nil && matched {
		return true
	}
	if !strings.Contains(pattern, "*") {
		return false
	}
	parts := strings.Split(pattern, "*")
	remaining := requestPath
	if parts[0] != "" {
		if !strings.HasPrefix(remaining, parts[0]) {
			return false
		}
		remaining = strings.TrimPrefix(remaining, parts[0])
	}
	for _, part := range parts[1 : len(parts)-1] {
		index := strings.Index(remaining, part)
		if index < 0 {
			return false
		}
		remaining = remaining[index+len(part):]
	}
	last := parts[len(parts)-1]
	return last == "" || strings.HasSuffix(remaining, last)
}

func writeFault(w http.ResponseWriter, rule FaultRule) {
	status := rule.Status
	if status == 0 {
		status = http.StatusInternalServerError
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	switch body := rule.Body.(type) {
	case nil:
		_, _ = io.WriteString(w, http.StatusText(status))
	case string:
		_, _ = io.WriteString(w, body)
	case []byte:
		_, _ = w.Write(body)
	case json.RawMessage:
		_, _ = w.Write(body)
	default:
		_ = json.NewEncoder(w).Encode(body)
	}
}

func (s *Server) writeSuccess(w http.ResponseWriter, extra map[string]any) {
	payload := map[string]any{"status": 1, "request": randomID()}
	for key, value := range extra {
		payload[key] = value
	}
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) writeError(w http.ResponseWriter, status int, field, message string) {
	s.store.mu.RLock()
	shape := s.store.errorShape
	s.store.mu.RUnlock()
	payload := map[string]any{"status": 0, "request": randomID()}
	if shape == ErrorShapeObject {
		payload["errors"] = map[string][]string{field: {message}}
	} else {
		payload["errors"] = []string{humanError(field, message)}
	}
	writeJSON(w, status, payload)
}

func (s *Server) writeFieldErrors(w http.ResponseWriter, status int, errors map[string][]string) {
	writeJSON(w, status, map[string]any{
		"status": 0, "request": randomID(), "errors": errors,
	})
}

func humanError(field, message string) string {
	if strings.Contains(message, field) || field == "request" {
		return message
	}
	return field + " " + message
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func randomID() string {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	data[6] = (data[6] & 0x0f) | 0x40
	data[8] = (data[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(data[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s", encoded[:8], encoded[8:12], encoded[12:16], encoded[16:20], encoded[20:])
}

func randomKey(length int) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	data := make([]byte, length)
	random := make([]byte, length)
	if _, err := rand.Read(random); err != nil {
		fallback := hex.EncodeToString([]byte(strconv.FormatInt(time.Now().UnixNano(), 10)))
		for i := range data {
			data[i] = fallback[i%len(fallback)]
		}
		return string(data)
	}
	for i := range data {
		data[i] = alphabet[int(random[i])%len(alphabet)]
	}
	return string(data)
}

func requireMethod(s *Server, w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}
	s.writeError(w, http.StatusMethodNotAllowed, "request", "method is not allowed")
	return false
}

func (s *Server) app(token string) (AppToken, bool) {
	s.store.mu.RLock()
	defer s.store.mu.RUnlock()
	app, ok := s.store.apps[token]
	return app, ok
}

func (s *Server) requireApp(w http.ResponseWriter, token string) (AppToken, bool) {
	app, ok := s.app(token)
	if !ok {
		s.writeError(w, http.StatusBadRequest, "token", "is invalid")
	}
	return app, ok
}
