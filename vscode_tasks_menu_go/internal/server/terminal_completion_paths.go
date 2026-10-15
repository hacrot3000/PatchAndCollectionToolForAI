package server

import (
 "context"
 "io"
 "net/http"
 "os"
 "os/exec"
 "path/filepath"
 "regexp"
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
 // In single-user mode the original interactive terminals have Kind="".
 // This is an established legacy contract (see attestAITerminal). Accept
 // only unowned, TaskID=0 local sessions; never relax shared-server checks.
 legacyLocal:=!s.Config.SharedServerEnabled && meta.Kind=="" && meta.TaskID==0 &&
  meta.OwnerUserID=="" && meta.ProjectID==""
 localTarget:=(strings.TrimSpace(meta.TargetType)=="" || strings.EqualFold(strings.TrimSpace(meta.TargetType),"local")) &&
  strings.TrimSpace(meta.TargetProfileID)==""
 if !(meta.Kind==tasks.SessionKindTerminal || legacyLocal) || meta.TaskID!=0 || !localTarget {
  http.Error(w, "local interactive terminal required", http.StatusForbidden); return
 }
 dir := r.URL.Query().Get("dir")
 prefix := r.URL.Query().Get("prefix")
 kind := r.URL.Query().Get("kind")
 if kind!="" && kind!="paths" && kind!="branches" && kind!="commands" && kind!="aliases" { http.Error(w,"invalid completion kind",http.StatusBadRequest); return }
 if (kind=="branches" || kind=="commands" || kind=="aliases") && dir!="" { http.Error(w,"branch lookup must use the active cwd",http.StatusBadRequest); return }
 if len(dir)>512 || len(prefix)>160 || strings.ContainsAny(dir+prefix, "\r\n\x00") ||
  strings.ContainsAny(dir+prefix, "*?[\\") || (kind!="branches" && strings.Contains(prefix, "/")) {
  http.Error(w, "invalid completion path", http.StatusBadRequest); return
 }
 if kind=="aliases" {
  w.Header().Set("Cache-Control","no-store")
  writeJSON(w,http.StatusOK,map[string]any{"aliases":terminalBashStartupAliases()})
  return
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
 // Shared users with permission to CONTROL this terminal can inspect
 // that terminal's CWD, even when it is outside TaskDeck's workspace.
 // Restrict nested completions to descendants of CWD, including symlinks:
 // neither absolute paths nor ../ may be used to enumerate arbitrary hosts.
 if s.Config.SharedServerEnabled {
  if filepath.IsAbs(dir) { http.Error(w,"absolute completion path unavailable in shared mode",http.StatusForbidden); return }
  resolvedRoot,rootErr:=filepath.EvalSymlinks(root)
  resolvedFolder,folderErr:=filepath.EvalSymlinks(folder)
  if rootErr!=nil || folderErr!=nil { http.Error(w,"directory unavailable",http.StatusForbidden); return }
  relative,relativeErr:=filepath.Rel(resolvedRoot,resolvedFolder)
  if relativeErr!=nil || relative==".." || strings.HasPrefix(relative,".."+string(filepath.Separator)) {
   http.Error(w,"completion outside terminal cwd unavailable in shared mode",http.StatusForbidden); return
  }
  folder=resolvedFolder
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

// terminalBashStartupAliases is a best-effort fallback when the active shell
// does not emit OSC 133 and TaskDeck cannot safely inject "alias -p".
// Parse only simple declarations, never source or execute shell init files.
// Live alias -p output always overrides these initial file-derived hints.
var terminalAliasPattern = regexp.MustCompile(`^\s*alias\s+([^\s=]+)=('([^']*)'|"([^"]*)")\s*$`)
var terminalSafeAliasExpansion = regexp.MustCompile(`^[[:alnum:]_./ -]+$`)

func terminalBashStartupAliases() []string {
 home,err:=os.UserHomeDir()
 if err!=nil || home=="" {return []string{}}
 aliases:=map[string]string{}
 for _,base:=range []string{".profile",".bash_profile",".bashrc",".bash_aliases"} {
  name:=filepath.Join(home,base)
  info,err:=os.Stat(name)
  if err!=nil || !info.Mode().IsRegular() || info.Size()>256*1024 {continue}
  data,err:=os.ReadFile(name)
  if err!=nil {continue}
  for _,line:=range strings.Split(string(data),"\n") {
   if len(line)>512 {continue}
   hit:=terminalAliasPattern.FindStringSubmatch(line)
   if len(hit)!=5 {continue}
   alias,expansion:=hit[1],hit[3]
   if hit[4]!="" {expansion=hit[4]}
   if len(alias)>64 || len(expansion)>256 || strings.ContainsAny(alias,"/\\\r\n\x00\t") || strings.ContainsAny(expansion,"\r\n\x00\t$`;|&<>{}()\\") {continue}
   // Return only simple commands relevant to completion; do not leak
   // arbitrary startup values, variables, passwords or other secrets.
   if !terminalSafeAliasExpansion.MatchString(expansion) {continue}
   fields:=strings.Fields(expansion)
   if len(fields)==0 {continue}
   switch fields[0] {
   case "git","ls","ll","cp","mv","cd","cat","less","more","head","tail",
    "rm","mkdir","rmdir","touch","find","grep","rg","du","pwd","df",
    "gc-safe","sudo","vim","nano","python","python3","node","bash","sh","source":
   default:continue
   }
   if len(aliases)<400 || aliases[alias]!="" {
    aliases[alias]="alias "+alias+"='"+expansion+"'"
   }
  }
 }
 output:=make([]string,0,len(aliases))
 for _,value:=range aliases {output=append(output,value)}
 sort.Strings(output)
 return output
}
