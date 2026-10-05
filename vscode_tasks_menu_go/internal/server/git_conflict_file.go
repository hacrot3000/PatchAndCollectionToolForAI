package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type gitConflictFileWriteRequest struct {
	Path           string `json:"path"`
	Content        string `json:"content"`
	ExpectedSHA256 string `json:"expected_sha256"`
}

type gitConflictTextSide struct {
	Exists  bool   `json:"exists"`
	Content string `json:"content,omitempty"`
	SHA256  string `json:"sha256,omitempty"`
}

func (s *Server) gitConflictFile(w http.ResponseWriter, r *http.Request) {
	repo, err := s.resolveGitRepository(r.URL.Query().Get("repo"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	r = r.WithContext(withGitRepository(r.Context(), repo))
	switch r.Method {
	case http.MethodGet:
		s.gitConflictFileRead(w, r, repo)
	case http.MethodPut:
		s.gitConflictFileWrite(w, r, repo)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) gitConflictFileRead(w http.ResponseWriter, r *http.Request, repo gitRepository) {
	pathValue, err := validGitRelativePath(r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	state, err := s.gitConflictState(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if state.Operation == "" {
		http.Error(w, "no Git conflict resolution operation is in progress", http.StatusConflict)
		return
	}
	item, ok := gitConflictFindFile(state, pathValue)
	if !ok {
		http.Error(w, pathValue+" is not currently an unmerged path", http.StatusNotFound)
		return
	}
	readStage := func(stage int, exists bool) (gitConflictTextSide, error) {
		if !exists {
			return gitConflictTextSide{}, nil
		}
		text, readErr := readGitConflictStageText(r.Context(), repo, stage, pathValue)
		if readErr != nil {
			return gitConflictTextSide{}, readErr
		}
		sum := sha256.Sum256([]byte(text))
		return gitConflictTextSide{Exists: true, Content: text, SHA256: hex.EncodeToString(sum[:])}, nil
	}
	base, err := readStage(1, item.HasBase)
	if err != nil {
		writeGitConflictTextError(w, err)
		return
	}
	current, err := readStage(2, item.HasCurrent)
	if err != nil {
		writeGitConflictTextError(w, err)
		return
	}
	incoming, err := readStage(3, item.HasIncoming)
	if err != nil {
		writeGitConflictTextError(w, err)
		return
	}
	result, err := readGitConflictWorkingText(repo, pathValue)
	if err != nil {
		writeGitConflictTextError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"repo_id": repo.ID,
		"operation": state.Operation,
		"branch": state.Branch,
		"path": item.Path,
		"project_path": item.ProjectPath,
		"status": item.Status,
		"base": base,
		"current": current,
		"incoming": incoming,
		"result": result,
		"conflicts": state.Files,
	})
}

var (
	errGitConflictTextTooLarge = errors.New("Git conflict text exceeds editor limit")
	errGitConflictTextBinary   = errors.New("Git conflict content is binary or unsupported text")
)

func readGitConflictStageText(parent context.Context, repo gitRepository, stage int, pathValue string) (string, error) {
	if stage < 1 || stage > 3 {
		return "", fmt.Errorf("invalid Git conflict stage")
	}
	ctx, cancel := context.WithTimeout(parent, 8*time.Second)
	defer cancel()
	spec := fmt.Sprintf(":%d:%s", stage, pathValue)
	cmd := exec.CommandContext(ctx, "git", "show", "--no-ext-diff", spec)
	cmd.Dir = repo.Root
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_PAGER=cat", "LC_ALL=C")
	stdout := &cappedGitBuffer{limit: int(projectEditableLimit) + 1}
	stderr := &cappedGitBuffer{limit: 64 << 10}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("Git conflict stage lookup timed out")
		}
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("%s", message)
	}
	if stdout.truncated || int64(len(stdout.buf.Bytes())) > projectEditableLimit {
		return "", errGitConflictTextTooLarge
	}
	data := append([]byte(nil), stdout.buf.Bytes()...)
	if !projectTextBytesValid(data) {
		return "", errGitConflictTextBinary
	}
	return string(data), nil
}

func readGitConflictWorkingText(repo gitRepository, pathValue string) (gitConflictTextSide, error) {
	full := filepath.Join(repo.Root, filepath.FromSlash(pathValue))
	info, err := os.Lstat(full)
	if errors.Is(err, os.ErrNotExist) {
		return gitConflictTextSide{}, nil
	}
	if err != nil {
		return gitConflictTextSide{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return gitConflictTextSide{}, fmt.Errorf("working conflict result is not a regular file")
	}
	if info.Size() > projectEditableLimit {
		return gitConflictTextSide{}, errGitConflictTextTooLarge
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return gitConflictTextSide{}, err
	}
	if !projectTextBytesValid(data) {
		return gitConflictTextSide{}, errGitConflictTextBinary
	}
	sum := sha256.Sum256(data)
	return gitConflictTextSide{Exists: true, Content: string(data), SHA256: hex.EncodeToString(sum[:])}, nil
}

func writeGitConflictTextError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errGitConflictTextTooLarge):
		http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
	case errors.Is(err, errGitConflictTextBinary):
		http.Error(w, err.Error(), http.StatusUnsupportedMediaType)
	default:
		http.Error(w, err.Error(), http.StatusConflict)
	}
}

func (s *Server) gitConflictFileWrite(w http.ResponseWriter, r *http.Request, repo gitRepository) {
	var req gitConflictFileWriteRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, projectEditableLimit+(64<<10)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "invalid Git conflict result request", http.StatusBadRequest)
		return
	}
	pathValue, err := validGitRelativePath(req.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req.ExpectedSHA256 = strings.ToLower(strings.TrimSpace(req.ExpectedSHA256))
	if !validCompareSHA256(req.ExpectedSHA256) {
		http.Error(w, "expected_sha256 is required", http.StatusBadRequest)
		return
	}
	content := []byte(req.Content)
	if int64(len(content)) > projectEditableLimit {
		http.Error(w, errGitConflictTextTooLarge.Error(), http.StatusRequestEntityTooLarge)
		return
	}
	if !projectTextBytesValid(content) {
		http.Error(w, errGitConflictTextBinary.Error(), http.StatusUnsupportedMediaType)
		return
	}
	state, err := s.gitConflictState(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	item, ok := gitConflictFindFile(state, pathValue)
	if state.Operation == "" || !ok {
		http.Error(w, pathValue+" is not currently an unmerged path", http.StatusConflict)
		return
	}
	if item.ProjectPath == "" {
		http.Error(w, "conflict result is outside the project workspace", http.StatusConflict)
		return
	}
	result, err := readGitConflictWorkingText(repo, pathValue)
	if err != nil {
		writeGitConflictTextError(w, err)
		return
	}
	if !result.Exists {
		http.Error(w, "working conflict result does not exist; choose a whole Current/Incoming side first", http.StatusConflict)
		return
	}
	if !strings.EqualFold(result.SHA256, req.ExpectedSHA256) {
		http.Error(w, "working conflict result changed after editor load", http.StatusConflict)
		return
	}
	if err := s.writeProjectReplaceCAS(item.ProjectPath, req.ExpectedSHA256, content); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	next, err := readGitConflictWorkingText(repo, pathValue)
	if err != nil {
		writeGitConflictTextError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"repo_id": repo.ID,
		"path": pathValue,
		"project_path": item.ProjectPath,
		"result": next,
	})
}
