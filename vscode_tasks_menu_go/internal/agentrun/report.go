package agentrun

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
)

const maxIntegrationDiffBytes = 8 << 20
const maxIntegrationPreviewBytes = 64 << 10
const maxIntegrationFiles = 1000

type IntegrationReport struct {
	RunID                   string    `json:"run_id"`
	ConversationID          string    `json:"conversation_id"`
	OwnerWorkspaceID        string    `json:"owner_workspace_id"`
	ExecutionWorkspaceID    string    `json:"execution_workspace_id"`
	BaseHEAD                string    `json:"base_head"`
	HEAD                    string    `json:"head"`
	ChangedFiles            []string  `json:"changed_files"`
	DiffSHA256              string    `json:"diff_sha256"`
	DiffPreview             string    `json:"diff_preview,omitempty"`
	DiffTruncated           bool      `json:"diff_truncated,omitempty"`
	StatusSHA256            string    `json:"status_sha256"`
	GeneratedAt             time.Time `json:"generated_at"`
	ApprovalRequired        bool      `json:"approval_required"`
	BranchMutationPerformed bool      `json:"branch_mutation_performed"`
}

type boundedOutput struct {
	buffer bytes.Buffer
	limit  int
}

type truncatedOutput struct {
	buffer    bytes.Buffer
	limit     int
	truncated bool
}

func (w *truncatedOutput) Write(data []byte) (int, error) {
	original := len(data)
	remaining := w.limit - w.buffer.Len()
	if remaining > 0 {
		if len(data) > remaining {
			data = data[:remaining]
			w.truncated = true
		}
		_, _ = w.buffer.Write(data)
	} else if original > 0 {
		w.truncated = true
	}
	return original, nil
}

func (b *boundedOutput) Write(data []byte) (int, error) {
	if b.buffer.Len()+len(data) > b.limit {
		return 0, errors.New("Git report output exceeds limit")
	}
	return b.buffer.Write(data)
}

func gitOutputRaw(ctx context.Context, dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return "", errors.New("Git workspace query failed")
	}
	return strings.TrimRight(string(out), "\\r\\n"), nil
}

func (r *WorkspaceRegistry) IntegrationReport(ctx context.Context, spec agentchat.RunSpec) (IntegrationReport, error) {
	binding, err := r.ResolveRun(ctx, spec)
	if err != nil {
		return IntegrationReport{}, err
	}
	if binding.ExecutionWorkspaceID == binding.OwnerWorkspaceID || binding.BaseHEAD == "" {
		return IntegrationReport{}, errors.New("integration report requires an isolated worktree with a base HEAD")
	}
	head, err := gitOutput(ctx, binding.WorktreePath, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return IntegrationReport{}, errors.New("worktree HEAD is unavailable")
	}
	status, err := gitOutputRaw(ctx, binding.WorktreePath, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return IntegrationReport{}, errors.New("worktree status is unavailable")
	}
	changed, untracked, err := parseChangedPaths(status)
	if err != nil {
		return IntegrationReport{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", binding.WorktreePath, "diff", "--no-ext-diff", "--binary", binding.BaseHEAD, "--")
	out := &boundedOutput{limit: maxIntegrationDiffBytes}
	cmd.Stdout, cmd.Stderr = out, &boundedOutput{limit: 64 << 10}
	if err := cmd.Run(); err != nil {
		return IntegrationReport{}, errors.New("cannot generate bounded worktree diff report")
	}
	diffHasher := sha256.New()
	_, _ = diffHasher.Write(out.buffer.Bytes())
	previewCommand := exec.CommandContext(ctx, "git", "-C", binding.WorktreePath, "diff", "--no-ext-diff", "--no-color", "--unified=3", binding.BaseHEAD, "--")
	preview := &truncatedOutput{limit: maxIntegrationPreviewBytes}
	previewCommand.Stdout, previewCommand.Stderr = preview, io.Discard
	if err := previewCommand.Run(); err != nil {
		return IntegrationReport{}, errors.New("cannot generate text integration preview")
	}
	for _, path := range untracked {
		content, readErr := readUntrackedReportFile(binding.WorktreePath, path)
		if readErr != nil {
			return IntegrationReport{}, readErr
		}
		_, _ = diffHasher.Write([]byte("\x00" + path + "\x00"))
		_, _ = diffHasher.Write(content)
		appendUntrackedPreview(preview, path, content)
	}
	diffDigest := diffHasher.Sum(nil)
	statusDigest := sha256.Sum256([]byte(status))
	return IntegrationReport{
		RunID: spec.RunID, ConversationID: spec.ConversationID,
		OwnerWorkspaceID: binding.OwnerWorkspaceID, ExecutionWorkspaceID: binding.ExecutionWorkspaceID,
		BaseHEAD: binding.BaseHEAD, HEAD: strings.TrimSpace(head), ChangedFiles: changed,
		DiffSHA256: hex.EncodeToString(diffDigest[:]), DiffPreview: preview.buffer.String(), DiffTruncated: preview.truncated,
		StatusSHA256: hex.EncodeToString(statusDigest[:]), GeneratedAt: time.Now().UTC(), ApprovalRequired: true, BranchMutationPerformed: false,
	}, nil
}

func parseChangedPaths(status string) ([]string, []string, error) {
	fields := strings.Split(status, "\x00")
	if len(fields) == 1 {
		fields = strings.Split(strings.ReplaceAll(status, "\r\n", "\n"), "\n")
	}
	paths := make([]string, 0, len(fields))
	untracked := make([]string, 0)
	seen := map[string]bool{}
	for i := 0; i < len(fields); i++ {
		field := fields[i]
		if len(field) < 4 || field[2] != ' ' {
			continue
		}
		statusCode := field[:2]
		path := filepath.ToSlash(filepath.Clean(field[3:]))
		if path == "." || path == ".." || strings.HasPrefix(path, "../") || filepath.IsAbs(path) {
			return nil, nil, errors.New("Git reported an unsafe changed path")
		}
		if !seen[path] {
			seen[path] = true
			paths = append(paths, path)
			if statusCode == "??" {
				untracked = append(untracked, path)
			}
		}
		if len(paths) > maxIntegrationFiles {
			return nil, nil, errors.New("integration report file count exceeds limit")
		}
		if strings.ContainsAny(statusCode, "RC") && i+1 < len(fields) {
			i++
		}
	}
	return paths, untracked, nil
}

func readUntrackedReportFile(root, relative string) ([]byte, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(root, filepath.FromSlash(relative))
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, errors.New("untracked report file is unavailable")
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, errors.New("untracked report path escapes worktree")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("untracked report file is not a regular file")
	}
	if info.Size() > 1<<20 {
		return nil, errors.New("untracked report file exceeds limit")
	}
	return os.ReadFile(path)
}

func appendUntrackedPreview(out *truncatedOutput, path string, content []byte) {
	if out.truncated {
		return
	}
	if len(content) > 64<<10 {
		content = content[:64<<10]
		out.truncated = true
	}
	if !utf8.Valid(content) {
		_, _ = fmt.Fprintf(out, "\n--- /dev/null\n+++ b/%s\n[binary or non-UTF8 untracked file omitted]\n", strings.Map(safeReportPathRune, path))
		return
	}
	_, _ = fmt.Fprintf(out, "\n--- /dev/null\n+++ b/%s\n", strings.Map(safeReportPathRune, path))
	lines := strings.Split(strings.TrimSuffix(string(content), "\n"), "\n")
	for _, line := range lines {
		_, _ = fmt.Fprintf(out, "+%s\n", line)
	}
}

func safeReportPathRune(r rune) rune {
	if r == '\n' || r == '\r' || r == 0 {
		return '_'
	}
	return r
}
