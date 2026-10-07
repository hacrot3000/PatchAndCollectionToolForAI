package server

import (
	"context"\n\t"encoding/json"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	projectCompletionMaxText = 4 << 20
	projectCompletionMaxItems = 100
)

type projectCompletionRequest struct {
	Path       string      `json:"path"`
	Language   string      `json:"language,omitempty"`
	Prefix     string      `json:"prefix,omitempty"`
	Text       string      `json:"text,omitempty"`
	LinePrefix string      `json:"line_prefix,omitempty"`
	Position   lspPosition `json:"position,omitempty"`
	Limit      int         `json:"limit,omitempty"`
}

type projectCompletionItem struct {
	Label      string `json:"label"`
	Kind       string `json:"kind,omitempty"`
	Detail     string `json:"detail,omitempty"`
	Path       string `json:"path,omitempty"`
	Line       int    `json:"line,omitempty"`
	InsertText string `json:"insert_text,omitempty"`
	Source     string `json:"source,omitempty"`
	Score      int    `json:"score,omitempty"`
}

type projectCompletionResponse struct {
	Items        []projectCompletionItem `json:"items"`
	IndexedFiles int                     `json:"indexed_files,omitempty"`
	IndexedBytes int64                   `json:"indexed_bytes,omitempty"`
	Truncated    bool                    `json:"truncated,omitempty"`
}

var (
	completionIncludePattern = regexp.MustCompile(`(?i)#\s*include\s*[<"]([^>"]*)$`)
	completionJSImportPattern = regexp.MustCompile(`(?i)(?:\bfrom\s+|\bimport\s*|\brequire\s*\()\s*["']([^"']*)$`)
	completionPythonImportPattern = regexp.MustCompile(`(?i)(?:^|\s)(?:from|import)\s+([A-Za-z0-9_.]*)$`)
	completionLuaRequirePattern = regexp.MustCompile(`(?i)\brequire\s*\(?\s*["']([^"']*)$`)
	completionNimImportPattern = regexp.MustCompile(`(?i)(?:^|\s)(?:import|include|from)\s+([A-Za-z0-9_./-]*)$`)
	completionASImportPattern = regexp.MustCompile(`(?i)(?:^|\s)import\s+([A-Za-z0-9_.]*)$`)
	completionGoImportPattern = regexp.MustCompile(`(?i)(?:^|\s)import\s+(?:\(\s*)?["']([^"']*)$`)
	completionPHPImportPattern = regexp.MustCompile(`(?i)\b(?:require|include)(?:_once)?\s*\(?\s*["']([^"']*)$`)
	completionDartImportPattern = regexp.MustCompile(`(?i)\bimport\s+["']([^"']*)$`)
)

func normalizeProjectCompletionRequest(req projectCompletionRequest) (projectCompletionRequest, error) {
	var err error
	req.Path, err = cleanProjectRelativePath(strings.TrimSpace(req.Path), false)
	if err != nil {
		return req, err
	}
	req.Language = strings.ToLower(strings.TrimSpace(req.Language))
	req.Prefix = strings.TrimSpace(req.Prefix)
	req.LinePrefix = strings.TrimSpace(req.LinePrefix)
	if len(req.Prefix) > 256 {
		req.Prefix = req.Prefix[:256]
	}
	if len(req.LinePrefix) > 4096 {
		req.LinePrefix = req.LinePrefix[len(req.LinePrefix)-4096:]
	}
	if len(req.Text) > projectCompletionMaxText {
		return req, errProjectCompletionDocumentTooLarge
	}
	if req.Limit <= 0 {
		req.Limit = 60
	}
	if req.Limit > projectCompletionMaxItems {
		req.Limit = projectCompletionMaxItems
	}
	if req.Position.Line < 0 || req.Position.Character < 0 {
		return req, errProjectCompletionPosition
	}
	return req, nil
}

var (
	errProjectCompletionDocumentTooLarge = &projectCompletionError{"completion document exceeds 4 MiB"}
	errProjectCompletionPosition = &projectCompletionError{"completion position must be non-negative"}
)

type projectCompletionError struct{ message string }
func (e *projectCompletionError) Error() string { return e.message }

func completionKindType(kind string) string {
	kind = strings.ToLower(strings.TrimSpace(kind))
	switch kind {
	case "struct", "class", "interface", "record", "object", "message":
		return "class"
	case "enum":
		return "enum"
	case "function", "func", "proc", "iterator", "macro", "template":
		return "function"
	case "method", "rpc":
		return "method"
	case "const":
		return "constant"
	case "var", "field", "property":
		return "variable"
	case "namespace", "module", "package", "service":
		return "namespace"
	case "type", "trait", "scalar", "union", "input", "distinct", "tuple", "ref-object":
		return "type"
	default:
		return kind
	}
}

func projectCompletionBoost(item projectSymbolResult, currentPath string, local bool) int {
	score := item.Score
	if local {
		score += 4000
	}
	if item.Path == currentPath {
		score += 2500
	} else if path.Dir(item.Path) == path.Dir(currentPath) {
		score += 1000
	}
	return score
}

func addProjectCompletion(items map[string]projectCompletionItem, item projectCompletionItem) {
	if item.Label == "" {
		return
	}
	key := strings.ToLower(item.Label) + "\x00" + strings.ToLower(item.Kind)
	if old, ok := items[key]; ok && old.Score >= item.Score {
		return
	}
	items[key] = item
}

func projectSymbolCompletionItems(idx *projectSymbolIndex, req projectCompletionRequest) []projectCompletionItem {
	if strings.TrimSpace(req.Prefix) == "" {
		return nil
	}
	items := make(map[string]projectCompletionItem)
	for _, symbol := range searchProjectSymbolIndex(idx, req.Prefix, req.Limit*3) {
		addProjectCompletion(items, projectCompletionItem{
			Label: symbol.Name, Kind: completionKindType(symbol.Kind), Detail: symbol.Signature,
			Path: symbol.Path, Line: symbol.Line, InsertText: symbol.Name, Source: "project",
			Score: projectCompletionBoost(symbol, req.Path, false),
		})
	}
	if req.Text != "" && projectSymbolLanguage(req.Path) != "" {
		for _, symbol := range projectSymbolsFromText(req.Path, req.Text, req.Prefix, req.Limit*3) {
			addProjectCompletion(items, projectCompletionItem{
				Label: symbol.Name, Kind: completionKindType(symbol.Kind), Detail: symbol.Signature,
				Path: req.Path, Line: symbol.Line, InsertText: symbol.Name, Source: "current-file",
				Score: projectCompletionBoost(symbol, req.Path, true),
			})
		}
	}
	out := make([]projectCompletionItem, 0, len(items))
	for _, item := range items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		if out[i].Label != out[j].Label {
			return out[i].Label < out[j].Label
		}
		return out[i].Path < out[j].Path
	})
	if len(out) > req.Limit {
		out = out[:req.Limit]
	}
	return out
}

func completionImportContext(language, linePrefix string) (kind, prefix string, ok bool) {
	language = strings.ToLower(strings.TrimSpace(language))
	linePrefix = strings.TrimSpace(linePrefix)
	check := func(re *regexp.Regexp, name string) (string, string, bool) {
		if m := re.FindStringSubmatch(linePrefix); len(m) > 1 {
			return name, m[1], true
		}
		return "", "", false
	}
	switch language {
	case "cpp":
		return check(completionIncludePattern, "include")
	case "javascript", "typescript":
		return check(completionJSImportPattern, "javascript")
	case "python":
		return check(completionPythonImportPattern, "python")
	case "lua":
		return check(completionLuaRequirePattern, "lua")
	case "nim":
		return check(completionNimImportPattern, "nim")
	case "actionscript":
		return check(completionASImportPattern, "actionscript")
	case "go":
		return check(completionGoImportPattern, "go")
	case "php":
		return check(completionPHPImportPattern, "path")
	case "dart":
		return check(completionDartImportPattern, "path")
	}
	return "", "", false
}

func completionPathAllowed(kind, candidate string) bool {
	ext := strings.ToLower(path.Ext(candidate))
	switch kind {
	case "include":
		return ext == ".h" || ext == ".hh" || ext == ".hpp" || ext == ".hxx" || ext == ".inc"
	case "javascript":
		return ext == ".js" || ext == ".jsx" || ext == ".mjs" || ext == ".cjs" || ext == ".ts" || ext == ".tsx" || ext == ".mts" || ext == ".cts" || ext == ".json"
	case "python":
		return ext == ".py" || ext == ".pyi"
	case "lua":
		return ext == ".lua"
	case "nim":
		return ext == ".nim" || ext == ".nims"
	case "actionscript":
		return ext == ".as"
	case "go":
		return ext == ".go"
	case "path":
		return true
	default:
		return false
	}
}

func relativeCompletionPath(currentPath, candidate string) string {
	base := path.Dir(currentPath)
	rel, err := filepath.Rel(filepath.FromSlash(base), filepath.FromSlash(candidate))
	if err != nil {
		return candidate
	}
	value := filepath.ToSlash(rel)
	if !strings.HasPrefix(value, ".") {
		value = "./" + value
	}
	return value
}

func completionDisplayPath(kind, currentPath, candidate, goModule string) (label, insert string) {
	switch kind {
	case "python":
		value := strings.TrimSuffix(candidate, path.Ext(candidate))
		value = strings.TrimSuffix(value, "/__init__")
		value = strings.ReplaceAll(value, "/", ".")
		return value, value
	case "lua":
		value := strings.TrimSuffix(candidate, ".lua")
		value = strings.ReplaceAll(value, "/", ".")
		return value, value
	case "nim":
		value := strings.TrimSuffix(strings.TrimSuffix(candidate, ".nim"), ".nims")
		return value, value
	case "actionscript":
		value := strings.TrimSuffix(candidate, ".as")
		value = strings.ReplaceAll(value, "/", ".")
		return value, value
	case "javascript":
		value := relativeCompletionPath(currentPath, candidate)
		ext := path.Ext(value)
		if ext == ".js" || ext == ".jsx" || ext == ".ts" || ext == ".tsx" || ext == ".mts" || ext == ".cts" {
			value = strings.TrimSuffix(value, ext)
		}
		return value, value
	case "path":
		value := relativeCompletionPath(currentPath, candidate)
		return value, value
	case "go":
		dir := path.Dir(candidate)
		if dir == "." {
			dir = ""
		}
		value := strings.TrimSuffix(goModule, "/")
		if dir != "" {
			if value != "" {
				value += "/" + dir
			} else {
				value = dir
			}
		}
		if value == "" {
			value = candidate
		}
		return value, value
	default:
		return candidate, candidate
	}
}

func projectGoModuleName(workspace string) string {
	data, err := os.ReadFile(filepath.Join(workspace, "go.mod"))
	if err != nil || len(data) > 256<<10 {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "module" {
			return strings.TrimSpace(fields[1])
		}
	}
	return ""
}

func projectImportCompletionItems(ctx context.Context, s *Server, req projectCompletionRequest) []projectCompletionItem {
	kind, prefix, ok := completionImportContext(req.Language, req.LinePrefix)
	if !ok {
		return nil
	}
	idx, err := s.currentProjectFileIndex(ctx)
	if err != nil || idx == nil {
		return nil
	}
	searchPrefix := prefix
	switch kind {
	case "python", "lua", "actionscript":
		searchPrefix = strings.ReplaceAll(prefix, ".", "/")
	}
	searchPrefix = strings.TrimLeft(searchPrefix, "./")
	if searchPrefix == "" {
		searchPrefix = path.Base(req.Path)
	}
	candidates := searchProjectFileIndex(idx, searchPrefix, req.Limit*4)
	goModule := ""
	if kind == "go" {
		goModule = projectGoModuleName(s.Workspace)
	}
	seen := map[string]bool{}
	out := make([]projectCompletionItem, 0, req.Limit)
	for _, candidate := range candidates {
		if candidate.Path == req.Path || !completionPathAllowed(kind, candidate.Path) {
			continue
		}
		label, insert := completionDisplayPath(kind, req.Path, candidate.Path, goModule)
		if label == "" || seen[insert] {
			continue
		}
		if prefix != "" && !strings.Contains(strings.ToLower(insert), strings.ToLower(prefix)) && !strings.Contains(strings.ToLower(label), strings.ToLower(prefix)) {
			continue
		}
		seen[insert] = true
		out = append(out, projectCompletionItem{
			Label: label, Kind: "module", Detail: candidate.Path, Path: candidate.Path,
			InsertText: insert, Source: "project-path", Score: candidate.Score + 5000,
		})
		if len(out) >= req.Limit {
			break
		}
	}
	return out
}

func mergeProjectCompletionItems(limit int, groups ...[]projectCompletionItem) []projectCompletionItem {
	items := make(map[string]projectCompletionItem)
	for _, group := range groups {
		for _, item := range group {
			addProjectCompletion(items, item)
		}
	}
	out := make([]projectCompletionItem, 0, len(items))
	for _, item := range items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		if out[i].Label != out[j].Label {
			return out[i].Label < out[j].Label
		}
		return out[i].Source < out[j].Source
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (s *Server) projectCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req projectCompletionRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, projectCompletionMaxText+(128<<10)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		http.Error(w, "invalid completion request", http.StatusBadRequest)
		return
	}
	req, err := normalizeProjectCompletionRequest(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	idx, err := s.currentProjectSymbolIndex(r.Context())
	if err != nil {
		http.Error(w, "project symbol index unavailable", http.StatusInternalServerError)
		return
	}
	symbols := projectSymbolCompletionItems(idx, req)
	paths := projectImportCompletionItems(r.Context(), s, req)
	writeJSON(w, http.StatusOK, projectCompletionResponse{
		Items: mergeProjectCompletionItems(req.Limit, paths, symbols),
		IndexedFiles: idx.ScannedFiles,
		IndexedBytes: idx.ScannedBytes,
		Truncated: idx.Truncated,
	})
}
