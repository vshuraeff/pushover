package mockpushover

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type ControlClient struct {
	baseURL string
	client  *http.Client
}

type WSStatus struct {
	Connected     bool   `json:"connected"`
	LoginDeviceID string `json:"login_device_id"`
	LoginOK       bool   `json:"login_ok"`
	ConnectCount  int    `json:"connect_count"`
}

func (s *Server) registerControlRoutes() {
	s.RegisterControlRoute("POST /_control/reset", http.HandlerFunc(s.handleControlReset))
	s.RegisterControlRoute("POST /_control/seed", http.HandlerFunc(s.handleControlSeed))
	s.RegisterControlRoute("POST /_control/fault", http.HandlerFunc(s.handleControlFault))
	s.RegisterControlRoute("GET /_control/requests", http.HandlerFunc(s.handleControlRequests))
	s.RegisterControlRoute("POST /_control/ws/send", http.HandlerFunc(s.handleControlWSSend))
	s.RegisterControlRoute("POST /_control/ws/close", http.HandlerFunc(s.handleControlWSClose))
	s.RegisterControlRoute("GET /_control/ws/status", http.HandlerFunc(s.handleControlWSStatus))
}

func (s *Server) handleControlReset(w http.ResponseWriter, _ *http.Request) {
	seed, err := cloneSeed(s.defaultSeed)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"status": 0})
		return
	}
	s.store.replace(seed)
	s.resetWebSocket()
	writeJSON(w, http.StatusOK, map[string]any{"status": 1})
}

func (s *Server) handleControlSeed(w http.ResponseWriter, r *http.Request) {
	var seed SeedState
	if err := decodeControlJSON(r, &seed); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"status": 0, "error": err.Error()})
		return
	}
	s.store.replacePreservingErrorShape(seed)
	writeJSON(w, http.StatusOK, map[string]any{"status": 1})
}

func (s *Server) handleControlFault(w http.ResponseWriter, r *http.Request) {
	var rule FaultRule
	if err := decodeControlJSON(r, &rule); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"status": 0, "error": err.Error()})
		return
	}
	if rule.PathGlob == "" || rule.Status < 100 || rule.Status > 599 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"status": 0, "error": "path_glob and a valid status are required"})
		return
	}
	if rule.Times == 0 {
		rule.Times = 1
	}
	if rule.Times < 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"status": 0, "error": "times must be positive"})
		return
	}
	rule.Method = strings.ToUpper(rule.Method)
	s.store.mu.Lock()
	s.store.faults = append(s.store.faults, rule)
	s.store.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"status": 1})
}

func (s *Server) handleControlRequests(w http.ResponseWriter, _ *http.Request) {
	s.store.mu.RLock()
	requests := make([]JournaledRequest, len(s.store.journal))
	for i := range s.store.journal {
		requests[i] = s.store.journal[i]
		requests[i].Headers = s.store.journal[i].Headers.Clone()
		requests[i].Fields = make(map[string][]string, len(s.store.journal[i].Fields))
		for key, values := range s.store.journal[i].Fields {
			requests[i].Fields[key] = append([]string(nil), values...)
		}
	}
	s.store.mu.RUnlock()
	writeJSON(w, http.StatusOK, requests)
}

func (s *Server) handleControlWSSend(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Frame string `json:"frame"`
	}
	if err := decodeControlJSON(r, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"status": 0, "error": err.Error()})
		return
	}
	if len(request.Frame) != 1 || !strings.Contains("#!REA", request.Frame) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"status": 0, "error": "frame must be one of #, !, R, E, or A"})
		return
	}
	if err := s.sendWebSocketFrame(request.Frame[0]); err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{"status": 0, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": 1})
}

func (s *Server) handleControlWSClose(w http.ResponseWriter, _ *http.Request) {
	s.closeWebSocket()
	writeJSON(w, http.StatusOK, map[string]any{"status": 1})
}

func (s *Server) handleControlWSStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.webSocketStatus())
}

func decodeControlJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 16<<20))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func (c *ControlClient) Seed(seed SeedState) error {
	return c.post("/_control/seed", seed)
}

func (c *ControlClient) Reset() error {
	return c.post("/_control/reset", struct{}{})
}

func (c *ControlClient) Fault(rule FaultRule) error {
	return c.post("/_control/fault", rule)
}

func (c *ControlClient) Requests() ([]JournaledRequest, error) {
	request, err := http.NewRequest(http.MethodGet, c.baseURL+"/_control/requests", nil)
	if err != nil {
		return nil, err
	}
	response, err := c.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
		return nil, fmt.Errorf("control requests: status %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var requests []JournaledRequest
	if err := json.NewDecoder(response.Body).Decode(&requests); err != nil {
		return nil, err
	}
	return requests, nil
}

func (c *ControlClient) WSSend(frame string) error {
	return c.post("/_control/ws/send", map[string]string{"frame": frame})
}

func (c *ControlClient) WSClose() error {
	return c.post("/_control/ws/close", struct{}{})
}

func (c *ControlClient) WSStatus() (WSStatus, error) {
	request, err := http.NewRequest(http.MethodGet, c.baseURL+"/_control/ws/status", nil)
	if err != nil {
		return WSStatus{}, err
	}
	response, err := c.client.Do(request)
	if err != nil {
		return WSStatus{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
		return WSStatus{}, fmt.Errorf("control ws status: status %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var status WSStatus
	if err := json.NewDecoder(response.Body).Decode(&status); err != nil {
		return WSStatus{}, err
	}
	return status, nil
}

func (c *ControlClient) post(endpoint string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequest(http.MethodPost, c.baseURL+endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
		return fmt.Errorf("control request %s: status %d: %s", endpoint, response.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	return nil
}
