package server

import (
 "context"
 "fmt"
 "net/http"
 "os"
 "os/exec"
 "strings"
 "time"
)

const gitTreeCompareMaxEntries = 10000
const gitTreeCompareOutputLimit = 4 << 20

type gitTreeCompareEntry struct {
 Path string `json:"path"`
 Kind string `json:"kind"`
 OID string `json:"oid"`
 Mode string `json:"mode"`
}

func parseGitTreeCompareEntries(raw string) ([]gitTreeCompareEntry, error) {
 entries := make([]gitTreeCompareEntry, 0)
 for _, line := range strings.Split(raw, "\x00") {
  if line == "" { continue }
  if len(entries) >= gitTreeCompareMaxEntries {
   return nil, fmt.Errorf("Git tree exceeds %d entries; compare a smaller branch tree", gitTreeCompareMaxEntries)
  }
  fields := strings.SplitN(line, "\t", 2)
  if len(fields) != 2 {return nil, fmt.Errorf("invalid git ls-tree record")}
  meta := strings.Fields(fields[0])
  if len(meta) != 3 || len(meta[2]) != 40 || !gitCompareCommitPattern.MatchString(meta[2]) {
   return nil, fmt.Errorf("invalid git ls-tree object")
  }
  path := fields[1]
  if path == "" || strings.HasPrefix(path,"/") || strings.ContainsRune(path,'\x00') {
   return nil, fmt.Errorf("invalid git tree path")
  }
  kind := "file"
  if meta[1] == "tree" {kind="dir"} else if meta[1]!="blob"&&meta[1]!="commit" {
   return nil,fmt.Errorf("unexpected git tree object type")
  }
  entries=append(entries,gitTreeCompareEntry{Path:path,Kind:kind,OID:meta[2],Mode:meta[0]})
 }
 return entries,nil
}

func (s *Server) gitCompareTreeReference(ctx context.Context, requested string) (string,error) {
 requested=strings.TrimSpace(requested)
 if requested=="HEAD" {
  out,stderr,_,err:=s.runGit(ctx,4*time.Second,"rev-parse","--verify","HEAD^{commit}")
  if err!=nil {return "",fmt.Errorf("resolve HEAD: %s",strings.TrimSpace(joinGitOutput(stderr,err.Error())))}
  return strings.TrimSpace(out),nil
 }
 if requested==""||strings.ContainsAny(requested,"\r\n\x00")||strings.HasPrefix(requested,"-")||strings.HasSuffix(requested,"/HEAD") {
  return "",fmt.Errorf("invalid Git compare branch")
 }
 // A branch name is never treated as an arbitrary revision expression. Only
 // existing refs/heads and refs/remotes may be selected. Their SHA is pinned
 // before listing and subsequently supplied to read-only File Compare.
 var qualified string
 if strings.HasPrefix(requested,"refs/heads/")||strings.HasPrefix(requested,"refs/remotes/"){
  _,_,_,err:=s.runGit(ctx,3*time.Second,"show-ref","--verify","--quiet",requested)
  if err==nil {qualified=requested}
 } else {
  for _,prefix:=range []string{"refs/heads/","refs/remotes/"}{
   _,_,_,err:=s.runGit(ctx,3*time.Second,"show-ref","--verify","--quiet",prefix+requested)
   if err==nil {qualified=prefix+requested;break}
  }
 }
 if qualified=="" {return "",fmt.Errorf("Git compare branch not found: %s",requested)}
 out,stderr,_,err:=s.runGit(ctx,4*time.Second,"rev-parse","--verify",qualified+"^{commit}")
 if err!=nil {return "",fmt.Errorf("resolve Git compare branch: %s",strings.TrimSpace(joinGitOutput(stderr,err.Error())))}
 commit:=strings.TrimSpace(out)
 if !gitCompareCommitPattern.MatchString(commit) {return "",fmt.Errorf("invalid resolved commit")}
 return commit,nil
}

func (s *Server) gitCompareTree(w http.ResponseWriter,r *http.Request){
 if r.Method!=http.MethodGet {http.Error(w,"method not allowed",http.StatusMethodNotAllowed);return}
 ref:=strings.TrimSpace(r.URL.Query().Get("ref"))
 commit,err:=s.gitCompareTreeReference(r.Context(),ref)
 if err!=nil {http.Error(w,err.Error(),http.StatusBadRequest);return}
 repo,ok:=gitRepositoryFromContext(r.Context())
 if !ok {http.Error(w,"Git repository unavailable",http.StatusConflict);return}
 ctx,cancel:=context.WithTimeout(r.Context(),12*time.Second);defer cancel()
 // -t includes folders, -r walks the complete tree and -z preserves filename
 // spaces, tabs and newlines. No checkout or workspace mutation occurs.
 cmd:=exec.CommandContext(ctx,"git","ls-tree","-r","-t","-z","--full-tree",commit)
 cmd.Dir=repo.Root
 cmd.Env=append(os.Environ(),"GIT_TERMINAL_PROMPT=0","GIT_PAGER=cat","LC_ALL=C")
 stdout:=&cappedGitBuffer{limit:gitTreeCompareOutputLimit}
 stderr:=&cappedGitBuffer{limit:64<<10}
 cmd.Stdout=stdout;cmd.Stderr=stderr
 if runErr:=cmd.Run();runErr!=nil {
  if ctx.Err()!=nil {http.Error(w,"Git tree scan timed out",http.StatusGatewayTimeout);return}
  http.Error(w,"Git tree scan failed: "+strings.TrimSpace(joinGitOutput(stderr.String(),runErr.Error())),http.StatusConflict);return
 }
 if stdout.truncated {http.Error(w,"Git tree too large for compare (4 MiB limit)",http.StatusRequestEntityTooLarge);return}
 entries,err:=parseGitTreeCompareEntries(stdout.String())
 if err!=nil {http.Error(w,err.Error(),http.StatusRequestEntityTooLarge);return}
 writeJSON(w,http.StatusOK,map[string]any{"repo_id":repo.ID,"ref":ref,"commit":commit,"entries":entries})
}
