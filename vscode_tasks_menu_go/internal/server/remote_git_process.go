package server

import (
 "os"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
 "bletonfc/vscode_tasks_menu/internal/tasks"
)

// remoteTrustedGitExecutable chooses a predictable OS-managed executable,
// never an executable from the daemon's PATH or the editable workspace.
//
// On macOS /usr/bin/git may be Apple's toolchain shim. The Homebrew paths
// are fallback only if the OS-managed Git is unavailable. The binary must
// be executable, outside this workspace even after resolving symlinks,
// and not group/world-writable. A missing safe Git fails closed.
func remoteTrustedGitExecutable(workspace string)(string,error){
 for _,candidate:=range []string{
  "/usr/bin/git",
  "/bin/git",
  "/opt/homebrew/bin/git",
  "/usr/local/bin/git",
 }{
  if tasks.RemoteExecutableAllowed(workspace,candidate){
   return candidate,nil
  }
 }
 return "",remotetransport.ErrRemoteActionDenied
}

// Never forward daemon credentials, HOME, GIT_CONFIG_COUNT,
// GIT_EXEC_PATH, GIT_TRACE, LD_PRELOAD or DYLD_* to a Git subprocess.
// They can turn an otherwise read-only Git command into arbitrary code
// execution or secret disclosure. No default os.Environ fallback.
func isolatedRemoteGitEnvironment() []string{
 return []string{
  "PATH=/usr/bin:/bin",
  "HOME=/nonexistent",
  "LANG=C",
  "LC_ALL=C",
  "GIT_CONFIG_NOSYSTEM=1",
  "GIT_CONFIG_GLOBAL="+os.DevNull,
  "GIT_OPTIONAL_LOCKS=0",
  "GIT_TERMINAL_PROMPT=0",
 }
}
