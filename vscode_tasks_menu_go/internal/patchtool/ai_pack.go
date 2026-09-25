package patchtool

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
)

const (
	aiPackGuideName = "HUONG_DAN_PYTHON_PATCH_TOOL.html"
	aiPackZipName   = "PATCH_TOOL_AI_PACK.zip"
	aiPackMetaName  = "PATCH_TOOL_AI_PACK.cache.json"
	aiPackMaxFiles  = 512
	aiPackMaxBytes  = 32 << 20
)

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

type aiPackManifest struct {
	Format      string   `json:"format"`
	Version     int      `json:"version"`
	Fingerprint string   `json:"fingerprint"`
	GeneratedAt string   `json:"generated_at"`
	Source      string   `json:"source"`
	Documents   []string `json:"documents"`
	Prompts     []string `json:"prompts"`
}

var aiPackTagRE = regexp.MustCompile(`(?is)<[^>]+>`)

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
	var files []aiPackFile
	total := 0
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("Patch Tool docs contains symlink: %s", path)
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("Patch Tool docs contains non-regular file: %s", path)
		}
		if len(files) >= aiPackMaxFiles {
			return fmt.Errorf("Patch Tool docs exceeds %d files", aiPackMaxFiles)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		total += len(raw)
		if total > aiPackMaxBytes {
			return fmt.Errorf("Patch Tool docs exceeds %d bytes", aiPackMaxBytes)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." || strings.HasPrefix(rel, "../") || strings.Contains(rel, "/../") {
			return fmt.Errorf("Patch Tool docs path escapes source root")
		}
		files = append(files, aiPackFile{Rel: rel, Data: raw})
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("Patch Tool docs directory is empty")
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Rel < files[j].Rel })
	return files, nil
}

func extractAIPackPrompt(raw []byte, id string) (string, error) {
	quoted := regexp.QuoteMeta(id)
	re, err := regexp.Compile(`(?is)<pre[^>]*\bid\s*=\s*["']` + quoted + `["'][^>]*>(.*?)</pre>`)
	if err != nil {
		return "", err
	}
	match := re.FindSubmatch(raw)
	if len(match) != 2 {
		return "", fmt.Errorf("standard prompt %q was not found in %s", id, aiPackGuideName)
	}
	body := strings.ReplaceAll(string(match[1]), "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	body = regexp.MustCompile(`(?i)<br\s*/?>`).ReplaceAllString(body, "\n")
	body = aiPackTagRE.ReplaceAllString(body, "")
	body = html.UnescapeString(body)
	body = strings.TrimSpace(body)
	if body == "" {
		return "", fmt.Errorf("standard prompt %q is empty", id)
	}
	return body, nil
}

func aiPackFingerprint(docs []aiPackFile, guide []byte) string {
	h := sha256.New()
	_, _ = io.WriteString(h, "taskdeck-patch-ai-pack-v1\x00")
	for _, file := range docs {
		_, _ = io.WriteString(h, file.Rel)
		_, _ = h.Write([]byte{0})
		_, _ = h.Write(file.Data)
		_, _ = h.Write([]byte{0})
	}
	_, _ = io.WriteString(h, aiPackGuideName)
	_, _ = h.Write([]byte{0})
	_, _ = h.Write(guide)
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

func cachedAIPackMatches(path, fingerprint string) bool {
	if !regularAIPackFile(path) {
		return false
	}
	zr, err := zip.OpenReader(path)
	if err != nil {
		return false
	}
	defer zr.Close()
	for _, file := range zr.File {
		if file.Name != "PATCH_TOOL_AI_PACK/MANIFEST.json" {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return false
		}
		raw, err := io.ReadAll(io.LimitReader(rc, 1<<20))
		_ = rc.Close()
		if err != nil {
			return false
		}
		var manifest aiPackManifest
		return json.Unmarshal(raw, &manifest) == nil && manifest.Fingerprint == fingerprint
	}
	return false
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

func writeAIPackZip(path string, docs []aiPackFile, guide []byte, prompts map[string]string, manifest aiPackManifest) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	zw := zip.NewWriter(tmp)
	write := func(name string, data []byte) error {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetMode(0o644)
		w, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	}
	prefix := "PATCH_TOOL_AI_PACK/"
	start := "# Patch Tool AI Pack\n\n" +
		"Upload this ZIP to the new AI chat, then paste PROMPT_VI.txt (or another prompt language).\n" +
		"The AI must read every file under docs/ before creating PATCH/COLLECT artifacts.\n" +
		"This pack was generated on demand from the currently installed Patch Tool documentation.\n"
	if err := write(prefix+"START_HERE.md", []byte(start)); err != nil {
		_ = zw.Close(); _ = tmp.Close(); return err
	}
	for id, prompt := range prompts {
		name := map[string]string{"prompt-vi":"PROMPT_VI.txt", "prompt-en":"PROMPT_EN.txt", "prompt-ru":"PROMPT_RU.txt"}[id]
		if name == "" {
			continue
		}
		if err := write(prefix+name, []byte(prompt+"\n")); err != nil {
			_ = zw.Close(); _ = tmp.Close(); return err
		}
	}
	if err := write(prefix+aiPackGuideName, guide); err != nil {
		_ = zw.Close(); _ = tmp.Close(); return err
	}
	for _, file := range docs {
		if err := write(prefix+"docs/"+file.Rel, file.Data); err != nil {
			_ = zw.Close(); _ = tmp.Close(); return err
		}
	}
	manifestRaw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		_ = zw.Close(); _ = tmp.Close(); return err
	}
	manifestRaw = append(manifestRaw, '\n')
	if err := write(prefix+"MANIFEST.json", manifestRaw); err != nil {
		_ = zw.Close(); _ = tmp.Close(); return err
	}
	if err := zw.Close(); err != nil {
		_ = tmp.Close(); return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close(); return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := replaceAIPackFile(name, path); err != nil {
		return err
	}
	return nil
}

func BuildAIPack(workspace, executable string) (AIPackResult, error) {
	runtimeSpec, err := Resolve(workspace, executable)
	if err != nil {
		return AIPackResult{}, err
	}
	sourceRoot := filepath.Dir(runtimeSpec.Path)
	docs, err := collectAIPackDocs(filepath.Join(sourceRoot, "_patch_lib", "docs"))
	if err != nil {
		return AIPackResult{}, err
	}
	guidePath := filepath.Join(sourceRoot, aiPackGuideName)
	if !regularAIPackFile(guidePath) {
		return AIPackResult{}, fmt.Errorf("%s is missing from the active Patch Tool runtime", aiPackGuideName)
	}
	guide, err := os.ReadFile(guidePath)
	if err != nil {
		return AIPackResult{}, err
	}
	if len(guide) > 8<<20 {
		return AIPackResult{}, fmt.Errorf("%s is unexpectedly large", aiPackGuideName)
	}
	promptVI, err := extractAIPackPrompt(guide, "prompt-vi")
	if err != nil {
		return AIPackResult{}, err
	}
	prompts := map[string]string{"prompt-vi": promptVI}
	for _, id := range []string{"prompt-en", "prompt-ru"} {
		if prompt, err := extractAIPackPrompt(guide, id); err == nil {
			prompts[id] = prompt
		}
	}
	fingerprint := aiPackFingerprint(docs, guide)
	cacheDir, err := ensureAIPackCacheDir(workspace)
	if err != nil {
		return AIPackResult{}, err
	}
	zipPath := filepath.Join(cacheDir, aiPackZipName)
	metaPath := filepath.Join(cacheDir, aiPackMetaName)
	if meta, ok := readAIPackCacheMeta(metaPath); ok &&
		meta.Fingerprint == fingerprint &&
		cachedAIPackMatches(zipPath, fingerprint) {
		rel, _ := filepath.Rel(workspace, zipPath)
		return AIPackResult{
			Path: filepath.ToSlash(rel), Prompt: promptVI, Fingerprint: fingerprint,
			Cached: true, DocCount: len(docs), Source: string(runtimeSpec.Kind), GeneratedAt: meta.GeneratedAt,
		}, nil
	}

	generatedAt := time.Now().UTC().Format(time.RFC3339Nano)
	docNames := make([]string, 0, len(docs))
	for _, file := range docs {
		docNames = append(docNames, file.Rel)
	}
	promptNames := make([]string, 0, len(prompts))
	for id := range prompts {
		promptNames = append(promptNames, id)
	}
	sort.Strings(promptNames)
	manifest := aiPackManifest{
		Format: "taskdeck-patch-ai-pack", Version: 1, Fingerprint: fingerprint,
		GeneratedAt: generatedAt, Source: string(runtimeSpec.Kind), Documents: docNames, Prompts: promptNames,
	}
	if err := writeAIPackZip(zipPath, docs, guide, prompts, manifest); err != nil {
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
