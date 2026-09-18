package server

import (
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
	idx, err := s.currentProjectFileIndex(r.Context())
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

func searchProjectFileIndex(idx *projectFileIndex, query string, limit int) []projectFileSearchResult {
	if idx == nil || limit <= 0 {
		return []projectFileSearchResult{}
	}
	results := make([]projectFileSearchResult, 0, limit*2)
	for i := range idx.offsets {
		candidate := idx.pathAt(i)
		score, ok := fuzzyProjectPathScore(candidate, query)
		if !ok {
			continue
		}
		results = append(results, projectFileSearchResult{
			Path:  candidate,
			Name:  path.Base(candidate),
			Score: score,
		})
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].Path < results[j].Path
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results
}

func fuzzyProjectPathScore(candidate, query string) (int, bool) {
	candidateLower := strings.ToLower(candidate)
	baseLower := strings.ToLower(path.Base(candidate))
	tokens := strings.Fields(strings.ToLower(strings.TrimSpace(query)))
	if len(tokens) == 0 {
		return 0, false
	}
	total := 0
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

func fuzzyProjectTokenScore(candidate, base, token string) (int, bool) {
	if token == "" {
		return 0, true
	}
	score := 0
	switch {
	case base == token:
		score += 2400
	case strings.HasPrefix(base, token):
		score += 1500
	case strings.Contains(base, token):
		score += 900
	case strings.Contains(candidate, token):
		score += 450
	}

	pos := 0
	last := -2
	for _, want := range token {
		found := -1
		for i, got := range candidate[pos:] {
			if got == want {
				found = pos + i
				break
			}
		}
		if found < 0 {
			return 0, false
		}
		score += 20
		if found == last+1 {
			score += 30
		}
		if found == 0 || strings.ContainsRune("/._-", rune(candidate[found-1])) {
			score += 25
		}
		if found >= len(candidate)-len(base) {
			score += 15
		}
		if last >= 0 && found-last > 1 {
			gap := found - last - 1
			if gap > 20 {
				gap = 20
			}
			score -= gap
		}
		last = found
		pos = found + 1
	}
	return score, true
}
