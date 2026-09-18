package server

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

const browserLeaseHeader = "X-TaskMenu-Lease"

type browserLease struct {
	mu      sync.Mutex
	current string
	revoked chan struct{}
}

func newBrowserLease() *browserLease {
	return &browserLease{}
}

func (l *browserLease) acquire() (string, error) {
	token, err := newBrowserLeaseToken()
	if err != nil {
		return "", err
	}
	l.mu.Lock()
	if l.revoked != nil {
		close(l.revoked)
	}
	l.current = token
	l.revoked = make(chan struct{})
	l.mu.Unlock()
	return token, nil
}

func (l *browserLease) active() bool {
	l.mu.Lock()
	active := l.current != ""
	l.mu.Unlock()
	return active
}

func (l *browserLease) valid(token string) bool {
	token = strings.TrimSpace(token)
	l.mu.Lock()
	ok := l.current == "" || (token != "" && token == l.current)
	l.mu.Unlock()
	return ok
}

func (l *browserLease) owns(token string) bool {
	token = strings.TrimSpace(token)
	l.mu.Lock()
	ok := l.current != "" && token != "" && token == l.current
	l.mu.Unlock()
	return ok
}

func (l *browserLease) watch(token string) (<-chan struct{}, bool) {
	token = strings.TrimSpace(token)
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.current == "" {
		return nil, true
	}
	if token == "" || token != l.current {
		return nil, false
	}
	return l.revoked, true
}

func newBrowserLeaseToken() (string, error) {
	var raw [24]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate browser lease: %w", err)
	}
	return hex.EncodeToString(raw[:]), nil
}

func (s *Server) browserLeaseState() *browserLease {
	s.browserLeaseMu.Lock()
	defer s.browserLeaseMu.Unlock()
	if s.browserLease == nil {
		s.browserLease = newBrowserLease()
	}
	return s.browserLease
}

func (s *Server) browserLeaseAPI(w http.ResponseWriter, r *http.Request) {
	lease := s.browserLeaseState()
	switch r.Method {
	case http.MethodPost:
		token, err := lease.acquire()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"lease": token})
	case http.MethodGet:
		if !lease.owns(r.Header.Get(browserLeaseHeader)) {
			writeLeaseRevoked(w)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func internalLoopbackLeaseExempt(r *http.Request) bool {
	if r.Method != http.MethodPost || r.URL.Path != "/api/state/tasks" || r.URL.Query().Get("scope") != "self-update" {
		return false
	}
	switch r.URL.Query().Get("action") {
	case "handoff", "detach":
		return loopbackRemote(r.RemoteAddr)
	default:
		return false
	}
}

func (s *Server) requireBrowserLease(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/browser/lease" || r.URL.Path == "/api/health" || internalLoopbackLeaseExempt(r) {
			next.ServeHTTP(w, r)
			return
		}
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			lease := s.browserLeaseState()
			if lease.active() && !lease.valid(r.Header.Get(browserLeaseHeader)) {
				writeLeaseRevoked(w)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func writeLeaseRevoked(w http.ResponseWriter) {
	w.Header().Set("X-TaskMenu-Lease-Revoked", "1")
	http.Error(w, "browser control lease revoked; reload this page to take control", http.StatusConflict)
}
