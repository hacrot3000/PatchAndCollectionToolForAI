package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	editorSessionVersion   = 1
	editorSessionMaxTabs   = 100
	editorSessionMaxPayload = int64(64 << 20)
	editorSwapMarker       = ".taskdeck-swap"
	editorSwapExcludeRule  = "*.taskdeck-swap*"
)

type editorSessionSelection struct {
	Anchor int `json:"anchor"`
	Head   int `json:"head"`
}

type editorSessionTab struct {
	Path            string                 `json:"path"`
	Dirty           bool                   `json:"dirty,omitempty"`
	Content         string                 `json:"content,omitempty"`
	SourceSHA256    string                 `json:"source_sha256,omitempty"`
	Selection       editorSessionSelection `json:"selection"`
	LineEnding      string                 `json:"line_ending,omitempty"`
	Encoding        string                 `json:"encoding,omitempty"`
	ExternalChanged bool                   `json:"external_changed,omitempty"`
}

type editorSessionState struct {
	Version   int                `json:"version"`
	Active    string             `json:"active,omitempty"`
	Tabs      []editorSessionTab `json:"tabs"`
	UpdatedNS int64              `json:"updated_ns"`
}

type editorSwapRecord struct {
	Version      int                    `json:"version"`
	Path         string                 `json:"path"`
	SourceSHA256 string                 `json:"source_sha256"`
	Content      string                 `json:"content"`
	Selection    editorSessionSelection `json:"selection"`
	LineEnding   string                 `json:"line_ending,omitempty"`
	Encoding     string                 `json:"encoding,omitempty"`
	UpdatedNS    int64                  `json:"updated_ns"`
}

func isTaskDeckSwapName(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	return strings.HasSuffix(lower, editorSwapMarker) || strings.Contains(lower, editorSwapMarker+".")
}

func editorSessionOwnerKey(r *http.Request) string {
	if principal, ok := PrincipalFromContext(r.Context()); ok {
		sum := sha256.Sum256([]byte(string(principal.UserID)))
		return hex.EncodeToString(sum[:6])
	}
	return "local"
}

func editorSessionCacheFile(workspace, owner string) (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(filepath.Clean(workspace)))
	return filepath.Join(cacheDir, "vscode_tasks_menu", "editor-session", fmt.Sprintf("%x-%s.json", sum[:12], owner)), nil
}

func writeEditorJSONAtomic(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	pattern := ".taskdeck-editor-*"
	if isTaskDeckSwapName(filepath.Base(path)) {
		pattern = ".taskdeck-swap.*"
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), pattern)
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}
	if err := tmp.Chmod(0o600); err != nil {
		cleanup()
		return err
	}
	enc := json.NewEncoder(tmp)
	if err := enc.Encode(value); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

func loadEditorSessionCache(workspace, owner string) (editorSessionState, error) {
	var state editorSessionState
	path, err := editorSessionCacheFile(workspace, owner)
	if err != nil {
		return state, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return state, err
	}
	if len(data) > 1<<20 {
		return state, fmt.Errorf("editor session cache too large")
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, err
	}
	if state.Version != editorSessionVersion || len(state.Tabs) > editorSessionMaxTabs {
		return editorSessionState{}, fmt.Errorf("invalid editor session cache")
	}
	return state, nil
}

func editorSwapPath(source, owner string) string {
	if owner == "local" {
		return source + editorSwapMarker
	}
	return source + editorSwapMarker + "." + owner
}

func ensureTaskDeckSwapGitExclude(sourcePath, root string) error {
	current := filepath.Dir(sourcePath)
	root = filepath.Clean(root)
	for pathWithin(root, current) {
		dotGit := filepath.Join(current, ".git")
		if info, err := os.Stat(dotGit); err == nil {
			gitDir := ""
			if info.IsDir() {
				gitDir = dotGit
			} else if info.Mode().IsRegular() {
				data, readErr := os.ReadFile(dotGit)
				if readErr == nil {
					line := strings.TrimSpace(string(data))
					if strings.HasPrefix(line, "gitdir:") {
						gitDir = strings.TrimSpace(strings.TrimPrefix(line, "gitdir:"))
						if !filepath.IsAbs(gitDir) {
							gitDir = filepath.Join(current, gitDir)
						}
						gitDir = filepath.Clean(gitDir)
					}
				}
			}
			if gitDir != "" {
				infoDir := filepath.Join(gitDir, "info")
				if err := os.MkdirAll(infoDir, 0o755); err != nil {
					return err
				}
				exclude := filepath.Join(infoDir, "exclude")
				data, _ := os.ReadFile(exclude)
				for _, line := range strings.Split(string(data), "\n") {
					if strings.TrimSpace(line) == editorSwapExcludeRule {
						return nil
					}
				}
				f, err := os.OpenFile(exclude, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
				if err != nil {
					return err
				}
				defer f.Close()
				prefix := ""
				if len(data) > 0 && data[len(data)-1] != '\n' {
					prefix = "\n"
				}
				_, err = f.WriteString(prefix + editorSwapExcludeRule + "\n")
				return err
			}
		}
		if current == root {
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return nil
}

func (s *Server) editorSessionResolveSource(rel string) (string, string, error) {
	rel, err := cleanProjectRelativePath(rel, false)
	if err != nil {
		return "", "", err
	}
	source, err := s.resolveProjectPath(rel, false, false)
	if err != nil {
		return "", "", err
	}
	rootView, _, err := s.projectRootForVirtualPath(rel)
	if err != nil {
		return "", "", err
	}
	return source, rootView.Path, nil
}

func readEditorSwap(path string) (editorSwapRecord, error) {
	var record editorSwapRecord
	data, err := os.ReadFile(path)
	if err != nil {
		return record, err
	}
	if int64(len(data)) > projectEditableLimit*2+(64<<10) {
		return record, fmt.Errorf("editor swap too large")
	}
	if err := json.Unmarshal(data, &record); err != nil {
		return record, err
	}
	if record.Version != editorSessionVersion || !utf8.ValidString(record.Content) || strings.ContainsRune(record.Content, '\x00') || int64(len(record.Content)) > projectEditableLimit {
		return editorSwapRecord{}, fmt.Errorf("invalid editor swap")
	}
	if _, err := cleanProjectRelativePath(record.Path, false); err != nil {
		return editorSwapRecord{}, fmt.Errorf("invalid editor swap path")
	}
	if len(record.SourceSHA256) != sha256.Size*2 {
		return editorSwapRecord{}, fmt.Errorf("invalid editor swap source hash")
	}
	if _, err := hex.DecodeString(record.SourceSHA256); err != nil {
		return editorSwapRecord{}, fmt.Errorf("invalid editor swap source hash")
	}
	if record.Selection.Anchor < 0 || record.Selection.Head < 0 {
		return editorSwapRecord{}, fmt.Errorf("invalid editor swap selection")
	}
	return record, nil
}

func (s *Server) loadEditorSession(r *http.Request) editorSessionState {
	owner := editorSessionOwnerKey(r)
	state, err := loadEditorSessionCache(s.Workspace, owner)
	if err != nil {
		return editorSessionState{Version: editorSessionVersion, Tabs: []editorSessionTab{}}
	}
	out := editorSessionState{Version: editorSessionVersion, Active: state.Active, Tabs: make([]editorSessionTab, 0, len(state.Tabs)), UpdatedNS: state.UpdatedNS}
	for _, tab := range state.Tabs {
		source, _, resolveErr := s.editorSessionResolveSource(tab.Path)
		if resolveErr != nil {
			continue
		}
		if tab.Dirty {
			swap, swapErr := readEditorSwap(editorSwapPath(source, owner))
			if swapErr == nil && swap.Path == tab.Path {
				tab.Content = swap.Content
				tab.SourceSHA256 = swap.SourceSHA256
				tab.Selection = swap.Selection
				tab.LineEnding = swap.LineEnding
				tab.Encoding = swap.Encoding
				if info, statErr := os.Stat(source); statErr == nil && info.Size() <= projectReadableLimit {
					if data, readErr := os.ReadFile(source); readErr == nil {
						sum := sha256.Sum256(data)
						tab.ExternalChanged = !strings.EqualFold(hex.EncodeToString(sum[:]), swap.SourceSHA256)
					}
				} else if statErr == nil {
					tab.ExternalChanged = true
				}
			} else {
				tab.Dirty = false
			}
		}
		out.Tabs = append(out.Tabs, tab)
	}
	return out
}

func validateEditorSessionTab(tab editorSessionTab) error {
	if tab.Selection.Anchor < 0 || tab.Selection.Head < 0 {
		return fmt.Errorf("editor selection must be non-negative")
	}
	if tab.Dirty {
		if !utf8.ValidString(tab.Content) || strings.ContainsRune(tab.Content, '\x00') {
			return fmt.Errorf("editor swap content must be valid UTF-8 text")
		}
		if int64(len(tab.Content)) > projectEditableLimit {
			return fmt.Errorf("editor swap content too large")
		}
		if len(tab.SourceSHA256) != sha256.Size*2 {
			return fmt.Errorf("source_sha256 is required for dirty editor")
		}
		if _, err := hex.DecodeString(tab.SourceSHA256); err != nil {
			return fmt.Errorf("invalid source_sha256")
		}
	}
	return nil
}

func (s *Server) saveEditorSession(r *http.Request, state editorSessionState) (editorSessionState, error) {
	if len(state.Tabs) > editorSessionMaxTabs {
		return editorSessionState{}, fmt.Errorf("too many editor tabs")
	}
	owner := editorSessionOwnerKey(r)
	previous, _ := loadEditorSessionCache(s.Workspace, owner)
	keepDirty := map[string]bool{}
	clean := editorSessionState{Version: editorSessionVersion, Active: strings.TrimSpace(state.Active), Tabs: make([]editorSessionTab, 0, len(state.Tabs)), UpdatedNS: time.Now().UnixNano()}
	seen := map[string]bool{}
	for _, tab := range state.Tabs {
		rel, err := cleanProjectRelativePath(tab.Path, false)
		if err != nil || seen[rel] {
			continue
		}
		tab.Path = rel
		seen[rel] = true
		if err := validateEditorSessionTab(tab); err != nil {
			return editorSessionState{}, err
		}
		source, root, err := s.editorSessionResolveSource(rel)
		if err != nil {
			continue
		}
		swapPath := editorSwapPath(source, owner)
		if tab.Dirty {
			record := editorSwapRecord{
				Version: editorSessionVersion, Path: rel, SourceSHA256: tab.SourceSHA256,
				Content: tab.Content, Selection: tab.Selection, LineEnding: tab.LineEnding,
				Encoding: tab.Encoding, UpdatedNS: clean.UpdatedNS,
			}
			if err := writeEditorJSONAtomic(swapPath, record); err != nil {
				return editorSessionState{}, err
			}
			_ = ensureTaskDeckSwapGitExclude(source, root)
			keepDirty[rel] = true
		} else {
			_ = os.Remove(swapPath)
		}
		tab.Content = ""
		tab.ExternalChanged = false
		clean.Tabs = append(clean.Tabs, tab)
	}
	for _, tab := range previous.Tabs {
		if !tab.Dirty || keepDirty[tab.Path] {
			continue
		}
		if source, _, err := s.editorSessionResolveSource(tab.Path); err == nil {
			_ = os.Remove(editorSwapPath(source, owner))
		}
	}
	cachePath, err := editorSessionCacheFile(s.Workspace, owner)
	if err != nil {
		return editorSessionState{}, err
	}
	if err := writeEditorJSONAtomic(cachePath, clean); err != nil {
		return editorSessionState{}, err
	}
	return clean, nil
}

func (s *Server) projectEditorSession(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.loadEditorSession(r))
	case http.MethodPut, http.MethodPost:
		var state editorSessionState
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, editorSessionMaxPayload))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&state); err != nil {
			http.Error(w, "invalid editor session payload", http.StatusBadRequest)
			return
		}
		saved, err := s.saveEditorSession(r, state)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, saved)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
