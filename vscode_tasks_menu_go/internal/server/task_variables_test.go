package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTaskEditorVariablesUseHostValidatedFile(t *testing.T) {
	workspace := t.TempDir()
	src := filepath.Join(workspace, "src")
	if err := os.MkdirAll(src, 0755); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(src, "hello.py"), []byte("print(1)\n"), 0644); err != nil { t.Fatal(err) }
	s := &Server{Workspace: workspace}
	root, err := s.primaryWorkspaceRoot()
	if err != nil { t.Fatal(err) }
	ctx, err := s.resolveTaskEditorVariables(root, &taskEditorVariableRequest{
		Path: "src/hello.py", LineNumber: 3, ColumnNumber: 9, SelectedText: "print",
	})
	if err != nil { t.Fatal(err) }
	if ctx.File != filepath.Join(src, "hello.py") || ctx.LineNumber != 3 ||
		ctx.ColumnNumber != 9 || ctx.SelectedText != "print" {
		t.Fatalf("wrong editor context: %#v", ctx)
	}
	if ctx.WorkspaceFolders[root.Name] != root.Path {
		t.Fatalf("named workspaces missing: %#v", ctx.WorkspaceFolders)
	}
}

func TestTaskEditorVariablesRejectPathEscape(t *testing.T) {
	workspace := t.TempDir()
	s := &Server{Workspace: workspace}
	root, err := s.primaryWorkspaceRoot()
	if err != nil { t.Fatal(err) }
	for _, path := range []string{"/tmp/absolute.py", "../outside.py", "missing.py", "@root/nonexistent/anything.py"} {
		_, err := s.resolveTaskEditorVariables(root, &taskEditorVariableRequest{Path:path})
		if err == nil { t.Fatalf("accepted invalid editor path %q",path) }
	}
	external := t.TempDir()
	outside := filepath.Join(external, "hidden.py")
	if err := os.WriteFile(outside, []byte("secret"), 0600); err != nil { t.Fatal(err) }
	if err := os.Symlink(outside, filepath.Join(workspace, "linked.py")); err != nil { t.Fatal(err) }
	_, err = s.resolveTaskEditorVariables(root, &taskEditorVariableRequest{Path:"linked.py"})
	if err == nil || !strings.Contains(err.Error(),"unavailable") {
		t.Fatalf("accepted symlink escape: %v",err)
	}
}
