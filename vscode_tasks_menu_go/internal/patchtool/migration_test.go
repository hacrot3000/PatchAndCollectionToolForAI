package patchtool

import (
	"os"
	"path/filepath"
	"testing"
)

func writeRuntimeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fakeBundledMigrationRuntime(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	release := filepath.Join(root, "release")
	exe := filepath.Join(release, "taskdeck")
	if err := os.MkdirAll(release, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("taskdeck"), 0o755); err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(release, "patchtool")
	writeRuntimeFile(t, bundle, "python_patch_entry.py", "entry\n")
	writeRuntimeFile(t, bundle, "run_python_patches.sh", "launcher\n")
	writeRuntimeFile(t, bundle, "_patch_lib/VERSION", "6.20.2\n")
	writeRuntimeFile(t, bundle, "_patch_lib/python_patch_runner.py", "runner\n")
	return exe, bundle
}

func TestPlanLegacyMigrationRequiresExactBundledHashes(t *testing.T) {
	exe, bundle := fakeBundledMigrationRuntime(t)
	workspace := t.TempDir()
	legacy := filepath.Join(workspace, "tools")
	for _, rel := range []string{"python_patch_entry.py", "run_python_patches.sh", "_patch_lib/VERSION", "_patch_lib/python_patch_runner.py"} {
		data, err := os.ReadFile(filepath.Join(bundle, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		writeRuntimeFile(t, legacy, rel, string(data))
	}

	plan, err := PlanLegacyMigration(workspace, exe)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Present || !plan.Safe || len(plan.Verified) != 4 || len(plan.Modified) != 0 || len(plan.Unknown) != 0 {
		t.Fatalf("unexpected plan: %#v", plan)
	}

	writeRuntimeFile(t, legacy, "_patch_lib/python_patch_runner.py", "locally modified\n")
	plan, err = PlanLegacyMigration(workspace, exe)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Safe || len(plan.Modified) != 1 || plan.Modified[0] != "_patch_lib/python_patch_runner.py" {
		t.Fatalf("modified runtime must block cleanup: %#v", plan)
	}
}

func TestPlanLegacyMigrationUnknownLibraryFileBlocksCleanup(t *testing.T) {
	exe, bundle := fakeBundledMigrationRuntime(t)
	workspace := t.TempDir()
	legacy := filepath.Join(workspace, "tools")
	data, err := os.ReadFile(filepath.Join(bundle, "_patch_lib", "VERSION"))
	if err != nil {
		t.Fatal(err)
	}
	writeRuntimeFile(t, legacy, "_patch_lib/VERSION", string(data))
	writeRuntimeFile(t, legacy, "_patch_lib/local_extension.py", "custom\n")

	plan, err := PlanLegacyMigration(workspace, exe)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Safe || len(plan.Unknown) != 1 || plan.Unknown[0] != "_patch_lib/local_extension.py" {
		t.Fatalf("unknown runtime file must block cleanup: %#v", plan)
	}
}

func TestPlanLegacyMigrationIgnoresProjectFilesOutsideRuntimeTree(t *testing.T) {
	exe, bundle := fakeBundledMigrationRuntime(t)
	workspace := t.TempDir()
	legacy := filepath.Join(workspace, "tools")
	data, err := os.ReadFile(filepath.Join(bundle, "run_python_patches.sh"))
	if err != nil {
		t.Fatal(err)
	}
	writeRuntimeFile(t, legacy, "run_python_patches.sh", string(data))
	writeRuntimeFile(t, legacy, "project_helper.sh", "keep me\n")

	plan, err := PlanLegacyMigration(workspace, exe)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Safe {
		t.Fatalf("unrelated tools/ file must not block scoped runtime cleanup: %#v", plan)
	}
}
