package server

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	projectSymbolIndexVersion    = 1
	projectSymbolIndexFreshFor   = 10 * time.Minute
	projectSymbolIndexMaxFiles   = 50000
	projectSymbolIndexMaxBytes   = int64(128 << 20)
	projectSymbolIndexMaxSymbols = 250000
)

type projectSymbolIndex struct {
	Version      int                   `json:"version"`
	Fingerprint  string                `json:"fingerprint"`
	BuiltAtNS    int64                 `json:"built_at_ns"`
	Symbols      []projectSymbolResult `json:"symbols"`
	ScannedFiles int                   `json:"scanned_files"`
	ScannedBytes int64                 `json:"scanned_bytes"`
	Truncated    bool                  `json:"truncated,omitempty"`
}

func (idx *projectSymbolIndex) builtAt() time.Time {
	if idx == nil || idx.BuiltAtNS <= 0 {
		return time.Time{}
	}
	return time.Unix(0, idx.BuiltAtNS)
}

func projectSymbolRootsFingerprint(roots []workspaceRootView) string {
	h := sha256.New()
	for _, root := range roots {
		if !root.Available {
			continue
		}
		_, _ = io.WriteString(h, root.ID)
		_, _ = io.WriteString(h, "\x00")
		_, _ = io.WriteString(h, filepath.Clean(root.Path))
		_, _ = io.WriteString(h, "\x00")
	}
	return hex.EncodeToString(h.Sum(nil))
}

func projectSymbolIndexCacheFile(workspace string) (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(filepath.Clean(workspace)))
	return filepath.Join(cacheDir, "vscode_tasks_menu", "symbol-index", fmt.Sprintf("%x.json.gz", sum[:12])), nil
}

func loadProjectSymbolIndexCache(workspace, fingerprint string) (*projectSymbolIndex, error) {
	path, err := projectSymbolIndexCacheFile(workspace)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	zr, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	decoder := json.NewDecoder(io.LimitReader(zr, 64<<20))
	var idx projectSymbolIndex
	if err := decoder.Decode(&idx); err != nil {
		return nil, err
	}
	if idx.Version != projectSymbolIndexVersion || idx.Fingerprint != fingerprint || idx.BuiltAtNS <= 0 {
		return nil, errors.New("stale project symbol index cache")
	}
	if len(idx.Symbols) > projectSymbolIndexMaxSymbols || idx.ScannedFiles > projectSymbolIndexMaxFiles || idx.ScannedBytes > projectSymbolIndexMaxBytes {
		return nil, errors.New("project symbol index cache exceeds limits")
	}
	return &idx, nil
}

func saveProjectSymbolIndexCache(workspace string, idx *projectSymbolIndex) error {
	if idx == nil {
		return errors.New("project symbol index is nil")
	}
	path, err := projectSymbolIndexCacheFile(workspace)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".symbol-index-*")
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
	zw := gzip.NewWriter(tmp)
	encoder := json.NewEncoder(zw)
	if err := encoder.Encode(idx); err != nil {
		_ = zw.Close()
		cleanup()
		return err
	}
	if err := zw.Close(); err != nil {
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

var errProjectSymbolIndexDone = errors.New("project symbol index limit reached")

func appendProjectSymbolIndexRoot(ctx context.Context, root workspaceRootView, idx *projectSymbolIndex) error {
	ignore := loadProjectRootIgnore(root.Path)
	return filepath.WalkDir(root.Path, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if idx.ScannedFiles >= projectSymbolIndexMaxFiles || idx.ScannedBytes >= projectSymbolIndexMaxBytes || len(idx.Symbols) >= projectSymbolIndexMaxSymbols {
			idx.Truncated = true
			return errProjectSymbolIndexDone
		}
		if current == root.Path {
			return nil
		}
		rel, err := filepath.Rel(root.Path, current)
		if err != nil {
			return nil
		}
		rel = normalizeProjectIndexPath(filepath.ToSlash(rel))
		if rel == "" {
			return nil
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Type()&os.ModeSymlink != 0 || ignore.matches(rel, true) {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() || ignore.matches(rel, false) || projectSymbolLanguage(rel) == "" {
			return nil
		}
		info, err := entry.Info()
		if err != nil || info.Size() < 0 || info.Size() > projectSymbolFileMaxBytes {
			return nil
		}
		if idx.ScannedBytes+info.Size() > projectSymbolIndexMaxBytes {
			idx.Truncated = true
			return errProjectSymbolIndexDone
		}
		pinned, err := openProjectPinnedFile(root.Path, current)
		if err != nil {
			return nil
		}
		data, _, readErr := pinned.readCurrent(projectSymbolFileMaxBytes)
		pinned.close()
		if readErr != nil || strings.IndexByte(string(data), 0) >= 0 {
			return nil
		}
		idx.ScannedFiles++
		idx.ScannedBytes += int64(len(data))
		virtual := workspaceVirtualPath(root.ID, rel)
		remaining := projectSymbolIndexMaxSymbols - len(idx.Symbols)
		if remaining <= 0 {
			idx.Truncated = true
			return errProjectSymbolIndexDone
		}
		idx.Symbols = append(idx.Symbols, projectSymbolsFromText(virtual, string(data), "", remaining)...)
		return nil
	})
}

func buildProjectSymbolIndex(ctx context.Context, roots []workspaceRootView) (*projectSymbolIndex, error) {
	idx := &projectSymbolIndex{
		Version:     projectSymbolIndexVersion,
		Fingerprint: projectSymbolRootsFingerprint(roots),
		BuiltAtNS:   time.Now().UnixNano(),
		Symbols:     []projectSymbolResult{},
	}
	for _, root := range roots {
		if !root.Available {
			continue
		}
		err := appendProjectSymbolIndexRoot(ctx, root, idx)
		if err != nil && !errors.Is(err, errProjectSymbolIndexDone) {
			return nil, err
		}
		if idx.Truncated {
			break
		}
	}
	sort.Slice(idx.Symbols, func(i, j int) bool {
		a, b := idx.Symbols[i], idx.Symbols[j]
		al, bl := strings.ToLower(a.Name), strings.ToLower(b.Name)
		if al != bl {
			return al < bl
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.Line < b.Line
	})
	return idx, nil
}

func (s *Server) currentProjectSymbolIndex(ctx context.Context) (*projectSymbolIndex, error) {
	roots, err := s.workspaceRootViews(!s.Config.SharedServerEnabled)
	if err != nil {
		return nil, err
	}
	fingerprint := projectSymbolRootsFingerprint(roots)

	s.projectSymbolIndexMu.Lock()
	if s.projectSymbolIndex != nil && s.projectSymbolIndex.Fingerprint == fingerprint {
		idx := s.projectSymbolIndex
		stale := time.Since(idx.builtAt()) > projectSymbolIndexFreshFor
		if stale && !s.projectSymbolIndexRefreshing {
			s.projectSymbolIndexRefreshing = true
			go s.refreshProjectSymbolIndex(roots, fingerprint)
		}
		s.projectSymbolIndexMu.Unlock()
		return idx, nil
	}
	if cached, cacheErr := loadProjectSymbolIndexCache(s.Workspace, fingerprint); cacheErr == nil {
		s.projectSymbolIndex = cached
		stale := time.Since(cached.builtAt()) > projectSymbolIndexFreshFor
		if stale && !s.projectSymbolIndexRefreshing {
			s.projectSymbolIndexRefreshing = true
			go s.refreshProjectSymbolIndex(roots, fingerprint)
		}
		s.projectSymbolIndexMu.Unlock()
		return cached, nil
	}
	s.projectSymbolIndexMu.Unlock()

	buildCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	idx, err := buildProjectSymbolIndex(buildCtx, roots)
	if err != nil {
		return nil, err
	}
	s.projectSymbolIndexMu.Lock()
	s.projectSymbolIndex = idx
	s.projectSymbolIndexRefreshing = false
	s.projectSymbolIndexMu.Unlock()
	_ = saveProjectSymbolIndexCache(s.Workspace, idx)
	return idx, nil
}

func (s *Server) refreshProjectSymbolIndex(roots []workspaceRootView, fingerprint string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	idx, err := buildProjectSymbolIndex(ctx, roots)
	s.projectSymbolIndexMu.Lock()
	defer s.projectSymbolIndexMu.Unlock()
	s.projectSymbolIndexRefreshing = false
	if err != nil || idx.Fingerprint != fingerprint {
		return
	}
	s.projectSymbolIndex = idx
	_ = saveProjectSymbolIndexCache(s.Workspace, idx)
}

func (s *Server) updateProjectSymbolIndexFile(pathValue, text string) {
	pathValue = normalizeProjectIndexPath(pathValue)
	if pathValue == "" {
		return
	}
	s.projectSymbolIndexMu.Lock()
	idx := s.projectSymbolIndex
	if idx == nil {
		s.projectSymbolIndexMu.Unlock()
		return
	}
	next := idx.Symbols[:0]
	for _, item := range idx.Symbols {
		if item.Path != pathValue {
			next = append(next, item)
		}
	}
	idx.Symbols = next
	if projectSymbolLanguage(pathValue) != "" {
		idx.Symbols = append(idx.Symbols, projectSymbolsFromText(pathValue, text, "", projectSymbolMaxResults*20)...)
	}
	idx.BuiltAtNS = time.Now().UnixNano()
	sort.Slice(idx.Symbols, func(i, j int) bool {
		a, b := idx.Symbols[i], idx.Symbols[j]
		al, bl := strings.ToLower(a.Name), strings.ToLower(b.Name)
		if al != bl {
			return al < bl
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.Line < b.Line
	})
	snapshot := *idx
	snapshot.Symbols = append([]projectSymbolResult(nil), idx.Symbols...)
	s.projectSymbolIndexMu.Unlock()
	_ = saveProjectSymbolIndexCache(s.Workspace, &snapshot)
}

func (s *Server) invalidateProjectSymbolIndex() {
	s.projectSymbolIndexMu.Lock()
	s.projectSymbolIndex = nil
	s.projectSymbolIndexRefreshing = false
	s.projectSymbolIndexMu.Unlock()
	if cacheFile, err := projectSymbolIndexCacheFile(s.Workspace); err == nil {
		_ = os.Remove(cacheFile)
	}
}

func (s *Server) invalidateProjectCompletionIndexes() {
	s.invalidateProjectSymbolIndex()
	s.projectIndexMu.Lock()
	s.projectIndex = nil
	s.projectIndexRefreshing = false
	s.projectIndexMu.Unlock()
	if root, err := s.projectRoot(); err == nil {
		if cacheFile, cacheErr := projectIndexCacheFile(root); cacheErr == nil {
			_ = os.Remove(cacheFile)
		}
	}
}

func searchProjectSymbolIndex(idx *projectSymbolIndex, query string, limit int) []projectSymbolResult {
	if idx == nil || limit <= 0 {
		return []projectSymbolResult{}
	}
	query = strings.TrimSpace(query)
	top := make(projectSymbolHeap, 0, limit)
	for _, raw := range idx.Symbols {
		item := raw
		score := 1
		ok := true
		if query != "" {
			score, ok = projectSymbolQueryScore(item.Name, query)
		}
		if !ok {
			continue
		}
		item.Score = score
		addProjectSymbolTop(&top, item, limit)
	}
	results := []projectSymbolResult(top)
	sort.Slice(results, func(i, j int) bool { return projectSymbolBetter(results[i], results[j]) })
	return results
}
