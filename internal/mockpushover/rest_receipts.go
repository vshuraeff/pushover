package mockpushover

import (
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (s *Server) handleReceipts(w http.ResponseWriter, r *http.Request) {
	remainder := strings.TrimPrefix(r.URL.Path, "/1/receipts/")
	if strings.HasPrefix(remainder, "cancel_by_tag/") && strings.HasSuffix(remainder, ".json") {
		s.handleCancelByTag(w, r, strings.TrimSuffix(strings.TrimPrefix(remainder, "cancel_by_tag/"), ".json"))
		return
	}
	parts := strings.Split(remainder, "/")
	if len(parts) == 1 && strings.HasSuffix(parts[0], ".json") {
		s.handleReceiptStatus(w, r, strings.TrimSuffix(parts[0], ".json"))
		return
	}
	if len(parts) == 2 && parts[1] == "cancel.json" {
		s.handleCancelReceipt(w, r, parts[0])
		return
	}
	if len(parts) == 2 && parts[1] == "acknowledge.json" {
		s.handleAcknowledgeReceipt(w, r, parts[0])
		return
	}
	s.writeError(w, http.StatusNotFound, "request", "endpoint was not found")
}

func (s *Server) handleReceiptStatus(w http.ResponseWriter, r *http.Request, receiptID string) {
	if !requireMethod(s, w, r, http.MethodGet) {
		return
	}
	token := r.FormValue("token")
	if _, ok := s.requireApp(w, token); !ok {
		return
	}
	s.store.mu.Lock()
	receipt, ok := s.store.receipts[receiptID]
	if ok && receipt.Token == token && receipt.ExpiresAt > 0 && receipt.ExpiresAt <= time.Now().Unix() {
		receipt.Expired = 1
		s.store.receipts[receiptID] = receipt
	}
	s.store.mu.Unlock()
	if !ok || receipt.Token != token {
		s.writeError(w, http.StatusBadRequest, "receipt", "is invalid")
		return
	}
	s.writeSuccess(w, map[string]any{
		"acknowledged": receipt.Acknowledged, "acknowledged_at": receipt.AcknowledgedAt,
		"acknowledged_by": receipt.AcknowledgedBy, "last_delivered_at": receipt.LastDeliveredAt,
		"expires_at": receipt.ExpiresAt, "expired": receipt.Expired,
		"called_back": receipt.CalledBack, "called_back_at": receipt.CalledBackAt,
		"cancelled": receipt.Cancelled, "cancelled_at": receipt.CancelledAt,
		"cancelled_by_tag": receipt.CancelledByTag,
	})
}

func (s *Server) handleCancelReceipt(w http.ResponseWriter, r *http.Request, receiptID string) {
	if !requireMethod(s, w, r, http.MethodPost) {
		return
	}
	token := r.FormValue("token")
	if _, ok := s.requireApp(w, token); !ok {
		return
	}
	s.store.mu.Lock()
	receipt, ok := s.store.receipts[receiptID]
	if ok && receipt.Token == token {
		receipt.Cancelled = true
		receipt.CancelledAt = time.Now().Unix()
		s.store.receipts[receiptID] = receipt
	}
	s.store.mu.Unlock()
	if !ok || receipt.Token != token {
		s.writeError(w, http.StatusBadRequest, "receipt", "is invalid")
		return
	}
	s.writeSuccess(w, nil)
}

func (s *Server) handleCancelByTag(w http.ResponseWriter, r *http.Request, escapedTag string) {
	if !requireMethod(s, w, r, http.MethodPost) {
		return
	}
	token := r.FormValue("token")
	if _, ok := s.requireApp(w, token); !ok {
		return
	}
	tag, err := url.PathUnescape(escapedTag)
	if err != nil || tag == "" {
		s.writeError(w, http.StatusBadRequest, "tag", "is invalid")
		return
	}
	now := time.Now().Unix()
	cancelled := 0
	s.store.mu.Lock()
	for id, receipt := range s.store.receipts {
		if receipt.Token != token || receipt.Tag != tag || receipt.Cancelled {
			continue
		}
		receipt.Cancelled = true
		receipt.CancelledAt = now
		receipt.CancelledByTag = true
		s.store.receipts[id] = receipt
		cancelled++
	}
	s.store.mu.Unlock()
	s.writeSuccess(w, map[string]any{"cancelled": cancelled})
}

func (s *Server) handleAcknowledgeReceipt(w http.ResponseWriter, r *http.Request, receiptID string) {
	if !requireMethod(s, w, r, http.MethodPost) {
		return
	}
	secret := r.FormValue("secret")
	now := time.Now().Unix()
	s.store.mu.Lock()
	receipt, ok := s.store.receipts[receiptID]
	acknowledgedBy := ""
	if ok {
		for _, deviceID := range receipt.DeviceIDs {
			device, exists := s.store.devices[deviceID]
			if exists && device.Secret == secret && secret != "" {
				acknowledgedBy = device.User
				break
			}
		}
	}
	if acknowledgedBy != "" {
		receipt.Acknowledged = 1
		receipt.AcknowledgedAt = now
		receipt.AcknowledgedBy = acknowledgedBy
		s.store.receipts[receiptID] = receipt
		for deviceID, messages := range s.store.queues {
			for i := range messages {
				if messages[i].Receipt == receiptID {
					messages[i].Acked = 1
				}
			}
			s.store.queues[deviceID] = messages
		}
	}
	s.store.mu.Unlock()
	if !ok || acknowledgedBy == "" {
		s.writeError(w, http.StatusBadRequest, "secret", "is invalid")
		return
	}
	s.writeSuccess(w, nil)
}
