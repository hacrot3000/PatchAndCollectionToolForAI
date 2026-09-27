package dbmysql

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestCredentialFilePermissionsEscapingAndCleanup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not authoritative on Windows")
	}
	secret := " pa\\ss\"word\tvalue "
	file, err := createCredentialFile(secret)
	if err != nil {
		t.Fatal(err)
	}
	if file == nil || file.path == "" || file.dir == "" {
		t.Fatalf("credential file=%+v", file)
	}
	dir := file.dir
	path := file.path

	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("directory mode=%o want 700", got)
	}
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fileInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("file mode=%o want 600", got)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		"[client]",
		"password=\"",
		"pa\\\\ss",
		"\\\"word",
		"\\tvalue",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("credential file missing %q: %q", want, text)
		}
	}
	if strings.Contains(file.Arg(), secret) {
		t.Fatalf("credential arg leaked password: %q", file.Arg())
	}

	file.Close()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("credential file still exists after Close: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("credential directory still exists after Close: %v", err)
	}
}

func TestCredentialFileRejectsLineBreaks(t *testing.T) {
	for _, secret := range []string{"a\nb", "a\rb", "a\x00b"} {
		if _, err := createCredentialFile(secret); err == nil {
			t.Fatalf("secret %q unexpectedly accepted", secret)
		}
	}
}

func TestEmptyCredentialDoesNotCreateFile(t *testing.T) {
	file, err := createCredentialFile("")
	if err != nil {
		t.Fatal(err)
	}
	if file != nil {
		t.Fatalf("empty secret created file: %+v", file)
	}
}
