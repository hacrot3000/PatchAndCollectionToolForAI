package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/secretstore"
	"bletonfc/vscode_tasks_menu/internal/sshprofile"
)

type sshProfileRequest struct {
	Name                       string                     `json:"name"`
	Host                       string                     `json:"host"`
	Port                       int                        `json:"port,omitempty"`
	Username                   string                     `json:"username"`
	AuthMethod                 sshprofile.AuthMethod      `json:"auth_method"`
	IdentityFile               string                     `json:"identity_file,omitempty"`
	Secret                     *string                    `json:"secret,omitempty"`
	CustomHomeDir              string                     `json:"custom_home_dir,omitempty"`
	PresetCommands             []sshprofile.PresetCommand `json:"preset_commands,omitempty"`
	ConnectTimeoutSeconds      int                        `json:"connect_timeout_seconds,omitempty"`
	ServerAliveIntervalSeconds int                        `json:"server_alive_interval_seconds,omitempty"`
	ServerAliveCountMax        int                        `json:"server_alive_count_max,omitempty"`
	ProxyJump                  string                     `json:"proxy_jump,omitempty"`
}

type sshProfileView struct {
	sshprofile.Profile
	HasSecret bool `json:"has_secret"`
}

func sshProfileProjection(profile sshprofile.Profile) sshProfileView {
	return sshProfileView{Profile: profile, HasSecret: profile.SecretRef != ""}
}

func (req sshProfileRequest) profile(id, secretRef string) sshprofile.Profile {
	return sshprofile.Profile{
		ID:                         id,
		Name:                       req.Name,
		Host:                       req.Host,
		Port:                       req.Port,
		Username:                   req.Username,
		AuthMethod:                 req.AuthMethod,
		IdentityFile:               req.IdentityFile,
		SecretRef:                  secretRef,
		CustomHomeDir:              req.CustomHomeDir,
		PresetCommands:             req.PresetCommands,
		ConnectTimeoutSeconds:      req.ConnectTimeoutSeconds,
		ServerAliveIntervalSeconds: req.ServerAliveIntervalSeconds,
		ServerAliveCountMax:        req.ServerAliveCountMax,
		ProxyJump:                  req.ProxyJump,
	}
}

func (s *Server) sshProfileStore() (*sshprofile.Store, error) {
	if s.SSHProfiles != nil {
		return s.SSHProfiles, nil
	}
	return sshprofile.DefaultStore()
}

func (s *Server) connectionSecretStore() (secretstore.Store, error) {
	if s.ConnectionSecrets != nil {
		return s.ConnectionSecrets, nil
	}
	return secretstore.NewDefaultFileStore()
}

func decodeSSHProfileRequest(w http.ResponseWriter, r *http.Request) (sshProfileRequest, error) {
	var req sshProfileRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		return sshProfileRequest{}, errors.New("invalid JSON")
	}
	return req, nil
}

func newSSHSecretRef(profileID string) (string, error) {
	suffix, err := sshprofile.NewID()
	if err != nil {
		return "", err
	}
	return "ssh/" + profileID + "/auth/" + suffix, nil
}

const maxSSHAuthenticationSecretBytes = 4096

func validateSSHAuthenticationSecret(value string) error {
	if value == "" {
		return errors.New("ssh authentication secret is empty")
	}
	if len(value) > maxSSHAuthenticationSecretBytes {
		return fmt.Errorf("ssh authentication secret exceeds %d bytes", maxSSHAuthenticationSecretBytes)
	}
	if strings.ContainsAny(value, "\x00\r\n") {
		return errors.New("ssh authentication secret must not contain NUL or line breaks")
	}
	return nil
}

func (s *Server) sshProfiles(w http.ResponseWriter, r *http.Request) {
	store, err := s.sshProfileStore()
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
		views := make([]sshProfileView, 0, len(profiles))
		for _, profile := range profiles {
			views = append(views, sshProfileProjection(profile))
		}
		writeJSON(w, http.StatusOK, map[string]any{"profiles": views})
	case http.MethodPost:
		req, err := decodeSSHProfileRequest(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		id, err := sshprofile.NewID()
		if err != nil {
			http.Error(w, "cannot generate ssh profile id", http.StatusInternalServerError)
			return
		}

		var secretRef string
		var secretValue []byte
		if req.Secret != nil && *req.Secret != "" {
			if err := validateSSHAuthenticationSecret(*req.Secret); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if req.AuthMethod == sshprofile.AuthAgent {
				http.Error(w, "agent authentication must not include a secret", http.StatusBadRequest)
				return
			}
			secretRef, err = newSSHSecretRef(id)
			if err != nil {
				http.Error(w, "cannot generate ssh secret reference", http.StatusInternalServerError)
				return
			}
			secretValue = []byte(*req.Secret)
		}
		profile, err := sshprofile.Normalize(req.profile(id, secretRef))
		if err != nil {
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
		writeJSON(w, http.StatusCreated, sshProfileProjection(created))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) sshProfileItem(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/ssh/profiles/"))
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	store, err := s.sshProfileStore()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	switch r.Method {
	case http.MethodGet:
		profile, err := store.Get(id)
		if errors.Is(err, sshprofile.ErrProfileNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, sshProfileProjection(profile))
	case http.MethodPut:
		existing, err := store.Get(id)
		if errors.Is(err, sshprofile.ErrProfileNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		req, err := decodeSSHProfileRequest(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		secretRef := ""
		if req.AuthMethod == existing.AuthMethod && req.Secret == nil {
			secretRef = existing.SecretRef
		}
		var newSecretRef string
		var newSecret []byte
		if req.Secret != nil && *req.Secret != "" {
			if err := validateSSHAuthenticationSecret(*req.Secret); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if req.AuthMethod == sshprofile.AuthAgent {
				http.Error(w, "agent authentication must not include a secret", http.StatusBadRequest)
				return
			}
			newSecretRef, err = newSSHSecretRef(id)
			if err != nil {
				http.Error(w, "cannot generate ssh secret reference", http.StatusInternalServerError)
				return
			}
			secretRef = newSecretRef
			newSecret = []byte(*req.Secret)
		}

		profile, err := sshprofile.Normalize(req.profile(id, secretRef))
		if err != nil {
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
			if errors.Is(err, sshprofile.ErrProfileNotFound) {
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
					s.Log.Printf("ssh secret cleanup warning profile=%s: %v", id, deleteErr)
				}
			}
		}
		writeJSON(w, http.StatusOK, sshProfileProjection(updated))
	case http.MethodDelete:
		deleted, err := store.Delete(id)
		if errors.Is(err, sshprofile.ErrProfileNotFound) {
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
					s.Log.Printf("ssh secret cleanup warning profile=%s: %v", id, deleteErr)
				}
			} else if s.Log != nil {
				s.Log.Printf("ssh secret store cleanup warning profile=%s: %v", id, secretErr)
			}
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, fmt.Sprintf("method %s not allowed", r.Method), http.StatusMethodNotAllowed)
	}
}
