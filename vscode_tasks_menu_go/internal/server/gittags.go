package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type gitTagRow struct {
	Name       string `json:"name"`
	SHA        string `json:"sha"`
	Date       string `json:"date,omitempty"`
	Subject    string `json:"subject,omitempty"`
	Annotated  bool   `json:"annotated,omitempty"`
}

func validGitTagName(ctx context.Context, s *Server, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "\x00\r\n") {
		return "", fmt.Errorf("invalid Git tag name")
	}
	if _, _, _, err := s.runGit(ctx, 3*time.Second, "check-ref-format", "refs/tags/"+value); err != nil {
		return "", fmt.Errorf("invalid Git tag name")
	}
	return value, nil
}

func (s *Server) gitTags(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	format := "%(refname:short)%09%(objectname)%09%(creatordate:iso-strict)%09%(subject)%09%(objecttype)"
	out, _, truncated, err := s.runGit(r.Context(), 5*time.Second, "for-each-ref", "--sort=-creatordate", "--format="+format, "refs/tags")
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	rows := []gitTagRow{}
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "" { continue }
		fields := strings.SplitN(line, "\t", 5)
		if len(fields) != 5 { continue }
		rows = append(rows, gitTagRow{
			Name: fields[0], SHA: fields[1], Date: fields[2], Subject: fields[3],
			Annotated: strings.TrimSpace(fields[4]) == "tag",
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"tags": rows, "truncated": truncated})
}

func (s *Server) gitCreateTagArgs(ctx context.Context, name, message, ref string) ([]string, error) {
	tag, err := validGitTagName(ctx, s, name)
	if err != nil { return nil, err }
	if _, _, _, err := s.runGit(ctx, 3*time.Second, "show-ref", "--verify", "--quiet", "refs/tags/"+tag); err == nil {
		return nil, fmt.Errorf("Git tag %q already exists", tag)
	}
	ref = strings.TrimSpace(ref)
	if ref == "" { ref = "HEAD" }
	out, _, _, err := s.runGit(ctx, 4*time.Second, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil || strings.TrimSpace(out) == "" {
		return nil, fmt.Errorf("Git tag target is not a valid commit")
	}
	message = strings.TrimSpace(message)
	if len(message) > 2000 || strings.ContainsRune(message, '\x00') {
		return nil, fmt.Errorf("Git tag message is invalid")
	}
	if message == "" {
		return []string{"tag", tag, strings.TrimSpace(out)}, nil
	}
	return []string{"tag", "-a", tag, "-m", message, strings.TrimSpace(out)}, nil
}
