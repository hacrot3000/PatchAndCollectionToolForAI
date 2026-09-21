package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

func lineCount(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return 0
	}
	return len(strings.Split(text, "\n"))
}

func TestRootLauncherSelfInstallsMissingSourceOnce(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("root launcher is bash")
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Clean(filepath.Join(wd, "..", "..", ".."))
	launcherData, err := os.ReadFile(filepath.Join(root, "vscode_tasks_menu"))
	if err != nil {
		t.Fatal(err)
	}

	installDir := t.TempDir()
	launcher := filepath.Join(installDir, "vscode_tasks_menu")
	if err := os.WriteFile(launcher, launcherData, 0o755); err != nil {
		t.Fatal(err)
	}

	fakeBin := filepath.Join(installDir, "fake-bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	curlLog := filepath.Join(installDir, "curl.log")
	goLog := filepath.Join(installDir, "go.log")
	execLog := filepath.Join(installDir, "exec.log")
	const revision = "0123456789abcdef0123456789abcdef01234567"

	writeExecutable(t, filepath.Join(fakeBin, "curl"), `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "$FAKE_CURL_LOG"
out=""
want_out=0
for arg in "$@"; do
  if [[ "$want_out" == "1" ]]; then out="$arg"; want_out=0; continue; fi
  if [[ "$arg" == "-o" ]]; then want_out=1; fi
done
if [[ -n "$out" ]]; then
  printf 'fake github archive\n' > "$out"
else
  printf '{"sha":"0123456789abcdef0123456789abcdef01234567"}\n'
fi
`)

	writeExecutable(t, filepath.Join(fakeBin, "tar"), `#!/usr/bin/env bash
set -euo pipefail
target=""
want_target=0
for arg in "$@"; do
  if [[ "$want_target" == "1" ]]; then target="$arg"; want_target=0; continue; fi
  if [[ "$arg" == "-C" ]]; then want_target=1; fi
done
[[ -n "$target" ]]
src="$target/PatchAndCollectionToolForAI-0123456/vscode_tasks_menu_go"
mkdir -p "$src/cmd/vscode_tasks_menu" "$src/internal/selfupdate" "$src/web"
printf 'module bletonfc/vscode_tasks_menu\n\ngo 1.19\n' > "$src/go.mod"
printf 'package main\n' > "$src/cmd/vscode_tasks_menu/main.go"
printf 'package selfupdate\n' > "$src/internal/selfupdate/selfupdate.go"
printf 'package web\n' > "$src/web/assets.go"
`)

	writeExecutable(t, filepath.Join(fakeBin, "go"), `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "$FAKE_GO_LOG"
out=""
want_out=0
for arg in "$@"; do
  if [[ "$want_out" == "1" ]]; then out="$arg"; want_out=0; continue; fi
  if [[ "$arg" == "-o" ]]; then want_out=1; fi
done
[[ -n "$out" ]]
cat > "$out" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$SELF_INSTALL_EXEC_LOG"
exit 0
EOF
chmod +x "$out"
`)

	run := func() string {
		t.Helper()
		cmd := exec.Command("bash", launcher, "--version")
		cmd.Dir = installDir
		cmd.Env = append(os.Environ(),
			"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
			"FAKE_CURL_LOG="+curlLog,
			"FAKE_GO_LOG="+goLog,
			"SELF_INSTALL_EXEC_LOG="+execLog,
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("launcher failed: %v\n%s", err, out)
		}
		return string(out)
	}

	firstOutput := run()
	if !strings.Contains(firstOutput, "Self-install: thiếu source vscode_tasks_menu_go") ||
		!strings.Contains(firstOutput, "source revision 0123456789ab đã sẵn sàng") {
		t.Fatalf("missing self-install progress output:\n%s", firstOutput)
	}

	for _, path := range []string{
		"vscode_tasks_menu_go/go.mod",
		"vscode_tasks_menu_go/cmd/vscode_tasks_menu/main.go",
		"vscode_tasks_menu_go/internal/selfupdate/selfupdate.go",
		"vscode_tasks_menu_go/web/assets.go",
		"vscode_tasks_menu_go/.build/vscode_tasks_menu",
	} {
		if _, err := os.Stat(filepath.Join(installDir, path)); err != nil {
			t.Fatalf("self-install missing %s: %v", path, err)
		}
	}

	for _, path := range []string{
		"vscode_tasks_menu_go/.build/source.revision",
		"vscode_tasks_menu_go/.build/vscode_tasks_menu.revision",
	} {
		data, err := os.ReadFile(filepath.Join(installDir, path))
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(string(data)) != revision {
			t.Fatalf("%s revision=%q want %q", path, strings.TrimSpace(string(data)), revision)
		}
	}

	goArgs, err := os.ReadFile(goLog)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(goArgs), "-ldflags -X main.buildRevision="+revision) {
		t.Fatalf("bootstrap build did not embed revision:\n%s", goArgs)
	}
	execArgs, err := os.ReadFile(execLog)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(execArgs), "--workspace "+installDir+" --version") {
		t.Fatalf("built binary did not receive launcher arguments:\n%s", execArgs)
	}
	if got := lineCount(t, curlLog); got != 2 {
		t.Fatalf("first launch curl calls=%d want 2", got)
	}
	if got := lineCount(t, goLog); got != 1 {
		t.Fatalf("first launch go calls=%d want 1", got)
	}

	secondOutput := run()
	if strings.Contains(secondOutput, "Self-install:") {
		t.Fatalf("second launch unexpectedly bootstrapped again:\n%s", secondOutput)
	}
	if got := lineCount(t, curlLog); got != 2 {
		t.Fatalf("second launch performed network calls; total=%d", got)
	}
	if got := lineCount(t, goLog); got != 1 {
		t.Fatalf("second launch rebuilt unchanged source; total=%d", got)
	}
	if got := lineCount(t, execLog); got != 2 {
		t.Fatalf("binary executions=%d want 2", got)
	}

	// Partial repair must fill missing files without pretending mixed/local source
	// is exactly the downloaded GitHub revision.
	localGoMod := []byte("module local/custom\n\ngo 1.19\n")
	if err := os.WriteFile(filepath.Join(installDir, "vscode_tasks_menu_go", "go.mod"), localGoMod, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(installDir, "vscode_tasks_menu_go", "web", "assets.go")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(installDir, "vscode_tasks_menu_go", ".build", "vscode_tasks_menu")); err != nil {
		t.Fatal(err)
	}

	thirdOutput := run()
	if !strings.Contains(thirdOutput, "giữ nguyên source hiện có (revision=dev)") {
		t.Fatalf("partial repair did not report mixed-source revision semantics:\n%s", thirdOutput)
	}
	gotGoMod, err := os.ReadFile(filepath.Join(installDir, "vscode_tasks_menu_go", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if string(gotGoMod) != string(localGoMod) {
		t.Fatalf("partial repair overwrote local go.mod:\n%s", gotGoMod)
	}
	if _, err := os.Stat(filepath.Join(installDir, "vscode_tasks_menu_go", "web", "assets.go")); err != nil {
		t.Fatalf("partial repair did not restore missing web/assets.go: %v", err)
	}
	for _, path := range []string{
		"vscode_tasks_menu_go/.build/source.revision",
		"vscode_tasks_menu_go/.build/vscode_tasks_menu.revision",
	} {
		if _, err := os.Stat(filepath.Join(installDir, path)); !os.IsNotExist(err) {
			t.Fatalf("partial repair must not keep trusted revision marker %s: %v", path, err)
		}
	}
	if got := lineCount(t, curlLog); got != 4 {
		t.Fatalf("partial repair curl calls total=%d want 4", got)
	}
	if got := lineCount(t, goLog); got != 2 {
		t.Fatalf("partial repair go calls total=%d want 2", got)
	}
	goArgsAfterRepair, err := os.ReadFile(goLog)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(goArgsAfterRepair)), "\n")
	if strings.Contains(lines[len(lines)-1], "-ldflags") {
		t.Fatalf("partial repair build must not embed remote revision: %s", lines[len(lines)-1])
	}
}

func TestRootLauncherSelfInstallContract(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Clean(filepath.Join(wd, "..", "..", ".."))
	data, err := os.ReadFile(filepath.Join(root, "vscode_tasks_menu"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	for _, want := range []string{
		"REPOSITORY=\"hacrot3000/PatchAndCollectionToolForAI\"",
		"BRANCH=\"main\"",
		"bootstrap_source",
		"https://api.github.com/repos/$REPOSITORY/commits/$BRANCH",
		"https://codeload.github.com/$REPOSITORY/tar.gz",
		"cp -a -n \"$staged_source/.\" \"$SRC_DIR/\"",
		"-ldflags \"-X main.buildRevision=$revision\"",
		"rm -f \"$BIN.revision\"",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("root launcher self-install contract missing %q", want)
		}
	}
}
