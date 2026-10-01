package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/filetransferprofile"
	"bletonfc/vscode_tasks_menu/internal/secretstore"
	"bletonfc/vscode_tasks_menu/internal/sshprofile"
)

type fileTransferProfileRequest struct {
	Name                  string                        `json:"name"`
	Protocol              filetransferprofile.Protocol `json:"protocol"`
	Host                  string                        `json:"host,omitempty"`
	Port                  int                           `json:"port,omitempty"`
	Username              string                        `json:"username,omitempty"`
	SSHProfileID          string                        `json:"ssh_profile_id,omitempty"`
	InitialPath           string                        `json:"initial_path,omitempty"`
	ConnectTimeoutSeconds int                           `json:"connect_timeout_seconds,omitempty"`
	Secret                *string                       `json:"secret,omitempty"`
	ClearSecret           bool                          `json:"clear_secret,omitempty"`
}

type fileTransferProfileView struct {
	filetransferprofile.Profile
	HasSecret bool `json:"has_secret"`
}

func fileTransferProfileProjection(profile filetransferprofile.Profile) fileTransferProfileView {
	return fileTransferProfileView{Profile: profile, HasSecret: profile.SecretRef != ""}
}

func (req fileTransferProfileRequest) profile(id, secretRef string) filetransferprofile.Profile {
	return filetransferprofile.Profile{
		ID:                    id,
		Name:                  req.Name,
		Protocol:              req.Protocol,
		Host:                  req.Host,
		Port:                  req.Port,
		Username:              req.Username,
		SSHProfileID:          req.SSHProfileID,
		InitialPath:           req.InitialPath,
		ConnectTimeoutSeconds: req.ConnectTimeoutSeconds,
		SecretRef:             secretRef,
	}
}

func (s *Server) fileTransferProfileStore() (*filetransferprofile.Store, error) {
	if s.FileTransferProfiles != nil {
		return s.FileTransferProfiles, nil
	}
	return filetransferprofile.DefaultStore()
}

func decodeFileTransferProfileRequest(w http.ResponseWriter, r *http.Request) (fileTransferProfileRequest, error) {
	var req fileTransferProfileRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		return fileTransferProfileRequest{}, errors.New("invalid JSON")
	}
	return req, nil
}

func validateFileTransferSecret(value string) error {
	if value == "" {
		return errors.New("ftp password is empty")
	}
	if len(value) > maxSSHAuthenticationSecretBytes {
		return fmt.Errorf("ftp password exceeds %d bytes", maxSSHAuthenticationSecretBytes)
	}
	if strings.ContainsAny(value, "\x00\r\n") {
		return errors.New("ftp password must not contain NUL or line breaks")
	}
	return nil
}

func newFileTransferSecretRef(profileID string) (string, error) {
	suffix, err := filetransferprofile.NewID()
	if err != nil {
		return "", err
	}
	return "file-transfer/" + profileID + "/password/" + suffix, nil
}

func (s *Server) validateFileTransferProfileReference(profile filetransferprofile.Profile) error {
	if profile.Protocol != filetransferprofile.ProtocolSFTP {
		return nil
	}
	store, err := s.sshProfileStore()
	if err != nil {
		return err
	}
	if _, err := store.Get(profile.SSHProfileID); err != nil {
		if errors.Is(err, sshprofile.ErrProfileNotFound) {
			return errors.New("referenced SSH profile not found")
		}
		return err
	}
	return nil
}

func (s *Server) fileTransferProfiles(w http.ResponseWriter, r *http.Request) {
	store, err := s.fileTransferProfileStore()
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
		views := make([]fileTransferProfileView, 0, len(profiles))
		for _, profile := range profiles {
			views = append(views, fileTransferProfileProjection(profile))
		}
		writeJSON(w, http.StatusOK, map[string]any{"profiles": views})
	case http.MethodPost:
		req, err := decodeFileTransferProfileRequest(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.ClearSecret {
			http.Error(w, "clear_secret is only valid when updating a profile", http.StatusBadRequest)
			return
		}
		id, err := filetransferprofile.NewID()
		if err != nil {
			http.Error(w, "cannot generate file-transfer profile id", http.StatusInternalServerError)
			return
		}

		var secretRef string
		var secretValue []byte
		if req.Secret != nil {
			if req.Protocol != filetransferprofile.ProtocolFTP {
				http.Error(w, "sftp profile must use the referenced SSH profile authentication", http.StatusBadRequest)
				return
			}
			if err := validateFileTransferSecret(*req.Secret); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			secretRef, err = newFileTransferSecretRef(id)
			if err != nil {
				http.Error(w, "cannot generate file-transfer secret reference", http.StatusInternalServerError)
				return
			}
			secretValue = []byte(*req.Secret)
		}
		profile, err := filetransferprofile.Normalize(req.profile(id, secretRef))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.validateFileTransferProfileReference(profile); err != nil {
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
		s.auditConnection(r, ConnectionAuditEvent{Kind: "file_transfer_profile", Action: "create", ProfileID: created.ID, Success: true})
		writeJSON(w, http.StatusCreated, fileTransferProfileProjection(created))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) fileTransferProfileItem(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/file-transfer/profiles/"))
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	store, err := s.fileTransferProfileStore()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	switch r.Method {
	case http.MethodGet:
		profile, err := store.Get(id)
		if errors.Is(err, filetransferprofile.ErrProfileNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, fileTransferProfileProjection(profile))
	case http.MethodPut:
		existing, err := store.Get(id)
		if errors.Is(err, filetransferprofile.ErrProfileNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		req, err := decodeFileTransferProfileRequest(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.ClearSecret && req.Secret != nil {
			http.Error(w, "secret and clear_secret are mutually exclusive", http.StatusBadRequest)
			return
		}

		secretRef := ""
		if req.Protocol == filetransferprofile.ProtocolFTP && existing.Protocol == filetransferprofile.ProtocolFTP && !req.ClearSecret && req.Secret == nil {
			secretRef = existing.SecretRef
		}
		var newSecretRef string
		var newSecret []byte
		if req.Secret != nil {
			if req.Protocol != filetransferprofile.ProtocolFTP {
				http.Error(w, "sftp profile must use the referenced SSH profile authentication", http.StatusBadRequest)
				return
			}
			if err := validateFileTransferSecret(*req.Secret); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			newSecretRef, err = newFileTransferSecretRef(id)
			if err != nil {
				http.Error(w, "cannot generate file-transfer secret reference", http.StatusInternalServerError)
				return
			}
			secretRef = newSecretRef
			newSecret = []byte(*req.Secret)
		}

		profile, err := filetransferprofile.Normalize(req.profile(id, secretRef))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.validateFileTransferProfileReference(profile); err != nil {
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
			if secrets != nil {
				_ = secrets.Delete(newSecretRef)
			}
			if errors.Is(err, filetransferprofile.ErrProfileNotFound) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if existing.SecretRef != "" && existing.SecretRef != updated.SecretRef {
			if secrets == nil {
				secrets, err = s.connectionSecretStore()
			}
			if err == nil && secrets != nil {
				if deleteErr := secrets.Delete(existing.SecretRef); deleteErr != nil && s.Log != nil {
					s.Log.Printf("file-transfer secret cleanup warning profile=%s: %v", id, deleteErr)
				}
			}
		}
		s.auditConnection(r, ConnectionAuditEvent{Kind: "file_transfer_profile", Action: "update", ProfileID: updated.ID, Success: true})
		writeJSON(w, http.StatusOK, fileTransferProfileProjection(updated))
	case http.MethodDelete:
		deleted, err := store.Delete(id)
		if errors.Is(err, filetransferprofile.ErrProfileNotFound) {
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
					s.Log.Printf("file-transfer secret cleanup warning profile=%s: %v", id, deleteErr)
				}
			} else if s.Log != nil {
				s.Log.Printf("file-transfer secret store cleanup warning profile=%s: %v", id, secretErr)
			}
		}
		s.auditConnection(r, ConnectionAuditEvent{Kind: "file_transfer_profile", Action: "delete", ProfileID: deleted.ID, Success: true})
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, fmt.Sprintf("method %s not allowed", r.Method), http.StatusMethodNotAllowed)
	}
}
