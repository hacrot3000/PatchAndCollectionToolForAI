package server

import (
 "encoding/json"
 "net/http"
 "net/url"
 "os"
 "path/filepath"
 "strings"
 "strconv"
 "testing"
)

func TestGitLogPagedResultsAndValidatedCommitCompare(t *testing.T){
 ws,s,_:=setupGitQuickRepo(t)
 first:=strings.TrimSpace(gitQuickRun(t,ws,"rev-parse","HEAD"))
 for _,name:=range []string{"alpha.txt","beta.txt"}{
  if err:=os.WriteFile(filepath.Join(ws,name),[]byte(name+"\n"),0600);err!=nil{t.Fatal(err)}
  gitQuickRun(t,ws,"add",name)
  gitQuickRun(t,ws,"commit","-m","Add "+name)
 }
 last:=strings.TrimSpace(gitQuickRun(t,ws,"rev-parse","HEAD"))
 type logPage struct{
  Commits []gitCommitRow `json:"commits"`
  HasMore bool `json:"has_more"`
  Skip int `json:"skip"`
 }
 load:=func(skip int)logPage{
  t.Helper()
  rr:=callGitStatusHandler(t,s,http.MethodGet,"/api/git/status?view=log&limit=1&skip="+url.QueryEscape(strconv.Itoa(skip)),"")
  if rr.Code!=http.StatusOK{t.Fatalf("log skip %d returned %d: %s",skip,rr.Code,rr.Body.String())}
  var item logPage
  if err:=json.Unmarshal(rr.Body.Bytes(),&item);err!=nil{t.Fatal(err)}
  return item
 }
 firstPage,nextPage,endPage:=load(0),load(1),load(2)
 if len(firstPage.Commits)!=1||firstPage.Commits[0].SHA!=last||!firstPage.HasMore||firstPage.Skip!=0{
  t.Fatalf("first page invalid: %+v",firstPage)
 }
 if len(nextPage.Commits)!=1||nextPage.Commits[0].SHA==last||!nextPage.HasMore||nextPage.Skip!=1{
  t.Fatalf("second page invalid: %+v",nextPage)
 }
 if len(endPage.Commits)!=1||endPage.Commits[0].SHA!=first||endPage.HasMore{
  t.Fatalf("last page invalid: %+v",endPage)
 }
 for _,bad:=range []string{"-1","10001","xyz"}{
  rr:=callGitStatusHandler(t,s,http.MethodGet,"/api/git/status?view=log&limit=1&skip="+bad,"")
  if rr.Code!=http.StatusBadRequest{t.Errorf("unexpectedly accepted log offset %q: HTTP %d",bad,rr.Code)}
 }
 // Commit Log multi-file diff must compare pinned snapshots without touching
 // a branch, index, or working tree. Arbitrary expressions remain forbidden.
 for _,ref:=range []string{first,last}{
  rr:=callGitStatusHandler(t,s,http.MethodGet,"/api/git/status?view=compare-tree&ref="+ref,"")
  if rr.Code!=http.StatusOK{t.Fatalf("commit SHA snapshot failed: %d %s",rr.Code,rr.Body.String())}
 }
 rr:=callGitStatusHandler(t,s,http.MethodGet,"/api/git/status?view=compare-tree-diff&left="+first+"&right="+last,"")
 if rr.Code!=http.StatusOK{t.Fatalf("commit diff by SHA failed: %d %s",rr.Code,rr.Body.String())}
 for _,ref:=range []string{"HEAD~1","HEAD..HEAD","--all",last[:12]}{
  q:=url.QueryEscape(ref)
  bad:=callGitStatusHandler(t,s,http.MethodGet,"/api/git/status?view=compare-tree&ref="+q,"")
  if bad.Code==http.StatusOK{t.Errorf("accepted unsafe or abbreviated ref %q",ref)}
 }
 if got:=strings.TrimSpace(gitQuickRun(t,ws,"status","--porcelain"));got!=""{
  t.Fatalf("commit snapshot comparisons changed worktree: %q",got)
 }
}
