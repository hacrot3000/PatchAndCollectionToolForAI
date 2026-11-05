package server

import (
 "errors"
 "os"
 "path/filepath"
 "runtime"
 "strings"

 "bletonfc/vscode_tasks_menu/internal/tasks"
)

// This internal-only plan does NOT start a shell, expose a RemoteAction,
// or grant PHP/Relay terminal permissions. It is the separately guarded
// execution template for a future approved, owner-bound PTY adapter.
// Never reuse the local terminal's untrusted SHELL, shell-integration
// hooks, profile arguments, user-provided terminal command or environment.
const remoteTerminalHostOptIn="TASKDECK_REMOTE_TERMINAL"
const remoteTerminalFixedShell="/bin/sh"

func remoteTerminalPlan(root,userID,projectID string)(tasks.Execution,error){
 denied:=errors.New("remote PTY host execution is not approved")
 if os.Getenv(remoteTerminalHostOptIn)!="1"||
  userID==""||projectID==""||strings.TrimSpace(root)==""||
  runtime.GOOS=="windows"{return tasks.Execution{},denied}
 if !filepath.IsAbs(root){return tasks.Execution{},denied}
 // The workspace root must be an ordinary directory with a canonical
 // identity. No linked workspace or relative cwd supplied via PHP.
 rootInfo,err:=os.Lstat(root)
 if err!=nil||!rootInfo.IsDir()||rootInfo.Mode()&os.ModeSymlink!=0{
  return tasks.Execution{},denied
 }
 clean:=filepath.Clean(root)
 canonical,err:=filepath.EvalSymlinks(clean)
 if err!=nil||canonical!=clean{return tasks.Execution{},denied}
 resolvedShell,err:=filepath.EvalSymlinks(remoteTerminalFixedShell)
 if err!=nil||!filepath.IsAbs(resolvedShell){return tasks.Execution{},denied}
 if strings.ContainsRune(resolvedShell,0)||
  pathWithinRemoteWorkspace(clean,resolvedShell){return tasks.Execution{},denied}
 shellInfo,err:=os.Stat(resolvedShell)
 if err!=nil||!shellInfo.Mode().IsRegular()||
  shellInfo.Mode().Perm()&0o111==0||
  shellInfo.Mode().Perm()&0o022!=0{
  return tasks.Execution{},denied
 }
 return tasks.Execution{
  TaskID:0,
  Label:"Remote Terminal",
  Detail:"Owner-bound host PTY in approved project root",
  Preview:"/bin/sh -i (host-approved Remote Terminal)",
  Command:resolvedShell,
  Args:[]string{"-i"},
  Cwd:clean,
  // Do not inherit Agent secrets, proxy URLs, SSH keys, Git helpers,
  // dynamic loaders, HOME, ENV, BASH_ENV, CDPATH, executable search
  // paths or user-controlled shell startup settings.
  Env:[]string{
   "PATH=/usr/bin:/bin",
   "HOME="+clean,
   "TERM=xterm",
   "LC_ALL=C",
  },
  SessionKind:tasks.SessionKindTerminal,
  OwnerUserID:userID,
  ProjectID:projectID,
  TargetType:"local",
 },nil
}
