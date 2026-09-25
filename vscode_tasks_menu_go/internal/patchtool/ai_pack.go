package patchtool

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	aiPackPromptDocName = "AI_STANDARD_PROMPTS.json"
	aiPackZipName       = "PATCH_TOOL_AI_PACK.zip"
	aiPackMetaName      = "PATCH_TOOL_AI_PACK.cache.json"
	aiPackMaxBytes      = 32 << 20
)

var aiPackRequiredDocs = []string{
	"AI_USAGE_CONTRACT.md",
	"CODE_COLLECTION_GUIDE.md",
	"COLLECT_ACTION_SCHEMA.json",
	"PATCH_PACKAGE_GUIDE.md",
	"PATCH_PACKAGE_SCHEMA.json",
	"PATCH_PACKAGE_CHECKLIST.json",
}

type AIPackResult struct {
	Path        string `json:"path"`
	Prompt      string `json:"prompt"`
	Fingerprint string `json:"fingerprint"`
	Cached      bool   `json:"cached"`
	DocCount    int    `json:"doc_count"`
	Source      string `json:"source"`
	GeneratedAt string `json:"generated_at"`
}

type aiPackFile struct {
	Rel  string
	Data []byte
}

type aiPackCacheMeta struct {
	Fingerprint string `json:"fingerprint"`
	GeneratedAt string `json:"generated_at"`
	DocCount    int    `json:"doc_count"`
	Source      string `json:"source"`
}

type aiPackPrompts struct {
	Version int    `json:"version"`
	VI      string `json:"vi"`
	EN      string `json:"en"`
	RU      string `json:"ru"`
}

func regularAIPackFile(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0
}

func collectAIPackDocs(root string) ([]aiPackFile, error) {
	info, err := os.Lstat(root)
	if err != nil {
		return nil, fmt.Errorf("Patch Tool docs unavailable: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("Patch Tool docs path is not a real directory")
	}
	files := make([]aiPackFile, 0, len(aiPackRequiredDocs))
	total := 0
	for _, rel := range aiPackRequiredDocs {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if !regularAIPackFile(path) {
			return nil, fmt.Errorf("required AI Pack document is missing or unsafe: %s", rel)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		total += len(raw)
		if total > aiPackMaxBytes {
			return nil, fmt.Errorf("essential AI Pack docs exceed %d bytes", aiPackMaxBytes)
		}
		files = append(files, aiPackFile{Rel: rel, Data: raw})
	}
	return files, nil
}

func loadAIPackPrompts(root string) (map[string]string, error) {
	path := filepath.Join(root, aiPackPromptDocName)
	if !regularAIPackFile(path) {
		return nil, fmt.Errorf("%s is missing from the active Patch Tool docs", aiPackPromptDocName)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(raw) > 1<<20 {
		return nil, fmt.Errorf("%s is unexpectedly large", aiPackPromptDocName)
	}
	var doc aiPackPrompts
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", aiPackPromptDocName, err)
	}
	if doc.Version != 1 {
		return nil, fmt.Errorf("%s version is unsupported", aiPackPromptDocName)
	}
	prompts := map[string]string{
		"prompt-vi": strings.TrimSpace(doc.VI),
		"prompt-en": strings.TrimSpace(doc.EN),
		"prompt-ru": strings.TrimSpace(doc.RU),
	}
	for id, prompt := range prompts {
		if prompt == "" {
			return nil, fmt.Errorf("%s prompt %q is empty", aiPackPromptDocName, id)
		}
	}
	return prompts, nil
}

func aiPackFingerprint(docs []aiPackFile) string {
	h := sha256.New()
	_, _ = io.WriteString(h, "taskdeck-patch-ai-pack-v3\x00")
	for _, file := range docs {
		_, _ = io.WriteString(h, file.Rel)
		_, _ = h.Write([]byte{0})
		_, _ = h.Write(file.Data)
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func ensureAIPackCacheDir(workspace string) (string, error) {
	root, err := filepath.Abs(workspace)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("workspace must be a real directory")
	}
	cur := root
	for _, part := range []string{"artifacts", "patch_tool", "ai_pack"} {
		next := filepath.Join(cur, part)
		info, err := os.Lstat(next)
		switch {
		case err == nil:
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return "", fmt.Errorf("AI Pack cache path is unsafe: %s", next)
			}
		case os.IsNotExist(err):
			if err := os.Mkdir(next, 0o755); err != nil {
				return "", err
			}
		default:
			return "", err
		}
		cur = next
	}
	return cur, nil
}

func readAIPackCacheMeta(path string) (aiPackCacheMeta, bool) {
	if !regularAIPackFile(path) {
		return aiPackCacheMeta{}, false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return aiPackCacheMeta{}, false
	}
	var meta aiPackCacheMeta
	if json.Unmarshal(raw, &meta) != nil || meta.Fingerprint == "" || meta.GeneratedAt == "" {
		return aiPackCacheMeta{}, false
	}
	return meta, true
}

func cachedAIPackMatches(path string, docs []aiPackFile) bool {
	if !regularAIPackFile(path) {
		return false
	}
	zr, err := zip.OpenReader(path)
	if err != nil {
		return false
	}
	defer zr.Close()
	if len(zr.File) != len(docs) {
		return false
	}
	expected := make(map[string][]byte, len(docs))
	for _, doc := range docs {
		expected[doc.Rel] = doc.Data
	}
	for _, file := range zr.File {
		want, ok := expected[file.Name]
		if !ok || file.FileInfo().IsDir() {
			return false
		}
		rc, err := file.Open()
		if err != nil {
			return false
		}
		raw, err := io.ReadAll(io.LimitReader(rc, int64(len(want))+1))
		_ = rc.Close()
		if err != nil || !bytes.Equal(raw, want) {
			return false
		}
		delete(expected, file.Name)
	}
	return len(expected) == 0
}

func replaceAIPackFile(temp, target string) error {
	if runtime.GOOS == "windows" {
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return os.Rename(temp, target)
}

func writeJSONAtomic(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return replaceAIPackFile(name, path)
}

func writeAIPackZip(path string, docs []aiPackFile) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	zw := zip.NewWriter(tmp)
	for _, file := range docs {
		header := &zip.FileHeader{Name: file.Rel, Method: zip.Deflate}
		header.SetMode(0o644)
		w, err := zw.CreateHeader(header)
		if err != nil {
			_ = zw.Close()
			_ = tmp.Close()
			return err
		}
		if _, err := w.Write(file.Data); err != nil {
			_ = zw.Close()
			_ = tmp.Close()
			return err
		}
	}
	if err := zw.Close(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return replaceAIPackFile(name, path)
}

func BuildAIPack(workspace, executable string) (AIPackResult, error) {
	runtimeSpec, err := Resolve(workspace, executable)
	if err != nil {
		return AIPackResult{}, err
	}
	sourceRoot := filepath.Dir(runtimeSpec.Path)
	docsRoot := filepath.Join(sourceRoot, "_patch_lib", "docs")
	docs, err := collectAIPackDocs(docsRoot)
	if err != nil {
		return AIPackResult{}, err
	}
	prompts, err := loadAIPackPrompts(docsRoot)
	if err != nil {
		return AIPackResult{}, err
	}
	promptVI := prompts["prompt-vi"]
	fingerprint := aiPackFingerprint(docs)
	cacheDir, err := ensureAIPackCacheDir(workspace)
	if err != nil {
		return AIPackResult{}, err
	}
	zipPath := filepath.Join(cacheDir, aiPackZipName)
	metaPath := filepath.Join(cacheDir, aiPackMetaName)
	if meta, ok := readAIPackCacheMeta(metaPath); ok &&
		meta.Fingerprint == fingerprint &&
		cachedAIPackMatches(zipPath, docs) {
		rel, _ := filepath.Rel(workspace, zipPath)
		return AIPackResult{
			Path: filepath.ToSlash(rel), Prompt: promptVI, Fingerprint: fingerprint,
			Cached: true, DocCount: len(docs), Source: string(runtimeSpec.Kind), GeneratedAt: meta.GeneratedAt,
		}, nil
	}
	generatedAt := time.Now().UTC().Format(time.RFC3339Nano)
	if err := writeAIPackZip(zipPath, docs); err != nil {
		return AIPackResult{}, err
	}
	if err := writeJSONAtomic(metaPath, aiPackCacheMeta{
		Fingerprint: fingerprint, GeneratedAt: generatedAt, DocCount: len(docs), Source: string(runtimeSpec.Kind),
	}); err != nil {
		return AIPackResult{}, err
	}
	rel, err := filepath.Rel(workspace, zipPath)
	if err != nil {
		return AIPackResult{}, err
	}
	return AIPackResult{
		Path: filepath.ToSlash(rel), Prompt: promptVI, Fingerprint: fingerprint,
		Cached: false, DocCount: len(docs), Source: string(runtimeSpec.Kind), GeneratedAt: generatedAt,
	}, nil
}
