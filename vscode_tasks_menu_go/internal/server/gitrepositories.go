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

	if !force && !s.gitReposAt.IsZero() && time.Since(s.gitReposAt) < gitRepositoryCacheTTL {
		return append([]gitRepository(nil), s.gitRepos...), s.gitSettings, nil
	}

	settings, err := config.ReadGitSettings(s.Workspace)
	if err != nil {
		return nil, config.GitSettings{}, err
	}
	rootViews, err := s.workspaceRootViews(!s.Config.SharedServerEnabled)
	if err != nil {
		return nil, settings, err
	}
	type discoveryBase struct {
		View workspaceRootView
		Root string
	}
	bases := make([]discoveryBase, 0, len(rootViews))
	baseByID := map[string]discoveryBase{}
	for _, view := range rootViews {
		if !view.Available {
			continue
		}
		root, resolveErr := canonicalExistingPath(view.Path)
		if resolveErr != nil {
			continue
		}
		base := discoveryBase{View: view, Root: root}
		bases = append(bases, base)
		baseByID[view.ID] = base
	}
	primary, ok := baseByID[workspacePrimaryRootID]
	if !ok {
		return nil, settings, fmt.Errorf("primary workspace root is unavailable")
	}

	virtualRepoID := func(base discoveryBase, root string) (string, error) {
		rel, err := repositoryID(base.Root, root)
		if err != nil {
			return "", err
		}
		if base.View.Primary {
			return rel, nil
		}
		if rel == "." {
			return workspaceVirtualPath(base.View.ID, ""), nil
		}
		return workspaceVirtualPath(base.View.ID, rel), nil
	}

	byID := map[string]gitRepository{}
	add := func(base discoveryBase, candidate, name string, explicit bool) {
		root, verifyErr := verifyGitRepository(base.Root, candidate)
		if verifyErr != nil {
			return
		}
		id, idErr := virtualRepoID(base, root)
		if idErr != nil {
			return
		}
		displayName := strings.TrimSpace(name)
		if displayName == "" {
			if root == base.Root {
				displayName = base.View.Name
			}
			if displayName == "" {
				displayName = filepath.Base(root)
			}
		}
		existing, exists := byID[id]
		if exists && existing.Explicit && !explicit {
			return
		}
		byID[id] = gitRepository{ID: id, Name: displayName, Path: id, Root: root, Explicit: explicit}
	}

	for name, rel := range settings.Repositories {
		candidate := primary.Root
		if rel != "." {
			candidate = filepath.Join(primary.Root, filepath.FromSlash(rel))
		}
		add(primary, candidate, name, true)
	}

	attachedInsidePrimary := map[string]bool{}
	for _, base := range bases {
		if base.View.Primary {
			continue
		}
		if pathInside(primary.Root, base.Root) {
			attachedInsidePrimary[base.Root] = true
		}
	}

	for _, base := range bases {
		if candidateHasGitMarker(base.Root) {
			add(base, base.Root, "", false)
		}
		if !settings.ScanEnabled {
			continue
		}
		_ = filepath.WalkDir(base.Root, func(current string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				if entry != nil && entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if !entry.IsDir() {
				return nil
			}
			resolvedCurrent := current
			if canonical, canonicalErr := canonicalExistingPath(current); canonicalErr == nil {
				resolvedCurrent = canonical
			}
			if base.View.Primary && current != base.Root && attachedInsidePrimary[resolvedCurrent] {
				return filepath.SkipDir
			}
			rel, relErr := filepath.Rel(base.Root, current)
			if relErr != nil {
				return filepath.SkipDir
			}
			depth := gitPathDepth(rel)
			if current != base.Root && gitScanIgnoredDirectories[entry.Name()] {
				return filepath.SkipDir
			}
			if candidateHasGitMarker(current) {
				add(base, current, "", false)
			}
			if depth >= settings.ScanDepth {
				return filepath.SkipDir
			}
			return nil
		})
	}

	baseForRepo := func(repo gitRepository) (discoveryBase, bool) {
		if strings.HasPrefix(repo.ID, "@root/") {
			rest := strings.TrimPrefix(repo.ID, "@root/")
			id := rest
			if index := strings.IndexByte(rest, '/'); index >= 0 {
				id = rest[:index]
			}
			base, ok := baseByID[id]
			return base, ok
		}
		return primary, true
	}

	for pass := 0; pass < 32; pass++ {
		before := len(byID)
		snapshot := make([]gitRepository, 0, len(byID))
		for _, item := range byID {
			snapshot = append(snapshot, item)
		}
		for _, parentRepo := range snapshot {
			base, baseOK := baseForRepo(parentRepo)
			if !baseOK {
				continue
			}
			ctx := withGitRepository(context.Background(), parentRepo)
			configs, configErr := s.gitSubmoduleConfigs(ctx)
			if configErr != nil {
				continue
			}
			for _, submodule := range configs {
				candidate := filepath.Join(parentRepo.Root, filepath.FromSlash(submodule.Path))
				if candidateHasGitMarker(candidate) {
					add(base, candidate, submodule.Name, false)
				}
			}
		}
		if len(byID) == before {
			break
		}
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
