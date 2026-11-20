package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
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
		return string(data[:12000]) + "\n[TRUNCATED: request a narrower source file or search]", nil
	}
	return string(data), nil
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
				value, err := agentCollectionRead(root, path)
				entry := map[string]string{"path": path}
				if err != nil {
					entry["error"] = err.Error()
				} else {
					entry["content"] = value
				}
				files = append(files, entry)
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
	response := map[string]any{"id": args.ID, "project": identity, "results": results}
	body, _ := json.Marshal(response)
	if len(body) > agentCollectionMaxOutput {
		// The model gets a truthful bounded error, never a sliced JSON fragment.
		body, _ = json.Marshal(map[string]any{"id": args.ID, "project": identity,
			"error": "Collection result too large; repeat with fewer actions or narrower paths"})
	}
	return string(body)
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
