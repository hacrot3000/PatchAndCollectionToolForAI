package server

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	projectIndexCacheMagic = "VTMIDX1\n"
	projectIndexMaxBytes   = 64 << 20
	projectIndexMaxFiles   = 500000
)

type projectFileIndex struct {
	blob    []byte
	offsets []uint32
	builtAt time.Time
}

func newProjectFileIndex(paths []string, builtAt time.Time) (*projectFileIndex, error) {
	sort.Strings(paths)
	uniq := paths[:0]
	last := ""
	for _, path := range paths {
		path = normalizeProjectIndexPath(path)
		if path == "" || path == last {
			continue
		}
		uniq = append(uniq, path)
		last = path
		if len(uniq) > projectIndexMaxFiles {
			return nil, fmt.Errorf("project contains too many files to index")
		}
	}
	var blob []byte
	offsets := make([]uint32, 0, len(uniq))
	for _, path := range uniq {
		if len(blob)+len(path)+1 > projectIndexMaxBytes {
			return nil, fmt.Errorf("project file index is too large")
		}
		offsets = append(offsets, uint32(len(blob)))
		blob = append(blob, path...)
		blob = append(blob, 0)
	}
	return &projectFileIndex{blob: blob, offsets: offsets, builtAt: builtAt}, nil
}

func (idx *projectFileIndex) pathAt(i int) string {
	if idx == nil || i < 0 || i >= len(idx.offsets) {
		return ""
	}
	start := int(idx.offsets[i])
	end := bytes.IndexByte(idx.blob[start:], 0)
	if end < 0 {
		return ""
	}
	return string(idx.blob[start : start+end])
}

func buildProjectFileIndex(ctx context.Context, root string) (*projectFileIndex, error) {
	if rg, err := exec.LookPath("rg"); err == nil {
		if paths, rgErr := projectIndexPathsRG(ctx, rg, root); rgErr == nil {
			return newProjectFileIndex(paths, time.Now())
		} else if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	paths, err := projectIndexPathsFallback(ctx, root)
	if err != nil {
		return nil, err
	}
	return newProjectFileIndex(paths, time.Now())
}

func projectIndexPathsRG(ctx context.Context, rg, root string) ([]string, error) {
	cmd := exec.CommandContext(ctx, rg, "--files", "-0", "--hidden", "--glob", "!.git/**")
	cmd.Dir = root
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	paths := make([]string, 0, 4096)
	totalBytes := 0
	scanner := bufio.NewScanner(stdout)
	scanner.Split(scanProjectIndexNUL)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return nil, err
		}
		path := normalizeProjectIndexPath(scanner.Text())
		if path == "" {
			continue
		}
		totalBytes += len(path) + 1
		if len(paths) >= projectIndexMaxFiles || totalBytes > projectIndexMaxBytes {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return nil, fmt.Errorf("project file index is too large")
		}
		paths = append(paths, path)
	}
	if err := scanner.Err(); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, err
	}
	if err := cmd.Wait(); err != nil {
		return nil, err
	}
	return paths, nil
}

func scanProjectIndexNUL(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if i := bytes.IndexByte(data, 0); i >= 0 {
		return i + 1, data[:i], nil
	}
	if atEOF {
		if len(data) == 0 {
			return 0, nil, nil
		}
		return 0, nil, fmt.Errorf("invalid rg file index output")
	}
	return 0, nil, nil
}

func projectIndexPathsFallback(ctx context.Context, root string) ([]string, error) {
	paths := make([]string, 0, 4096)
	ignore := loadProjectRootIgnore(root)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = normalizeProjectIndexPath(filepath.ToSlash(rel))
		if rel == "" {
			return nil
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || ignore.matches(rel, true) {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() || ignore.matches(rel, false) {
			return nil
		}
		paths = append(paths, rel)
		if len(paths) > projectIndexMaxFiles {
			return fmt.Errorf("project contains too many files to index")
		}
		return nil
	})
	return paths, err
}

func normalizeProjectIndexPath(path string) string {
	path = strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
	if path == "" || strings.ContainsRune(path, '\x00') || strings.HasPrefix(path, "/") {
		return ""
	}
	clean := filepath.Clean(filepath.FromSlash(path))
	if clean == "." || clean == ".." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return ""
	}
	return filepath.ToSlash(clean)
}

func projectIndexCacheFile(root string) (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(root))
	dir := filepath.Join(cacheDir, "vscode_tasks_menu", "file-index")
	return filepath.Join(dir, fmt.Sprintf("%x.idx", sum[:12])), nil
}

func loadProjectIndexCache(root string) (*projectFileIndex, error) {
	path, err := projectIndexCacheFile(root)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	headerLen := len(projectIndexCacheMagic) + 8
	if len(data) < headerLen || len(data) > projectIndexMaxBytes+headerLen || string(data[:len(projectIndexCacheMagic)]) != projectIndexCacheMagic {
		return nil, fmt.Errorf("invalid project index cache")
	}
	stamp := int64(binary.LittleEndian.Uint64(data[len(projectIndexCacheMagic):headerLen]))
	blob := append([]byte(nil), data[headerLen:]...)
	offsets, err := projectIndexOffsets(blob)
	if err != nil {
		return nil, err
	}
	if len(offsets) > projectIndexMaxFiles {
		return nil, fmt.Errorf("project index cache contains too many files")
	}
	return &projectFileIndex{blob: blob, offsets: offsets, builtAt: time.Unix(0, stamp)}, nil
}

func saveProjectIndexCache(root string, idx *projectFileIndex) error {
	if idx == nil {
		return fmt.Errorf("project index is nil")
	}
	path, err := projectIndexCacheFile(root)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data := make([]byte, len(projectIndexCacheMagic)+8, len(projectIndexCacheMagic)+8+len(idx.blob))
	copy(data, projectIndexCacheMagic)
	binary.LittleEndian.PutUint64(data[len(projectIndexCacheMagic):], uint64(idx.builtAt.UnixNano()))
	data = append(data, idx.blob...)
	tmp, err := os.CreateTemp(filepath.Dir(path), ".file-index-*")
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
	if _, err := tmp.Write(data); err != nil {
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

func projectIndexOffsets(blob []byte) ([]uint32, error) {
	if len(blob) == 0 {
		return nil, nil
	}
	if blob[len(blob)-1] != 0 {
		return nil, fmt.Errorf("invalid project index cache payload")
	}
	offsets := make([]uint32, 0, bytes.Count(blob, []byte{0}))
	start := 0
	for start < len(blob) {
		end := bytes.IndexByte(blob[start:], 0)
		if end <= 0 {
			return nil, fmt.Errorf("invalid project index cache entry")
		}
		offsets = append(offsets, uint32(start))
		start += end + 1
	}
	return offsets, nil
}
