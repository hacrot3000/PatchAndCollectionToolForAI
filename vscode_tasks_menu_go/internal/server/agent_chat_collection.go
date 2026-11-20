package server

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"bletonfc/vscode_tasks_menu/internal/patchtool"
)

// Agent Collection is deliberately read-only. It returns bounded, source-path
// annotated evidence to the model; mutation continues through the existing
// approval-gated native Patch Tool, never through this reader.
const (
	agentCollectionMaxActions = 6
	agentCollectionMaxEntries = 100
	agentCollectionMaxOutput = 20000
)

type agentCollectionAction struct {
	Type       string   `json:"type"`
	Path       string   `json:"path,omitempty"`
	Paths      []string `json:"paths,omitempty"`
	Patterns   []string `json:"patterns,omitempty"`
	Query      string   `json:"query,omitempty"`
	MaxEntries int      `json:"max_entries,omitempty"`
	MaxResults int      `json:"max_results,omitempty"`
	MaxMatches int      `json:"max_matches,omitempty"`
	StartLine  int      `json:"start_line,omitempty"`
	MaxLines   int      `json:"max_lines,omitempty"`
}
type agentCollectionArguments struct {
	ID      string                     `json:"id,omitempty"`
	Title   string                     `json:"title,omitempty"`
	Project patchtool.ProjectIdentity  `json:"project"`
	Actions []agentCollectionAction    `json:"actions"`
}

func parseAgentCollectionArguments(raw string, identity patchtool.ProjectIdentity) (agentCollectionArguments, error) {
	var args agentCollectionArguments
	if len(raw) == 0 || len(raw) > 16000 || !utf8.ValidString(raw) {
		return args, errors.New("Collection request is empty or too large")
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&args); err != nil {
		return args, fmt.Errorf("invalid Collection request: %w", err)
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		return args, errors.New("Collection requires exactly one JSON object")
	}
	// An absent project is valid only for a native function call. Callers
	// accepting a model's plain text must require both identity fields.
	if args.Project.ID != "" || args.Project.Key != "" {
		if args.Project.ID != identity.ID || args.Project.Key != identity.Key {
			return args, errors.New("Collection project identity mismatch")
		}
	}
	if len(args.Actions) == 0 || len(args.Actions) > agentCollectionMaxActions {
		return args, errors.New("Collection requires 1-6 actions")
	}
	for _, action := range args.Actions {
		if action.StartLine < 0 || action.MaxLines < 0 || action.MaxLines > 200 ||
			(action.Type != "read" && (action.StartLine != 0 || action.MaxLines != 0)) {
			return args, errors.New("start_line/max_lines are supported only for read; start_line >= 1 and max_lines 1..200")
		}
		switch action.Type {
		case "ls", "tree", "pack", "read", "find", "search":
		default:
			return args, fmt.Errorf("unsupported read-only Collection action %q", action.Type)
		}
	}
	return args, nil
}

// Reject symlink traversal and project escape, including relative .. paths.
// The same guard is used for explicit reads and recursively discovered files.
func agentCollectionPath(root, relative string) (string, string, error) {
	if relative == "" {
		relative = "."
	}
	if filepath.IsAbs(relative) || filepath.VolumeName(relative) != "" || strings.Contains(relative, "\\") {
		return "", "", errors.New("absolute, Windows or backslash paths not allowed")
	}
	relative = filepath.Clean(filepath.FromSlash(relative))
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", "", errors.New("path escapes the project")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return "", "", err
	}
	current := root
	if relative != "." {
		for _, part := range strings.Split(relative, string(filepath.Separator)) {
			if agentCollectionProtected(part) {
				return "", "", errors.New("protected path is unavailable to the AI")
			}
			current = filepath.Join(current, part)
			fi, err := os.Lstat(current)
			if err != nil {
				return "", "", err
			}
			if fi.Mode()&os.ModeSymlink != 0 {
				return "", "", errors.New("symlink paths are not followed")
			}
		}
	}
	return current, filepath.ToSlash(relative), nil
}

func agentCollectionProtected(name string) bool {
	name = strings.ToLower(name)
	return name == ".git" || name == ".env" || strings.HasPrefix(name, ".env.") ||
		name == "id_rsa" || name == "id_ed25519" || name == "credentials.json" ||
		strings.HasSuffix(name, ".pem") || strings.HasSuffix(name, ".p12") ||
		strings.HasSuffix(name, ".pfx") || strings.HasSuffix(name, ".key")
}

func agentCollectionBound(v, fallback int) int {
	if v <= 0 || v > agentCollectionMaxEntries {
		return fallback
	}
	return v
}

func agentCollectionRead(root, rel string) (string, error) {
	path, _, err := agentCollectionPath(root, rel)
	if err != nil {
		return "", err
	}
	fi, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !fi.Mode().IsRegular() {
		return "", errors.New("not a regular file")
	}
	if fi.Size() > 128<<10 {
		return "", errors.New("file is larger than the 128 KiB inline inspection limit")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, (128<<10)+1))
	if err != nil {
		return "", err
	}
	if len(data) > 128<<10 || !utf8.Valid(data) || strings.IndexByte(string(data), 0) >= 0 {
		return "", errors.New("binary or oversized file cannot be included inline")
	}
	if len(data) > 12000 {
		return agentCollectionUTF8Prefix(string(data), 12000) + "\n[TRUNCATED: read one source file with start_line/max_lines or narrow search]", nil
	}
	return string(data), nil
}


// Keep partial read responses valid UTF-8. The legacy read helper deliberately
// stays bounded; for large source files the explicit read action additionally
// supports start_line/max_lines so the model can page without repeated errors.
func agentCollectionUTF8Prefix(source string, maximum int) string {
 if maximum <= 0 { return "" }
 if len(source) <= maximum { return source }
 for maximum > 0 && !utf8.ValidString(source[:maximum]) { maximum-- }
 return source[:maximum]
}

func agentCollectionReadPage(root, rel string, startLine, maxLines int) (map[string]string, error) {
 path,_,err:=agentCollectionPath(root,rel);if err!=nil{return nil,err}
 fi,err:=os.Lstat(path);if err!=nil{return nil,err}
 if !fi.Mode().IsRegular(){return nil,errors.New("not a regular file")}
 f,err:=os.Open(path);if err!=nil{return nil,err};defer f.Close()
 if startLine<=0{startLine=1}
 if maxLines<=0{maxLines=200}
 // Stream a bounded selection even from a large file: a 128 KiB source-size
 // limit no longer blocks paged reads. Scanner enforces a 2 MiB per-line
 // memory ceiling, independent of the total file size.
 scanner:=bufio.NewScanner(f)
 scanner.Buffer(make([]byte,64<<10),2<<20)
 var builder strings.Builder
 total,end:=0,startLine-1
 partialLongLine:=false
 capture:=true
 for scanner.Scan(){
  total++
  if total<startLine||total>=startLine+maxLines||!capture{continue}
  line:=scanner.Text()
  if !utf8.ValidString(line)||strings.IndexByte(line,0)>=0{return nil,errors.New("binary or invalid UTF-8 file cannot be included inline")}
  line+="\n"
  if builder.Len()+len(line)>12000{
   if builder.Len()==0{builder.WriteString(agentCollectionUTF8Prefix(line,12000));partialLongLine=true}
   capture=false
   continue
  }
  builder.WriteString(line)
  end=total
 }
 if err:=scanner.Err();err!=nil{return nil,fmt.Errorf("cannot read file as bounded lines: %w",err)}
 value:=builder.String()
 // Scanner removes line terminators; restore the absence of a final LF.
 if end==total&&total>0&&fi.Size()>0{
  last:=[]byte{0};if _,err:=f.ReadAt(last,fi.Size()-1);err==nil&&last[0]!='\n'{
   value=strings.TrimSuffix(value,"\n")
  }
 }
 out:=map[string]string{"content":value,"start_line":strconv.Itoa(startLine),
  "total_lines":strconv.Itoa(total),"truncated":strconv.FormatBool(end<total||partialLongLine)}
 if end<total{
  if partialLongLine{out["error"]="source line exceeds 12 KiB inline page; use narrower search"} else{out["next_line"]=strconv.Itoa(end+1)}
 }
 return out,nil
}

// Oversized batches must preserve structured, truthful partial observations,
// rather than discarding all successful reads with one generic error. Shrink
// large data values progressively, then large listings, and annotate losses.
// Never release another byte of protected data; the path reader above is still
// the only source of file bytes.
func agentCollectionBoundResponse(identity patchtool.ProjectIdentity, id string, results []map[string]any) string {
 envelope:=map[string]any{"id":id,"project":identity,"results":results}
 body,_:=json.Marshal(envelope)
 if len(body)<=agentCollectionMaxOutput {return string(body)}
 envelope["response_truncated"]=true
 envelope["continuation_hint"]="Results exceeded the read-only response limit. Read one specific file at a time with start_line/max_lines (1..200), or narrow tree/search max_results; continue the original task."
 for round:=0;round<14;round++{
  changed:=false
  for _,out:=range results {
   if files,ok:=out["files"].([]map[string]string);ok {
    for _,file:=range files {
     value:=file["content"]
     if len(value)>192 {
      cap:=len(value)/2
      if cap<192 {cap=192}
      prefix:=agentCollectionUTF8Prefix(value,cap)
      if len(prefix)<len(value) {
       file["content"]=prefix
       file["content_truncated"]="true"
       // A shortened read may no longer reach its original next_line.
       // Expose a conservative overlapping cursor so nothing is skipped.
       start,_:=strconv.Atoi(file["start_line"]);if start<1{start=1}
       resume:=strconv.Itoa(start+strings.Count(prefix,"\n"))
       file["continuation_start_line"]=resume
       if file["start_line"]!="" {file["next_line"]=resume}
       out["output_truncated"]=true
       changed=true
      }
     }
    }
   }
   for _,field:=range []string{"entries","matches"} {
    if items,ok:=out[field].([]string);ok && len(items)>1 {
     n:=(len(items)+1)/2
     out[field]=items[:n]
     out["truncated"]=true
     out["output_truncated"]=true
     changed=true
    }
   }
  }
  body,_=json.Marshal(envelope)
  if len(body)<=agentCollectionMaxOutput {return string(body)}
  if !changed {break}
 }
 body,_=json.Marshal(map[string]any{"id":id,"project":identity,
  "error":map[string]any{"code":"COLLECTION_RESULT_BUDGET_EXCEEDED",
  "message":"Even compacted read-only data cannot fit; request one file or narrower paths",
  "retry_allowed":true}})
 return string(body)
}

func agentCollectionWalk(root, scope string, visit func(string, os.DirEntry) bool) error {
	base, normalized, err := agentCollectionPath(root, scope)
	if err != nil {
		return err
	}
	seen := 0
	return filepath.WalkDir(base, func(path string, item os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path != base && (agentCollectionProtected(item.Name()) || item.Type()&os.ModeSymlink != 0) {
			if item.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		seen++
		if seen > 1200 {
			return filepath.SkipAll
		}
		rel := filepath.ToSlash(filepath.Join(normalized, strings.TrimPrefix(strings.TrimPrefix(path, base), string(filepath.Separator))))
		if path == base {
			rel = normalized
		}
		if !visit(rel, item) {
			return filepath.SkipAll
		}
		return nil
	})
}

func agentCollectionMatches(pattern, rel string) bool {
	pattern = strings.TrimPrefix(pattern, "./")
	if strings.HasPrefix(pattern, "**/") {
		pattern = strings.TrimPrefix(pattern, "**/")
	}
	a, _ := filepath.Match(pattern, filepath.Base(rel))
	b, _ := filepath.Match(pattern, rel)
	return a || b
}

func executeAgentCollection(root string, identity patchtool.ProjectIdentity, args agentCollectionArguments) string {
	results := make([]map[string]any, 0, len(args.Actions))
	for _, action := range args.Actions {
		out := map[string]any{"type": action.Type}
		switch action.Type {
		case "ls":
			target, rel, err := agentCollectionPath(root, action.Path)
			out["path"] = rel
			if err == nil {
				var entries []string
				var dir []os.DirEntry
				dir, err = os.ReadDir(target)
				max := agentCollectionBound(action.MaxEntries, 60)
				for _, item := range dir {
					if len(entries) >= max {
						break
					}
					if agentCollectionProtected(item.Name()) || item.Type()&os.ModeSymlink != 0 {
						continue
					}
					suffix := ""
					if item.IsDir() {
						suffix = "/"
					}
					entries = append(entries, item.Name()+suffix)
				}
				out["entries"], out["truncated"] = entries, len(dir) > max
			}
			if err != nil {
				out["error"] = err.Error()
			}
		case "read", "pack":
			paths := action.Paths
			if action.Type == "read" {
				paths = []string{action.Path}
			}
			if len(paths) == 0 || len(paths) > 4 {
				out["error"] = "read/pack requires 1-4 project-relative paths"
				break
			}
			files := make([]map[string]string, 0, len(paths))
			for _, path := range paths {
				var entry map[string]string
				var err error
				if action.Type=="read" {entry,err=agentCollectionReadPage(root,path,action.StartLine,action.MaxLines)} else {var value string;value,err=agentCollectionRead(root,path);entry=map[string]string{"content":value}}
				if entry==nil {entry=map[string]string{}}
				entry["path"]=path
				if err!=nil {entry["error"]=err.Error();delete(entry,"content")}
				files=append(files,entry)
			}
			out["files"] = files
		case "tree", "find", "search":
			scope := action.Path
			if scope == "" {
				scope = "."
			}
			max := agentCollectionBound(action.MaxEntries, 60)
			if action.MaxResults > 0 {
				max = agentCollectionBound(action.MaxResults, 60)
			}
			if action.MaxMatches > 0 {
				max = agentCollectionBound(action.MaxMatches, 60)
			}
			items := make([]string, 0, max)
			err := agentCollectionWalk(root, scope, func(rel string, item os.DirEntry) bool {
				if rel == scope || rel == "." {
					return true
				}
				if action.Type == "tree" {
					if item.IsDir() {
						rel += "/"
					}
					items = append(items, rel)
				} else if action.Type == "find" {
					if len(action.Patterns) == 0 || len(action.Patterns) > 8 {
						return false
					}
					for _, pattern := range action.Patterns {
						if agentCollectionMatches(pattern, rel) {
							items = append(items, rel)
							break
						}
					}
				} else if !item.IsDir() && len(action.Query) > 0 && len(action.Query) <= 120 {
					content, readErr := agentCollectionRead(root, rel)
					if readErr == nil {
						for idx, line := range strings.Split(content, "\n") {
							if strings.Contains(line, action.Query) {
								if len(line) > 200 {
									line = line[:200]
								}
								items = append(items, fmt.Sprintf("%s:%d: %s", rel, idx+1, line))
								if len(items) >= max {
									break
								}
							}
						}
					}
				}
				return len(items) < max
			})
			sort.Strings(items)
			out["matches"] = items
			out["truncated"] = len(items) >= max
			if err != nil {
				out["error"] = err.Error()
			}
		}
		results = append(results, out)
	}
	return agentCollectionBoundResponse(identity,args.ID,results)
}

// A provider without function calling sometimes prints an AI Collection JSON
// envelope as plain text. Recover only the exact known, project-bound,
// READ-ONLY format. Never interpret arbitrary prose or a text Patch as code.
func agentCollectionTextFallback(text string, identity patchtool.ProjectIdentity) (agentCollectionArguments, bool) {
	text = strings.TrimSpace(text)
	// Accept exactly one standalone JSON envelope, optionally wrapped in a
	// single Markdown JSON fence or blockquote. Never extract commands from
	// surrounding prose, so instructions embedded in prose stay inert.
	if strings.HasPrefix(text, "```json\n") && strings.HasSuffix(text, "\n```") {
		text = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(text, "```json\n"), "\n```"))
	} else if strings.HasPrefix(text, "```\n") && strings.HasSuffix(text, "\n```") {
		text = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(text, "```\n"), "\n```"))
	} else if strings.HasPrefix(text, "> ") && !strings.Contains(text, "\n") {
		text = strings.TrimSpace(strings.TrimPrefix(text, "> "))
	}
	if !strings.HasPrefix(text, "{") || !strings.HasSuffix(text, "}") {
		return agentCollectionArguments{}, false
	}
	args, err := parseAgentCollectionArguments(text, identity)
	return args, err == nil && args.Project.ID == identity.ID && args.Project.Key == identity.Key
}
