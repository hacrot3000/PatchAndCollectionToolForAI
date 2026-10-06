package server

import (
	"container/heap"
	"errors"
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	projectSymbolSearchMaxFiles = 25000
	projectSymbolSearchMaxBytes = int64(32 << 20)
	projectSymbolFileMaxBytes   = int64(2 << 20)
	projectSymbolMaxResults     = 100
)

type projectSymbolResult struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Column    int    `json:"column,omitempty"`
	Signature string `json:"signature,omitempty"`
	Score     int    `json:"score,omitempty"`
}

type projectSymbolSearchResponse struct {
	Results      []projectSymbolResult `json:"results"`
	ScannedFiles int                   `json:"scanned_files"`
	ScannedBytes int64                 `json:"scanned_bytes"`
	Truncated    bool                  `json:"truncated,omitempty"`
}

var (
	symbolGoFunc = regexp.MustCompile(`^\s*func\s*(?:\([^)]*\)\s*)?([A-Za-z_]\w*)\s*\(`)
	symbolGoType = regexp.MustCompile(`^\s*type\s+([A-Za-z_]\w*)\s+(struct|interface)\b`)
	symbolPython = regexp.MustCompile(`^\s*(?:async\s+)?(def|class)\s+([A-Za-z_]\w*)\b`)
	symbolJSFunc = regexp.MustCompile(`^\s*(?:export\s+(?:default\s+)?)?(?:async\s+)?function\s+([A-Za-z_$][\w$]*)\b`)
	symbolJSType = regexp.MustCompile(`^\s*(?:export\s+(?:default\s+)?)?(class|interface|enum|type)\s+([A-Za-z_$][\w$]*)\b`)
	symbolJSArrow = regexp.MustCompile(`^\s*(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*=\s*(?:async\s*)?(?:\([^)]*\)|[A-Za-z_$][\w$]*)\s*=>`)
	symbolJSMethod = regexp.MustCompile(`^\s*(?:(?:public|private|protected|static|async|get|set|override|readonly)\s+)*([A-Za-z_$][\w$]*)\s*\([^;]*\)\s*(?::[^={]+)?\s*\{`)
	symbolCFamilyType = regexp.MustCompile(`^\s*(?:(?:public|private|protected|internal|static|final|abstract|sealed|open|data|record)\s+)*(class|struct|interface|enum|namespace)\s+([A-Za-z_]\w*)\b`)
	symbolCFamilyMethod = regexp.MustCompile(`^\s*(?:(?:public|private|protected|internal|static|virtual|override|inline|constexpr|extern|synchronized|native|final|abstract|async)\s+)*(?:[A-Za-z_][\w:<>,.?*&\[\]\s]+\s+)?([A-Za-z_]\w*)\s*\([^;{}]*\)\s*(?:const\s*)?(?:->[^\{]+)?\s*\{`)
	symbolPHPType = regexp.MustCompile(`(?i)^\s*(?:(?:final|abstract|readonly)\s+)?(class|interface|trait|enum)\s+([A-Za-z_]\w*)\b`)
	symbolPHPFunc = regexp.MustCompile(`(?i)^\s*(?:(?:public|private|protected|static|final|abstract)\s+)*(?:async\s+)?function\s+&?\s*([A-Za-z_]\w*)\s*\(`)
	symbolRustFunc = regexp.MustCompile(`^\s*(?:pub(?:\([^)]*\))?\s+)?(?:async\s+)?fn\s+([A-Za-z_]\w*)\b`)
	symbolRustType = regexp.MustCompile(`^\s*(?:pub(?:\([^)]*\))?\s+)?(struct|enum|trait|type|mod)\s+([A-Za-z_]\w*)\b`)
	symbolRustImpl = regexp.MustCompile(`^\s*impl(?:<[^>]+>)?\s+([^\s{]+(?:\s+for\s+[^\s{]+)?)\s*\{`)
	symbolShellFunc = regexp.MustCompile(`^\s*(?:function\s+)?([A-Za-z_][\w.-]*)\s*(?:\(\s*\))?\s*\{`)
)

var errProjectSymbolScanDone = errors.New("project symbol scan done")

var projectSymbolControlWords = map[string]bool{
	"if": true, "for": true, "while": true, "switch": true, "catch": true,
	"else": true, "do": true, "return": true, "new": true, "sizeof": true,
	"typeof": true, "delete": true, "case": true, "with": true, "when": true,
}

func projectSymbolLanguage(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go"
	case ".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx":
		return "javascript"
	case ".py", ".pyw":
		return "python"
	case ".java", ".kt", ".kts", ".cs", ".c", ".h", ".cc", ".cpp", ".cxx", ".hpp", ".hh", ".hxx":
		return "c-family"
	case ".php", ".phtml":
		return "php"
	case ".rs":
		return "rust"
	case ".sh", ".bash", ".zsh", ".fish":
		return "shell"
	default:
		return ""
	}
}

func projectSymbolFromLine(line, language string) (name, kind string, ok bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "*") {
		return "", "", false
	}
	match := func(re *regexp.Regexp) []string { return re.FindStringSubmatch(line) }
	switch language {
	case "go":
		if m := match(symbolGoFunc); len(m) > 1 { return m[1], "function", true }
		if m := match(symbolGoType); len(m) > 2 { return m[1], m[2], true }
	case "python":
		if m := match(symbolPython); len(m) > 2 {
			kind := "class"; if m[1] == "def" { kind = "function" }
			return m[2], kind, true
		}
	case "javascript":
		if m := match(symbolJSFunc); len(m) > 1 { return m[1], "function", true }
		if m := match(symbolJSType); len(m) > 2 { return m[2], m[1], true }
		if m := match(symbolJSArrow); len(m) > 1 { return m[1], "function", true }
		if m := match(symbolJSMethod); len(m) > 1 && !projectSymbolControlWords[m[1]] { return m[1], "method", true }
	case "c-family":
		if m := match(symbolCFamilyType); len(m) > 2 { return m[2], m[1], true }
		if m := match(symbolCFamilyMethod); len(m) > 1 && !projectSymbolControlWords[m[1]] { return m[1], "method", true }
	case "php":
		if m := match(symbolPHPType); len(m) > 2 { return m[2], strings.ToLower(m[1]), true }
		if m := match(symbolPHPFunc); len(m) > 1 { return m[1], "function", true }
	case "rust":
		if m := match(symbolRustFunc); len(m) > 1 { return m[1], "function", true }
		if m := match(symbolRustType); len(m) > 2 { return m[2], m[1], true }
		if m := match(symbolRustImpl); len(m) > 1 { return m[1], "impl", true }
	case "shell":
		if m := match(symbolShellFunc); len(m) > 1 && !projectSymbolControlWords[m[1]] { return m[1], "function", true }
	}
	return "", "", false
}

func projectSymbolQueryScore(name, query string) (int, bool) {
	nameLower := strings.ToLower(strings.TrimSpace(name))
	queryLower := strings.ToLower(strings.TrimSpace(query))
	if nameLower == "" || queryLower == "" { return 0, false }
	switch {
	case nameLower == queryLower:
		return 10000, true
	case strings.HasPrefix(nameLower, queryLower):
		return 8000 - len(nameLower), true
	case strings.Contains(nameLower, queryLower):
		return 6000 - len(nameLower), true
	default:
		score, ok := subsequenceProjectScore(nameLower, queryLower)
		if !ok { return 0, false }
		return 3000 + score, true
	}
}

func projectSymbolsFromText(path, text, query string, limit int) []projectSymbolResult {
	language := projectSymbolLanguage(path)
	if language == "" || limit <= 0 { return nil }
	lines := strings.Split(text, "\n")
	out := make([]projectSymbolResult, 0, projectSearchMin(limit, 32))
	for i, raw := range lines {
		name, kind, ok := projectSymbolFromLine(strings.TrimSuffix(raw, "\r"), language)
		if !ok { continue }
		score := 1
		if strings.TrimSpace(query) != "" {
			score, ok = projectSymbolQueryScore(name, query)
			if !ok { continue }
		}
		column := len(raw) - len(strings.TrimLeft(raw, " \t")) + 1
		out = append(out, projectSymbolResult{
			Name: name, Kind: kind, Path: path, Line: i + 1, Column: column,
			Signature: strings.TrimSpace(raw), Score: score,
		})
		if len(out) >= limit { break }
	}
	return out
}

type projectSymbolHeap []projectSymbolResult
func (h projectSymbolHeap) Len() int { return len(h) }
func (h projectSymbolHeap) Less(i,j int) bool {
	if h[i].Score != h[j].Score { return h[i].Score < h[j].Score }
	if h[i].Name != h[j].Name { return h[i].Name > h[j].Name }
	if h[i].Path != h[j].Path { return h[i].Path > h[j].Path }
	return h[i].Line > h[j].Line
}
func (h projectSymbolHeap) Swap(i,j int){ h[i],h[j]=h[j],h[i] }
func (h *projectSymbolHeap) Push(v any){ *h=append(*h,v.(projectSymbolResult)) }
func (h *projectSymbolHeap) Pop() any { old:=*h;n:=len(old);v:=old[n-1];*h=old[:n-1];return v }

func projectSymbolBetter(a,b projectSymbolResult) bool {
	if a.Score != b.Score { return a.Score > b.Score }
	if a.Name != b.Name { return a.Name < b.Name }
	if a.Path != b.Path { return a.Path < b.Path }
	return a.Line < b.Line
}

func addProjectSymbolTop(top *projectSymbolHeap, item projectSymbolResult, limit int) {
	if top.Len() < limit { heap.Push(top,item);return }
	if projectSymbolBetter(item,(*top)[0]) { (*top)[0]=item;heap.Fix(top,0) }
}

func scanProjectSymbolsRoot(ctx context.Context, root workspaceRootView, query string, limit int, top *projectSymbolHeap, response *projectSymbolSearchResponse) error {
	ignore := loadProjectRootIgnore(root.Path)
	err := filepath.WalkDir(root.Path, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if entry != nil && entry.IsDir() { return filepath.SkipDir }
			return nil
		}
		if err := ctx.Err(); err != nil { return err }
		if response.ScannedFiles >= projectSymbolSearchMaxFiles || response.ScannedBytes >= projectSymbolSearchMaxBytes {
			response.Truncated = true
			return errProjectSymbolScanDone
		}
		if current == root.Path { return nil }
		rel, err := filepath.Rel(root.Path,current);if err != nil{return nil}
		rel=normalizeProjectIndexPath(filepath.ToSlash(rel));if rel==""{return nil}
		if entry.IsDir() {
			if entry.Name()==".git" || entry.Type()&os.ModeSymlink!=0 || ignore.matches(rel,true) { return filepath.SkipDir }
			return nil
		}
		if entry.Type()&os.ModeSymlink!=0 || !entry.Type().IsRegular() || ignore.matches(rel,false) || projectSymbolLanguage(rel)=="" { return nil }
		info,err:=entry.Info();if err!=nil||info.Size()<0||info.Size()>projectSymbolFileMaxBytes{return nil}
		if response.ScannedBytes+info.Size()>projectSymbolSearchMaxBytes { response.Truncated=true;return errProjectSymbolScanDone }
		pinned,err:=openProjectPinnedFile(root.Path,current);if err!=nil{return nil}
		data,_,readErr:=pinned.readCurrent(projectSymbolFileMaxBytes);pinned.close()
		if readErr!=nil||strings.IndexByte(string(data),0)>=0{return nil}
		response.ScannedFiles++;response.ScannedBytes+=int64(len(data))
		virtual:=workspaceVirtualPath(root.ID,rel)
		for _,item:=range projectSymbolsFromText(virtual,string(data),query,projectSymbolMaxResults){
			addProjectSymbolTop(top,item,limit)
		}
		return nil
	})
	if errors.Is(err, errProjectSymbolScanDone) { return nil }
	return err
}

func (s *Server) projectSymbols(w http.ResponseWriter,r *http.Request){
	if r.Method!=http.MethodGet { http.Error(w,"method not allowed",http.StatusMethodNotAllowed);return }
	query:=strings.TrimSpace(r.URL.Query().Get("q"))
	pathValue:=strings.TrimSpace(r.URL.Query().Get("path"))
	limit:=50
	if raw:=strings.TrimSpace(r.URL.Query().Get("limit"));raw!="" { if parsed,err:=strconv.Atoi(raw);err==nil{limit=parsed} }
	if limit<1{limit=1};if limit>projectSymbolMaxResults{limit=projectSymbolMaxResults}
	if pathValue!="" {
		rel,err:=cleanProjectRelativePath(pathValue,false);if err!=nil{http.Error(w,err.Error(),http.StatusBadRequest);return}
		resolved,err:=s.resolveProjectPath(rel,false,false);if err!=nil{http.Error(w,"project file unavailable",http.StatusNotFound);return}
		rootView,_,rootErr:=s.projectRootForVirtualPath(rel);if rootErr!=nil{http.Error(w,"project root unavailable",http.StatusNotFound);return}
		pinned,err:=openProjectPinnedFile(rootView.Path,resolved);if err!=nil{http.Error(w,"project file unavailable",http.StatusNotFound);return}
		data,info,readErr:=pinned.readCurrent(projectSymbolFileMaxBytes);pinned.close()
		if readErr!=nil||info==nil||!info.Mode().IsRegular(){
			if readErr==errPinnedProjectFileTooLarge{http.Error(w,"project file is too large for symbol outline",http.StatusRequestEntityTooLarge);return}
			http.Error(w,"project file unavailable",http.StatusNotFound);return
		}
		results:=projectSymbolsFromText(rel,string(data),query,limit)
		writeJSON(w,http.StatusOK,projectSymbolSearchResponse{Results:results,ScannedFiles:1,ScannedBytes:int64(len(data))});return
	}
	if query=="" { writeJSON(w,http.StatusOK,projectSymbolSearchResponse{Results:[]projectSymbolResult{}});return }
	roots,err:=s.workspaceRootViews(!s.Config.SharedServerEnabled);if err!=nil{http.Error(w,"workspace roots unavailable",http.StatusInternalServerError);return}
	response:=projectSymbolSearchResponse{Results:[]projectSymbolResult{}}
	top:=make(projectSymbolHeap,0,limit);heap.Init(&top)
	for _,root:=range roots {
		if !root.Available{continue}
		err:=scanProjectSymbolsRoot(r.Context(),root,query,limit,&top,&response)
		if err!=nil&&err!=context.Canceled&&err!=context.DeadlineExceeded{http.Error(w,fmt.Sprintf("symbol search failed: %v",err),http.StatusInternalServerError);return}
		if r.Context().Err()!=nil{return}
		if response.Truncated{break}
	}
	response.Results=[]projectSymbolResult(top)
	sort.Slice(response.Results,func(i,j int)bool{return projectSymbolBetter(response.Results[i],response.Results[j])})
	writeJSON(w,http.StatusOK,response)
}
