package server

import (
 "context"
 "io"
 "net/http"
 "os"
 "os/exec"
 "path/filepath"
 "sort"
 "strings"
 "time"

 "bletonfc/vscode_tasks_menu/internal/tasks"
)

// sessionCompletionPaths is a bounded, read-only directory lookup for an
// authorized LOCAL interactive terminal. It never invokes a shell, never
// writes to Git, and never recursively enumerates a directory tree.
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
 if kind!="" && kind!="paths" && kind!="branches" && kind!="commands" { http.Error(w,"invalid completion kind",http.StatusBadRequest); return }
 if (kind=="branches" || kind=="commands") && dir!="" { http.Error(w,"branch lookup must use the active cwd",http.StatusBadRequest); return }
 if len(dir)>512 || len(prefix)>160 || strings.ContainsAny(dir+prefix, "\r\n\x00") ||
  strings.ContainsAny(dir+prefix, "*?[\\") || (kind!="branches" && strings.Contains(prefix, "/")) {
  http.Error(w, "invalid completion path", http.StatusBadRequest); return
 }
 if kind=="commands" {
  w.Header().Set("Cache-Control","no-store")
  writeJSON(w,http.StatusOK,map[string]any{"commands":terminalExecutableNames(os.Getenv("PATH"),prefix)})
  return
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

// terminalExecutableNames returns executable basenames from the TaskDeck host's
// actual PATH directories, without a shell command, network call or hardcoded
// dictionary. The interactive shell's separate compgen inventory still covers
// aliases/functions and shell-specific PATH overrides.
func terminalExecutableNames(pathEnv, prefix string) []string {
 if len(prefix)<2 || len(prefix)>160 || strings.ContainsAny(prefix,"/\\\r\n\x00") { return []string{} }
 roots:=filepath.SplitList(pathEnv)
 seen:=make(map[string]struct{},128)
 results:=make([]string,0,64)
 totalScanned:=0
 for index,rawDir:=range roots {
  if index>=80 || totalScanned>=24000 || len(results)>=160 {break}
  if !filepath.IsAbs(rawDir) || len(rawDir)>4096 {continue}
  folder,err:=os.Open(rawDir)
  if err!=nil {continue}
  scanned:=0
  for scanned<5000 && totalScanned<24000 && len(results)<160 {
   rows,readErr:=folder.ReadDir(256)
   for _,entry:=range rows {
    scanned++;totalScanned++
    if scanned>5000 || totalScanned>24000 {break}
    name:=entry.Name()
    if name=="" || len(name)>128 || !strings.HasPrefix(strings.ToLower(name),strings.ToLower(prefix)) ||
     strings.ContainsAny(name,"\r\n\x00\t") {continue}
    if _,exists:=seen[name];exists {continue}
    // Stat follows executable symlinks such as /usr/bin/editor; never
    // return directory names or a plain non-executable text file.
    info,statErr:=os.Stat(filepath.Join(rawDir,name))
    if statErr!=nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111==0 {continue}
    seen[name]=struct{}{}
    results=append(results,name)
    if len(results)>=160 {break}
   }
   if readErr==io.EOF || readErr!=nil || len(rows)==0 {break}
  }
  folder.Close()
 }
 sort.Slice(results,func(i,j int) bool {
  li,lj:=strings.ToLower(results[i]),strings.ToLower(results[j])
  if li==lj {return results[i]<results[j]}
  return li<lj
 })
 return results
}
