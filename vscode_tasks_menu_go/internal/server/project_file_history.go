package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"bletonfc/vscode_tasks_menu/internal/projectfiles"
)

const (
	projectFileHistoryDirName    = "vscode_tasks_menu.file_history"
	projectFileHistoryIndexName  = "index.json"
	projectFileHistoryMaxEntries = 12
	projectFileHistoryIndexMax   = 128 << 10
)

type projectFileHistoryEntry struct {
	ID        string `json:"id"`
	SavedAt   string `json:"saved_at"`
	SHA256    string `json:"sha256"`
	Size      int64  `json:"size"`
}

type projectFileHistoryIndex struct {
	Version int                       `json:"version"`
	Path    string                    `json:"path"`
	Entries []projectFileHistoryEntry `json:"entries"`
}

type projectFileHistoryContent struct {
	projectFileHistoryEntry
	Path       string `json:"path"`
	Content    string `json:"content"`
	Encoding   string `json:"encoding"`
	LineEnding string `json:"line_ending"`
	BOM        bool   `json:"bom"`
}

func projectFileHistoryPathKey(rel string) string {
	sum := sha256.Sum256([]byte(filepath.ToSlash(rel)))
	return hex.EncodeToString(sum[:])
}

func projectFileHistoryRoot(workspace string) string {
	return projectfiles.Path(workspace, projectFileHistoryDirName)
}

func ensurePrivateHistoryDir(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		if err := os.Mkdir(path, 0o700); err != nil {
			return err
		}
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("file history path is not a real directory")
	}
	if info.Mode().Perm()&0o077 != 0 {
		if err := os.Chmod(path, 0o700); err != nil {
			return err
		}
	}
	return nil
}

func projectFileHistoryDir(workspace, rel string, create bool) (string, error) {
	if err := projectfiles.EnsureDir(workspace); err != nil {
		return "", err
	}
	root := projectFileHistoryRoot(workspace)
	if create {
		if err := ensurePrivateHistoryDir(root); err != nil {
			return "", fmt.Errorf("prepare file history root: %w", err)
		}
	} else {
		info, err := os.Lstat(root)
		if os.IsNotExist(err) {
			return "", os.ErrNotExist
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return "", fmt.Errorf("file history root is unsafe")
		}
	}
	dir := filepath.Join(root, projectFileHistoryPathKey(rel))
	if create {
		if err := ensurePrivateHistoryDir(dir); err != nil {
			return "", fmt.Errorf("prepare file history directory: %w", err)
		}
	} else {
		info, err := os.Lstat(dir)
		if os.IsNotExist(err) {
			return "", os.ErrNotExist
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return "", fmt.Errorf("file history directory is unsafe")
		}
	}
	return dir, nil
}

func defaultProjectFileHistoryIndex(rel string) projectFileHistoryIndex {
	return projectFileHistoryIndex{Version: 1, Path: filepath.ToSlash(rel), Entries: []projectFileHistoryEntry{}}
}

func validProjectFileHistoryID(id string) bool {
	if len(id) < 15 || len(id) > 48 || strings.ContainsAny(id, "/\\\x00\r\n") {
		return false
	}
	parts := strings.Split(id, "-")
	if len(parts) != 2 || len(parts[1]) != 12 {
		return false
	}
	if _, err := strconv.ParseInt(parts[0], 10, 64); err != nil {
		return false
	}
	_, err := hex.DecodeString(parts[1])
	return err == nil
}

func readProjectFileHistoryIndex(workspace, rel string) (projectFileHistoryIndex, error) {
	out := defaultProjectFileHistoryIndex(rel)
	dir, err := projectFileHistoryDir(workspace, rel, false)
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	indexPath := filepath.Join(dir, projectFileHistoryIndexName)
	info, err := os.Lstat(indexPath)
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() > projectFileHistoryIndexMax {
		return out, fmt.Errorf("file history index is unsafe")
	}
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return defaultProjectFileHistoryIndex(rel), fmt.Errorf("parse file history index: %w", err)
	}
	if out.Version != 1 || filepath.ToSlash(out.Path) != filepath.ToSlash(rel) {
		return defaultProjectFileHistoryIndex(rel), fmt.Errorf("file history index identity mismatch")
	}
	clean := make([]projectFileHistoryEntry, 0, len(out.Entries))
	seen := map[string]bool{}
	for _, entry := range out.Entries {
		if !validProjectFileHistoryID(entry.ID) || len(entry.SHA256) != sha256.Size*2 || entry.Size < 0 || entry.Size > projectEditableLimit || seen[entry.ID] {
			continue
		}
		if _, err := hex.DecodeString(entry.SHA256); err != nil {
			continue
		}
		seen[entry.ID] = true
		clean = append(clean, entry)
		if len(clean) >= projectFileHistoryMaxEntries {
			break
		}
	}
	out.Entries = clean
	return out, nil
}

func writeProjectFileHistoryIndex(workspace, rel string, index projectFileHistoryIndex) error {
	dir, err := projectFileHistoryDir(workspace, rel, true)
	if err != nil {
		return err
	}
	index.Version = 1
	index.Path = filepath.ToSlash(rel)
	if len(index.Entries) > projectFileHistoryMaxEntries {
		index.Entries = index.Entries[:projectFileHistoryMaxEntries]
	}
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if len(data) > projectFileHistoryIndexMax {
		return fmt.Errorf("file history index exceeds size limit")
	}
	tmp, err := os.CreateTemp(dir, ".index.*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	target := filepath.Join(dir, projectFileHistoryIndexName)
	if err := os.Rename(name, target); err != nil {
		return err
	}
	return os.Chmod(target, 0o600)
}

func writeProjectFileHistoryBytes(dir, id string, data []byte) error {
	if !validProjectFileHistoryID(id) {
		return fmt.Errorf("invalid file history id")
	}
	tmp, err := os.CreateTemp(dir, ".entry.*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	target := filepath.Join(dir, id+".bin")
	if err := os.Rename(name, target); err != nil {
		return err
	}
	return os.Chmod(target, 0o600)
}

func recordProjectFileHistory(workspace, rel string, data []byte) error {
	if int64(len(data)) > projectEditableLimit || !projectTextBytesValid(data) {
		return nil
	}
	sum := sha256.Sum256(data)
	sha := hex.EncodeToString(sum[:])
	index, err := readProjectFileHistoryIndex(workspace, rel)
	if err != nil {
		return err
	}
	if len(index.Entries) > 0 && strings.EqualFold(index.Entries[0].SHA256, sha) {
		return nil
	}
	dir, err := projectFileHistoryDir(workspace, rel, true)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	id := strconv.FormatInt(now.UnixNano(), 10) + "-" + sha[:12]
	if err := writeProjectFileHistoryBytes(dir, id, data); err != nil {
		return fmt.Errorf("write file history entry: %w", err)
	}
	entry := projectFileHistoryEntry{
		ID: id, SavedAt: now.Format(time.RFC3339Nano), SHA256: sha, Size: int64(len(data)),
	}
	index.Entries = append([]projectFileHistoryEntry{entry}, index.Entries...)
	var removed []projectFileHistoryEntry
	if len(index.Entries) > projectFileHistoryMaxEntries {
		removed = append(removed, index.Entries[projectFileHistoryMaxEntries:]...)
		index.Entries = index.Entries[:projectFileHistoryMaxEntries]
	}
	if err := writeProjectFileHistoryIndex(workspace, rel, index); err != nil {
		_ = os.Remove(filepath.Join(dir, id+".bin"))
		return err
	}
	for _, old := range removed {
		if validProjectFileHistoryID(old.ID) {
			_ = os.Remove(filepath.Join(dir, old.ID+".bin"))
		}
	}
	return nil
}

func readProjectFileHistoryBytes(workspace, rel, id string) ([]byte, projectFileHistoryEntry, error) {
	if !validProjectFileHistoryID(id) {
		return nil, projectFileHistoryEntry{}, fmt.Errorf("invalid file history id")
	}
	index, err := readProjectFileHistoryIndex(workspace, rel)
	if err != nil {
		return nil, projectFileHistoryEntry{}, err
	}
	var selected projectFileHistoryEntry
	found := false
	for _, entry := range index.Entries {
		if entry.ID == id {
			selected, found = entry, true
			break
		}
	}
	if !found {
		return nil, projectFileHistoryEntry{}, os.ErrNotExist
	}
	dir, err := projectFileHistoryDir(workspace, rel, false)
	if err != nil {
		return nil, projectFileHistoryEntry{}, err
	}
	path := filepath.Join(dir, id+".bin")
	info, err := os.Lstat(path)
	if err != nil {
		return nil, projectFileHistoryEntry{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() != selected.Size || info.Size() > projectEditableLimit {
		return nil, projectFileHistoryEntry{}, fmt.Errorf("file history entry is unsafe")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, projectFileHistoryEntry{}, err
	}
	sum := sha256.Sum256(data)
	if !strings.EqualFold(hex.EncodeToString(sum[:]), selected.SHA256) {
		return nil, projectFileHistoryEntry{}, fmt.Errorf("file history entry checksum mismatch")
	}
	return data, selected, nil
}

func (s *Server) projectFileHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rel, err := cleanProjectRelativePath(r.URL.Query().Get("path"), false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		index, err := readProjectFileHistoryIndex(s.Workspace, rel)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		entries := append([]projectFileHistoryEntry(nil), index.Entries...)
		sort.SliceStable(entries, func(i, j int) bool { return entries[i].SavedAt > entries[j].SavedAt })
		writeJSON(w, http.StatusOK, map[string]any{"path": rel, "entries": entries})
		return
	}
	data, entry, err := readProjectFileHistoryBytes(s.Workspace, rel, id)
	if os.IsNotExist(err) {
		http.Error(w, "file history entry not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if !projectTextBytesValid(data) {
		http.Error(w, "file history entry is not editable UTF-8 text", http.StatusUnsupportedMediaType)
		return
	}
	bom := len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF
	text := data
	if bom {
		text = text[3:]
	}
	writeJSON(w, http.StatusOK, projectFileHistoryContent{
		projectFileHistoryEntry: entry,
		Path: rel, Content: string(text), Encoding: "utf-8",
		LineEnding: detectProjectLineEnding(text), BOM: bom,
	})
}
