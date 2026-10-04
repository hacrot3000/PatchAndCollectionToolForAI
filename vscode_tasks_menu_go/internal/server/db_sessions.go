package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bletonfc/vscode_tasks_menu/internal/dbadapter"
	"bletonfc/vscode_tasks_menu/internal/dbprofile"
	"bletonfc/vscode_tasks_menu/internal/dbsession"
)

const (
	databaseOpenTimeout = 20 * time.Second
	databaseImportTimeout = 30 * time.Minute
	databaseImportMaxBytes int64 = 2 << 30
)

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
		openCtx, cancel := context.WithTimeout(r.Context(), databaseOpenTimeout)
		defer cancel()

		effectiveHost, effectivePort, cleanup, err := s.prepareDatabaseTransport(openCtx, profile)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		cleanupOwned := cleanup != nil
		defer func() {
			if cleanupOwned {
				cleanup()
			}
		}()

		connect := dbadapter.ConnectPayload{
			Host:     effectiveHost,
			Port:     effectivePort,
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
		meta, err := manager.OpenWithCleanup(
			openCtx,
			profile.ID,
			profile.AdapterID,
			dbadapter.ProcessOptions{Dir: s.Workspace},
			connect,
			cleanup,
		)
		connect.Secret = ""
		cleanupOwned = false
		if err != nil {
			s.auditConnection(r, ConnectionAuditEvent{Kind: "db_session", Action: "open", ProfileID: profile.ID, Success: false})
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		s.auditConnection(r, ConnectionAuditEvent{Kind: "db_session", Action: "open", ProfileID: profile.ID, SessionID: meta.ID, Success: true})
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
			meta, _ := manager.Get(id)
			if err := manager.CloseSession(id); err != nil {
				s.auditConnection(r, ConnectionAuditEvent{Kind: "db_session", Action: "close", ProfileID: meta.ProfileID, SessionID: id, Success: false})
				if errors.Is(err, dbsession.ErrSessionNotFound) {
					http.NotFound(w, r)
					return
				}
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			s.auditConnection(r, ConnectionAuditEvent{Kind: "db_session", Action: "close", ProfileID: meta.ProfileID, SessionID: id, Success: true})
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	if len(parts) == 2 && parts[1] == "import" && r.Method == http.MethodPost {
		s.dbSessionImportSQL(w, r, manager, id)
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
	sessionMeta, _ := manager.Get(id)
	requestCtx, cancel := context.WithTimeout(r.Context(), databaseOpenTimeout)
	response, err := manager.Request(requestCtx, id, req.Operation, payload)
	cancel()
	if err != nil {
		s.auditConnection(r, ConnectionAuditEvent{Kind: "db_session", Action: string(req.Operation), ProfileID: sessionMeta.ProfileID, SessionID: id, Success: false})
		if errors.Is(err, dbsession.ErrSessionNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	s.auditConnection(r, ConnectionAuditEvent{Kind: "db_session", Action: string(req.Operation), ProfileID: sessionMeta.ProfileID, SessionID: id, Success: true})
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"operation": response.Operation,
		"result":    response.Payload,
	})
}

func (s *Server) dbSessionImportSQL(w http.ResponseWriter, r *http.Request, manager *dbsession.Manager, id string) {
	meta, err := manager.Get(id)
	if errors.Is(err, dbsession.ErrSessionNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var (
		path    string
		catalog string
		cleanup func()
	)
	contentType := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	if strings.HasPrefix(contentType, "multipart/form-data") {
		r.Body = http.MaxBytesReader(w, r.Body, databaseImportMaxBytes+(1<<20))
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			http.Error(w, "invalid or oversized SQL import upload", http.StatusBadRequest)
			return
		}
		if r.MultipartForm != nil {
			defer r.MultipartForm.RemoveAll()
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "SQL import file is required", http.StatusBadRequest)
			return
		}
		defer file.Close()
		if !sqlScriptExtensionAllowed(header.Filename) {
			http.Error(w, "SQL import file must end in .sql or .txt", http.StatusUnsupportedMediaType)
			return
		}
		temp, err := os.CreateTemp("", "taskdeck-sql-import-*.sql")
		if err != nil {
			http.Error(w, "cannot prepare SQL import", http.StatusInternalServerError)
			return
		}
		_ = temp.Chmod(0o600)
		tempPath := temp.Name()
		cleanup = func() { _ = os.Remove(tempPath) }
		defer cleanup()
		written, copyErr := io.Copy(temp, io.LimitReader(file, databaseImportMaxBytes+1))
		closeErr := temp.Close()
		if copyErr != nil || closeErr != nil {
			http.Error(w, "cannot stage SQL import upload", http.StatusInternalServerError)
			return
		}
		if written > databaseImportMaxBytes {
			http.Error(w, "SQL import exceeds 2 GiB limit", http.StatusRequestEntityTooLarge)
			return
		}
		path = tempPath
		catalog = strings.TrimSpace(r.FormValue("catalog"))
	} else {
		var req struct {
			HostPath string `json:"host_path"`
			Catalog  string `json:"catalog,omitempty"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil {
			http.Error(w, "invalid SQL import request", http.StatusBadRequest)
			return
		}
		if !sqlScriptExtensionAllowed(req.HostPath) {
			http.Error(w, "SQL import file must end in .sql or .txt", http.StatusUnsupportedMediaType)
			return
		}
		resolved, err := s.resolveProjectPath(req.HostPath, false, false)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		info, err := os.Stat(resolved)
		if err != nil || !info.Mode().IsRegular() {
			http.Error(w, "SQL import file is unavailable", http.StatusNotFound)
			return
		}
		if info.Size() > databaseImportMaxBytes {
			http.Error(w, "SQL import exceeds 2 GiB limit", http.StatusRequestEntityTooLarge)
			return
		}
		path = resolved
		catalog = strings.TrimSpace(req.Catalog)
	}
	requestCtx, cancel := context.WithTimeout(r.Context(), databaseImportTimeout)
	response, err := manager.Request(requestCtx, id, dbadapter.OpImportSQL, dbadapter.ImportSQLPayload{Path: path, Catalog: catalog})
	cancel()
	if err != nil {
		s.auditConnection(r, ConnectionAuditEvent{Kind: "db_session", Action: string(dbadapter.OpImportSQL), ProfileID: meta.ProfileID, SessionID: id, Success: false})
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	s.auditConnection(r, ConnectionAuditEvent{Kind: "db_session", Action: string(dbadapter.OpImportSQL), ProfileID: meta.ProfileID, SessionID: id, Success: true})
	writeJSON(w, http.StatusOK, map[string]interface{}{"result": response.Payload})
}

func sqlScriptExtensionAllowed(name string) bool {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(name))) {
	case ".sql", ".txt":
		return true
	default:
		return false
	}
}

func normalizeBrowserDBOperation(operation dbadapter.Operation, raw json.RawMessage) (interface{}, error) {
	switch operation {
	case dbadapter.OpPing, dbadapter.OpListCatalogs, dbadapter.OpBegin, dbadapter.OpCommit, dbadapter.OpRollback:
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
	case dbadapter.OpBrowseRows:
		var payload dbadapter.BrowseRowsPayload
		if err := decodeDBOperationPayload(raw, &payload); err != nil {
			return nil, err
		}
		return dbadapter.NormalizeBrowseRowsPayload(payload)
	case dbadapter.OpMutateRows:
		var payload dbadapter.MutateRowsPayload
		if err := decodeDBOperationPayload(raw, &payload); err != nil {
			return nil, err
		}
		return dbadapter.NormalizeMutateRowsPayload(payload)
	case dbadapter.OpObjectAction:
		var payload dbadapter.ObjectActionPayload
		if err := decodeDBOperationPayload(raw, &payload); err != nil {
			return nil, err
		}
		return dbadapter.NormalizeObjectActionPayload(payload)
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
