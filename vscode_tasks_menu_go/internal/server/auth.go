package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	authFailureWindow   = time.Minute
	authFailureLimit    = 8
	authBlockDuration   = 5 * time.Minute
	authFailureMapLimit = 1024
)

type authFailureState struct {
	Count        int
	WindowStart  time.Time
	BlockedUntil time.Time
	LastSeen     time.Time
}

func constantTimeCredentialEqual(got, want string) bool {
	gotHash := sha256.Sum256([]byte(got))
	wantHash := sha256.Sum256([]byte(want))
	return subtle.ConstantTimeCompare(gotHash[:], wantHash[:]) == 1
}

func authRemoteKey(remote string) string {
	remote = strings.TrimSpace(remote)
	if host, _, err := net.SplitHostPort(remote); err == nil {
		return strings.Trim(host, "[]")
	}
	if remote == "" {
		return "unknown"
	}
	return remote
}

func (s *Server) authBlocked(remote string, now time.Time) (bool, time.Duration) {
	key := authRemoteKey(remote)
	s.authMu.Lock()
	defer s.authMu.Unlock()
	state, ok := s.authFailures[key]
	if !ok {
		return false, 0
	}
	state.LastSeen = now
	if state.BlockedUntil.After(now) {
		s.authFailures[key] = state
		return true, time.Until(state.BlockedUntil)
	}
	if now.Sub(state.WindowStart) > authFailureWindow {
		delete(s.authFailures, key)
		return false, 0
	}
	s.authFailures[key] = state
	return false, 0
}

func (s *Server) authRecordFailure(remote string, now time.Time) {
	key := authRemoteKey(remote)
	s.authMu.Lock()
	defer s.authMu.Unlock()
	if s.authFailures == nil {
		s.authFailures = make(map[string]authFailureState)
	}
	if len(s.authFailures) >= authFailureMapLimit {
		for candidate, state := range s.authFailures {
			if now.Sub(state.LastSeen) > authBlockDuration+authFailureWindow {
				delete(s.authFailures, candidate)
			}
		}
		if len(s.authFailures) >= authFailureMapLimit {
			var oldestKey string
			var oldest time.Time
			for candidate, state := range s.authFailures {
				if oldestKey == "" || state.LastSeen.Before(oldest) {
					oldestKey, oldest = candidate, state.LastSeen
				}
			}
			delete(s.authFailures, oldestKey)
		}
	}
	state := s.authFailures[key]
	if state.WindowStart.IsZero() || now.Sub(state.WindowStart) > authFailureWindow {
		state = authFailureState{WindowStart: now}
	}
	state.Count++
	state.LastSeen = now
	if state.Count >= authFailureLimit {
		state.BlockedUntil = now.Add(authBlockDuration)
	}
	s.authFailures[key] = state
}

func (s *Server) authRecordSuccess(remote string) {
	key := authRemoteKey(remote)
	s.authMu.Lock()
	defer s.authMu.Unlock()
	delete(s.authFailures, key)
}

func writeAuthRateLimit(w http.ResponseWriter, retry time.Duration) {
	seconds := int(retry.Seconds())
	if seconds < 1 {
		seconds = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
	http.Error(w, "too many authentication failures", http.StatusTooManyRequests)
}
