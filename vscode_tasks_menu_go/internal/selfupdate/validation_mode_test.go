package selfupdate

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeValidationFixture(t *testing.T, failingGoTest, failingPythonTest bool) (string, string) {
	t.Helper()
	root := t.TempDir()
	goSource := filepath.Join(root, "vscode_tasks_menu_go")
	if err := os.MkdirAll(goSource, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(goSource, "go.mod"), []byte("module example.com/taskdeck-selfupdate-test\ngo 1.19\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(goSource, "sample.go"), []byte("package sample\nfunc Value() int { return 1 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if failingGoTest {
		testSource := "package sample\nimport \"testing\"\nfunc TestIntentionalFailure(t *testing.T){ t.Fatal(\"intentional full validation failure\") }\n"
		if err := os.WriteFile(filepath.Join(goSource, "sample_test.go"), []byte(testSource), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "python_patch_entry.py"), []byte("def main():\n    return 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pythonTest := "print('patch tests pass')\n"
	if failingPythonTest {
		pythonTest = "raise SystemExit('intentional patch validation failure')\n"
	}
	if err := os.WriteFile(filepath.Join(root, "test_python_patch_entry.py"), []byte(pythonTest), 0o644); err != nil {
		t.Fatal(err)
	}
	return goSource, root
}

func requireSelfUpdatePython(t *testing.T) {
	t.Helper()
	if _, err := pythonForSelfUpdate(context.Background()); err != nil {
		t.Skipf("self-update Python unavailable: %v", err)
	}
}


func TestPythonForSelfUpdatePrefersVersionedRuntimeOverLegacyPython3(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}
	bin := t.TempDir()
	python310 := filepath.Join(bin, "python3.10")
	legacyPython3 := filepath.Join(bin, "python3")
	if err := os.WriteFile(python310, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyPython3, []byte("#!/bin/sh\nexit 3\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("TASKDECK_PYTHON", "")

	found, err := pythonForSelfUpdate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if found != python310 {
		t.Fatalf("python=%q want %q", found, python310)
	}
}

func TestValidateCandidateSourcesSkipsFullTestsByDefault(t *testing.T) {
	requireSelfUpdatePython(t)
	goSource, root := writeValidationFixture(t, true, true)
	if err := validateCandidateSources(context.Background(), goSource, root, false); err != nil {
		t.Fatalf("light validation unexpectedly ran full tests: %v", err)
	}
}

func TestValidateCandidateSourcesRunsGoTestsWhenEnabled(t *testing.T) {
	requireSelfUpdatePython(t)
	goSource, root := writeValidationFixture(t, true, false)
	err := validateCandidateSources(context.Background(), goSource, root, true)
	if err == nil || !strings.Contains(err.Error(), "go test failed") {
		t.Fatalf("full validation error=%v, want go test failure", err)
	}
}

func TestValidateCandidateSourcesRunsPatchTestsWhenEnabled(t *testing.T) {
	requireSelfUpdatePython(t)
	goSource, root := writeValidationFixture(t, false, true)
	err := validateCandidateSources(context.Background(), goSource, root, true)
	if err == nil || !strings.Contains(err.Error(), "Patch entry validation failed") {
		t.Fatalf("full validation error=%v, want Patch entry validation failure", err)
	}
}
