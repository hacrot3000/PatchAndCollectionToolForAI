package server

import (
	"container/heap"
	"context"
	"net/http"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

const projectIndexFreshFor = 30 * time.Second

type projectFileSearchResult struct {
	Path  string `json:"path"`
	Name  string `json:"name"`
	Score int    `json:"score"`
}

func (s *Server) projectFileSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	limit := 50
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 50 {
		limit = 50
	}
	if query == "" {
		writeJSON(w, http.StatusOK, map[string]any{"results": []projectFileSearchResult{}})
		return
	}
	var idx *projectFileIndex
	var err error
	if r.URL.Query().Get("refresh") == "1" {
		idx, err = s.refreshProjectFileIndexNow(r.Context())
	} else {
		idx, err = s.currentProjectFileIndex(r.Context())
	}
	if err != nil {
		http.Error(w, "project file index unavailable", http.StatusInternalServerError)
		return
	}
	results := searchProjectFileIndex(idx, query, limit)
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

func (s *Server) currentProjectFileIndex(ctx context.Context) (*projectFileIndex, error) {
	root, err := s.projectRoot()
	if err != nil {
		return nil, err
	}

	s.projectIndexMu.Lock()
	if s.projectIndex != nil {
		idx := s.projectIndex
		stale := time.Since(idx.builtAt) > projectIndexFreshFor
		if stale && !s.projectIndexRefreshing {
			s.projectIndexRefreshing = true
			go s.refreshProjectFileIndex(root)
		}
		s.projectIndexMu.Unlock()
		return idx, nil
	}

	if cached, cacheErr := loadProjectIndexCache(root); cacheErr == nil {
		s.projectIndex = cached
		stale := time.Since(cached.builtAt) > projectIndexFreshFor
		if stale && !s.projectIndexRefreshing {
			s.projectIndexRefreshing = true
			go s.refreshProjectFileIndex(root)
		}
		s.projectIndexMu.Unlock()
		return cached, nil
	}

	buildCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	idx, buildErr := buildProjectFileIndex(buildCtx, root)
	cancel()
	if buildErr == nil {
		s.projectIndex = idx
	}
	s.projectIndexMu.Unlock()
	if buildErr != nil {
		return nil, buildErr
	}
	_ = saveProjectIndexCache(root, idx)
	return idx, nil
}

func (s *Server) refreshProjectFileIndex(root string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	idx, err := buildProjectFileIndex(ctx, root)

	s.projectIndexMu.Lock()
	defer s.projectIndexMu.Unlock()
	s.projectIndexRefreshing = false
	if err != nil {
		return
	}
	s.projectIndex = idx
	_ = saveProjectIndexCache(root, idx)
}

func (s *Server) refreshProjectFileIndexNow(ctx context.Context) (*projectFileIndex, error) {
	root, err := s.projectRoot()
	if err != nil {
		return nil, err
	}
	s.projectIndexMu.Lock()
	if s.projectIndex != nil && time.Since(s.projectIndex.builtAt) < 2*time.Second {
		idx := s.projectIndex
		s.projectIndexMu.Unlock()
		return idx, nil
	}
	s.projectIndexMu.Unlock()

	buildCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	idx, err := buildProjectFileIndex(buildCtx, root)
	if err != nil {
		return nil, err
	}
	s.projectIndexMu.Lock()
	s.projectIndex = idx
	s.projectIndexRefreshing = false
	s.projectIndexMu.Unlock()
	_ = saveProjectIndexCache(root, idx)
	return idx, nil
}

type projectFileSearchHeap []projectFileSearchResult

func (h projectFileSearchHeap) Len() int { return len(h) }
func (h projectFileSearchHeap) Less(i, j int) bool {
	if h[i].Score != h[j].Score {
		return h[i].Score < h[j].Score
	}
	return h[i].Path > h[j].Path
}
func (h projectFileSearchHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *projectFileSearchHeap) Push(value any) {
	*h = append(*h, value.(projectFileSearchResult))
}
func (h *projectFileSearchHeap) Pop() any {
	old := *h
	n := len(old)
	value := old[n-1]
	*h = old[:n-1]
	return value
}

func projectFileSearchBetter(a, b projectFileSearchResult) bool {
	if a.Score != b.Score {
		return a.Score > b.Score
	}
	return a.Path < b.Path
}

func searchProjectFileIndex(idx *projectFileIndex, query string, limit int) []projectFileSearchResult {
	if idx == nil || limit <= 0 {
		return []projectFileSearchResult{}
	}
	top := make(projectFileSearchHeap, 0, limit)
	heap.Init(&top)
	for i := range idx.offsets {
		candidate := idx.pathAt(i)
		score, ok := fuzzyProjectPathScore(candidate, query)
		if !ok {
			continue
		}
		result := projectFileSearchResult{
			Path: candidate, Name: path.Base(candidate), Score: score,
		}
		if top.Len() < limit {
			heap.Push(&top, result)
			continue
		}
		if projectFileSearchBetter(result, top[0]) {
			top[0] = result
			heap.Fix(&top, 0)
		}
	}
	results := []projectFileSearchResult(top)
	sort.Slice(results, func(i, j int) bool {
		return projectFileSearchBetter(results[i], results[j])
	})
	return results
}

func fuzzyProjectPathScore(candidate, query string) (int, bool) {
	candidateLower := strings.ToLower(candidate)
	baseLower := strings.ToLower(path.Base(candidate))
	queryLower := strings.ToLower(strings.TrimSpace(query))
	if queryLower == "" {
		return 0, false
	}

	if strings.ContainsAny(queryLower, "*?%") {
		switch {
		case wildcardProjectMatch(baseLower, queryLower):
			return 7000 + maxInt(0, 500-len(baseLower)), true
		case wildcardProjectMatch(candidateLower, queryLower):
			return 3000 + maxInt(0, 300-len(candidateLower)), true
		default:
			return 0, false
		}
	}

	tokens := strings.Fields(queryLower)
	if len(tokens) == 0 {
		return 0, false
	}
	total := projectBaseTokenSequenceBonus(baseLower, tokens)
	for _, token := range tokens {
		score, ok := fuzzyProjectTokenScore(candidateLower, baseLower, token)
		if !ok {
			return 0, false
		}
		total += score
	}
	if len(candidate) < 200 {
		total += 200 - len(candidate)
	}
	return total, true
}

func projectBaseTokenSequenceBonus(base string, queryTokens []string) int {
	queryJoined := strings.Join(queryTokens, " ")
	if base == queryJoined {
		return 10000
	}
	stem := strings.TrimSuffix(base, path.Ext(base))
	if stem == queryJoined {
		return 8500
	}
	parts := strings.FieldsFunc(stem, func(r rune) bool {
		return r == '_' || r == '-' || r == '.' || r == ' ' || r == '/'
	})
	if len(parts) == 0 || len(queryTokens) == 0 {
		return 0
	}
	if len(parts) == len(queryTokens) {
		for i := range parts {
			if parts[i] != queryTokens[i] {
				return 0
			}
		}
		return 2400
	}
	if len(parts) > len(queryTokens) {
		for i := range queryTokens {
			if parts[i] != queryTokens[i] {
				return 0
			}
		}
		return 900
	}
	return 0
}

func fuzzyProjectTokenScore(candidate, base, token string) (int, bool) {
	if token == "" {
		return 0, true
	}
	switch {
	case base == token:
		return 5200, true
	case strings.HasPrefix(base, token):
		return 3900 + len(token)*20, true
	case strings.Contains(base, token):
		return 3000 + len(token)*15, true
	}

	if score, ok := subsequenceProjectScore(base, token); ok {
		return 1900 + score, true
	}
	if strings.Contains(candidate, token) {
		return 1200 + len(token)*10, true
	}
	if score, ok := subsequenceProjectScore(candidate, token); ok {
		return 500 + score, true
	}
	return 0, false
}

func subsequenceProjectScore(value, token string) (int, bool) {
	valueRunes := []rune(value)
	tokenRunes := []rune(token)
	if len(tokenRunes) == 0 {
		return 0, true
	}
	pos := 0
	last := -2
	first := -1
	score := 0
	for _, want := range tokenRunes {
		found := -1
		for i := pos; i < len(valueRunes); i++ {
			if valueRunes[i] == want {
				found = i
				break
			}
		}
		if found < 0 {
			return 0, false
		}
		if first < 0 {
			first = found
		}
		score += 35
		if found == last+1 {
			score += 90
		}
		if found == 0 || strings.ContainsRune("/._- ", valueRunes[found-1]) {
			score += 45
		}
		if last >= 0 && found-last > 1 {
			gap := found - last - 1
			if gap > 30 {
				gap = 30
			}
			score -= gap * 3
		}
		last = found
		pos = found + 1
	}
	if first == 0 {
		score += 180
	}
	score -= maxInt(0, len(valueRunes)-len(tokenRunes))
	return score, true
}

func wildcardProjectMatch(value, pattern string) bool {
	v := []rune(value)
	p := []rune(pattern)
	i, j := 0, 0
	star, matched := -1, 0
	for i < len(v) {
		if j < len(p) && (p[j] == '?' || p[j] == v[i]) {
			i++
			j++
			continue
		}
		if j < len(p) && (p[j] == '*' || p[j] == '%') {
			star = j
			matched = i
			j++
			continue
		}
		if star >= 0 {
			j = star + 1
			matched++
			i = matched
			continue
		}
		return false
	}
	for j < len(p) && (p[j] == '*' || p[j] == '%') {
		j++
	}
	return j == len(p)
}

