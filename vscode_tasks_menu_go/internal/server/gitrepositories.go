package server

import (
	"context"
	"errors"
	"fmt"
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
	// Use the process working directory rather than "git -C": Git 1.7.x
	// (still installed on older RHEL/CentOS hosts) does not support -C.
	// Normal Git commands already use cmd.Dir via runGitInDirectory.
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = candidateRoot
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_PAGER=cat", "LC_ALL=C")
	out, err := cmd.Output()
	if err != nil {
		reason := err.Error()
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if stderr := strings.TrimSpace(string(exitErr.Stderr)); stderr != "" {
				reason = stderr
			}
		}
		if len(reason) > 350 {
			reason = reason[:350] + "…"
		}
		return "", fmt.Errorf("git rev-parse: %s", reason)
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

// Give conventional source roots priority over large generated trees. This
// makes a shallow repo in projects/ visible before the directory/time budget
// is exhausted exploring artifacts/ or other build outputs.
func gitDiscoveryDirectoryPriority(name string) int {
	switch strings.ToLower(name) {
	case "projects", "apps", "repositories", "repos", "packages", "src":
		return 0
	default:
		return 1
	}
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
	scanWarnings := make([]string, 0, 8)
	add := func(base discoveryBase, candidate, name string, explicit bool) {
		root, verifyErr := verifyGitRepository(base.Root, candidate)
		if verifyErr != nil {
			if len(scanWarnings) < 16 {
				rel, relErr := filepath.Rel(base.Root, candidate)
				if relErr != nil { rel = candidate }
				scanWarnings = append(scanWarnings, filepath.ToSlash(rel)+": "+verifyErr.Error())
			}
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

	// Breadth-first scan finds shallow application repositories before opening
	// large build/cache trees. Bound the effort; an explicit [git.repositories]
	// path can still be used when a tree exceeds the limit.
	for _, base := range bases {
		if candidateHasGitMarker(base.Root) {
			add(base, base.Root, "", false)
		}
		if !settings.ScanEnabled {
			continue
		}
		type scanDirectory struct {
			Path string
			Depth int
		}
		queue := []scanDirectory{{Path: base.Root, Depth: 0}}
		deadline := time.Now().Add(5 * time.Second)
		const maxScanDirectories = 12000
		truncated := false
		for index := 0; index < len(queue); index++ {
			if index >= maxScanDirectories || time.Now().After(deadline) {
				truncated = true
				break
			}
			current := queue[index]
			if base.View.Primary && current.Depth > 0 && len(attachedInsidePrimary) > 0 {
				resolved, resolveErr := canonicalExistingPath(current.Path)
				if resolveErr != nil || attachedInsidePrimary[resolved] {
					continue
				}
			}
			if current.Depth > 0 && candidateHasGitMarker(current.Path) {
				add(base, current.Path, "", false)
			}
			if current.Depth >= settings.ScanDepth {
				continue
			}
			children, readErr := os.ReadDir(current.Path)
			if readErr == nil {
				sort.SliceStable(children, func(i, j int) bool {
					return gitDiscoveryDirectoryPriority(children[i].Name()) < gitDiscoveryDirectoryPriority(children[j].Name())
				})
			}
			if readErr != nil {
				if len(scanWarnings) < 16 {
					rel, _ := filepath.Rel(base.Root, current.Path)
					scanWarnings = append(scanWarnings, filepath.ToSlash(rel)+": cannot list directory: "+readErr.Error())
				}
				continue
			}
			for _, entry := range children {
				if !entry.IsDir() || gitScanIgnoredDirectories[entry.Name()] {
					continue
				}
				if len(queue) >= maxScanDirectories {
					truncated = true
					break
				}
				queue = append(queue, scanDirectory{Path: filepath.Join(current.Path, entry.Name()), Depth: current.Depth + 1})
			}
			if truncated {
				break
			}
		}
		if truncated && len(scanWarnings) < 16 {
			scanWarnings = append(scanWarnings, "Git scan reached the time/directory budget under "+base.View.Name+"; configure [git.repositories] for any missing repositories.")
		}
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
	s.gitScanWarnings = scanWarnings
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
