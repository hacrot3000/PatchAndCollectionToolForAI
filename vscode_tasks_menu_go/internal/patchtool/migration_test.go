package patchtool

import (
	"os"
	"path/filepath"
	"strings"
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


func TestApplyLegacyMigrationKeepsLauncherShimAndProjectFiles(t *testing.T) {
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
	writeRuntimeFile(t, legacy, "project_helper.sh", "keep me\n")

	plan, err := ApplyLegacyMigration(workspace, exe)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Safe {
		t.Fatalf("plan unexpectedly unsafe: %#v", plan)
	}
	shim, err := os.ReadFile(filepath.Join(legacy, "run_python_patches.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(shim), "taskdeck patch") {
		t.Fatalf("launcher was not replaced by TaskDeck compatibility shim: %q", shim)
	}
	for _, rel := range []string{"python_patch_entry.py", "_patch_lib/VERSION", "_patch_lib/python_patch_runner.py"} {
		if _, err := os.Stat(filepath.Join(legacy, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Fatalf("verified runtime payload still exists %s: %v", rel, err)
		}
	}
	if data, err := os.ReadFile(filepath.Join(legacy, "project_helper.sh")); err != nil || string(data) != "keep me\n" {
		t.Fatalf("project helper changed data=%q err=%v", data, err)
	}
	afterPlan, err := PlanLegacyMigration(workspace, exe)
	if err != nil {
		t.Fatal(err)
	}
	if afterPlan.Present {
		t.Fatalf("TaskDeck compatibility shim must count as already migrated, got %#v", afterPlan)
	}
}

func TestApplyLegacyMigrationRefusesModifiedRuntimeWithoutWritingShim(t *testing.T) {
	exe, bundle := fakeBundledMigrationRuntime(t)
	workspace := t.TempDir()
	legacy := filepath.Join(workspace, "tools")
	for _, rel := range []string{"run_python_patches.sh", "_patch_lib/VERSION"} {
		data, err := os.ReadFile(filepath.Join(bundle, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		writeRuntimeFile(t, legacy, rel, string(data))
	}
	writeRuntimeFile(t, legacy, "_patch_lib/VERSION", "custom-version\n")
	before, err := os.ReadFile(filepath.Join(legacy, "run_python_patches.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyLegacyMigration(workspace, exe); err == nil {
		t.Fatal("modified runtime migration should fail closed")
	}
	after, err := os.ReadFile(filepath.Join(legacy, "run_python_patches.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("launcher changed even though migration was unsafe")
	}
}

func TestCompatibilityLaunchersForwardToTaskdeckPatch(t *testing.T) {
	for name, spec := range compatibilityLaunchers {
		if !strings.Contains(spec.body, "taskdeck") || !strings.Contains(spec.body, "patch") {
			t.Fatalf("compatibility launcher %s does not forward to taskdeck patch", name)
		}
	}
}
