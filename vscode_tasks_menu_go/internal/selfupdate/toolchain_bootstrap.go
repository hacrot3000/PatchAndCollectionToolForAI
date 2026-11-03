package selfupdate

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	selfUpdateGoVersion      = "go1.27.3"
	selfUpdateGoArchiveLimit = int64(160 << 20)
	selfUpdateGoExtractLimit = int64(1024 << 20)
)

// Keep the pinned official Go distribution in sync with install.sh.
// Never run a shell profile or execute a compiler from an unchecked download.
func selfUpdateGoDistribution() (platform, digest string, err error) {
	switch runtime.GOOS + "-" + runtime.GOARCH {
	case "linux-amd64":
		return "linux-amd64", "65efbc80fdc1fa2cd2631cad5b481777a68e8725f34e89f50fc6ac714e379b40", nil
	case "linux-arm64":
		return "linux-arm64", "a06e99309dbcbdca1a8ccd2282c3ee146e55930886e26b4f1193269587165f49", nil
	case "darwin-amd64":
		return "darwin-amd64", "b6f8e931e4ab497fcde6da8ae4499a58f9d258e55c8a80824a516eae5950a750", nil
	case "darwin-arm64":
		return "darwin-arm64", "dd3908cf20959d798c1e06ebe5b72213f0505ce01373b40ce3110975a6cd99c2", nil
	default:
		return "", "", fmt.Errorf("automatic Go bootstrap is unsupported on %s/%s; configure TASKDECK_GO", runtime.GOOS, runtime.GOARCH)
	}
}

func selfUpdateSharedToolchainDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil { return "", fmt.Errorf("resolve home for TaskDeck Go SDK: %w", err) }
	if home == "" { return "", fmt.Errorf("cannot determine home for TaskDeck Go SDK") }
	// Intentionally independent of per-project TASKDECK_APP_DIR overrides.
	return filepath.Join(home, ".local", "lib", "taskdeck", "toolchains"), nil
}

func selfUpdateGoPath() (string, error) {
	platform, _, err := selfUpdateGoDistribution()
	if err != nil { return "", err }
	root, err := selfUpdateSharedToolchainDir()
	if err != nil { return "", err }
	return filepath.Join(root, selfUpdateGoVersion+"-"+platform, "bin", "go"), nil
}

func validBootstrappedGo(ctx context.Context, path string) bool {
	if !ExecutableExists(path) { return false }
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(checkCtx, path, "version").Output()
	return err == nil && strings.Contains(string(out), " "+selfUpdateGoVersion+" ") &&
		strings.Contains(string(out), " "+runtime.GOOS+"/"+runtime.GOARCH)
}

// Extract only regular files and directories below go/. Never honor archive
// symlinks, hardlinks, devices, absolute paths, traversal or excessive size.
func extractVerifiedGoArchive(archivePath, destination string) error {
	file, err := os.Open(archivePath)
	if err != nil { return err }
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil { return err }
	defer gz.Close()
	tr := tar.NewReader(gz)
	var extracted int64
	entries := 0
	for {
		header, err := tr.Next()
		if err == io.EOF { return nil }
		if err != nil { return err }
		entries++
		if entries > 200000 { return fmt.Errorf("Go archive contains too many entries") }
		name := strings.TrimSuffix(header.Name, "/")
		if name != "go" && !strings.HasPrefix(name, "go/") {
			return fmt.Errorf("Go archive path outside root: %q", header.Name)
		}
		if name == "go" {
			if header.Typeflag != tar.TypeDir { return fmt.Errorf("invalid Go archive root") }
			continue
		}
		rel := strings.TrimPrefix(name, "go/")
		if rel == "" || rel == "." || filepath.IsAbs(rel) ||
			filepath.Clean(rel) != rel || rel == ".." ||
			strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("unsafe Go archive entry: %q", header.Name)
		}
		target := filepath.Join(destination, rel)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o700); err != nil { return err }
		case tar.TypeReg, tar.TypeRegA:
			if header.Size < 0 || header.Size > selfUpdateGoExtractLimit-extracted {
				return fmt.Errorf("Go archive uncompressed size exceeds limit")
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil { return err }
			mode := header.FileInfo().Mode().Perm() &^ 0o022
			fd, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
			if err != nil { return err }
			written, copyErr := io.CopyN(fd, tr, header.Size)
			closeErr := fd.Close()
			if copyErr != nil { return copyErr }
			if closeErr != nil { return closeErr }
			if written != header.Size { return fmt.Errorf("truncated Go archive member") }
			extracted += written
		default:
			return fmt.Errorf("unsupported Go archive entry type %d: %q", header.Typeflag, name)
		}
	}
}

func downloadPinnedGoArchive(ctx context.Context, platform, digest, dest string) error {
	url := "https://go.dev/dl/" + selfUpdateGoVersion + "." + platform + ".tar.gz"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil { return err }
	client := &http.Client{Timeout: 10*time.Minute}
	resp, err := client.Do(req)
	if err != nil { return fmt.Errorf("download official Go SDK: %w", err) }
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("official Go SDK download returned HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > selfUpdateGoArchiveLimit {
		return fmt.Errorf("official Go SDK archive exceeds size limit")
	}
	fd, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil { return err }
	hash := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(fd, hash), io.LimitReader(resp.Body, selfUpdateGoArchiveLimit+1))
	closeErr := fd.Close()
	if copyErr != nil { return copyErr }
	if closeErr != nil { return closeErr }
	if size < 1<<20 || size > selfUpdateGoArchiveLimit {
		return fmt.Errorf("invalid Go SDK archive size %d", size)
	}
	if actual := hex.EncodeToString(hash.Sum(nil)); !strings.EqualFold(actual, digest) {
		return fmt.Errorf("official Go SDK SHA-256 mismatch; refusing installation")
	}
	return nil
}


// A successful updater can publish its *actual* compiler root for other
// TaskDeck daemons of this user, even if their launch PATHs are different.
func knownWorkingGoExecutable() string {
	root, err := selfUpdateSharedToolchainDir()
	if err != nil { return "" }
	data, err := os.ReadFile(filepath.Join(root, ".last-working-go"))
	if err != nil || len(data) > 4096 { return "" }
	path := strings.TrimSpace(string(data))
	if !filepath.IsAbs(path) || !ExecutableExists(path) { return "" }
	return path
}

func rememberWorkingGoExecutable(ctx context.Context, usedBinary string) {
	// Persist the physical GOROOT/bin/go, not a shell-dependent shim. This
	// also proves the program we ran is a real Go SDK rather than a stub.
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(checkCtx, usedBinary, "env", "GOROOT")
	output, err := cmd.Output()
	if err != nil { return }
	root := strings.TrimSpace(string(output))
	if !filepath.IsAbs(root) { return }
	actual := filepath.Join(root, "bin", "go")
	if !ExecutableExists(actual) { return }
	version, err := exec.CommandContext(checkCtx, actual, "version").Output()
	if err != nil || !strings.Contains(string(version), " go1.") { return }
	directory, err := selfUpdateSharedToolchainDir()
	if err != nil { return }
	if err := os.MkdirAll(directory, 0o700); err != nil { return }
	temp, err := os.CreateTemp(directory, ".go-pointer-*")
	if err != nil { return }
	tmp := temp.Name()
	defer os.Remove(tmp)
	if _, err = temp.WriteString(actual+"\n"); err != nil { temp.Close(); return }
	if err = temp.Chmod(0o600); err != nil { temp.Close(); return }
	if err = temp.Close(); err != nil { return }
	_ = os.Rename(tmp, filepath.Join(directory, ".last-working-go"))
}

// Multi-instance-safe shared toolchain bootstrap. Downloaded bytes are pinned
// to the same checksum as install.sh, extracted privately and activated only
// after version/platform verification; no daemon is stopped in this phase.
func bootstrapGoForSelfUpdate(ctx context.Context) (string, error) {
	binary, err := selfUpdateGoPath()
	if err != nil { return "", err }
	if validBootstrappedGo(ctx, binary) { return binary, nil }
	root := filepath.Dir(filepath.Dir(filepath.Dir(binary)))
	if err := os.MkdirAll(root, 0o700); err != nil { return "", err }
	unlock, err := acquireDirectoryLock(ctx, filepath.Join(root, ".bootstrap-"+selfUpdateGoVersion), 35*time.Minute)
	if err != nil { return "", err }
	defer unlock()
	if validBootstrappedGo(ctx, binary) { return binary, nil }
	final := filepath.Dir(filepath.Dir(binary))
	if _, err := os.Lstat(final); err == nil {
		return "", fmt.Errorf("existing Go SDK directory is invalid: %s", final)
	} else if !os.IsNotExist(err) { return "", err }
	platform, digest, err := selfUpdateGoDistribution()
	if err != nil { return "", err }
	tmp, err := os.MkdirTemp(root, ".go-bootstrap-*")
	if err != nil { return "", err }
	defer os.RemoveAll(tmp)
	archive := filepath.Join(tmp, "go.tar.gz")
	if err := downloadPinnedGoArchive(ctx, platform, digest, archive); err != nil { return "", err }
	staged := filepath.Join(tmp, "extracted")
	if err := os.Mkdir(staged, 0o700); err != nil { return "", err }
	if err := extractVerifiedGoArchive(archive, staged); err != nil { return "", err }
	if !validBootstrappedGo(ctx, filepath.Join(staged, "bin", "go")) {
		return "", fmt.Errorf("extracted Go SDK failed version/platform validation")
	}
	if err := os.Rename(staged, final); err != nil { return "", err }
	return binary, nil
}

// Shared atomic directory lock: no package-level mutex (which would not
// synchronize different project daemons). Crash leftovers eventually expire.
func acquireDirectoryLock(ctx context.Context, directory string, staleAfter time.Duration) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(directory), 0o700); err != nil { return nil, err }
	for {
		err := os.Mkdir(directory, 0o700)
		if err == nil { return func(){ _ = os.Remove(directory) }, nil }
		if !os.IsExist(err) { return nil, err }
		if info, statErr := os.Lstat(directory); statErr == nil {
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return nil, fmt.Errorf("unsafe TaskDeck update lock: %s", directory)
			}
			if time.Since(info.ModTime()) > staleAfter {
				_ = os.Remove(directory)
				continue
			}
		}
		select {
		case <-ctx.Done(): return nil, ctx.Err()
		case <-time.After(200*time.Millisecond):
		}
	}
}

// Hold through activation/handoff/rollback so other TaskDeck projects cannot
// race a shared global symlink or restore an unrelated project's release.
func AcquireGlobalUpdateLock(ctx context.Context) (func(), error) {
	root, err := GlobalAppRoot()
	if err != nil { return nil, err }
	return acquireDirectoryLock(ctx, filepath.Join(root, ".self-update-global.lock"), 35*time.Minute)
}
