package server

import (
 "io"
 "context"
 "os/exec"
 "time"
 "net/http"
 "os"
 "path/filepath"
 "strings"

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
  (strings.TrimSpace(meta.TargetType)!="" && !strings.EqualFold(strings.TrimSpace(meta.TargetType), "local")) ||
  strings.TrimSpace(meta.TargetProfileID) != "" {
  http.Error(w, "local interactive terminal required", http.StatusForbidden); return
 }
 dir := r.URL.Query().Get("dir")
 prefix := r.URL.Query().Get("prefix")
 kind := r.URL.Query().Get("kind")
 if kind!="" && kind!="paths" && kind!="branches" { http.Error(w,"invalid completion kind",http.StatusBadRequest); return }
 if len(dir)>512 || len(prefix)>160 || strings.ContainsAny(dir+prefix, "\r\n\x00") ||
  strings.ContainsAny(dir+prefix, "*?[\\") || (kind!="branches" && strings.Contains(prefix, "/")) {
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
 if kind=="branches" {
  // Git for-each-ref is read-only, never touches index.lock, and is only
  // launched on demand for the authorized live terminal CWD. No shell.
  ctx,cancel:=context.WithTimeout(r.Context(),1500*time.Millisecond)
  defer cancel()
  cmd:=exec.CommandContext(ctx,"git","-C",folder,"for-each-ref","--count=300","--format=%(refname:short)","refs/heads")
  cmd.Env=append(os.Environ(),"GIT_OPTIONAL_LOCKS=0")
  raw,err:=cmd.Output()
  branches:=make([]string,0,64)
  if err==nil && len(raw)<96000 {
   for _,branch:=range strings.Split(string(raw),"\n"){
    branch=strings.TrimSpace(branch)
    if branch=="" || len(branch)>200 || strings.ContainsAny(branch,"\r\n\x00\t") ||
     !strings.HasPrefix(strings.ToLower(branch),strings.ToLower(prefix)) {continue}
    branches=append(branches,branch)
    if len(branches)>=120 {break}
   }
  }
  w.Header().Set("Cache-Control","no-store")
  writeJSON(w,http.StatusOK,map[string]any{"branches":branches})
  return
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
   if name=="" || (!strings.HasPrefix(strings.ToLower(name),strings.ToLower(prefix))) || (strings.HasPrefix(name,".") && !strings.HasPrefix(prefix,".")) ||
    strings.ContainsAny(name,"\r\n\x00\t") { continue }
   entries=append(entries,suggestion{Name:name,Dir:item.IsDir()})
  }
  if readErr==io.EOF || (readErr!=nil && len(next)==0) || len(next)==0 { break }
 }
 w.Header().Set("Cache-Control","no-store")
 writeJSON(w,http.StatusOK,map[string]any{"entries":entries,"cwd":root})
}
