package server

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/approval"
	"bletonfc/vscode_tasks_menu/internal/identity"
)

func (s *Server) dangerousApprovalStore() (*approval.Store, error) {
	s.approvalStoreMu.Lock()
	defer s.approvalStoreMu.Unlock()
	if s.approvalStore != nil {
		return s.approvalStore, nil
	}
	store, err := approval.NewStore(filepath.Join(s.Workspace, ".vscode", "vscode_tasks_menu.approvals.json"))
	if err != nil {
		return nil, err
	}
	s.approvalStore = store
	return store, nil
}

func approvalRequester(r *http.Request) (id, username string) {
	if principal, ok := PrincipalFromContext(r.Context()); ok {
		return string(principal.UserID), principal.Username
	}
	return "local", "local"
}

func (s *Server) approvalPolicyAPI(w http.ResponseWriter, r *http.Request) {
	store, err := s.dangerousApprovalStore()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	switch r.Method {
	case http.MethodGet:
		policy, err := store.Policy()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, policy)
	case http.MethodPut:
		var policy approval.Policy
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&policy); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		saved, err := store.SetPolicy(policy)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.auditSharedSuccess(r, "approval.policy.update", "approval_policy", "project", map[string]any{"rules": len(saved.Rules)})
		writeJSON(w, http.StatusOK, saved)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) approvalsAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	store, err := s.dangerousApprovalStore()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	items, err := store.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"requests": items})
}

func (s *Server) approvalRequestAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Action       string `json:"action"`
		Resource     string `json:"resource"`
		Confirmation string `json:"confirmation,omitempty"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	store, err := s.dangerousApprovalStore()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	mode, err := store.ModeFor(req.Action)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	switch mode {
	case approval.ModeOff:
		writeJSON(w, http.StatusOK, map[string]any{"required": false, "mode": approval.ModeOff})
		return
	case approval.ModeConfirm:
		if strings.TrimSpace(req.Confirmation) != "CONFIRM" {
			writeJSON(w, http.StatusConflict, map[string]any{
				"required": true, "mode": mode, "expected": "CONFIRM",
				"message": "Type CONFIRM to authorize this dangerous action.",
			})
			return
		}
	case approval.ModeType:
		if strings.TrimSpace(req.Confirmation) != strings.TrimSpace(req.Resource) || strings.TrimSpace(req.Resource) == "" {
			writeJSON(w, http.StatusConflict, map[string]any{
				"required": true, "mode": mode, "expected": strings.TrimSpace(req.Resource),
				"message": "Type the exact resource name to authorize this dangerous action.",
			})
			return
		}
	case approval.ModeAdmin:
	default:
		http.Error(w, "unsupported approval mode", http.StatusBadRequest)
		return
	}
	requesterID, requester := approvalRequester(r)
	created, err := store.Create(req.Action, req.Resource, requesterID, requester, strings.TrimSpace(req.Confirmation))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.auditSharedSuccess(r, "approval.request", "approval", created.ID, map[string]any{
		"action": created.Action, "resource": created.Resource, "mode": created.Mode, "status": created.Status,
	})
	status := http.StatusCreated
	if created.Status == "pending" {
		status = http.StatusAccepted
	}
	writeJSON(w, status, map[string]any{"required": true, "request": created})
}

func (s *Server) approvalResolveAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	principal, ok := PrincipalFromContext(r.Context())
	resolverID := "local"
	if ok {
		resolverID = string(principal.UserID)
		if !principal.Allowed(identity.PermissionApprovalsManage) && !principal.Allowed(identity.PermissionProjectAdmin) {
			writePermissionDenied(w)
			return
		}
	}
	store, err := s.dangerousApprovalStore()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resolved, err := store.Resolve(req.ID, resolverID, req.Status)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	s.auditSharedSuccess(r, "approval.resolve", "approval", resolved.ID, map[string]any{
		"action": resolved.Action, "resource": resolved.Resource, "status": resolved.Status,
	})
	writeJSON(w, http.StatusOK, resolved)
}

func (s *Server) requireDangerousApproval(w http.ResponseWriter, r *http.Request, action, resource string) bool {
	store, err := s.dangerousApprovalStore()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return false
	}
	mode, err := store.ModeFor(action)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return false
	}
	if mode == approval.ModeOff {
		return true
	}
	approvalID := strings.TrimSpace(r.Header.Get("X-TaskDeck-Approval-ID"))
	if approvalID == "" {
		writeJSON(w, http.StatusPreconditionRequired, map[string]any{
			"approval_required": true, "action": action, "resource": resource, "mode": mode,
		})
		return false
	}
	requesterID, _ := approvalRequester(r)
	granted, err := store.Consume(approvalID, action, resource, requesterID)
	if err != nil {
		writeJSON(w, http.StatusPreconditionFailed, map[string]any{
			"approval_required": true, "action": action, "resource": resource, "mode": mode, "error": err.Error(),
		})
		return false
	}
	s.auditSharedSuccess(r, "approval.consume", "approval", granted.ID, map[string]any{
		"action": action, "resource": resource, "mode": granted.Mode,
	})
	return true
}
