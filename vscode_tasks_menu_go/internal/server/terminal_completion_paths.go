package server

import (
 "io"
 "net/http"
 "os"
 "path/filepath"
 "strings"
 "time"

 "bletonfc/vscode_tasks_menu/internal/tasks"
)

// sessionCompletionPaths is a bounded, read-only directory lookup for an
// authorized LOCAL interactive terminal. It never invokes a shell or Git,
// follows no directory entries, and never enumerates a directory tree.
func (s *Server) sessionCompletionPaths(w http.ResponseWriter, r *http.Request, id string) {
 if r.Method != http.MethodGet {
  http.Error(w, "method not allowed", http.StatusMethodNotAllowed); return
 }
 meta, ok := s.Sessions.Metadata(id)
 if !ok { http.NotFound(w, r); return }
 if meta.Kind != tasks.SessionKindTerminal || meta.TaskID != 0 ||
  strings.EqualFold(strings.TrimSpace(meta.TargetType), "ssh") ||
  strings.TrimSpace(meta.TargetProfileID) != "" {
  http.Error(w, "local interactive terminal required", http.StatusForbidden); return
 }
 dir := r.URL.Query().Get("dir")
 prefix := r.URL.Query().Get("prefix")
 if len(dir)>512 || len(prefix)>160 || strings.ContainsAny(dir+prefix, "\r\n\x00") ||
  strings.ContainsAny(dir+prefix, "*?[\\") || strings.Contains(prefix, "/") {
  http.Error(w, "invalid completion path", http.StatusBadRequest); return
 }
 cwd, err := s.Sessions.CurrentCwd(id)
 if err != nil || strings.TrimSpace(cwd)=="" { cwd=meta.Cwd }
 if !filepath.IsAbs(cwd) { http.Error(w, "live terminal cwd unavailable", http.StatusConflict); return }
 root := filepath.Clean(cwd)
 folder := filepath.Clean(filepath.Join(root, filepath.FromSlash(dir)))
 if filepath.IsAbs(dir) { folder=filepath.Clean(dir) }
 // In shared-server mode, filesystem lookup must not escape the workspace,
 // including via a symlink in the completed directory.
 if s.Config.SharedServerEnabled {
  workspace, errorWorkspace := filepath.EvalSymlinks(s.Workspace)
  resolved, errorFolder := filepath.EvalSymlinks(folder)
  if errorWorkspace!=nil || errorFolder!=nil { http.Error(w,"directory unavailable",http.StatusForbidden); return }
  relative, relativeError := filepath.Rel(workspace,resolved)
  if relativeError!=nil || relative==".." || strings.HasPrefix(relative,".."+string(filepath.Separator)) {
   http.Error(w,"completion outside workspace unavailable in shared mode",http.StatusForbidden); return
  }
  folder=resolved
 }
 handle, err := os.Open(folder)
 if err != nil { writeJSON(w,http.StatusOK,map[string]any{"entries":[]any{}}); return }
 defer handle.Close()
 info,err:=handle.Stat()
 if err!=nil || !info.IsDir() { writeJSON(w,http.StatusOK,map[string]any{"entries":[]any{}}); return }
 type suggestion struct {
  Name string `json:"name"`
  Dir bool `json:"dir"`
 }
 entries:=make([]suggestion,0,48)
 scanned:=0
 // ReadDir(n) only consumes a bounded batch, unlike os.ReadDir(dir) which
 // materializes an entire directory containing possibly millions of files.
 for scanned<2048 && len(entries)<100 {
  next,readErr:=handle.ReadDir(128)
  for _,item:=range next {
   scanned++
   if scanned>2048 || len(entries)>=100 { break }
   name:=item.Name()
   if name=="" || (!strings.HasPrefix(name,prefix)) || (strings.HasPrefix(name,".") && !strings.HasPrefix(prefix,".")) ||
    strings.ContainsAny(name,"\r\n\x00\t") { continue }
   entries=append(entries,suggestion{Name:name,Dir:item.IsDir()})
  }
  if readErr==io.EOF || (readErr!=nil && len(next)==0) || len(next)==0 { break }
 }
 w.Header().Set("Cache-Control","no-store")
 // Keep an upper bound on handler work even when a filesystem is slow.
 _=time.Second
 writeJSON(w,http.StatusOK,map[string]any{"entries":entries,"cwd":root})
}
