package server

import (
	"os"
	"path/filepath"
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
	for _,want:=range []string{"Terminal: project root","Terminal: Custom…","options?.cwd","payload,cwd:saved()","terminal-cwd:"}{if !strings.Contains(js,want){t.Fatalf("terminal cwd module missing %q",want)}}
}
