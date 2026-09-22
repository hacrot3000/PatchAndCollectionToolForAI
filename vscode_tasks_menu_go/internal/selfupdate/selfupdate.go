package selfupdate

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"bletonfc/vscode_tasks_menu/internal/state"
)

const (
	Repository     = "hacrot3000/PatchAndCollectionToolForAI"
	Branch         = "main"
	maxArchiveSize = 128 << 20
)

type Request struct {
	ID          string `json:"id"`
	Revision    string `json:"revision"`
	Status      string `json:"status"`
	Message     string `json:"message,omitempty"`
	CurrentURL  string `json:"current_url,omitempty"`
	TargetURL   string `json:"target_url,omitempty"`
	Error       string `json:"error,omitempty"`
	RequestedAt string `json:"requested_at"`
	ConfirmedAt string `json:"confirmed_at,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`
}

func RequestPath(workspace string) string {
	return filepath.Join(state.Dir(workspace), "self-update.json")
}

func randomID() (string, error) {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func CreateRequest(workspace, revision, currentURL string, confirmed bool) (Request, error) {
	if err := state.EnsureDir(workspace); err != nil {
		return Request{}, err
	}
	id, err := randomID()
	if err != nil {
		return Request{}, err
	}
	status := "awaiting_confirmation"
	confirmedAt := ""
	if confirmed {
		status = "confirmed"
		confirmedAt = time.Now().Format(time.RFC3339)
	}
	req := Request{ID: id, Revision: revision, Status: status, CurrentURL: currentURL, RequestedAt: time.Now().Format(time.RFC3339), ConfirmedAt: confirmedAt}
	return req, Save(workspace, req)
}

func Load(workspace string) (Request, error) {
	data, err := os.ReadFile(RequestPath(workspace))
	if err != nil {
		return Request{}, err
	}
	var req Request
	if err := json.Unmarshal(data, &req); err != nil {
		return Request{}, err
	}
	return req, nil
}

func Save(workspace string, req Request) error {
	if err := state.EnsureDir(workspace); err != nil {
		return err
	}
	data, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		return err
	}
	path := RequestPath(workspace)
	tmp, err := os.CreateTemp(filepath.Dir(path), ".self-update.*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func Update(workspace, id, status, message, targetURL, errText string) (Request, error) {
	req, err := Load(workspace)
	if err != nil {
		return Request{}, err
	}
	if req.ID != id {
		return Request{}, fmt.Errorf("self-update request changed")
	}
	req.Status = status
	req.Message = message
	if targetURL != "" {
		req.TargetURL = targetURL
	}
	if errText != "" {
		req.Error = errText
	}
	now := time.Now().Format(time.RFC3339)
	if status == "confirmed" && req.ConfirmedAt == "" {
		req.ConfirmedAt = now
	}
	if status == "completed" || status == "failed" || status == "cancelled" {
		req.CompletedAt = now
	}
	return req, Save(workspace, req)
}

func WaitForDecision(workspace, id string, timeout time.Duration) (Request, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		req, err := Load(workspace)
		if err == nil && req.ID == id {
			switch req.Status {
			case "confirmed":
				return req, nil
			case "cancelled":
				return req, fmt.Errorf("update cancelled")
			case "failed":
				return req, fmt.Errorf("update failed: %s", req.Error)
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return Request{}, fmt.Errorf("timed out waiting for update confirmation")
}

func RemoteRevision(ctx context.Context) (string, error) {
	url := "https://api.github.com/repos/" + Repository + "/commits/" + Branch
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "vscode_tasks_menu-self-update")
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub revision query returned %s", resp.Status)
	}
	var value struct{ SHA string `json:"sha"` }
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&value); err != nil {
		return "", err
	}
	if len(value.SHA) < 12 {
		return "", fmt.Errorf("GitHub returned invalid revision")
	}
	return value.SHA, nil
}

func MarkerPath(binary string) string { return binary + ".revision" }

func GlobalBinaryPath() (string, error) {
	if dir := strings.TrimSpace(os.Getenv("TASKDECK_INSTALL_DIR")); dir != "" {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return "", err
		}
		return filepath.Join(abs, "taskdeck"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".local", "bin", "taskdeck"), nil
}

func ExecutableExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
}

func SameExecutablePath(left, right string) bool {
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	if leftErr != nil || rightErr != nil {
		return filepath.Clean(left) == filepath.Clean(right)
	}
	return filepath.Clean(leftAbs) == filepath.Clean(rightAbs)
}

func PreferredBinary(current string) string {
	global, err := GlobalBinaryPath()
	if err == nil && ExecutableExists(global) {
		return global
	}
	return current
}

func InstalledRevision(binary string) string {
	data, err := os.ReadFile(MarkerPath(binary))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func launcherPathForBinary(targetBinary string) (string, bool) {
	targetBinary = filepath.Clean(targetBinary)
	buildDir := filepath.Dir(targetBinary)
	sourceDir := filepath.Dir(buildDir)
	if filepath.Base(targetBinary) != "vscode_tasks_menu" || filepath.Base(buildDir) != ".build" || filepath.Base(sourceDir) != "vscode_tasks_menu_go" {
		return "", false
	}
	return filepath.Join(filepath.Dir(sourceDir), "vscode_tasks_menu"), true
}

func stagedLauncherPath(stagedBinary string) string {
	return stagedBinary + ".launcher"
}

func Prepare(ctx context.Context, revision, targetBinary string, progress func(status, message string)) (string, error) {
	if progress == nil {
		progress = func(string, string) {}
	}
	tmpRoot, err := os.MkdirTemp("", "vscode_tasks_menu-update-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpRoot)

	progress("downloading", "Đang tải source mới từ GitHub…")
	if err := downloadSource(ctx, revision, tmpRoot); err != nil {
		return "", err
	}
	source := filepath.Join(tmpRoot, "vscode_tasks_menu_go")
	if _, err := os.Stat(filepath.Join(source, "go.mod")); err != nil {
		return "", fmt.Errorf("archive thiếu vscode_tasks_menu_go/go.mod")
	}

	progress("testing", "Đang chạy go test trước khi cài…")
	if out, err := runGo(ctx, source, "test", "./..."); err != nil {
		return "", fmt.Errorf("go test failed: %w\n%s", err, trimOutput(out))
	}

	progress("building", "Đang compile binary mới…")
	dir := filepath.Dir(targetBinary)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	staged, err := os.CreateTemp(dir, ".vscode_tasks_menu.new.*")
	if err != nil {
		return "", err
	}
	stagedPath := staged.Name()
	if err := staged.Close(); err != nil {
		os.Remove(stagedPath)
		return "", err
	}
	os.Remove(stagedPath)
	ldflags := "-X main.buildRevision=" + revision
	if out, err := runGo(ctx, source, "build", "-trimpath", "-ldflags", ldflags, "-o", stagedPath, "./cmd/vscode_tasks_menu"); err != nil {
		os.Remove(stagedPath)
		return "", fmt.Errorf("go build failed: %w\n%s", err, trimOutput(out))
	}
	if err := os.Chmod(stagedPath, 0o755); err != nil {
		os.Remove(stagedPath)
		return "", err
	}
	check := exec.CommandContext(ctx, stagedPath, "--version")
	out, err := check.CombinedOutput()
	if err != nil || !strings.Contains(string(out), revision) {
		os.Remove(stagedPath)
		return "", fmt.Errorf("new binary validation failed: %v %s", err, trimOutput(out))
	}
	if _, ok := launcherPathForBinary(targetBinary); ok {
		launcherSource := filepath.Join(tmpRoot, "vscode_tasks_menu")
		launcherData, readErr := os.ReadFile(launcherSource)
		if readErr != nil {
			os.Remove(stagedPath)
			return "", fmt.Errorf("archive thiếu root launcher vscode_tasks_menu: %w", readErr)
		}
		launcherStage := stagedLauncherPath(stagedPath)
		if writeErr := os.WriteFile(launcherStage, launcherData, 0o755); writeErr != nil {
			os.Remove(stagedPath)
			return "", fmt.Errorf("stage root launcher: %w", writeErr)
		}
	}
	return stagedPath, nil
}

func Install(stagedPath, targetBinary, revision string) error {
	if stagedPath == "" || targetBinary == "" {
		return fmt.Errorf("invalid self-update install path")
	}
	defer os.Remove(stagedLauncherPath(stagedPath))
	if launcherTarget, ok := launcherPathForBinary(targetBinary); ok {
		launcherStage := stagedLauncherPath(stagedPath)
		if _, err := os.Stat(launcherStage); err == nil {
			if err := os.Rename(launcherStage, launcherTarget); err != nil {
				return fmt.Errorf("replace root launcher: %w", err)
			}
			if err := os.Chmod(launcherTarget, 0o755); err != nil {
				return fmt.Errorf("chmod root launcher: %w", err)
			}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect staged root launcher: %w", err)
		}
	}
	if err := os.Rename(stagedPath, targetBinary); err != nil {
		return fmt.Errorf("replace executable: %w", err)
	}
	if err := os.Chmod(targetBinary, 0o755); err != nil {
		return err
	}
	tmp := MarkerPath(targetBinary) + ".tmp"
	if err := os.WriteFile(tmp, []byte(revision+"\n"), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, MarkerPath(targetBinary))
}

func runGo(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOPROXY=off", "GOSUMDB=off")
	return cmd.CombinedOutput()
}

func trimOutput(data []byte) string {
	const max = 16 << 10
	if len(data) > max {
		data = data[len(data)-max:]
	}
	return strings.TrimSpace(string(data))
}

func downloadSource(ctx context.Context, revision, dst string) error {
	archiveErr := downloadAndExtract(ctx, revision, dst)
	if archiveErr == nil {
		return nil
	}
	if err := downloadWithGit(ctx, revision, dst); err != nil {
		return fmt.Errorf("GitHub archive download failed: %v; git fallback failed: %w", archiveErr, err)
	}
	return nil
}

func downloadWithGit(ctx context.Context, revision, dst string) error {
	git, err := exec.LookPath("git")
	if err != nil {
		return fmt.Errorf("git executable not found")
	}
	checkout := filepath.Join(dst, ".git-fallback")
	_ = os.RemoveAll(checkout)
	defer os.RemoveAll(checkout)
	if err := os.MkdirAll(checkout, 0o755); err != nil {
		return err
	}
	run := func(args ...string) error {
		cmd := exec.CommandContext(ctx, git, args...)
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%s: %w: %s", strings.Join(args, " "), err, trimOutput(out))
		}
		return nil
	}
	if err := run("init", "-q", checkout); err != nil {
		return err
	}
	if err := run("-C", checkout, "remote", "add", "origin", "https://github.com/"+Repository+".git"); err != nil {
		return err
	}
	if err := run("-C", checkout, "fetch", "-q", "--depth", "1", "origin", revision); err != nil {
		return err
	}
	if err := run("-C", checkout, "checkout", "-q", "--detach", "FETCH_HEAD"); err != nil {
		return err
	}
	source := filepath.Join(checkout, "vscode_tasks_menu_go")
	if _, err := os.Stat(filepath.Join(source, "go.mod")); err != nil {
		return fmt.Errorf("git fallback checkout missing vscode_tasks_menu_go/go.mod")
	}
	if err := copyTreeWithoutBuild(source, filepath.Join(dst, "vscode_tasks_menu_go")); err != nil {
		return err
	}
	launcherData, err := os.ReadFile(filepath.Join(checkout, "vscode_tasks_menu"))
	if err != nil {
		return fmt.Errorf("git fallback checkout missing root launcher: %w", err)
	}
	return os.WriteFile(filepath.Join(dst, "vscode_tasks_menu"), launcherData, 0o755)
}

func copyTreeWithoutBuild(src, dst string) error {
	return filepath.WalkDir(src, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}
		if rel == ".build" || strings.HasPrefix(rel, ".build"+string(filepath.Separator)) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		mode := info.Mode().Perm()
		if mode == 0 {
			mode = 0o644
		}
		return os.WriteFile(target, data, mode)
	})
}

func downloadAndExtract(ctx context.Context, revision, dst string) error {
	url := "https://codeload.github.com/" + Repository + "/tar.gz/" + revision
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "vscode_tasks_menu-self-update")
	resp, err := (&http.Client{Timeout: 90 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub archive returned %s", resp.Status)
	}
	gz, err := gzip.NewReader(io.LimitReader(resp.Body, maxArchiveSize))
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var total int64
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := filepath.ToSlash(hdr.Name)
		rootSlash := strings.IndexByte(name, '/')
		if rootSlash < 0 || rootSlash == len(name)-1 {
			continue
		}
		repoRel := name[rootSlash+1:]
		var target string
		if repoRel == "vscode_tasks_menu" {
			target = filepath.Join(dst, "vscode_tasks_menu")
		} else if strings.HasPrefix(repoRel, "vscode_tasks_menu_go/") {
			rel := strings.TrimPrefix(repoRel, "vscode_tasks_menu_go/")
			if rel == "" || strings.HasPrefix(rel, ".build/") || rel == ".build" {
				continue
			}
			clean := filepath.Clean(filepath.FromSlash(rel))
			if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || filepath.IsAbs(clean) {
				return fmt.Errorf("unsafe archive path %q", hdr.Name)
			}
			target = filepath.Join(dst, "vscode_tasks_menu_go", clean)
		} else {
			continue
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			total += hdr.Size
			if total > maxArchiveSize {
				return fmt.Errorf("self-update archive exceeds extracted size limit")
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
			if err != nil {
				return err
			}
			_, copyErr := io.CopyN(f, tr, hdr.Size)
			closeErr := f.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		}
	}
	return nil
}
