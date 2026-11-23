package agentrun

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/patchtool"
)

type WorkspaceBinding struct {
	OwnerWorkspaceID               string    `json:"owner_workspace_id"`
	ExecutionWorkspaceID           string    `json:"execution_workspace_id"`
	WorktreePath                   string    `json:"worktree_path"`
	ConversationID                 string    `json:"conversation_id"`
	RunID                          string    `json:"run_id"`
	ProviderConfigurationReference string    `json:"provider_configuration_reference"`
	PermissionScope                []string  `json:"permission_scope"`
	LifecycleStatus                string    `json:"lifecycle_status"`
	ProjectID                      string    `json:"project_id"`
	GitCommonDir                   string    `json:"git_common_dir"`
	BaseHEAD                       string    `json:"base_head"`
	CreatedAt                      time.Time `json:"created_at"`
	UpdatedAt                      time.Time `json:"updated_at"`
}

type workspaceRegistryFile struct {
	Version int                `json:"version"`
	Entries []WorkspaceBinding `json:"entries"`
}

var workspaceRegistryLocks sync.Map

type WorkspaceResolver interface {
	Resolve(context.Context, agentchat.RunSpec) (WorkspaceBinding, error)
}

type WorkspaceRegistry struct {
	lock *sync.Mutex
	path string
	root string
}

func NewWorkspaceRegistry(path, ownerRoot string) (*WorkspaceRegistry, error) {
	if path == "" || !filepath.IsAbs(path) || ownerRoot == "" || !filepath.IsAbs(ownerRoot) {
		return nil, errors.New("absolute registry and owner workspace paths are required")
	}
	root, err := canonicalDirectory(ownerRoot)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	cleanPath := filepath.Clean(path)
	lockValue, _ := workspaceRegistryLocks.LoadOrStore(cleanPath, &sync.Mutex{})
	return &WorkspaceRegistry{lock: lockValue.(*sync.Mutex), path: cleanPath, root: root}, nil
}

func canonicalDirectory(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(resolved)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("workspace must be a real directory")
	}
	return filepath.Clean(resolved), nil
}

func WorkspacePath(path string) (string, error) { return canonicalDirectory(path) }

func workspaceID(path string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(path)))
	return hex.EncodeToString(sum[:])
}

func WorkspaceID(path string) (string, error) {
	canonical, err := canonicalDirectory(path)
	if err != nil {
		return "", err
	}
	return workspaceID(canonical), nil
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return "", errors.New("Git workspace validation failed")
	}
	return strings.TrimSpace(string(out)), nil
}

func gitCommonDir(ctx context.Context, path string) (string, error) {
	out, err := gitOutput(ctx, path, "rev-parse", "--git-common-dir")
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(out) {
		out = filepath.Join(path, out)
	}
	resolved, err := filepath.EvalSymlinks(out)
	if err != nil {
		return "", errors.New("Git common directory is unavailable")
	}
	return filepath.Clean(resolved), nil
}

func isListedWorktree(ctx context.Context, owner, execution string) (string, error) {
	out, err := gitOutput(ctx, owner, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		out, err = gitOutput(ctx, owner, "worktree", "list", "--porcelain")
	}
	if err != nil {
		return "", err
	}
	fields := strings.Split(out, "\x00")
	if len(fields) == 1 {
		fields = strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n")
	}
	for _, field := range fields {
		if strings.HasPrefix(field, "worktree ") {
			candidate := strings.TrimPrefix(field, "worktree ")
			path, pathErr := canonicalDirectory(candidate)
			if pathErr == nil && path == execution {
				head, headErr := gitOutput(ctx, execution, "rev-parse", "--verify", "HEAD^{commit}")
				if headErr != nil {
					return "", headErr
				}
				return head, nil
			}
		}
	}
	return "", errors.New("execution directory is not a registered worktree of owner repository")
}

func (r *WorkspaceRegistry) CreateDetached(ctx context.Context, targetPath, projectID, conversationID, runID, providerRef string, permissionScope []string) (WorkspaceBinding, error) {
	ownerIdentity, err := patchtool.ReadProjectIdentity(r.root)
	if err != nil || ownerIdentity.ID != projectID {
		return WorkspaceBinding{}, errors.New("owner project identity mismatch")
	}
	parent, err := canonicalDirectory(filepath.Dir(targetPath))
	if err != nil {
		return WorkspaceBinding{}, errors.New("worktree parent directory is unavailable")
	}
	targetPath = filepath.Join(parent, filepath.Base(targetPath))
	relative, relErr := filepath.Rel(r.root, targetPath)
	if targetPath == r.root || !filepath.IsAbs(targetPath) || relErr != nil || (!filepath.IsAbs(relative) && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))) {
		return WorkspaceBinding{}, errors.New("worktree target must be outside the owner worktree")
	}
	if _, err := os.Lstat(targetPath); err == nil {
		return WorkspaceBinding{}, errors.New("worktree target already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return WorkspaceBinding{}, errors.New("cannot inspect worktree target")
	}
	if _, err := gitOutput(ctx, r.root, "worktree", "add", "--detach", "--", targetPath, "HEAD"); err != nil {
		return WorkspaceBinding{}, err
	}
	configPath := filepath.Join(targetPath, ".python_patch_tool.json")
	if _, err := os.Lstat(configPath); errors.Is(err, os.ErrNotExist) {
		data, _ := json.Marshal(map[string]any{"project": ownerIdentity})
		file, createErr := os.OpenFile(configPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if createErr != nil {
			return WorkspaceBinding{}, errors.New("cannot bind owner project identity to worktree")
		}
		_, writeErr := file.Write(append(data, '\n'))
		if writeErr == nil {
			writeErr = file.Sync()
		}
		closeErr := file.Close()
		if writeErr == nil {
			writeErr = closeErr
		}
		if writeErr != nil {
			return WorkspaceBinding{}, errors.New("cannot persist worktree project identity")
		}
	} else if err != nil {
		return WorkspaceBinding{}, errors.New("unsafe worktree project identity file")
	}
	return r.Register(ctx, targetPath, projectID, conversationID, runID, providerRef, permissionScope)
}

func (r *WorkspaceRegistry) Register(ctx context.Context, executionPath, projectID, conversationID, runID, providerRef string, permissionScope []string) (WorkspaceBinding, error) {
	r.lock.Lock()
	defer r.lock.Unlock()
	execution, err := canonicalDirectory(executionPath)
	if err != nil {
		return WorkspaceBinding{}, err
	}
	if execution == r.root || projectID == "" || conversationID == "" || runID == "" || providerRef == "" || len(permissionScope) == 0 {
		return WorkspaceBinding{}, errors.New("worktree binding fields are incomplete")
	}
	ownerIdentity, err := patchtool.ReadProjectIdentity(r.root)
	if err != nil || ownerIdentity.ID != projectID {
		return WorkspaceBinding{}, errors.New("owner project identity mismatch")
	}
	childIdentity, err := patchtool.ReadProjectIdentity(execution)
	if err != nil || childIdentity.ID != projectID || childIdentity.Key != ownerIdentity.Key {
		return WorkspaceBinding{}, errors.New("worktree project identity mismatch")
	}
	ownerGit, err := gitCommonDir(ctx, r.root)
	if err != nil {
		return WorkspaceBinding{}, err
	}
	childGit, err := gitCommonDir(ctx, execution)
	if err != nil || !samePath(ownerGit, childGit) {
		return WorkspaceBinding{}, errors.New("worktree belongs to a different repository")
	}
	head, err := isListedWorktree(ctx, r.root, execution)
	if err != nil {
		return WorkspaceBinding{}, err
	}
	now := time.Now().UTC()
	binding := WorkspaceBinding{
		OwnerWorkspaceID: workspaceID(r.root), ExecutionWorkspaceID: workspaceID(execution),
		WorktreePath: execution, ConversationID: conversationID, RunID: runID,
		ProviderConfigurationReference: providerRef, PermissionScope: append([]string(nil), permissionScope...),
		LifecycleStatus: "active", ProjectID: projectID, GitCommonDir: ownerGit, BaseHEAD: head,
		CreatedAt: now, UpdatedAt: now,
	}
	registry, err := r.readLocked()
	if err != nil {
		return WorkspaceBinding{}, err
	}
	for _, current := range registry.Entries {
		if current.ExecutionWorkspaceID == binding.ExecutionWorkspaceID {
			return WorkspaceBinding{}, errors.New("worktree is already registered")
		}
	}
	registry.Entries = append(registry.Entries, binding)
	if err := r.writeLocked(registry); err != nil {
		return WorkspaceBinding{}, err
	}
	return binding, nil
}

func (r *WorkspaceRegistry) ResolveRun(ctx context.Context, spec agentchat.RunSpec) (WorkspaceBinding, error) {
	if err := spec.Validate(); err != nil {
		return WorkspaceBinding{}, err
	}
	ownerID := workspaceID(r.root)
	if spec.OwnerWorkspaceID != ownerID {
		return WorkspaceBinding{}, errors.New("run owner workspace mismatch")
	}
	identity, err := patchtool.ReadProjectIdentity(r.root)
	if err != nil {
		return WorkspaceBinding{}, errors.New("owner project identity unavailable")
	}
	if spec.ExecutionWorkspaceID == ownerID {
		commonDir, commonErr := gitCommonDir(ctx, r.root)
		if commonErr != nil {
			commonDir = ""
		}
		return WorkspaceBinding{
			OwnerWorkspaceID: ownerID, ExecutionWorkspaceID: ownerID, WorktreePath: r.root,
			ConversationID: spec.ConversationID, RunID: spec.RunID,
			ProviderConfigurationReference: spec.ProviderID + "@" + spec.ProviderConfigVersion,
			PermissionScope:                []string{"files.read", "patch.propose", "patch.execute.approval_required"},
			LifecycleStatus:                "active", ProjectID: identity.ID, GitCommonDir: commonDir,
		}, nil
	}
	return r.Resolve(ctx, spec.ExecutionWorkspaceID, spec.ConversationID, spec.RunID, identity.ID)
}

func (r *WorkspaceRegistry) Resolve(ctx context.Context, executionID, conversationID, runID, projectID string) (WorkspaceBinding, error) {
	r.lock.Lock()
	defer r.lock.Unlock()
	registry, err := r.readLocked()
	if err != nil {
		return WorkspaceBinding{}, err
	}
	for _, binding := range registry.Entries {
		if binding.ExecutionWorkspaceID != executionID {
			continue
		}
		if binding.OwnerWorkspaceID != workspaceID(r.root) || binding.ConversationID != conversationID || binding.RunID != runID || binding.ProjectID != projectID || binding.LifecycleStatus != "active" {
			return WorkspaceBinding{}, errors.New("worktree binding scope mismatch")
		}
		path, err := canonicalDirectory(binding.WorktreePath)
		if err != nil || path != binding.WorktreePath {
			return WorkspaceBinding{}, errors.New("registered worktree path changed")
		}
		identity, err := patchtool.ReadProjectIdentity(path)
		if err != nil || identity.ID != projectID {
			return WorkspaceBinding{}, errors.New("registered worktree identity changed")
		}
		ownerGit, err := gitCommonDir(ctx, r.root)
		if err != nil || !samePath(ownerGit, binding.GitCommonDir) {
			return WorkspaceBinding{}, errors.New("owner Git repository changed")
		}
		childGit, err := gitCommonDir(ctx, path)
		if err != nil || !samePath(childGit, ownerGit) {
			return WorkspaceBinding{}, errors.New("worktree repository changed")
		}
		if _, err := isListedWorktree(ctx, r.root, path); err != nil {
			return WorkspaceBinding{}, err
		}
		return binding, nil
	}
	return WorkspaceBinding{}, errors.New("worktree is not registered for this owner")
}

func samePath(a, b string) bool {
	a, _ = filepath.Abs(a)
	b, _ = filepath.Abs(b)
	if os.PathSeparator == '\\' {
		return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func (r *WorkspaceRegistry) readLocked() (workspaceRegistryFile, error) {
	info, err := os.Lstat(r.path)
	if errors.Is(err, os.ErrNotExist) {
		return workspaceRegistryFile{Version: 1, Entries: []WorkspaceBinding{}}, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 || info.Size() > 1<<20 {
		return workspaceRegistryFile{}, errors.New("unsafe workspace registry")
	}
	body, err := os.ReadFile(r.path)
	if err != nil {
		return workspaceRegistryFile{}, err
	}
	var registry workspaceRegistryFile
	if err := json.Unmarshal(body, &registry); err != nil || registry.Version != 1 || len(registry.Entries) > 512 {
		return workspaceRegistryFile{}, errors.New("invalid workspace registry")
	}
	return registry, nil
}

func (r *WorkspaceRegistry) writeLocked(registry workspaceRegistryFile) error {
	body, err := json.Marshal(registry)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(r.path), ".workspaces-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(append(body, '\n'))
	}
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, r.path)
}
