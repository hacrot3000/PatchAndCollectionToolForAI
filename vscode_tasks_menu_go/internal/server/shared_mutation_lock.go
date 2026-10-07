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

func (l *sharedMutationLock) bindOperationResource(operation, resourceID string) bool {
	operation = strings.TrimSpace(operation)
	resourceID = strings.TrimSpace(resourceID)
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.holder == nil || l.holder.Operation != operation {
		return false
	}
	l.holder.ResourceID = resourceID
	return true
}

func (l *sharedMutationLock) releaseOperation(operation string) bool {
	operation = strings.TrimSpace(operation)
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.holder == nil || l.holder.Operation != operation {
		return false
	}
	l.token = ""
	l.holder = nil
	return true
}

func sharedPatchMutationRequired(mode, uiMode string) bool {
	mode = strings.ToLower(strings.TrimSpace(mode))
	uiMode = strings.ToLower(strings.TrimSpace(uiMode))
	if uiMode == "" {
		uiMode = "native"
	}
	switch mode {
	case "history", "plan", "health":
		return false
	case "", "queue", "resume":
		// Native Queue/Resume are selector/read surfaces until a concrete PATCH
		// execution command is submitted. Holding the global workspace lock while
		// waiting at those prompts blocks unrelated Git/self-update work for no
		// safety benefit. Terminal UI remains conservative because its raw PTY
		// input bypasses the structured command endpoints where deferred locking
		// can be enforced.
		return uiMode == "terminal"
	default:
		return true
	}
}

func (s *Server) refreshSharedMutationLock() {
	holder, ok := s.sharedMutation.snapshot()
	if !ok {
		return
	}
	switch holder.Operation {
	case "patch.run":
		if holder.ResourceID == "" || s.Sessions == nil {
			return
		}
		meta, exists := s.Sessions.Metadata(holder.ResourceID)
		if !exists || meta.Status != "running" {
			s.sharedMutation.releaseResource(holder.Operation, holder.ResourceID)
		}
	case "selfupdate.run":
		s.refreshSharedSelfUpdateMutationLock(holder)
	}
}

func (s *Server) releaseSharedMutationForSession(sessionID string) {
	s.sharedMutation.releaseResource("patch.run", strings.TrimSpace(sessionID))
}

func (s *Server) ensureSharedPatchMutationForSession(w http.ResponseWriter, r *http.Request, sessionID string) (sharedMutationLease, bool) {
	sessionID = strings.TrimSpace(sessionID)
	if !s.Config.SharedServerEnabled {
		return sharedMutationLease{}, true
	}
	if sessionID == "" {
		http.Error(w, "Patch session id is required for workspace mutation lock", http.StatusBadRequest)
		return sharedMutationLease{}, false
	}
	s.refreshSharedMutationLock()
	if holder, ok := s.sharedMutation.snapshot(); ok &&
		holder.Operation == "patch.run" && holder.ResourceID == sessionID {
		return sharedMutationLease{}, true
	}
	lease, ok := s.acquireSharedMutation(w, r, "patch.run", "")
	if !ok {
		return sharedMutationLease{}, false
	}
	if lease.token != "" && !s.sharedMutation.bindResource(lease.token, sessionID) {
		s.releaseSharedMutation(lease)
		http.Error(w, "workspace mutation lock lost while binding Patch session", http.StatusServiceUnavailable)
		return sharedMutationLease{}, false
	}
	return lease, true
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

type sharedMutationStatusResponse struct {
	Locked bool                 `json:"locked"`
	Holder *sharedMutationOwner `json:"holder,omitempty"`
}

func (s *Server) sharedMutationStatus(w http.ResponseWriter, r *http.Request) {
	if !s.Config.SharedServerEnabled {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.refreshSharedMutationLock()
		holder, ok := s.sharedMutation.snapshot()
		if !ok {
			writeJSON(w, http.StatusOK, sharedMutationStatusResponse{Locked: false})
			return
		}
		writeJSON(w, http.StatusOK, sharedMutationStatusResponse{Locked: true, Holder: &holder})
	case http.MethodDelete:
		if !s.requireSharedActionPermission(w, r, identity.PermissionProjectAdmin, "mutation.force_release", "workspace") {
			return
		}
		if r.URL.Query().Get("confirm") != "1" {
			http.Error(w, "explicit confirmation is required", http.StatusBadRequest)
			return
		}
		s.refreshSharedMutationLock()
		holder, ok := s.sharedMutation.snapshot()
		if !ok {
			writeJSON(w, http.StatusOK, sharedMutationStatusResponse{Locked: false})
			return
		}
		expectedOperation := strings.TrimSpace(r.URL.Query().Get("operation"))
		expectedResource := strings.TrimSpace(r.URL.Query().Get("resource_id"))
		if expectedOperation == "" || expectedOperation != holder.Operation || expectedResource != holder.ResourceID {
			http.Error(w, "mutation lock changed; refresh before releasing it", http.StatusConflict)
			return
		}
		if holder.Operation != "patch.run" || holder.ResourceID == "" {
			http.Error(w, "only a Patch session lock can be force-released safely from the UI", http.StatusConflict)
			return
		}
		if s.Sessions != nil {
			if meta, exists := s.Sessions.Metadata(holder.ResourceID); exists && meta.Status == "running" {
				if err := s.Sessions.Stop(holder.ResourceID); err != nil {
					http.Error(w, "cannot stop active Patch session before releasing lock", http.StatusConflict)
					return
				}
			}
		}
		s.releaseSharedMutationForSession(holder.ResourceID)
		principal, _ := PrincipalFromContext(r.Context())
		s.appendSharedAudit(r, &principal, nil, "mutation.force_release", "workspace", holder.ResourceID, "success", map[string]any{
			"holder_user_id":     holder.UserID,
			"holder_username":    holder.Username,
			"holder_operation":   holder.Operation,
			"holder_resource_id": holder.ResourceID,
		})
		writeJSON(w, http.StatusOK, sharedMutationStatusResponse{Locked: false})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
