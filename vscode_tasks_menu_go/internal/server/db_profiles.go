package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/dbadapter"
	"bletonfc/vscode_tasks_menu/internal/dbprofile"
	"bletonfc/vscode_tasks_menu/internal/secretstore"
	"bletonfc/vscode_tasks_menu/internal/sshprofile"
)

const maxDBAuthenticationSecretBytes = 4096

type dbProfileRequest struct {
	Name         string            `json:"name"`
	AdapterID    string            `json:"adapter_id"`
	Transport    dbprofile.Transport `json:"transport,omitempty"`
	Host         string            `json:"host,omitempty"`
	Port         int               `json:"port,omitempty"`
	Username     string            `json:"username,omitempty"`
	Database     string            `json:"database,omitempty"`
	File         string            `json:"file,omitempty"`
	SSHProfileID string            `json:"ssh_profile_id,omitempty"`
	ReadOnly     bool              `json:"read_only,omitempty"`
	Options      map[string]string `json:"options,omitempty"`
	Secret       *string           `json:"secret,omitempty"`
}

type dbProfileView struct {
	dbprofile.Profile
	HasSecret bool `json:"has_secret"`
}

func (s *Server) databaseProfileStore() (*dbprofile.Store, error) {
	if s.DBProfiles != nil {
		return s.DBProfiles, nil
	}
	return dbprofile.DefaultStore()
}

func (s *Server) databaseAdapterRegistry() (*dbadapter.Registry, error) {
	if s.DBAdapters == nil {
		return nil, errors.New("database adapters are not configured")
	}
	return s.DBAdapters, nil
}

func dbProfileProjection(profile dbprofile.Profile) dbProfileView {
	return dbProfileView{Profile: profile, HasSecret: profile.SecretRef != ""}
}

func (req dbProfileRequest) profile(id, secretRef string) dbprofile.Profile {
	return dbprofile.Profile{
		ID:           id,
		Name:         req.Name,
		AdapterID:    req.AdapterID,
		Transport:    req.Transport,
		Host:         req.Host,
		Port:         req.Port,
		Username:     req.Username,
		Database:     req.Database,
		File:         req.File,
		SSHProfileID: req.SSHProfileID,
		ReadOnly:     req.ReadOnly,
		Options:      req.Options,
		SecretRef:    secretRef,
	}
}

func decodeDBProfileRequest(w http.ResponseWriter, r *http.Request) (dbProfileRequest, error) {
	var req dbProfileRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		return dbProfileRequest{}, errors.New("invalid JSON")
	}
	return req, nil
}

func validateDBAuthenticationSecret(value string) error {
	if value == "" {
		return nil
	}
	if len(value) > maxDBAuthenticationSecretBytes {
		return fmt.Errorf("database authentication secret exceeds %d bytes", maxDBAuthenticationSecretBytes)
	}
	if strings.ContainsAny(value, "\x00\r\n") {
		return errors.New("database authentication secret must not contain NUL or line breaks")
	}
	return nil
}

func newDBSecretRef(profileID string) (string, error) {
	suffix, err := dbprofile.NewID()
	if err != nil {
		return "", err
	}
	return "db/" + profileID + "/auth/" + suffix, nil
}

func (s *Server) validateDBProfileReferences(profile dbprofile.Profile) error {
	registry, err := s.databaseAdapterRegistry()
	if err != nil {
		return err
	}
	manifest, err := registry.Get(profile.AdapterID)
	if err != nil {
		if errors.Is(err, dbadapter.ErrAdapterNotFound) {
			return fmt.Errorf("database adapter %q is not available", profile.AdapterID)
		}
		return err
	}
	if manifest.Kind == "sqlite" {
		if profile.Transport != dbprofile.TransportDirect {
			return errors.New("SQLite profiles support direct local-file transport only")
		}
		if strings.TrimSpace(profile.File) == "" {
			return errors.New("SQLite profile requires a database file")
		}
		if strings.TrimSpace(profile.Host) != "" || profile.Port != 0 ||
			strings.TrimSpace(profile.Username) != "" || strings.TrimSpace(profile.Database) != "" ||
			profile.SecretRef != "" {
			return errors.New("SQLite profile must not contain network or authentication fields")
		}
	} else if strings.TrimSpace(profile.File) != "" {
		return fmt.Errorf("database adapter %q does not accept a local file", manifest.ID)
	}
	if profile.Transport == dbprofile.TransportSSHTunnel {
		sshStore, err := s.sshProfileStore()
		if err != nil {
			return err
		}
		if _, err := sshStore.Get(profile.SSHProfileID); err != nil {
			if errors.Is(err, sshprofile.ErrProfileNotFound) {
				return fmt.Errorf("ssh profile %q is not available", profile.SSHProfileID)
			}
			return err
		}
	}
	return nil
}

func (s *Server) dbAdapters(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	registry, err := s.databaseAdapterRegistry()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"adapters": []dbadapter.Manifest{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"adapters": registry.List()})
}

func (s *Server) dbProfiles(w http.ResponseWriter, r *http.Request) {
	store, err := s.databaseProfileStore()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	switch r.Method {
	case http.MethodGet:
		profiles, err := store.Load()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		views := make([]dbProfileView, 0, len(profiles))
		for _, profile := range profiles {
			views = append(views, dbProfileProjection(profile))
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"profiles": views})
	case http.MethodPost:
		req, err := decodeDBProfileRequest(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		id, err := dbprofile.NewID()
		if err != nil {
			http.Error(w, "cannot generate database profile id", http.StatusInternalServerError)
			return
		}
		secretRef := ""
		var secretValue []byte
		if req.Secret != nil && *req.Secret != "" {
			if err := validateDBAuthenticationSecret(*req.Secret); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			secretRef, err = newDBSecretRef(id)
			if err != nil {
				http.Error(w, "cannot generate database secret reference", http.StatusInternalServerError)
				return
			}
			secretValue = []byte(*req.Secret)
		}
		profile, err := dbprofile.Normalize(req.profile(id, secretRef))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.validateDBProfileReferences(profile); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var secrets secretstore.Store
		if secretRef != "" {
			secrets, err = s.connectionSecretStore()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if err := secrets.Put(secretRef, secretValue); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		created, err := store.Create(profile)
		if err != nil {
			if secrets != nil {
				_ = secrets.Delete(secretRef)
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.auditConnection(r, ConnectionAuditEvent{Kind: "db_profile", Action: "create", ProfileID: created.ID, Success: true})
		writeJSON(w, http.StatusCreated, dbProfileProjection(created))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) dbProfileItem(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/db/profiles/"))
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	store, err := s.databaseProfileStore()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	switch r.Method {
	case http.MethodGet:
		profile, err := store.Get(id)
		if errors.Is(err, dbprofile.ErrProfileNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, dbProfileProjection(profile))
	case http.MethodPut:
		existing, err := store.Get(id)
		if errors.Is(err, dbprofile.ErrProfileNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		req, err := decodeDBProfileRequest(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		secretRef := existing.SecretRef
		var newSecretRef string
		var newSecret []byte
		clearOldSecret := false
		if req.Secret != nil {
			if err := validateDBAuthenticationSecret(*req.Secret); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			clearOldSecret = existing.SecretRef != ""
			if *req.Secret == "" {
				secretRef = ""
			} else {
				newSecretRef, err = newDBSecretRef(id)
				if err != nil {
					http.Error(w, "cannot generate database secret reference", http.StatusInternalServerError)
					return
				}
				secretRef = newSecretRef
				newSecret = []byte(*req.Secret)
			}
		}
		profile, err := dbprofile.Normalize(req.profile(id, secretRef))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.validateDBProfileReferences(profile); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var secrets secretstore.Store
		if newSecretRef != "" {
			secrets, err = s.connectionSecretStore()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if err := secrets.Put(newSecretRef, newSecret); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		updated, err := store.Replace(id, profile)
		if err != nil {
			if secrets != nil && newSecretRef != "" {
				_ = secrets.Delete(newSecretRef)
			}
			if errors.Is(err, dbprofile.ErrProfileNotFound) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if clearOldSecret && existing.SecretRef != updated.SecretRef {
			if secrets == nil {
				secrets, err = s.connectionSecretStore()
			}
			if err == nil && secrets != nil {
				if deleteErr := secrets.Delete(existing.SecretRef); deleteErr != nil && s.Log != nil {
					s.Log.Printf("database secret cleanup warning profile=%s: %v", id, deleteErr)
				}
			}
		}
		s.auditConnection(r, ConnectionAuditEvent{Kind: "db_profile", Action: "update", ProfileID: updated.ID, Success: true})
		writeJSON(w, http.StatusOK, dbProfileProjection(updated))
	case http.MethodDelete:
		deleted, err := store.Delete(id)
		if errors.Is(err, dbprofile.ErrProfileNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if deleted.SecretRef != "" {
			if secrets, secretErr := s.connectionSecretStore(); secretErr == nil {
				if deleteErr := secrets.Delete(deleted.SecretRef); deleteErr != nil && s.Log != nil {
					s.Log.Printf("database secret cleanup warning profile=%s: %v", id, deleteErr)
				}
			} else if s.Log != nil {
				s.Log.Printf("database secret store cleanup warning profile=%s: %v", id, secretErr)
			}
		}
		s.auditConnection(r, ConnectionAuditEvent{Kind: "db_profile", Action: "delete", ProfileID: deleted.ID, Success: true})
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
