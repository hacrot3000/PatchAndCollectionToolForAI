package secretstore

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestFileStoreRoundTripEncryptsAtRest(t *testing.T) {
	store, err := NewFileStore(filepath.Join(t.TempDir(), "taskdeck"))
	if err != nil {
		t.Fatal(err)
	}
	const id = "ssh/prod/password"
	secret := []byte("correct horse battery staple")
	if err := store.Put(id, secret); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(store.dataPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), string(secret)) {
		t.Fatalf("plaintext secret leaked at rest: %s", data)
	}

	got, err := store.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(secret) {
		t.Fatalf("secret = %q", got)
	}

	if err := store.Delete(id); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after Delete error = %v, want ErrNotFound", err)
	}
}

func TestFileStoreUsesRestrictedPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not authoritative on Windows")
	}
	dir := filepath.Join(t.TempDir(), "taskdeck")
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put("ssh/prod/password", []byte("secret")); err != nil {
		t.Fatal(err)
	}

	for path, want := range map[string]os.FileMode{
		dir:            0o700,
		store.keyPath:  0o600,
		store.dataPath: 0o600,
		store.lockPath: 0o600,
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Fatalf("%s mode = %o, want %o", path, got, want)
		}
	}
}

func TestCiphertextIsBoundToSecretID(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "taskdeck")
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put("ssh/a/password", []byte("alpha")); err != nil {
		t.Fatal(err)
	}
	if err := store.Put("ssh/b/password", []byte("beta")); err != nil {
		t.Fatal(err)
	}

	data, err := store.loadFileForTest()
	if err != nil {
		t.Fatal(err)
	}
	data.Records["ssh/b/password"] = data.Records["ssh/a/password"]
	if err := store.withExclusiveLock(func() error { return store.writeFileLocked(data) }); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Get("ssh/b/password"); err == nil || !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("tampered secret error = %v", err)
	}
}

func TestFileStoreRejectsInvalidMasterKey(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "taskdeck")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "master.key"), []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put("ssh/prod/password", []byte("secret")); err == nil || !strings.Contains(err.Error(), "invalid length") {
		t.Fatalf("error = %v", err)
	}
}

func TestNewFileStoreRequiresAbsoluteDirectory(t *testing.T) {
	if _, err := NewFileStore("relative/taskdeck"); err == nil {
		t.Fatal("expected absolute path validation error")
	}
}

func (s *FileStore) loadFileForTest() (secretFile, error) {
	var out secretFile
	err := s.withExclusiveLock(func() error {
		var err error
		out, err = s.loadFileLocked()
		return err
	})
	return out, err
}
