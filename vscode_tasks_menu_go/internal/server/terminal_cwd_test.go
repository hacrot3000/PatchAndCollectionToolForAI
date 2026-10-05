package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestWorkspaceTerminalExecutionAtSafeSubdir(t *testing.T) {
	root := t.TempDir();sub := filepath.Join(root,"tools");if err:=os.Mkdir(sub,0o755);err!=nil{t.Fatal(err)}
	spec,err:=workspaceTerminalExecutionAt(root,"tools");if err!=nil{t.Fatal(err)}
	if spec.Cwd!=sub{t.Fatalf("cwd=%q want %q",spec.Cwd,sub)}
}
func TestWorkspaceTerminalExecutionAtRejectsEscape(t *testing.T){
	root:=t.TempDir();if _,err:=workspaceTerminalExecutionAt(root,"../");err==nil{t.Fatal("expected traversal rejection")}
	outside:=t.TempDir();if err:=os.Symlink(outside,filepath.Join(root,"link"));err!=nil{t.Fatal(err)}
	if _,err:=workspaceTerminalExecutionAt(root,"link");err==nil{t.Fatal("expected symlink escape rejection")}
}
func TestTerminalCwdFeatureModule(t *testing.T){
	data,err:=webassets.Files.ReadFile("featuremods/terminalcwd.js");if err!=nil{t.Fatal(err)};js:=string(data)
	for _,want:=range []string{
		"Terminal: project root",
		"Terminal: Add directory…",
		"/api/config/terminal-cwds",
		"TaskMenuDirectoryBrowser?.choose",
		"Choose terminal directory",
		"Terminal directory relative to the workspace:",
		"payload,cwd:selectedCWD()",
		"localStorage.removeItem(key)",
	}{
		if !strings.Contains(js,want){t.Fatalf("terminal cwd module missing %q",want)}
	}
	for _,forbidden:=range []string{
		"window.prompt('Relative workspace directory for new terminals:'",
		"localStorage.setItem(",
		"options?.cwd",
	}{
		if strings.Contains(js,forbidden){t.Fatalf("terminal cwd module still contains legacy behavior %q",forbidden)}
	}
}

func TestWorkspaceTerminalExecutionAtAttachedRoot(t *testing.T) {
	base := t.TempDir()
	primary := filepath.Join(base, "main")
	attached := filepath.Join(base, "client")
	sub := filepath.Join(attached, "tools")
	if err := os.MkdirAll(primary, 0o755); err != nil { t.Fatal(err) }
	if err := os.MkdirAll(sub, 0o755); err != nil { t.Fatal(err) }
	s := &Server{Workspace: primary}
	create := callWorkspaceRoots(t, s, http.MethodPost, "/api/workspace-roots", `{"path":"../client","name":"Client"}`)
	if create.Code != http.StatusCreated { t.Fatalf("attach status=%d body=%s", create.Code, create.Body.String()) }
	var root workspaceRootView
	if err := json.Unmarshal(create.Body.Bytes(), &root); err != nil { t.Fatal(err) }

	virtual := workspaceVirtualPath(root.ID, "tools")
	spec, err := s.workspaceTerminalExecutionAtProjectPath(virtual)
	if err != nil { t.Fatal(err) }
	if spec.Cwd != sub { t.Fatalf("cwd=%q want=%q", spec.Cwd, sub) }
	if !strings.Contains(spec.Detail, virtual) { t.Fatalf("detail=%q want virtual path %q", spec.Detail, virtual) }
}

func TestWorkspaceTerminalExecutionAtAttachedRootRejectsEscape(t *testing.T) {
	if runtime.GOOS == "windows" { t.Skip("symlink test") }
	base := t.TempDir()
	primary := filepath.Join(base, "main")
	attached := filepath.Join(base, "client")
	outside := filepath.Join(base, "outside")
	for _, dir := range []string{primary, attached, outside} {
		if err := os.MkdirAll(dir, 0o755); err != nil { t.Fatal(err) }
	}
	if err := os.Symlink(outside, filepath.Join(attached, "escape")); err != nil { t.Skipf("symlink unavailable: %v", err) }
	s := &Server{Workspace: primary}
	create := callWorkspaceRoots(t, s, http.MethodPost, "/api/workspace-roots", `{"path":"../client"}`)
	if create.Code != http.StatusCreated { t.Fatal(create.Body.String()) }
	var root workspaceRootView
	if err := json.Unmarshal(create.Body.Bytes(), &root); err != nil { t.Fatal(err) }
	if _, err := s.workspaceTerminalExecutionAtProjectPath(workspaceVirtualPath(root.ID, "escape")); err == nil {
		t.Fatal("attached-root terminal followed symlink outside root")
	}
}
