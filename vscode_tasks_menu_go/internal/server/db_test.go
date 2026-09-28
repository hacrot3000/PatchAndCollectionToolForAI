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
)

func (s *Server) prepareDatabaseTestProfile(profileID string, req dbProfileRequest) (dbprofile.Profile, *string, error) {
	profileID = strings.TrimSpace(profileID)
	secretRef := ""
	id := "test"
	if profileID != "" {
		store, err := s.databaseProfileStore()
		if err != nil {
			return dbprofile.Profile{}, nil, err
		}
		existing, err := store.Get(profileID)
		if err != nil {
			if errors.Is(err, dbprofile.ErrProfileNotFound) {
				return dbprofile.Profile{}, nil, errors.New("database profile not found")
			}
			return dbprofile.Profile{}, nil, err
		}
		id = existing.ID
		if req.Secret == nil {
			secretRef = existing.SecretRef
		}
	}

	if req.Secret != nil {
		if err := validateDBAuthenticationSecret(*req.Secret); err != nil {
			return dbprofile.Profile{}, nil, err
		}
	}

	profile, err := dbprofile.Normalize(req.profile(id, secretRef))
	if err != nil {
		return dbprofile.Profile{}, nil, err
	}
	if err := s.validateDBProfileReferences(profile); err != nil {
		return dbprofile.Profile{}, nil, err
	}
	return profile, req.Secret, nil
}

func (s *Server) runDatabaseConnectionTest(ctx context.Context, profile dbprofile.Profile, explicitSecret *string) error {
	manager, err := s.databaseSessionManager()
	if err != nil {
		return err
	}

	effectiveHost, effectivePort, cleanup, err := s.prepareDatabaseTransport(ctx, profile)
	if err != nil {
		return err
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
	if explicitSecret != nil {
		connect.Secret = *explicitSecret
	} else if profile.SecretRef != "" {
		secrets, err := s.connectionSecretStore()
		if err != nil {
			return err
		}
		secret, err := secrets.Get(profile.SecretRef)
		if err != nil {
			return errors.New("database authentication secret is unavailable")
		}
		connect.Secret = string(secret)
		for i := range secret {
			secret[i] = 0
		}
	}

	meta, err := manager.OpenWithCleanup(
		ctx,
		profile.ID,
		profile.AdapterID,
		dbadapter.ProcessOptions{Dir: s.Workspace},
		connect,
		cleanup,
	)
	connect.Secret = ""
	cleanupOwned = false
	if err != nil {
		return err
	}
	defer func() {
		_ = manager.CloseSession(meta.ID)
	}()

	if _, err := manager.Request(ctx, meta.ID, dbadapter.OpPing, nil); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}
	return nil
}

func (s *Server) dbTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ProfileID string           `json:"profile_id,omitempty"`
		Profile   dbProfileRequest `json:"profile"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	profile, explicitSecret, err := s.prepareDatabaseTestProfile(req.ProfileID, req.Profile)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), databaseOpenTimeout)
	defer cancel()
	started := time.Now()
	err = s.runDatabaseConnectionTest(ctx, profile, explicitSecret)
	elapsed := time.Since(started).Milliseconds()
	if err != nil {
		s.auditConnection(r, ConnectionAuditEvent{
			Kind:      "db_connection",
			Action:    "test",
			ProfileID: strings.TrimSpace(req.ProfileID),
			Success:   false,
		})
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	s.auditConnection(r, ConnectionAuditEvent{
		Kind:      "db_connection",
		Action:    "test",
		ProfileID: strings.TrimSpace(req.ProfileID),
		Success:   true,
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":         true,
		"elapsed_ms": elapsed,
	})
}
