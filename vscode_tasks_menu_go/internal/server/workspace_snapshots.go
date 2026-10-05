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
	workspaceSnapshotsFile       = "vscode_tasks_menu.snapshots.json"
	workspaceSnapshotsMaxBytes   = 2 << 20
	workspaceSnapshotsMaxPerUser = 40
	workspaceSnapshotMaxEditors  = 80
	workspaceSnapshotMaxTabs     = 120
	workspaceSnapshotMaxDBViews  = 20
	workspaceSnapshotMaxQueries  = 40
	workspaceSnapshotMaxTransfers = 20
)

type workspaceSnapshotQuery struct {
	Key      string `json:"key,omitempty"`
	Title    string `json:"title,omitempty"`
	Text     string `json:"text,omitempty"`
	FilePath string `json:"file_path,omitempty"`
}

type workspaceSnapshotDatabase struct {
	ProfileID      string                   `json:"profile_id"`
	ActiveQueryKey string                   `json:"active_query_key,omitempty"`
	Queries        []workspaceSnapshotQuery `json:"queries,omitempty"`
}

type workspaceSnapshotTransfer struct {
	ProfileID  string `json:"profile_id"`
	LocalMode  string `json:"local_mode,omitempty"`
	LocalPath  string `json:"local_path,omitempty"`
	RemotePath string `json:"remote_path,omitempty"`
}

type workspaceSnapshotGit struct {
	RepositoryID string `json:"repository_id,omitempty"`
	Branch       string `json:"branch,omitempty"`
	Head         string `json:"head,omitempty"`
}

type workspaceSnapshotClientState struct {
	TabOrder         []string                    `json:"tab_order,omitempty"`
	ActiveTab        string                      `json:"active_tab,omitempty"`
	EditorFiles      []string                    `json:"editor_files,omitempty"`
	ActiveEditor     string                      `json:"active_editor,omitempty"`
	ExplorerExpanded []string                    `json:"explorer_expanded,omitempty"`
	Databases        []workspaceSnapshotDatabase `json:"databases,omitempty"`
	Transfers        []workspaceSnapshotTransfer `json:"transfers,omitempty"`
	GitRepositoryID  string                      `json:"git_repository_id,omitempty"`
}

type workspaceSnapshot struct {
	ID        string                       `json:"id"`
	Name      string                       `json:"name"`
	CreatedAt string                       `json:"created_at"`
	UpdatedAt string                       `json:"updated_at"`
	State     workspaceSnapshotClientState `json:"state"`
	Terminal  projectTerminalState         `json:"terminal"`
	Git       workspaceSnapshotGit         `json:"git"`
}

type workspaceSnapshotRecord struct {
	OwnerKey string            `json:"owner_key"`
	Snapshot workspaceSnapshot `json:"snapshot"`
}

type workspaceSnapshotStore struct {
	Version   int                       `json:"version"`
	Snapshots []workspaceSnapshotRecord `json:"snapshots"`
}

type workspaceSnapshotSaveRequest struct {
	ID    string                       `json:"id,omitempty"`
	Name  string                       `json:"name"`
	State workspaceSnapshotClientState `json:"state"`
}

func workspaceSnapshotOwnerKey(s *Server, r *http.Request) (string, error) {
	if !s.Config.SharedServerEnabled {
		return "local", nil
	}
	principal, ok := PrincipalFromContext(r.Context())
	if !ok || principal.UserID == "" || principal.ProjectID == "" {
		return "", fmt.Errorf("workspace snapshot owner is unavailable")
	}
	return "project:" + string(principal.ProjectID) + ":user:" + string(principal.UserID), nil
}

func newWorkspaceSnapshotID() (string, error) {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func workspaceSnapshotsPath(workspace string) (string, error) {
	return projectfiles.Resolve(workspace, workspaceSnapshotsFile)
}

func readWorkspaceSnapshotStore(workspace string) (workspaceSnapshotStore, error) {
	out := workspaceSnapshotStore{Version: 1, Snapshots: []workspaceSnapshotRecord{}}
	path, err := workspaceSnapshotsPath(workspace)
	if err != nil {
		return out, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return out, fmt.Errorf("read workspace snapshots: %w", err)
	}
	if len(data) > workspaceSnapshotsMaxBytes {
		return out, fmt.Errorf("workspace snapshot file is too large")
	}
	if strings.TrimSpace(string(data)) == "" {
		return out, nil
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return workspaceSnapshotStore{Version: 1, Snapshots: []workspaceSnapshotRecord{}}, fmt.Errorf("parse workspace snapshots: %w", err)
	}
	if out.Version != 1 {
		return workspaceSnapshotStore{Version: 1, Snapshots: []workspaceSnapshotRecord{}}, fmt.Errorf("unsupported workspace snapshot version")
	}
	if out.Snapshots == nil {
		out.Snapshots = []workspaceSnapshotRecord{}
	}
	return out, nil
}

func writeWorkspaceSnapshotStore(workspace string, store workspaceSnapshotStore) error {
	store.Version = 1
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("encode workspace snapshots: %w", err)
	}
	data = append(data, '\n')
	if len(data) > workspaceSnapshotsMaxBytes {
		return fmt.Errorf("workspace snapshots exceed size limit")
	}
	target, err := workspaceSnapshotsPath(workspace)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".vscode_tasks_menu.snapshots.*.tmp")
	if err != nil {
		return fmt.Errorf("create workspace snapshot temp file: %w", err)
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write workspace snapshot temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync workspace snapshot temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close workspace snapshot temp file: %w", err)
	}
	if err := os.Rename(name, target); err != nil {
		return fmt.Errorf("replace workspace snapshots: %w", err)
	}
	return os.Chmod(target, 0o600)
}

func normalizeWorkspaceSnapshotName(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 120 {
		value = value[:120]
	}
	if strings.ContainsAny(value, "\r\n\x00") {
		return ""
	}
	return value
}

func normalizeWorkspaceSnapshotString(value string, max int) string {
	value = strings.TrimSpace(value)
	if max > 0 && len(value) > max {
		value = value[:max]
	}
	if strings.ContainsRune(value, '\x00') {
		return ""
	}
	return value
}

func normalizeSnapshotPath(value string, allowEmpty bool) string {
	value = strings.TrimSpace(value)
	if value == "" && allowEmpty {
		return ""
	}
	clean, err := validGitRelativePath(value)
	if err != nil {
		return ""
	}
	return filepath.ToSlash(clean)
}

func normalizeSnapshotStringList(values []string, limit, maxString int, paths bool) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		if paths {
			value = normalizeSnapshotPath(value, false)
		} else {
			value = normalizeWorkspaceSnapshotString(value, maxString)
		}
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func normalizeWorkspaceSnapshotClientState(value workspaceSnapshotClientState) workspaceSnapshotClientState {
	out := workspaceSnapshotClientState{}
	out.TabOrder = normalizeSnapshotStringList(value.TabOrder, workspaceSnapshotMaxTabs, 240, false)
	out.ActiveTab = normalizeWorkspaceSnapshotString(value.ActiveTab, 240)
	out.EditorFiles = normalizeSnapshotStringList(value.EditorFiles, workspaceSnapshotMaxEditors, 0, true)
	out.ActiveEditor = normalizeSnapshotPath(value.ActiveEditor, true)
	out.ExplorerExpanded = normalizeSnapshotStringList(value.ExplorerExpanded, workspaceSnapshotMaxEditors, 0, true)
	if out.ActiveEditor != "" {
		found := false
		for _, path := range out.EditorFiles {
			if path == out.ActiveEditor {
				found = true
				break
			}
		}
		if !found {
			out.ActiveEditor = ""
		}
	}
	for _, db := range value.Databases {
		profileID := normalizeWorkspaceSnapshotString(db.ProfileID, 160)
		if profileID == "" {
			continue
		}
		item := workspaceSnapshotDatabase{
			ProfileID: profileID,
			ActiveQueryKey: normalizeWorkspaceSnapshotString(db.ActiveQueryKey, 160),
			Queries: []workspaceSnapshotQuery{},
		}
		for _, query := range db.Queries {
			q := workspaceSnapshotQuery{
				Key: normalizeWorkspaceSnapshotString(query.Key, 160),
				Title: normalizeWorkspaceSnapshotString(query.Title, 240),
				Text: query.Text,
				FilePath: normalizeSnapshotPath(query.FilePath, true),
			}
			if len(q.Text) > 64<<10 {
				q.Text = q.Text[:64<<10]
			}
			if strings.ContainsRune(q.Text, '\x00') {
				q.Text = ""
			}
			item.Queries = append(item.Queries, q)
			if len(item.Queries) >= workspaceSnapshotMaxQueries {
				break
			}
		}
		out.Databases = append(out.Databases, item)
		if len(out.Databases) >= workspaceSnapshotMaxDBViews {
			break
		}
	}
	for _, transfer := range value.Transfers {
		profileID := normalizeWorkspaceSnapshotString(transfer.ProfileID, 160)
		if profileID == "" {
			continue
		}
		item := workspaceSnapshotTransfer{
			ProfileID: profileID,
			LocalMode: normalizeWorkspaceSnapshotString(transfer.LocalMode, 24),
			LocalPath: normalizeSnapshotPath(transfer.LocalPath, true),
			RemotePath: normalizeWorkspaceSnapshotString(transfer.RemotePath, 4096),
		}
		if strings.ContainsAny(item.RemotePath, "\r\n\x00") {
			item.RemotePath = ""
		}
		out.Transfers = append(out.Transfers, item)
		if len(out.Transfers) >= workspaceSnapshotMaxTransfers {
			break
		}
	}
	out.GitRepositoryID = normalizeWorkspaceSnapshotString(value.GitRepositoryID, 320)
	return out
}

func snapshotTerminalState(workspace string) projectTerminalState {
	state, err := readProjectTerminalStateProfile(workspace, "desktop")
	if err != nil {
		return defaultProjectTerminalState()
	}
	state = normalizeProjectTerminalState(state)
	for i := range state.Terminals {
		state.Terminals[i].SessionID = ""
	}
	state.LiveSplits = []terminalSnapshotSplitRequest{}
	return state
}

func (s *Server) snapshotGitState(repoID string) workspaceSnapshotGit {
	repoID = strings.TrimSpace(repoID)
	if repoID == "" {
		return workspaceSnapshotGit{}
	}
	repos, _, err := s.discoverGitRepositories(false)
	if err != nil {
		return workspaceSnapshotGit{}
	}
	for _, repo := range repos {
		if repo.ID != repoID {
			continue
		}
		ctx := withGitRepository(context.Background(), repo)
		branch, _ := s.gitCurrentBranch(ctx)
		head, _, _, _ := s.runGit(ctx, 4*time.Second, "rev-parse", "--verify", "HEAD^{commit}")
		return workspaceSnapshotGit{
			RepositoryID: repo.ID,
			Branch: normalizeWorkspaceSnapshotString(branch, 240),
			Head: strings.TrimSpace(head),
		}
	}
	return workspaceSnapshotGit{}
}

func filterWorkspaceSnapshots(store workspaceSnapshotStore, owner string) []workspaceSnapshot {
	rows := []workspaceSnapshot{}
	for _, record := range store.Snapshots {
		if record.OwnerKey == owner {
			rows = append(rows, record.Snapshot)
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].UpdatedAt > rows[j].UpdatedAt
	})
	return rows
}

func (s *Server) workspaceSnapshots(w http.ResponseWriter, r *http.Request) {
	owner, err := workspaceSnapshotOwnerKey(s, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	s.workspaceSnapshotsMu.Lock()
	defer s.workspaceSnapshotsMu.Unlock()

	store, err := readWorkspaceSnapshotStore(s.Workspace)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"snapshots": filterWorkspaceSnapshots(store, owner)})
	case http.MethodPost, http.MethodPut:
		var req workspaceSnapshotSaveRequest
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, workspaceSnapshotsMaxBytes))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		name := normalizeWorkspaceSnapshotName(req.Name)
		if name == "" {
			http.Error(w, "snapshot name is required", http.StatusBadRequest)
			return
		}
		state := normalizeWorkspaceSnapshotClientState(req.State)
		now := time.Now().UTC().Format(time.RFC3339Nano)
		id := strings.TrimSpace(req.ID)
		index := -1
		if id != "" {
			for i := range store.Snapshots {
				if store.Snapshots[i].OwnerKey == owner && store.Snapshots[i].Snapshot.ID == id {
					index = i
					break
				}
			}
			if index < 0 {
				http.Error(w, "workspace snapshot not found", http.StatusNotFound)
				return
			}
		} else {
			owned := filterWorkspaceSnapshots(store, owner)
			if len(owned) >= workspaceSnapshotsMaxPerUser {
				http.Error(w, "workspace snapshot limit reached", http.StatusConflict)
				return
			}
			id, err = newWorkspaceSnapshotID()
			if err != nil {
				http.Error(w, "cannot allocate workspace snapshot id", http.StatusInternalServerError)
				return
			}
		}
		snapshot := workspaceSnapshot{
			ID: id, Name: name, UpdatedAt: now, State: state,
			Terminal: snapshotTerminalState(s.Workspace),
			Git: s.snapshotGitState(state.GitRepositoryID),
		}
		if index >= 0 {
			snapshot.CreatedAt = store.Snapshots[index].Snapshot.CreatedAt
			if snapshot.CreatedAt == "" {
				snapshot.CreatedAt = now
			}
			store.Snapshots[index].Snapshot = snapshot
		} else {
			snapshot.CreatedAt = now
			store.Snapshots = append(store.Snapshots, workspaceSnapshotRecord{OwnerKey: owner, Snapshot: snapshot})
		}
		if err := writeWorkspaceSnapshotStore(s.Workspace, store); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.auditSharedSuccess(r, "workspace_snapshot.save", "workspace_snapshot", snapshot.ID, map[string]any{"name": snapshot.Name})
		writeJSON(w, http.StatusOK, snapshot)
	case http.MethodDelete:
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		if id == "" {
			http.Error(w, "workspace snapshot id is required", http.StatusBadRequest)
			return
		}
		found := false
		next := store.Snapshots[:0]
		for _, record := range store.Snapshots {
			if record.OwnerKey == owner && record.Snapshot.ID == id {
				found = true
				continue
			}
			next = append(next, record)
		}
		if !found {
			http.Error(w, "workspace snapshot not found", http.StatusNotFound)
			return
		}
		store.Snapshots = next
		if err := writeWorkspaceSnapshotStore(s.Workspace, store); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.auditSharedSuccess(r, "workspace_snapshot.delete", "workspace_snapshot", id, nil)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
