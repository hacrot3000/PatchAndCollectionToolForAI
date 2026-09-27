package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"bletonfc/vscode_tasks_menu/internal/dbadapter"
	"bletonfc/vscode_tasks_menu/internal/dbprofile"
	"bletonfc/vscode_tasks_menu/internal/dbsession"
)

const databaseOpenTimeout = 20 * time.Second

func (s *Server) databaseSessionManager() (*dbsession.Manager, error) {
	if s.DBSessions == nil {
		return nil, errors.New("database session manager is not configured")
	}
	return s.DBSessions, nil
}

func (s *Server) dbSessions(w http.ResponseWriter, r *http.Request) {
	manager, err := s.databaseSessionManager()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]interface{}{"sessions": manager.List()})
	case http.MethodPost:
		var req struct {
			ProfileID string `json:"profile_id"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		req.ProfileID = strings.TrimSpace(req.ProfileID)
		if req.ProfileID == "" {
			http.Error(w, "database profile id is required", http.StatusBadRequest)
			return
		}
		profileStore, err := s.databaseProfileStore()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		profile, err := profileStore.Get(req.ProfileID)
		if errors.Is(err, dbprofile.ErrProfileNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if profile.Transport != dbprofile.TransportDirect {
			http.Error(w, "database SSH tunnel transport is not enabled yet", http.StatusConflict)
			return
		}

		connect := dbadapter.ConnectPayload{
			Host:     profile.Host,
			Port:     profile.Port,
			Username: profile.Username,
			Database: profile.Database,
			File:     profile.File,
			ReadOnly: profile.ReadOnly,
			Options:  profile.Options,
		}
		if profile.SecretRef != "" {
			secrets, err := s.connectionSecretStore()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			secret, err := secrets.Get(profile.SecretRef)
			if err != nil {
				http.Error(w, "database authentication secret is unavailable", http.StatusConflict)
				return
			}
			connect.Secret = string(secret)
			for i := range secret {
				secret[i] = 0
			}
		}
		openCtx, cancel := context.WithTimeout(r.Context(), databaseOpenTimeout)
		meta, err := manager.Open(
			openCtx,
			profile.ID,
			profile.AdapterID,
			dbadapter.ProcessOptions{Dir: s.Workspace},
			connect,
		)
		connect.Secret = ""
		cancel()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, http.StatusCreated, meta)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) dbSessionItem(w http.ResponseWriter, r *http.Request) {
	tail := strings.TrimPrefix(r.URL.Path, "/api/db/sessions/")
	parts := strings.Split(strings.Trim(tail, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	manager, err := s.databaseSessionManager()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	id := parts[0]

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			meta, err := manager.Get(id)
			if errors.Is(err, dbsession.ErrSessionNotFound) {
				http.NotFound(w, r)
				return
			}
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusOK, meta)
		case http.MethodDelete:
			if err := manager.CloseSession(id); err != nil {
				if errors.Is(err, dbsession.ErrSessionNotFound) {
					http.NotFound(w, r)
					return
				}
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	if len(parts) != 2 || parts[1] != "request" || r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	var req struct {
		Operation dbadapter.Operation `json:"operation"`
		Payload   json.RawMessage     `json:"payload,omitempty"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, dbadapter.MaxMessageBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	payload, err := normalizeBrowserDBOperation(req.Operation, req.Payload)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	requestCtx, cancel := context.WithTimeout(r.Context(), databaseOpenTimeout)
	response, err := manager.Request(requestCtx, id, req.Operation, payload)
	cancel()
	if err != nil {
		if errors.Is(err, dbsession.ErrSessionNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"operation": response.Operation,
		"result":    response.Payload,
	})
}

func normalizeBrowserDBOperation(operation dbadapter.Operation, raw json.RawMessage) (interface{}, error) {
	switch operation {
	case dbadapter.OpPing, dbadapter.OpListCatalogs:
		if len(raw) != 0 && string(raw) != "null" && string(raw) != "{}" {
			return nil, fmt.Errorf("database operation %q does not accept a payload", operation)
		}
		return nil, nil
	case dbadapter.OpListObjects:
		var payload dbadapter.ListObjectsPayload
		if err := decodeDBOperationPayload(raw, &payload); err != nil {
			return nil, err
		}
		return payload, nil
	case dbadapter.OpDescribeObject:
		var payload dbadapter.DescribeObjectPayload
		if err := decodeDBOperationPayload(raw, &payload); err != nil {
			return nil, err
		}
		payload.Name = strings.TrimSpace(payload.Name)
		if payload.Name == "" {
			return nil, errors.New("database object name is required")
		}
		return payload, nil
	case dbadapter.OpExecute:
		var payload dbadapter.ExecutePayload
		if err := decodeDBOperationPayload(raw, &payload); err != nil {
			return nil, err
		}
		return dbadapter.NormalizeExecutePayload(payload)
	default:
		return nil, fmt.Errorf("database operation %q is not exposed to the browser", operation)
	}
}

func decodeDBOperationPayload(raw json.RawMessage, target interface{}) error {
	if len(raw) == 0 || string(raw) == "null" {
		raw = json.RawMessage("{}")
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errors.New("invalid database operation payload")
	}
	return nil
}
