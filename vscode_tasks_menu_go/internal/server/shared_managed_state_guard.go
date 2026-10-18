package server

import (
 "path"
 "path/filepath"
 "strings"
)

// TaskDeck's server-owned files are not ordinary project source. In shared
// mode, treating them as project files would expose passwords, credentials,
// private queue state, history and metadata using the daemon's OS identity.
// Detect each basename, including legacy workspace-root copies and files
// below .vscode. Slash normalization also covers Windows project paths.
func isTaskDeckManagedProjectPath(value string) bool {
 clean:=strings.ReplaceAll(strings.TrimSpace(value),"\\","/")
 clean=path.Clean(clean)
 if clean=="."||clean=="/" {return false}
 for _,part:=range strings.Split(clean,"/"){
  if strings.HasPrefix(strings.ToLower(part),"vscode_tasks_menu.") {
   return true
  }
 }
 return false
}

// Resolved symlinks can disguise a managed file behind an innocent project
// filename. Reject the canonical target as well as the user supplied path.
func sharedManagedResolvedPath(root, resolved string) bool {
 relative,err:=filepath.Rel(root,resolved)
 return err==nil&&isTaskDeckManagedProjectPath(filepath.ToSlash(relative))
}
