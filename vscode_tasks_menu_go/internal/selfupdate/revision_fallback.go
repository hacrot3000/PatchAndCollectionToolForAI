package selfupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// fullCommitSHA accepts only an immutable Git commit object ID (SHA-1).
// The repository currently uses 40-hex SHA-1 identifiers.
func fullCommitSHA(value string) bool {
	if len(value) != 40 {
		return false
	}
	for _, ch := range value {
		if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') ||
			(ch >= 'A' && ch <= 'F')) {
			return false
		}
	}
	return true
}

// validRemoteRevisionBranch avoids ambiguous ref patterns and ensures that a
// user-configured branch is passed as one exact ref argument, never shell text.
func validRemoteRevisionBranch(branch string) bool {
	if branch == "" || len(branch) > 200 ||
		strings.HasPrefix(branch, "/") || strings.HasSuffix(branch, "/") ||
		strings.Contains(branch, "//") || strings.Contains(branch, "..") ||
		strings.Contains(branch, "@{") || strings.ContainsRune(branch, '\\') ||
		strings.HasSuffix(branch, ".") {
		return false
	}
	for _, ch := range branch {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') || ch == '-' || ch == '_' || ch == '.' || ch == '/' {
			continue
		}
		return false
	}
	for _, part := range strings.Split(branch, "/") {
		if part == "" || strings.HasPrefix(part, ".") || strings.HasSuffix(strings.ToLower(part), ".lock") {
			return false
		}
	}
	return true
}

func revisionAPIError(branch string, resp *http.Response) error {
	message := fmt.Sprintf("GitHub revision query for branch %q returned %s", branch, resp.Status)
	switch {
	case resp.Header.Get("X-RateLimit-Remaining") == "0":
		message += " (GitHub API rate limit reached"
		if stamp, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil && stamp > 0 {
			message += "; resets " + time.Unix(stamp, 0).UTC().Format(time.RFC3339)
		}
		message += ")"
	case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests:
		message += " (API access forbidden or throttled)"
	}
	return fmt.Errorf("%s", message)
}

func remoteRevisionWithFallback(ctx context.Context, branch string, client *http.Client, gitLookup func(context.Context, string) (string, error)) (string, error) {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		branch = Branch
	}
	if !validRemoteRevisionBranch(branch) {
		return "", fmt.Errorf("invalid self-update branch %q", branch)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubCommitURL(branch), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "vscode_tasks_menu-self-update")
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return fallbackGitRevision(ctx, branch, fmt.Errorf("GitHub revision API request: %w", err), gitLookup)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		apiErr := revisionAPIError(branch, resp)
		switch resp.StatusCode {
		case http.StatusForbidden, http.StatusTooManyRequests,
			http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return fallbackGitRevision(ctx, branch, apiErr, gitLookup)
		default:
			return "", apiErr // Never hide 401/404/invalid branches with a fallback.
		}
	}
	var value struct {
		SHA string `json:"sha"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&value); err != nil {
		return "", fmt.Errorf("decode GitHub revision response: %w", err)
	}
	if !fullCommitSHA(value.SHA) {
		return "", fmt.Errorf("GitHub returned invalid revision for branch %q", branch)
	}
	return strings.ToLower(value.SHA), nil
}

func fallbackGitRevision(ctx context.Context, branch string, apiErr error, lookup func(context.Context, string) (string, error)) (string, error) {
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if lookup == nil {
		return "", apiErr
	}
	sha, err := lookup(ctx, branch)
	if err != nil {
		return "", fmt.Errorf("%w; HTTPS git ls-remote fallback failed: %v", apiErr, err)
	}
	if !fullCommitSHA(sha) {
		return "", fmt.Errorf("%w; HTTPS git ls-remote returned invalid commit SHA", apiErr)
	}
	return strings.ToLower(sha), nil
}

// gitRemoteRevisionForBranch resolves an EXACT public branch ref using git's
// HTTPS smart protocol; it does not consume GitHub REST API quota. No shell,
// token, user credentials or mutable local working tree is involved.
func gitRemoteRevisionForBranch(ctx context.Context, branch string) (string, error) {
	if !validRemoteRevisionBranch(branch) {
		return "", fmt.Errorf("invalid git branch %q", branch)
	}
	bin, err := exec.LookPath("git")
	if err != nil {
		return "", fmt.Errorf("git executable is not installed or not in PATH: %w", err)
	}
	gitCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	ref := "refs/heads/" + branch
	cmd := exec.CommandContext(gitCtx, bin,
		"-c", "credential.helper=",
		"ls-remote", "--heads", "--exit-code",
		"https://github.com/"+Repository+".git", ref)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=", "GCM_INTERACTIVE=never")
	out, err := cmd.Output()
	if err != nil {
		if gitCtx.Err() != nil {
			return "", gitCtx.Err()
		}
		return "", fmt.Errorf("resolve public branch %q over HTTPS: %w", branch, err)
	}
	// Git's ref pattern matching can return more than one line. Match the
	// FULL ref name and verify exactly one 40-hex SHA before trusting it.
	if len(out) > 64<<10 {
		return "", fmt.Errorf("git remote revision response is too large")
	}
	var sha string
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || fields[1] != ref {
			continue
		}
		if !fullCommitSHA(fields[0]) || sha != "" {
			return "", fmt.Errorf("git remote revision returned malformed or ambiguous ref")
		}
		sha = fields[0]
	}
	if sha == "" {
		return "", fmt.Errorf("git remote branch %q was not found", branch)
	}
	return strings.ToLower(sha), nil
}
