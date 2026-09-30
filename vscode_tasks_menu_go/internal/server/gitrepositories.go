package server

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"bletonfc/vscode_tasks_menu/internal/config"
)

const gitRepositoryCacheTTL = 15 * time.Second

type gitRepository struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	Root     string `json:"-"`
	Explicit bool   `json:"explicit,omitempty"`
	Default  bool   `json:"default,omitempty"`
}

type gitRepositoryStatus struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	Explicit bool   `json:"explicit,omitempty"`
	Default  bool   `json:"default,omitempty"`
	Branch   string `json:"branch,omitempty"`
	Head     string `json:"head,omitempty"`
	Changed  int    `json:"changed"`
	Ahead    int    `json:"ahead"`
	Behind   int    `json:"behind"`
	Error    string `json:"error,omitempty"`
}

type gitRepositoryContextKey struct{}

var gitScanIgnoredDirectories = map[string]bool{
	".git": true, ".hg": true, ".svn": true,
	"node_modules": true, "vendor": true, ".cache": true,
	"build": true, "dist": true, "out": true, "target": true,
	"Library": true, "Temp": true,
}

func canonicalExistingPath(value string) (string, error) {
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}

func pathInside(base, candidate string) bool {
	rel, err := filepath.Rel(base, candidate)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func repositoryID(workspace, root string) (string, error) {
	rel, err := filepath.Rel(workspace, root)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("git repository nằm ngoài workspace")
	}
	if rel == "." {
		return ".", nil
	}
	return filepath.ToSlash(rel), nil
}

func candidateHasGitMarker(root string) bool {
	info, err := os.Lstat(filepath.Join(root, ".git"))
	if err != nil {
		return false
	}
	return info.IsDir() || info.Mode().IsRegular()
}

func verifyGitRepository(workspace, candidate string) (string, error) {
	workspaceRoot, err := canonicalExistingPath(workspace)
	if err != nil {
		return "", err
	}
	candidateRoot, err := canonicalExistingPath(candidate)
	if err != nil {
		return "", err
	}
	if !pathInside(workspaceRoot, candidateRoot) {
		return "", fmt.Errorf("git repository nằm ngoài workspace")
	}
	cmd := exec.Command("git", "-C", candidateRoot, "rev-parse", "--show-toplevel")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_PAGER=cat", "LC_ALL=C")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("không phải Git repository")
	}
	top := strings.TrimSpace(string(out))
	if top == "" {
		return "", fmt.Errorf("Git repository root trống")
	}
	topRoot, err := canonicalExistingPath(top)
	if err != nil {
		return "", err
	}
	if !pathInside(workspaceRoot, topRoot) {
		return "", fmt.Errorf("git repository nằm ngoài workspace")
	}
	return topRoot, nil
}

func gitPathDepth(rel string) int {
	if rel == "." || rel == "" {
		return 0
	}
	return len(strings.Split(filepath.ToSlash(rel), "/"))
}

func (s *Server) discoverGitRepositories(force bool) ([]gitRepository, config.GitSettings, error) {
	s.gitReposMu.Lock()
	defer s.gitReposMu.Unlock()

	if !force && len(s.gitRepos) > 0 && time.Since(s.gitReposAt) < gitRepositoryCacheTTL {
		return append([]gitRepository(nil), s.gitRepos...), s.gitSettings, nil
	}

	settings, err := config.ReadGitSettings(s.Workspace)
	if err != nil {
		return nil, config.GitSettings{}, err
	}
	workspaceRoot, err := canonicalExistingPath(s.Workspace)
	if err != nil {
		return nil, settings, err
	}

	byID := map[string]gitRepository{}
	add := func(root, name string, explicit bool) {
		root, err = verifyGitRepository(workspaceRoot, root)
		if err != nil {
			return
		}
		id, err := repositoryID(workspaceRoot, root)
		if err != nil {
			return
		}
		displayName := strings.TrimSpace(name)
		if displayName == "" {
			if id == "." {
				displayName = filepath.Base(workspaceRoot)
			} else {
				displayName = filepath.Base(root)
			}
		}
		existing, ok := byID[id]
		if ok && existing.Explicit && !explicit {
			return
		}
		byID[id] = gitRepository{ID: id, Name: displayName, Path: id, Root: root, Explicit: explicit}
	}

	for name, rel := range settings.Repositories {
		candidate := workspaceRoot
		if rel != "." {
			candidate = filepath.Join(workspaceRoot, filepath.FromSlash(rel))
		}
		add(candidate, name, true)
	}

	if settings.ScanEnabled {
		_ = filepath.WalkDir(workspaceRoot, func(current string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				if entry != nil && entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if !entry.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(workspaceRoot, current)
			if err != nil {
				return filepath.SkipDir
			}
			depth := gitPathDepth(rel)
			if current != workspaceRoot && gitScanIgnoredDirectories[entry.Name()] {
				return filepath.SkipDir
			}
			if candidateHasGitMarker(current) {
				add(current, "", false)
			}
			if depth >= settings.ScanDepth {
				return filepath.SkipDir
			}
			return nil
		})
	}

	repos := make([]gitRepository, 0, len(byID))
	for _, item := range byID {
		repos = append(repos, item)
	}
	sort.Slice(repos, func(i, j int) bool {
		di, dj := gitPathDepth(repos[i].ID), gitPathDepth(repos[j].ID)
		if di != dj {
			return di < dj
		}
		return repos[i].ID < repos[j].ID
	})

	defaultID := settings.DefaultRepository
	if _, ok := byID[defaultID]; !ok {
		if _, ok := byID["."]; ok {
			defaultID = "."
		} else if len(repos) > 0 {
			defaultID = repos[0].ID
		} else {
			defaultID = ""
		}
	}
	for i := range repos {
		repos[i].Default = repos[i].ID == defaultID
	}

	s.gitRepos = append([]gitRepository(nil), repos...)
	s.gitReposAt = time.Now()
	s.gitSettings = settings
	s.gitDefaultRepo = defaultID
	return append([]gitRepository(nil), repos...), settings, nil
}

func (s *Server) resolveGitRepository(id string) (gitRepository, error) {
	repos, _, err := s.discoverGitRepositories(false)
	if err != nil {
		return gitRepository{}, err
	}
	id = strings.TrimSpace(strings.ReplaceAll(id, "\\", "/"))
	if id == "" {
		id = s.gitDefaultRepo
	}
	for _, item := range repos {
		if item.ID == id {
			return item, nil
		}
	}
	if id == "" {
		return gitRepository{}, fmt.Errorf("no Git repository found in workspace")
	}
	return gitRepository{}, fmt.Errorf("unknown Git repository %q", id)
}

func withGitRepository(ctx context.Context, repo gitRepository) context.Context {
	return context.WithValue(ctx, gitRepositoryContextKey{}, repo)
}

func gitRepositoryFromContext(ctx context.Context) (gitRepository, bool) {
	repo, ok := ctx.Value(gitRepositoryContextKey{}).(gitRepository)
	return repo, ok
}

func (s *Server) gitDirectory(ctx context.Context) string {
	if repo, ok := gitRepositoryFromContext(ctx); ok && repo.Root != "" {
		return repo.Root
	}
	return s.Workspace
}
