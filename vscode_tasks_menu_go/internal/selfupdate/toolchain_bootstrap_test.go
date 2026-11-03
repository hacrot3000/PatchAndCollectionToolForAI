package selfupdate

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestSharedGoToolchainLocationIndependentOfProjectAppRoot(t *testing.T) {
	if runtime.GOOS == "windows" { t.Skip("home directory conventions") }
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("TASKDECK_APP_DIR", filepath.Join(t.TempDir(), "another-project-app"))
	got, err := selfUpdateSharedToolchainDir()
	if err != nil { t.Fatal(err) }
	want := filepath.Join(home, ".local", "lib", "taskdeck", "toolchains")
	if got != want { t.Fatalf("toolchains %q, want %q", got, want) }
	platform, digest, err := selfUpdateGoDistribution()
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		if err != nil || !strings.HasPrefix(platform, runtime.GOOS+"-") || len(digest) != 64 {
			t.Fatalf("platform=%q digest=%q err=%v", platform, digest, err)
		}
	}
}

func TestGlobalLockMutualExclusionAcrossIndependentCallers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("TASKDECK_APP_DIR", filepath.Join(t.TempDir(), "project-one-app"))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	unlock, err := AcquireGlobalUpdateLock(ctx)
	if err != nil { t.Fatal(err) }
	// Different app roots for the same user must not create two locks.
	t.Setenv("TASKDECK_APP_DIR", filepath.Join(t.TempDir(), "project-two-app"))
	blocked, stop := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer stop()
	if second, err := AcquireGlobalUpdateLock(blocked); err == nil {
		second()
		t.Fatal("different project acquired the global activation lock concurrently")
	}
	unlock()
	unlock, err = AcquireGlobalUpdateLock(ctx)
	if err != nil { t.Fatalf("global lock not available after release: %v", err) }
	unlock()
}

func TestGlobalLockCannotFollowSymlink(t *testing.T) {
	if runtime.GOOS == "windows" { t.Skip("symlink test requires POSIX") }
	home := t.TempDir()
	t.Setenv("HOME", home)
	lock := filepath.Join(home, ".local", "lib", "taskdeck", ".self-update-global.lock")
	if err := os.MkdirAll(filepath.Dir(lock), 0o700); err != nil { t.Fatal(err) }
	if err := os.Symlink(t.TempDir(), lock); err != nil { t.Skipf("symlink: %v", err) }
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	unlock, err := AcquireGlobalUpdateLock(ctx)
	if err == nil { unlock(); t.Fatal("symlink lock must be rejected") }
	if !strings.Contains(err.Error(), "unsafe") { t.Fatalf("unexpected error: %v", err) }
}

func TestExpiredDirectoryLockCanBeRecovered(t *testing.T) {
	lock := filepath.Join(t.TempDir(), "stale.lock")
	if err := os.Mkdir(lock, 0o700); err != nil { t.Fatal(err) }
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(lock, old, old); err != nil { t.Fatal(err) }
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	unlock, err := acquireDirectoryLock(ctx, lock, 30*time.Minute)
	if err != nil { t.Fatal(err) }
	unlock()
}

type archiveEntry struct {
	name string
	typeFlag byte
	body string
}

func writeSyntheticGoArchive(t *testing.T, entries []archiveEntry) string {
	t.Helper()
	dest := filepath.Join(t.TempDir(), "synthetic.tar.gz")
	fd, err := os.Create(dest)
	if err != nil { t.Fatal(err) }
	gz := gzip.NewWriter(fd)
	tarWriter := tar.NewWriter(gz)
	for _, entry := range entries {
		size := int64(len(entry.body))
		mode := int64(0o644)
		if entry.typeFlag == tar.TypeDir {
			size, mode = 0, 0o755
		}
		header := &tar.Header{Name: entry.name, Typeflag: entry.typeFlag, Mode: mode, Size: size}
		if err := tarWriter.WriteHeader(header); err != nil { t.Fatal(err) }
		if size > 0 {
			if _, err := tarWriter.Write([]byte(entry.body)); err != nil { t.Fatal(err) }
		}
	}
	if err := tarWriter.Close(); err != nil { t.Fatal(err) }
	if err := gz.Close(); err != nil { t.Fatal(err) }
	if err := fd.Close(); err != nil { t.Fatal(err) }
	return dest
}

func TestExtractGoArchivePermitsRegularFilesOnly(t *testing.T) {
	archive := writeSyntheticGoArchive(t, []archiveEntry{
		{name:"go/", typeFlag:tar.TypeDir},
		{name:"go/bin/", typeFlag:tar.TypeDir},
		{name:"go/src/runtime/runtime.go", typeFlag:tar.TypeReg, body:"package runtime\n"},
		{name:"go/bin/go", typeFlag:tar.TypeReg, body:"Go program"},
	})
	destination := t.TempDir()
	if err := extractVerifiedGoArchive(archive, destination); err != nil { t.Fatal(err) }
	data, err := os.ReadFile(filepath.Join(destination, "bin", "go"))
	if err != nil || string(data) != "Go program" { t.Fatalf("go tool missing: %q %v", data, err) }
	data, err = os.ReadFile(filepath.Join(destination, "src", "runtime", "runtime.go"))
	if err != nil || string(data) != "package runtime\n" { t.Fatalf("source missing: %q %v", data, err) }
}

func TestExtractGoArchiveRejectsTraversalAndSymlinks(t *testing.T) {
	cases := []struct{ name string; entries []archiveEntry }{
		{"dotdot", []archiveEntry{{name:"go/../escape", typeFlag:tar.TypeReg, body:"malicious"}}},
		{"outside", []archiveEntry{{name:"outside/file", typeFlag:tar.TypeReg, body:"malicious"}}},
		{"symlink", []archiveEntry{{name:"go/link", typeFlag:tar.TypeSymlink}}},
		{"device", []archiveEntry{{name:"go/tty", typeFlag:tar.TypeChar}}},
	}
	for _, tc := range cases {
		t.Run(tc.name,func(t *testing.T){
			archive := writeSyntheticGoArchive(t, tc.entries)
			if err := extractVerifiedGoArchive(archive, t.TempDir()); err == nil {
				t.Fatal("unsafe Go archive accepted")
			}
		})
	}
}

func TestGoBootstrapHintReusableWithDifferentDaemonPATH(t *testing.T) {
	if runtime.GOOS == "windows" { t.Skip("POSIX script test") }
	home := t.TempDir()
	t.Setenv("HOME",home)
	root := filepath.Join(home, "unusual", "gosdk")
	binary := filepath.Join(root, "bin", "go")
	if err:=os.MkdirAll(filepath.Dir(binary),0o700);err!=nil{t.Fatal(err)}
	script := "#!/bin/sh\ncase \"$1\" in\n  env) echo "+root+" ;;\n  version) echo 'go version go1.27.3 "+runtime.GOOS+"/"+runtime.GOARCH+"' ;;\n  *) exit 1 ;;\nesac\n"
	if err:=os.WriteFile(binary,[]byte(script),0o700);err!=nil{t.Fatal(err)}
	rememberWorkingGoExecutable(context.Background(),binary)
	got:=knownWorkingGoExecutable()
	if got!=binary {t.Fatalf("recorded compiler=%q, want %q",got,binary)}
	t.Setenv("PATH",filepath.Join(home,"no-compiler-path"))
	t.Setenv("TASKDECK_GO","")
	t.Setenv("GOROOT","")
	// A CI host can have a valid system-wide Go (such as /usr/local/go).
	// That installation is allowed to win before the shared hint. Validate
	// the hint itself, then require the resolver to find a usable compiler.
	if got := firstExecutableGoCandidate([]string{knownWorkingGoExecutable()}); got != binary {
		t.Fatalf("shared working compiler hint=%q, want=%q", got, binary)
	}
	resolved,err:=resolveGoExecutable()
	if err!=nil||!ExecutableExists(resolved) {t.Fatalf("different project resolver=%q err=%v",resolved,err)}
}
