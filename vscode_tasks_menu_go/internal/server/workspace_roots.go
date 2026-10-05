package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"bletonfc/vscode_tasks_menu/internal/projectfiles"
)

const (
	workspaceRootsFile     = "vscode_tasks_menu.roots.json"
	workspaceRootsMaxBytes = 256 << 10
	workspaceRootsMaxCount = 32
	workspacePrimaryRootID = "primary"
)

type workspaceRootRecord struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	CreatedAt string `json:"created_at"`
}

type workspaceRootStore struct {
	Version int                   `json:"version"`
	Roots   []workspaceRootRecord `json:"roots"`
}

type workspaceRootView struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	Primary   bool   `json:"primary,omitempty"`
	Attached  bool   `json:"attached,omitempty"`
	Available bool   `json:"available"`
	CreatedAt string `json:"created_at,omitempty"`
}

type workspaceRootAttachRequest struct {
	Path string `json:"path"`
	Name string `json:"name,omitempty"`
}

type workspaceRootRenameRequest struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func workspaceRootsPath(workspace string) (string, error) {
	return projectfiles.Resolve(workspace, workspaceRootsFile)
}

func canonicalWorkspaceRoot(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsRune(value, '\x00') {
		return "", fmt.Errorf("workspace root path is required")
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("invalid workspace root path")
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("workspace root path does not exist")
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("workspace root path is not a directory")
	}
	return filepath.Clean(resolved), nil
}

func normalizeWorkspaceRootName(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" { value = strings.TrimSpace(fallback) }
	if len(value) > 120 { value = value[:120] }
	if strings.ContainsAny(value, "\r\n\x00") { return "" }
	return value
}

func normalizeWorkspaceRootID(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 80 || strings.ContainsAny(value, "\r\n\x00/\\") { return "" }
	return value
}

func newWorkspaceRootID() (string, error) {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil { return "", err }
	return "root-" + hex.EncodeToString(raw[:]), nil
}

func readWorkspaceRootStore(workspace string) (workspaceRootStore, error) {
	out := workspaceRootStore{Version: 1, Roots: []workspaceRootRecord{}}
	path, err := workspaceRootsPath(workspace)
	if err != nil { return out, err }
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) { return out, nil }
	if err != nil { return out, fmt.Errorf("read workspace roots: %w", err) }
	if len(data) > workspaceRootsMaxBytes { return out, fmt.Errorf("workspace roots file is too large") }
	if strings.TrimSpace(string(data)) == "" { return out, nil }
	if err := json.Unmarshal(data, &out); err != nil { return workspaceRootStore{Version:1,Roots:[]workspaceRootRecord{}}, fmt.Errorf("parse workspace roots: %w", err) }
	if out.Version != 1 { return workspaceRootStore{Version:1,Roots:[]workspaceRootRecord{}}, fmt.Errorf("unsupported workspace roots version") }
	if len(out.Roots) > workspaceRootsMaxCount { return workspaceRootStore{}, fmt.Errorf("workspace roots exceed %d entries", workspaceRootsMaxCount) }
	seenIDs := map[string]bool{}
	for i := range out.Roots {
		item := &out.Roots[i]
		item.ID = normalizeWorkspaceRootID(item.ID)
		item.Name = normalizeWorkspaceRootName(item.Name, filepath.Base(item.Path))
		item.Path = filepath.Clean(strings.TrimSpace(item.Path))
		item.CreatedAt = strings.TrimSpace(item.CreatedAt)
		if item.ID == "" || item.ID == workspacePrimaryRootID || seenIDs[item.ID] || item.Name == "" || item.Path == "" || !filepath.IsAbs(item.Path) || strings.ContainsRune(item.Path, '\x00') {
			return workspaceRootStore{}, fmt.Errorf("workspace roots file contains an invalid root")
		}
		seenIDs[item.ID] = true
	}
	return out, nil
}

func writeWorkspaceRootStore(workspace string, store workspaceRootStore) error {
	store.Version = 1
	if len(store.Roots) > workspaceRootsMaxCount { return fmt.Errorf("workspace roots exceed %d entries", workspaceRootsMaxCount) }
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil { return fmt.Errorf("encode workspace roots: %w", err) }
	data = append(data, '\n')
	if len(data) > workspaceRootsMaxBytes { return fmt.Errorf("workspace roots exceed size limit") }
	target, err := workspaceRootsPath(workspace)
	if err != nil { return err }
	tmp, err := os.CreateTemp(filepath.Dir(target), ".vscode_tasks_menu.roots.*.tmp")
	if err != nil { return fmt.Errorf("create workspace roots temp file: %w", err) }
	name := tmp.Name(); defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil { tmp.Close(); return err }
	if _, err := tmp.Write(data); err != nil { tmp.Close(); return fmt.Errorf("write workspace roots temp file: %w", err) }
	if err := tmp.Sync(); err != nil { tmp.Close(); return fmt.Errorf("sync workspace roots temp file: %w", err) }
	if err := tmp.Close(); err != nil { return fmt.Errorf("close workspace roots temp file: %w", err) }
	if err := os.Rename(name, target); err != nil { return fmt.Errorf("replace workspace roots: %w", err) }
	return os.Chmod(target, 0o600)
}

func (s *Server) primaryWorkspaceRoot() (workspaceRootView, error) {
	path, err := canonicalWorkspaceRoot(s.Workspace)
	if err != nil { return workspaceRootView{}, err }
	return workspaceRootView{ID:workspacePrimaryRootID,Name:normalizeWorkspaceRootName(filepath.Base(path),"Workspace"),Path:path,Primary:true,Available:true}, nil
}

func (s *Server) workspaceRootViews(includeAttached bool) ([]workspaceRootView, error) {
	primary, err := s.primaryWorkspaceRoot()
	if err != nil { return nil, err }
	rows := []workspaceRootView{primary}
	if !includeAttached { return rows, nil }
	store, err := readWorkspaceRootStore(s.Workspace)
	if err != nil { return nil, err }
	for _, item := range store.Roots {
		view := workspaceRootView{ID:item.ID,Name:item.Name,Path:item.Path,Attached:true,CreatedAt:item.CreatedAt}
		if canonical, resolveErr := canonicalWorkspaceRoot(item.Path); resolveErr == nil { view.Path=canonical; view.Available=true }
		rows = append(rows, view)
	}
	sort.SliceStable(rows[1:], func(i,j int) bool { return strings.ToLower(rows[i+1].Name) < strings.ToLower(rows[j+1].Name) })
	return rows, nil
}

func (s *Server) resolveWorkspaceRoot(id string) (workspaceRootView, error) {
	id = normalizeWorkspaceRootID(id)
	if id == "" || id == workspacePrimaryRootID { return s.primaryWorkspaceRoot() }
	if s.Config.SharedServerEnabled { return workspaceRootView{}, fmt.Errorf("attached workspace roots are disabled in shared-server mode") }
	store, err := readWorkspaceRootStore(s.Workspace)
	if err != nil { return workspaceRootView{}, err }
	for _, item := range store.Roots {
		if item.ID != id { continue }
		path, err := canonicalWorkspaceRoot(item.Path)
		if err != nil { return workspaceRootView{}, fmt.Errorf("workspace root %q is unavailable", item.Name) }
		return workspaceRootView{ID:item.ID,Name:item.Name,Path:path,Attached:true,Available:true,CreatedAt:item.CreatedAt}, nil
	}
	return workspaceRootView{}, fmt.Errorf("workspace root not found")
}

func (s *Server) workspaceRoots(w http.ResponseWriter, r *http.Request) {
	s.workspaceRootsMu.Lock()
	defer s.workspaceRootsMu.Unlock()
	if r.Method == http.MethodGet {
		rows, err := s.workspaceRootViews(!s.Config.SharedServerEnabled)
		if err != nil { http.Error(w, err.Error(), http.StatusInternalServerError); return }
		writeJSON(w,http.StatusOK,map[string]any{"roots":rows,"attached_enabled":!s.Config.SharedServerEnabled})
		return
	}
	if s.Config.SharedServerEnabled {
		http.Error(w,"attached workspace roots are disabled in shared-server mode",http.StatusForbidden)
		return
	}
	store, err := readWorkspaceRootStore(s.Workspace)
	if err != nil { http.Error(w,err.Error(),http.StatusInternalServerError); return }
	primary, err := s.primaryWorkspaceRoot()
	if err != nil { http.Error(w,err.Error(),http.StatusInternalServerError); return }
	switch r.Method {
	case http.MethodPost:
		var req workspaceRootAttachRequest
		dec:=json.NewDecoder(http.MaxBytesReader(w,r.Body,32<<10)); dec.DisallowUnknownFields()
		if err:=dec.Decode(&req); err!=nil { http.Error(w,"invalid JSON",http.StatusBadRequest); return }
		rawPath:=strings.TrimSpace(req.Path)
		if rawPath=="" { http.Error(w,"workspace root path is required",http.StatusBadRequest); return }
		if !filepath.IsAbs(rawPath) { rawPath=filepath.Join(primary.Path,filepath.FromSlash(rawPath)) }
		path, err:=canonicalWorkspaceRoot(rawPath)
		if err!=nil { http.Error(w,err.Error(),http.StatusBadRequest); return }
		if path==primary.Path { http.Error(w,"workspace root is already the primary root",http.StatusConflict); return }
		for _,item:=range store.Roots {
			if existing,resolveErr:=canonicalWorkspaceRoot(item.Path); resolveErr==nil && existing==path { http.Error(w,"workspace root is already attached",http.StatusConflict); return }
		}
		if len(store.Roots)>=workspaceRootsMaxCount { http.Error(w,"workspace root limit reached",http.StatusConflict); return }
		id,err:=newWorkspaceRootID(); if err!=nil { http.Error(w,"cannot allocate workspace root id",http.StatusInternalServerError); return }
		name:=normalizeWorkspaceRootName(req.Name,filepath.Base(path)); if name=="" { http.Error(w,"workspace root name is invalid",http.StatusBadRequest); return }
		item:=workspaceRootRecord{ID:id,Name:name,Path:path,CreatedAt:time.Now().UTC().Format(time.RFC3339Nano)}
		store.Roots=append(store.Roots,item)
		if err:=writeWorkspaceRootStore(s.Workspace,store); err!=nil { http.Error(w,err.Error(),http.StatusBadRequest); return }
		writeJSON(w,http.StatusCreated,workspaceRootView{ID:item.ID,Name:item.Name,Path:item.Path,Attached:true,Available:true,CreatedAt:item.CreatedAt})
	case http.MethodPut:
		var req workspaceRootRenameRequest
		dec:=json.NewDecoder(http.MaxBytesReader(w,r.Body,16<<10)); dec.DisallowUnknownFields()
		if err:=dec.Decode(&req); err!=nil { http.Error(w,"invalid JSON",http.StatusBadRequest); return }
		id:=normalizeWorkspaceRootID(req.ID); name:=normalizeWorkspaceRootName(req.Name,"")
		if id==""||id==workspacePrimaryRootID||name=="" { http.Error(w,"workspace root id and valid name are required",http.StatusBadRequest); return }
		found:=-1; for i:=range store.Roots { if store.Roots[i].ID==id { found=i; break } }
		if found<0 { http.Error(w,"workspace root not found",http.StatusNotFound); return }
		store.Roots[found].Name=name
		if err:=writeWorkspaceRootStore(s.Workspace,store); err!=nil { http.Error(w,err.Error(),http.StatusBadRequest); return }
		item:=store.Roots[found]; available:=false; path:=item.Path; if resolved,e:=canonicalWorkspaceRoot(item.Path); e==nil { path=resolved; available=true }
		writeJSON(w,http.StatusOK,workspaceRootView{ID:item.ID,Name:item.Name,Path:path,Attached:true,Available:available,CreatedAt:item.CreatedAt})
	case http.MethodDelete:
		id:=normalizeWorkspaceRootID(r.URL.Query().Get("id"))
		if id==""||id==workspacePrimaryRootID { http.Error(w,"attached workspace root id is required",http.StatusBadRequest); return }
		found:=false; next:=store.Roots[:0]; for _,item:=range store.Roots { if item.ID==id { found=true; continue }; next=append(next,item) }
		if !found { http.Error(w,"workspace root not found",http.StatusNotFound); return }
		store.Roots=next
		if err:=writeWorkspaceRootStore(s.Workspace,store); err!=nil { http.Error(w,err.Error(),http.StatusInternalServerError); return }
		writeJSON(w,http.StatusOK,map[string]any{"ok":true,"id":id})
	default:
		http.Error(w,"method not allowed",http.StatusMethodNotAllowed)
	}
}
