package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/secretstore"
)

const managedSecretMaxBytes = 64 << 10

type secretUsageView struct {
	Kind      string `json:"kind"`
	ProfileID string `json:"profile_id"`
	Name      string `json:"name,omitempty"`
}

type managedSecretView struct {
	ID           string            `json:"id"`
	Kind         string            `json:"kind"`
	Present      bool              `json:"present"`
	Referenced   bool              `json:"referenced"`
	ReferencedBy []secretUsageView `json:"referenced_by,omitempty"`
}

type managedSecretRequest struct {
	Action string `json:"action"`
	ID     string `json:"id,omitempty"`
	Kind   string `json:"kind,omitempty"`
	Value  string `json:"value,omitempty"`
}

func normalizeManagedSecretKind(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "ssh", "database", "ftp", "git-token", "deploy", "generic":
		return value, nil
	default:
		return "", errors.New("secret kind must be ssh, database, ftp, git-token, deploy, or generic")
	}
}

func managedSecretKindFromID(id string) string {
	switch {
	case strings.HasPrefix(id, "ssh/"):
		return "ssh"
	case strings.HasPrefix(id, "db/"):
		return "database"
	case strings.HasPrefix(id, "file-transfer/"):
		return "ftp"
	case strings.HasPrefix(id, "managed/git-token/"):
		return "git-token"
	case strings.HasPrefix(id, "managed/deploy/"):
		return "deploy"
	default:
		return "generic"
	}
}

func newManagedSecretRef(kind string) (string, error) {
	kind, err := normalizeManagedSecretKind(kind)
	if err != nil {
		return "", err
	}
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate secret reference: %w", err)
	}
	return "managed/" + kind + "/" + hex.EncodeToString(raw[:]), nil
}

func (s *Server) secretUsage() (map[string][]secretUsageView, error) {
	usage := map[string][]secretUsageView{}
	sshStore, err := s.sshProfileStore()
	if err != nil {
		return nil, err
	}
	sshProfiles, err := sshStore.Load()
	if err != nil {
		return nil, err
	}
	for _, profile := range sshProfiles {
		if profile.SecretRef == "" {
			continue
		}
		usage[profile.SecretRef] = append(usage[profile.SecretRef], secretUsageView{
			Kind: "ssh", ProfileID: profile.ID, Name: profile.Name,
		})
	}
	dbStore, err := s.databaseProfileStore()
	if err != nil {
		return nil, err
	}
	dbProfiles, err := dbStore.Load()
	if err != nil {
		return nil, err
	}
	for _, profile := range dbProfiles {
		if profile.SecretRef == "" {
			continue
		}
		usage[profile.SecretRef] = append(usage[profile.SecretRef], secretUsageView{
			Kind: "database", ProfileID: profile.ID, Name: profile.Name,
		})
	}
	transferStore, err := s.fileTransferProfileStore()
	if err != nil {
		return nil, err
	}
	transferProfiles, err := transferStore.Load()
	if err != nil {
		return nil, err
	}
	for _, profile := range transferProfiles {
		if profile.SecretRef == "" {
			continue
		}
		usage[profile.SecretRef] = append(usage[profile.SecretRef], secretUsageView{
			Kind: "file-transfer", ProfileID: profile.ID, Name: profile.Name,
		})
	}
	return usage, nil
}

func (s *Server) managedSecretList() ([]managedSecretView, error) {
	store, err := s.connectionSecretStore()
	if err != nil {
		return nil, err
	}
	usage, err := s.secretUsage()
	if err != nil {
		return nil, err
	}
	present := map[string]bool{}
	if lister, ok := store.(secretstore.Lister); ok {
		ids, err := lister.ListIDs()
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			present[id] = true
		}
	}
	all := map[string]bool{}
	for id := range present {
		all[id] = true
	}
	for id := range usage {
		all[id] = true
	}
	ids := make([]string, 0, len(all))
	for id := range all {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]managedSecretView, 0, len(ids))
	for _, id := range ids {
		refs := append([]secretUsageView(nil), usage[id]...)
		sort.Slice(refs, func(i, j int) bool {
			if refs[i].Kind != refs[j].Kind {
				return refs[i].Kind < refs[j].Kind
			}
			return refs[i].ProfileID < refs[j].ProfileID
		})
		out = append(out, managedSecretView{
			ID: id, Kind: managedSecretKindFromID(id), Present: present[id],
			Referenced: len(refs) > 0, ReferencedBy: refs,
		})
	}
	return out, nil
}

func decodeManagedSecretRequest(w http.ResponseWriter, r *http.Request) (managedSecretRequest, error) {
	var req managedSecretRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, managedSecretMaxBytes+(16<<10)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		return managedSecretRequest{}, errors.New("invalid secret request")
	}
	req.Action = strings.ToLower(strings.TrimSpace(req.Action))
	req.ID = strings.TrimSpace(req.ID)
	req.Kind = strings.TrimSpace(req.Kind)
	if len(req.Value) > managedSecretMaxBytes {
		return managedSecretRequest{}, fmt.Errorf("secret exceeds %d bytes", managedSecretMaxBytes)
	}
	return req, nil
}

func (s *Server) secretsAPI(w http.ResponseWriter, r *http.Request) {
	store, err := s.connectionSecretStore()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	switch r.Method {
	case http.MethodGet:
		items, err := s.managedSecretList()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"secrets": items})
	case http.MethodPost:
		req, err := decodeManagedSecretRequest(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Value == "" {
			http.Error(w, "secret value is required", http.StatusBadRequest)
			return
		}
		action := req.Action
		if action == "" {
			action = "create"
		}
		switch action {
		case "create":
			if req.ID != "" {
				http.Error(w, "secret id is generated by TaskDeck", http.StatusBadRequest)
				return
			}
			id, err := newManagedSecretRef(req.Kind)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if err := store.Put(id, []byte(req.Value)); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			s.auditSharedSuccess(r, "secret.create", "secret", id, map[string]any{"kind": managedSecretKindFromID(id)})
			writeJSON(w, http.StatusCreated, managedSecretView{ID: id, Kind: managedSecretKindFromID(id), Present: true})
		case "rotate":
			if req.ID == "" {
				http.Error(w, "secret id is required", http.StatusBadRequest)
				return
			}
			if _, err := store.Get(req.ID); err != nil {
				if errors.Is(err, secretstore.ErrNotFound) {
					http.NotFound(w, r)
					return
				}
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if err := store.Put(req.ID, []byte(req.Value)); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			s.auditSharedSuccess(r, "secret.rotate", "secret", req.ID, map[string]any{"kind": managedSecretKindFromID(req.ID)})
			writeJSON(w, http.StatusOK, managedSecretView{ID: req.ID, Kind: managedSecretKindFromID(req.ID), Present: true})
		default:
			http.Error(w, "unsupported secret action", http.StatusBadRequest)
		}
	case http.MethodDelete:
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		if id == "" {
			http.Error(w, "secret id is required", http.StatusBadRequest)
			return
		}
		usage, err := s.secretUsage()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if refs := usage[id]; len(refs) > 0 {
			writeJSON(w, http.StatusConflict, map[string]any{
				"error": "secret is still referenced by connection profiles",
				"referenced_by": refs,
			})
			return
		}
		if err := store.Delete(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.auditSharedSuccess(r, "secret.delete", "secret", id, map[string]any{"kind": managedSecretKindFromID(id)})
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

