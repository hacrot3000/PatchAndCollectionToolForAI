package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	projectContentSearchDefaultLimit = 100
	projectContentSearchMaxLimit     = 200
	projectContentSearchMaxLineBytes = 256 << 10
)

type projectContentSearchResult struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
	Preview string `json:"preview"`
}

type rgJSONText struct {
	Text string `json:"text"`
}

type rgJSONMatch struct {
	Type string `json:"type"`
	Data struct {
		Path       rgJSONText `json:"path"`
		LineNumber int        `json:"line_number"`
		Lines      rgJSONText `json:"lines"`
		Submatches []struct {
			Start int `json:"start"`
		} `json:"submatches"`
	} `json:"data"`
}

func (s *Server) projectContentSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	limit := projectContentSearchDefaultLimit
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	if limit < 1 {
		limit = 1
	}
	if limit > projectContentSearchMaxLimit {
		limit = projectContentSearchMaxLimit
	}
	if query == "" {
		writeJSON(w, http.StatusOK, map[string]any{"results": []projectContentSearchResult{}})
		return
	}
	root, err := s.projectRoot()
	if err != nil {
		http.Error(w, "project root unavailable", http.StatusInternalServerError)
		return
	}
	results, err := searchProjectContent(r.Context(), root, query, limit)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return
		}
		http.Error(w, "project content search failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

func searchProjectContent(ctx context.Context, root, query string, limit int) ([]projectContentSearchResult, error) {
	if limit <= 0 {
		return []projectContentSearchResult{}, nil
	}
	if rg, err := exec.LookPath("rg"); err == nil {
		if results, err := searchProjectContentRG(ctx, rg, root, query, limit); err == nil {
			return results, nil
		} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
	}
	return searchProjectContentFallback(ctx, root, query, limit)
}

func searchProjectContentRG(parent context.Context, rg, root, query string, limit int) ([]projectContentSearchResult, error) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	cmd := exec.CommandContext(ctx, rg,
		"--json",
		"--color", "never",
		"--line-number",
		"--column",
		"--fixed-strings",
		"--glob", "!.git/**",
		"--",
		query,
		".",
	)
	cmd.Dir = root
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	results := make([]projectContentSearchResult, 0, minInt(limit, 32))
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64<<10), projectContentSearchMaxLineBytes)
	for scanner.Scan() {
		if err := parent.Err(); err != nil {
			cancel()
			_ = cmd.Wait()
			return nil, err
		}
		var event rgJSONMatch
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil || event.Type != "match" {
			continue
		}
		rel := normalizeProjectIndexPath(strings.TrimPrefix(event.Data.Path.Text, "./"))
		if rel == "" {
			continue
		}
		column := 1
		if len(event.Data.Submatches) > 0 {
			column = event.Data.Submatches[0].Start + 1
		}
		results = append(results, projectContentSearchResult{
			Path: rel, Line: event.Data.LineNumber, Column: column,
			Preview: strings.TrimRight(event.Data.Lines.Text, "\r\n"),
		})
		if len(results) >= limit {
			cancel()
			break
		}
	}
	scanErr := scanner.Err()
	waitErr := cmd.Wait()
	if scanErr != nil {
		return nil, scanErr
	}
	if len(results) >= limit {
		return results, nil
	}
	if err := parent.Err(); err != nil {
		return nil, err
	}
	if waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) && exitErr.ExitCode() == 1 {
			return results, nil
		}
		if ctx.Err() != nil && parent.Err() == nil {
			return results, nil
		}
		return nil, waitErr
	}
	return results, nil
}

func searchProjectContentFallback(ctx context.Context, root, query string, limit int) ([]projectContentSearchResult, error) {
	needle := []byte(query)
	results := make([]projectContentSearchResult, 0, minInt(limit, 32))
	err := filepath.WalkDir(root, func(full string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if full == root {
			return nil
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, full)
		if err != nil {
			return nil
		}
		rel = normalizeProjectIndexPath(filepath.ToSlash(rel))
		if rel == "" {
			return nil
		}
		data, err := os.ReadFile(full)
		if err != nil {
			return nil
		}
		if len(data) == 0 || !projectContentTextBytes(data) {
			return nil
		}
		for lineNumber, start := 1, 0; start <= len(data); lineNumber++ {
			if err := ctx.Err(); err != nil {
				return err
			}
			end := bytes.IndexByte(data[start:], '\n')
			next := len(data)
			if end >= 0 {
				next = start + end
			}
			line := bytes.TrimSuffix(data[start:next], []byte{'\r'})
			searchFrom := 0
			for searchFrom <= len(line) {
				at := bytes.Index(line[searchFrom:], needle)
				if at < 0 {
					break
				}
				at += searchFrom
				results = append(results, projectContentSearchResult{
					Path: rel, Line: lineNumber, Column: at + 1, Preview: string(line),
				})
				if len(results) >= limit {
					return errProjectContentSearchLimit
				}
				searchFrom = at + maxInt(1, len(needle))
			}
			if end < 0 {
				break
			}
			start = next + 1
		}
		return nil
	})
	if errors.Is(err, errProjectContentSearchLimit) {
		return results, nil
	}
	return results, err
}

var errProjectContentSearchLimit = errors.New("project content search result limit reached")

func projectContentTextBytes(data []byte) bool {
	sample := data
	if len(sample) > projectBinarySample {
		sample = sample[:projectBinarySample]
	}
	if bytes.IndexByte(sample, 0) >= 0 {
		return false
	}
	if bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) {
		data = data[3:]
	}
	return utf8.Valid(data)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

var _ io.Reader
