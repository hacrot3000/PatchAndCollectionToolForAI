package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"bletonfc/vscode_tasks_menu/internal/identity"
)

const (
	addonManifestVersion = 1
	addonMaxManifestBytes = 1 << 20
	addonMaxOutputBytes   = 2 << 20
	addonActionTimeout    = 5 * time.Minute
)

type addonAction struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Keywords    string   `json:"keywords,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
	Background  bool     `json:"background,omitempty"`
}

type addonPanel struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	ActionID string `json:"action_id"`
}

type addonContextMenu struct {
	ID         string   `json:"id"`
	Label      string   `json:"label"`
	ActionID   string   `json:"action_id"`
	Scopes     []string `json:"scopes,omitempty"`
	Extensions []string `json:"extensions,omitempty"`
}

type addonFilePreview struct {
	ID         string   `json:"id"`
	Label      string   `json:"label"`
	ActionID   string   `json:"action_id"`
	Extensions []string `json:"extensions"`
}

type addonManifest struct {
	Version      int                `json:"version"`
	ID           string             `json:"id"`
	Name         string             `json:"name"`
	Description  string             `json:"description,omitempty"`
	Command      string             `json:"command"`
	Args         []string           `json:"args,omitempty"`
	Actions      []addonAction      `json:"actions,omitempty"`
	Panels       []addonPanel       `json:"panels,omitempty"`
	ContextMenus []addonContextMenu `json:"context_menus,omitempty"`
	FilePreviews []addonFilePreview `json:"file_previews,omitempty"`
}

type addonInvokeRequest struct {
	ActionID string         `json:"action_id"`
	Context  map[string]any `json:"context,omitempty"`
}

type addonProcessRequest struct {
	Version   int            `json:"version"`
	AddonID   string         `json:"addon_id"`
	ActionID  string         `json:"action_id"`
	Workspace string         `json:"workspace"`
	Context   map[string]any `json:"context,omitempty"`
}

type addonProcessResponse struct {
	OK      bool           `json:"ok"`
	Message string         `json:"message,omitempty"`
	Content string         `json:"content,omitempty"`
	Data    map[string]any `json:"data,omitempty"`
	Error   string         `json:"error,omitempty"`
}

func addonConfigDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(base, "taskdeck", "addons"), nil
}

func validAddonToken(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 96 {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func normalizeAddonExtensions(values []string) ([]string, error) {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if !strings.HasPrefix(value, ".") {
			value = "." + value
		}
		if strings.ContainsAny(value, "/\\ 	
") || len(value) > 32 {
			return nil, fmt.Errorf("invalid add-on file extension %q", value)
		}
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out, nil
}

func normalizeAddonManifest(manifest addonManifest) (addonManifest, error) {
	if manifest.Version != addonManifestVersion {
		return manifest, fmt.Errorf("unsupported add-on manifest version %d", manifest.Version)
	}
	manifest.ID = strings.TrimSpace(manifest.ID)
	manifest.Name = strings.TrimSpace(manifest.Name)
	manifest.Description = strings.TrimSpace(manifest.Description)
	manifest.Command = filepath.Clean(strings.TrimSpace(manifest.Command))
	if !validAddonToken(manifest.ID) || manifest.Name == "" {
		return manifest, errors.New("add-on id/name is invalid")
	}
	if !filepath.IsAbs(manifest.Command) {
		return manifest, errors.New("add-on command must be an absolute path")
	}
	info, err := os.Stat(manifest.Command)
	if err != nil {
		return manifest, fmt.Errorf("stat add-on command: %w", err)
	}
	if !info.Mode().IsRegular() {
		return manifest, errors.New("add-on command must be a regular file")
	}
	if info.Mode().Perm()&0o111 == 0 {
		return manifest, errors.New("add-on command is not executable")
	}
	actions := map[string]bool{}
	for i := range manifest.Actions {
		action := &manifest.Actions[i]
		action.ID = strings.TrimSpace(action.ID)
		action.Label = strings.TrimSpace(action.Label)
		action.Keywords = strings.TrimSpace(action.Keywords)
		if !validAddonToken(action.ID) || action.Label == "" || actions[action.ID] {
			return manifest, fmt.Errorf("add-on action %d has invalid or duplicate identity", i+1)
		}
		actions[action.ID] = true
		seenPermission := map[string]bool{}
		permissions := make([]string, 0, len(action.Permissions))
		for _, permission := range action.Permissions {
			permission = strings.TrimSpace(permission)
			if permission == "" || !identity.KnownPermission(permission) {
				return manifest, fmt.Errorf("add-on action %q declares unknown permission %q", action.ID, permission)
			}
			if !seenPermission[permission] {
				seenPermission[permission] = true
				permissions = append(permissions, permission)
			}
		}
		action.Permissions = permissions
	}
	validateActionRef := func(kind, id, actionID string) error {
		if !validAddonToken(id) || !actions[actionID] {
			return fmt.Errorf("add-on %s %q references invalid action %q", kind, id, actionID)
		}
		return nil
	}
	for i := range manifest.Panels {
		item := &manifest.Panels[i]
		item.ID, item.Title, item.ActionID = strings.TrimSpace(item.ID), strings.TrimSpace(item.Title), strings.TrimSpace(item.ActionID)
		if item.Title == "" {
			return manifest, fmt.Errorf("add-on panel %d title is required", i+1)
		}
		if err := validateActionRef("panel", item.ID, item.ActionID); err != nil { return manifest, err }
	}
	for i := range manifest.ContextMenus {
		item := &manifest.ContextMenus[i]
		item.ID, item.Label, item.ActionID = strings.TrimSpace(item.ID), strings.TrimSpace(item.Label), strings.TrimSpace(item.ActionID)
		if item.Label == "" {
			return manifest, fmt.Errorf("add-on context menu %d label is required", i+1)
		}
		if err := validateActionRef("context menu", item.ID, item.ActionID); err != nil { return manifest, err }
		var err error
		if item.Extensions, err = normalizeAddonExtensions(item.Extensions); err != nil { return manifest, err }
		for _, scope := range item.Scopes {
			switch strings.TrimSpace(scope) {
			case "file", "directory", "project":
			default:
				return manifest, fmt.Errorf("add-on context menu %q has invalid scope %q", item.ID, scope)
			}
		}
	}
	for i := range manifest.FilePreviews {
		item := &manifest.FilePreviews[i]
		item.ID, item.Label, item.ActionID = strings.TrimSpace(item.ID), strings.TrimSpace(item.Label), strings.TrimSpace(item.ActionID)
		if item.Label == "" {
			return manifest, fmt.Errorf("add-on file preview %d label is required", i+1)
		}
		if err := validateActionRef("file preview", item.ID, item.ActionID); err != nil { return manifest, err }
		var err error
		if item.Extensions, err = normalizeAddonExtensions(item.Extensions); err != nil { return manifest, err }
		if len(item.Extensions) == 0 {
			return manifest, fmt.Errorf("add-on file preview %q requires at least one extension", item.ID)
		}
	}
	return manifest, nil
}

func loadAddonManifests() ([]addonManifest, error) {
	dir, err := addonConfigDir()
	if err != nil { return nil, err }
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create add-on config directory: %w", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil { return nil, err }
	out := make([]addonManifest, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || strings.ToLower(filepath.Ext(entry.Name())) != ".json" { continue }
		path := filepath.Join(dir, entry.Name())
		info, err := os.Lstat(path)
		if err != nil { return nil, err }
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("add-on manifest must be a regular non-symlink file: %s", entry.Name())
		}
		if info.Size() > addonMaxManifestBytes {
			return nil, fmt.Errorf("add-on manifest %s exceeds %d bytes", entry.Name(), addonMaxManifestBytes)
		}
		data, err := os.ReadFile(path)
		if err != nil { return nil, err }
		var manifest addonManifest
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&manifest); err != nil {
			return nil, fmt.Errorf("decode add-on manifest %s: %w", entry.Name(), err)
		}
		manifest, err = normalizeAddonManifest(manifest)
		if err != nil { return nil, fmt.Errorf("validate add-on manifest %s: %w", entry.Name(), err) }
		out = append(out, manifest)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	seen := map[string]bool{}
	for _, manifest := range out {
		if seen[manifest.ID] { return nil, fmt.Errorf("duplicate add-on id %q", manifest.ID) }
		seen[manifest.ID] = true
	}
	return out, nil
}

func addonPublicManifest(manifest addonManifest) addonManifest {
	manifest.Command = ""
	manifest.Args = nil
	return manifest
}

func addonActionByID(manifest addonManifest, id string) (addonAction, bool) {
	for _, action := range manifest.Actions {
		if action.ID == id { return action, true }
	}
	return addonAction{}, false
}

func (s *Server) addonAllowed(r *http.Request, action addonAction) bool {
	principal, shared := PrincipalFromContext(r.Context())
	if !shared { return true }
	for _, permission := range action.Permissions {
		if !principal.Allowed(permission) && !principal.Allowed(identity.PermissionProjectAdmin) {
			return false
		}
	}
	return true
}

func runAddonProcess(ctx context.Context, manifest addonManifest, request addonProcessRequest) (addonProcessResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, addonActionTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, manifest.Command, manifest.Args...)
	payload, err := json.Marshal(request)
	if err != nil { return addonProcessResponse{}, err }
	cmd.Stdin = bytes.NewReader(append(payload, '
'))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &limitedWriter{Writer:&stdout, Remaining:addonMaxOutputBytes}
	cmd.Stderr = &limitedWriter{Writer:&stderr, Remaining:addonMaxOutputBytes}
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return addonProcessResponse{}, errors.New("add-on action timed out")
		}
		message := strings.TrimSpace(stderr.String())
		if message == "" { message = err.Error() }
		return addonProcessResponse{}, fmt.Errorf("add-on process failed: %s", message)
	}
	var response addonProcessResponse
	decoder := json.NewDecoder(io.LimitReader(bytes.NewReader(stdout.Bytes()), addonMaxOutputBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil {
		return addonProcessResponse{}, fmt.Errorf("decode add-on response: %w", err)
	}
	if !response.OK {
		if strings.TrimSpace(response.Error) == "" { response.Error = "add-on action failed" }
		return response, errors.New(response.Error)
	}
	return response, nil
}

type limitedWriter struct {
	Writer io.Writer
	Remaining int64
}

func (w *limitedWriter) Write(p []byte) (int,error) {
	original:=len(p)
	if w.Remaining<=0 { return original,nil }
	if int64(len(p))>w.Remaining { p=p[:w.Remaining] }
	n,err:=w.Writer.Write(p);w.Remaining-=int64(n)
	if err!=nil{return n,err}
	return original,nil
}

func (s *Server) addonsAPI(w http.ResponseWriter, r *http.Request) {
	manifests, err := loadAddonManifests()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	switch r.Method {
	case http.MethodGet:
		public := make([]addonManifest, 0, len(manifests))
		for _, manifest := range manifests { public = append(public, addonPublicManifest(manifest)) }
		writeJSON(w, http.StatusOK, map[string]any{"version":addonManifestVersion,"addons":public})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) addonActionAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	addonID := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/addons/"))
	if strings.Contains(addonID, "/") || !validAddonToken(addonID) {
		http.NotFound(w, r)
		return
	}
	var invoke addonInvokeRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&invoke); err != nil {
		http.Error(w, "invalid add-on action JSON", http.StatusBadRequest)
		return
	}
	invoke.ActionID = strings.TrimSpace(invoke.ActionID)
	manifests, err := loadAddonManifests()
	if err != nil { http.Error(w,err.Error(),http.StatusInternalServerError);return }
	var manifest *addonManifest
	for i := range manifests {
		if manifests[i].ID == addonID { manifest=&manifests[i];break }
	}
	if manifest == nil { http.NotFound(w,r);return }
	action, ok := addonActionByID(*manifest, invoke.ActionID)
	if !ok { http.Error(w,"add-on action not found",http.StatusNotFound);return }
	if !s.addonAllowed(r,action) { writePermissionDenied(w);return }

	response, err := runAddonProcess(r.Context(),*manifest,addonProcessRequest{
		Version:1,AddonID:manifest.ID,ActionID:action.ID,Workspace:s.Workspace,Context:invoke.Context,
	})
	if err != nil {
		s.auditSharedSuccess(r,"addon.action.error","addon",manifest.ID+":"+action.ID,map[string]any{"error":true})
		http.Error(w,err.Error(),http.StatusBadGateway)
		return
	}
	s.auditSharedSuccess(r,"addon.action","addon",manifest.ID+":"+action.ID,map[string]any{"background":action.Background})
	writeJSON(w,http.StatusOK,response)
}
