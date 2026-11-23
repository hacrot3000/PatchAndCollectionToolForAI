package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
)

// agentWorkspaceSnapshot is server-derived evidence of the state a Patch was
// approved against. It never accepts Git metadata supplied by a model or a
// browser. A workspace that is not a Git repository is reported explicitly so
// callers can fail closed instead of assuming HEAD equality.
type agentWorkspaceSnapshot struct {
	IsRepo       bool
	Head         string
	Branch       string
	StatusSHA256 string
}

// errAgentDriftRefused marks a fail-closed refusal caused by workspace drift.
var errAgentDriftRefused = errors.New("workspace or Git state changed since approval")

func agentWorkspaceSnapshotOf(ctx context.Context, workspace string) (agentWorkspaceSnapshot, error) {
    return agentWorkspaceSnapshotWithQueueExcluded(ctx, workspace, "")
}

// After an Agent queues its immutable package, that exact untracked ZIP may
// appear in git status. Filter only this server-owned package (never all of
// patchs/ or artifacts/) when comparing the original approval snapshot.
func agentWorkspaceSnapshotWithQueueExcluded(ctx context.Context, workspace, queuePath string) (agentWorkspaceSnapshot, error) {
    if queuePath != "" {
        if !strings.HasPrefix(queuePath, "patchs/") ||
            !strings.HasSuffix(queuePath, ".zip") ||
            strings.TrimPrefix(queuePath, "patchs/") == "" ||
            strings.Contains(strings.TrimPrefix(queuePath, "patchs/"), "/") ||
            strings.ContainsAny(queuePath, "\x00\r\n") {
            return agentWorkspaceSnapshot{}, errAgentDriftRefused
        }
    }
	if workspace == "" {
		return agentWorkspaceSnapshot{}, errors.New("workspace unavailable")
	}
	var snap agentWorkspaceSnapshot
	head, _, _, err := runGitInDirectory(ctx, 5*time.Second, workspace, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		// Not a repository (or an unreadable one) must never be treated as an
		// empty-but-equal commit.
		return agentWorkspaceSnapshot{IsRepo: false}, nil
	}
	snap.IsRepo = true
	snap.Head = strings.TrimSpace(head)
	if len(snap.Head) != 40 && len(snap.Head) != 64 {
		return agentWorkspaceSnapshot{}, errors.New("unexpected Git HEAD")
	}
	branch, _, _, err := runGitInDirectory(ctx, 5*time.Second, workspace, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return agentWorkspaceSnapshot{}, errors.New("Git branch unavailable")
	}
	snap.Branch = strings.TrimSpace(branch)
	status, _, _, err := runGitInDirectory(ctx, 10*time.Second, workspace, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return agentWorkspaceSnapshot{}, errors.New("Git worktree status unavailable")
	}
    if queuePath != "" {
        // Git porcelain -z produces a NUL-terminated row for an untracked
        // file. Remove the exact queue row only; tracked/modified files and
        // every other untracked file must still fail the drift comparison.
        var filtered strings.Builder
        for _, row := range strings.SplitAfter(status, "\x00") {
            if row == "?? "+queuePath+"\x00" { continue }
            filtered.WriteString(row)
        }
        status = filtered.String()
    }
	digest := sha256.Sum256([]byte(status))
	snap.StatusSHA256 = hex.EncodeToString(digest[:])
	return snap, nil
}

// agentDriftMatches compares a fresh server-side snapshot with the recorded
// approval snapshot. An empty expected value is not permission to skip the
// check: for a repository the recorded fields must be present and equal.
func agentDriftMatches(expectedHead, expectedBranch, expectedStatus string, isRepo bool, fresh agentWorkspaceSnapshot) bool {
	if !isRepo || !fresh.IsRepo {
		return !isRepo && !fresh.IsRepo
	}
	return expectedHead != "" && expectedHead == fresh.Head &&
		expectedBranch != "" && expectedBranch == fresh.Branch &&
		expectedStatus != "" && expectedStatus == fresh.StatusSHA256
}

// agentVerifyApprovalStillValid re-derives workspace evidence and enforces the
// approval expiry. It is called before each side effect (queue, launch,
// selection) so a crash/retry cannot apply an approval to a changed worktree.
func (s *Server) agentVerifyApprovalStillValid(ctx context.Context, call *agentchat.ToolCallRecord) error {
	if call == nil {
		return errAgentDriftRefused
	}
	if call.ApprovalExpiresAt == nil || time.Now().UTC().After(*call.ApprovalExpiresAt) {
		return errAgentDriftRefused
	}
	fresh, err := agentWorkspaceSnapshotOf(ctx, s.Workspace)
	if err != nil {
		return err
	}
	if !agentDriftMatches(call.ExpectedHead, call.ExpectedBranch, call.WorkspaceStatusSHA256, call.GitIsRepo, fresh) {
		return errAgentDriftRefused
	}
	return nil
}

// agentVerifyApprovalAfterOwnQueue preserves the approval's original Git
// boundary while permitting ONLY the exact ZIP TaskDeck just placed in its
// native queue. It does not waive source drift, branch changes or expiry.
func (s *Server) agentVerifyApprovalAfterOwnQueue(ctx context.Context, call *agentchat.ToolCallRecord, queuePath string) error {
    if call == nil || call.ApprovalExpiresAt == nil ||
       time.Now().UTC().After(*call.ApprovalExpiresAt) {
        return errAgentDriftRefused
    }
    fresh, err := agentWorkspaceSnapshotWithQueueExcluded(ctx, s.Workspace, queuePath)
    if err != nil { return err }
    if !agentDriftMatches(call.ExpectedHead, call.ExpectedBranch, call.WorkspaceStatusSHA256, call.GitIsRepo, fresh) {
        return errAgentDriftRefused
    }
    return nil
}
