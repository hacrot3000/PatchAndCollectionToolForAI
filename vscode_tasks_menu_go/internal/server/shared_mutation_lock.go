package server

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"bletonfc/vscode_tasks_menu/internal/identity"
)

type sharedMutationOwner struct {
	UserID     identity.ID `json:"user_id"`
	Username   string      `json:"username"`
	Operation  string      `json:"operation"`
	ResourceID string      `json:"resource_id,omitempty"`
	AcquiredAt time.Time   `json:"acquired_at"`
}

type sharedMutationLease struct {
	token string
	owner sharedMutationOwner
}

// sharedMutationLock is process-local because one TaskDeck daemon owns one
// workspace. Its zero value is ready for use and contains no filesystem or
// network dependency.
type sharedMutationLock struct {
	mu     sync.Mutex
	token  string
	holder *sharedMutationOwner
}

func (l *sharedMutationLock) acquire(principal identity.Principal, operation, resourceID string, now time.Time) (sharedMutationLease, *sharedMutationOwner, error) {
	operation = strings.TrimSpace(operation)
	resourceID = strings.TrimSpace(resourceID)
	if principal.UserID == "" || operation == "" || now.IsZero() {
		return sharedMutationLease{}, nil, fmt.Errorf("shared mutation lock requires principal, operation and timestamp")
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.holder != nil {
		conflict := *l.holder
		return sharedMutationLease{}, &conflict, nil
	}
	tokenID, err := identity.NewID()
	if err != nil {
		return sharedMutationLease{}, nil, err
	}
	owner := sharedMutationOwner{
		UserID: principal.UserID,
		Username: principal.Username,
		Operation: operation,
		ResourceID: resourceID,
		AcquiredAt: now.UTC(),
	}
	l.token = string(tokenID)
	l.holder = &owner
	return sharedMutationLease{token: l.token, owner: owner}, nil, nil
}

func (l *sharedMutationLock) bindResource(token, resourceID string) bool {
	resourceID = strings.TrimSpace(resourceID)
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.holder == nil || token == "" || token != l.token {
		return false
	}
	l.holder.ResourceID = resourceID
	return true
}

func (l *sharedMutationLock) release(token string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.holder == nil || token == "" || token != l.token {
		return false
	}
	l.token = ""
	l.holder = nil
	return true
}

func (l *sharedMutationLock) snapshot() (sharedMutationOwner, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.holder == nil {
		return sharedMutationOwner{}, false
	}
	return *l.holder, true
}

func (l *sharedMutationLock) releaseResource(operation, resourceID string) bool {
	operation = strings.TrimSpace(operation)
	resourceID = strings.TrimSpace(resourceID)
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.holder == nil || l.holder.Operation != operation || l.holder.ResourceID != resourceID {
		return false
	}
	l.token = ""
	l.holder = nil
	return true
}

func sharedPatchMutationRequired(mode string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "history", "plan", "health":
		return false
	default:
		return true
	}
}

func (s *Server) refreshSharedMutationLock() {
	holder, ok := s.sharedMutation.snapshot()
	if !ok || holder.Operation != "patch.run" || holder.ResourceID == "" || s.Sessions == nil {
		return
	}
	meta, exists := s.Sessions.Metadata(holder.ResourceID)
	if !exists || meta.Status != "running" {
		s.sharedMutation.releaseResource(holder.Operation, holder.ResourceID)
	}
}

func (s *Server) releaseSharedMutationForSession(sessionID string) {
	s.sharedMutation.releaseResource("patch.run", strings.TrimSpace(sessionID))
}

func (s *Server) acquireSharedMutation(w http.ResponseWriter, r *http.Request, operation, resourceID string) (sharedMutationLease, bool) {
	if !s.Config.SharedServerEnabled {
		return sharedMutationLease{}, true
	}
	s.refreshSharedMutationLock()
	principal, ok := PrincipalFromContext(r.Context())
	if !ok {
		sharedAuthError(w, identity.ErrUnauthenticated)
		return sharedMutationLease{}, false
	}
	lease, conflict, err := s.sharedMutation.acquire(principal, operation, resourceID, time.Now().UTC())
	if err != nil {
		http.Error(w, "workspace mutation lock unavailable", http.StatusServiceUnavailable)
		return sharedMutationLease{}, false
	}
	if conflict != nil {
		s.appendSharedAudit(r, &principal, nil, "mutation.lock_conflict", "workspace", resourceID, "denied", map[string]any{
			"requested_operation": operation,
			"holder_user_id":      conflict.UserID,
			"holder_username":     conflict.Username,
			"holder_operation":    conflict.Operation,
			"holder_resource_id":  conflict.ResourceID,
			"holder_acquired_at":  conflict.AcquiredAt,
		})
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":  "workspace mutation locked",
			"holder": conflict,
		})
		return sharedMutationLease{}, false
	}
	return lease, true
}

func (s *Server) releaseSharedMutation(lease sharedMutationLease) {
	if lease.token != "" {
		s.sharedMutation.release(lease.token)
	}
}
