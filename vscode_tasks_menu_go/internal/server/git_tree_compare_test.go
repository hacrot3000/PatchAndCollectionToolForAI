package server

import (
 "encoding/json"
 "net/http"
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func TestGitCompareTreeReadOnlyBranchSnapshots(t *testing.T){
 ws,s,_:=setupGitQuickRepo(t)
 base:=gitQuickRun(t,ws,"rev-parse","--abbrev-ref","HEAD")
 base=strings.TrimSpace(base)
 // Keep both versions as independent refs, including filenames containing
 // spaces and subfolders; no checkout occurs during either tree request.
 gitQuickRun(t,ws,"branch","feature/left")
 gitQuickRun(t,ws,"checkout","-b","feature/right")
 if err:=os.MkdirAll(filepath.Join(ws,"nested"),0o755);err!=nil {t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(ws,"nested","changed name.txt"),[]byte("right\n"),0o644);err!=nil {t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(ws,"right-only.txt"),[]byte("right\n"),0o644);err!=nil {t.Fatal(err)}
 gitQuickRun(t,ws,"add",".")
 gitQuickRun(t,ws,"commit","-m","right files")
 gitQuickRun(t,ws,"checkout","feature/left")
 if err:=os.MkdirAll(filepath.Join(ws,"nested"),0o755);err!=nil {t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(ws,"nested","changed name.txt"),[]byte("left\n"),0o644);err!=nil {t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(ws,"left-only.txt"),[]byte("left\n"),0o644);err!=nil {t.Fatal(err)}
 gitQuickRun(t,ws,"add",".")
 gitQuickRun(t,ws,"commit","-m","left files")
 gitQuickRun(t,ws,"checkout",base)
 branchBefore:=strings.TrimSpace(gitQuickRun(t,ws,"rev-parse","--abbrev-ref","HEAD"))
 statusBefore:=gitQuickRun(t,ws,"status","--porcelain")
 type tree struct {
  RepoID string `json:"repo_id"`
  Ref string `json:"ref"`
  Commit string `json:"commit"`
  Entries []gitTreeCompareEntry `json:"entries"`
 }
 fetch:=func(ref string)tree{
  t.Helper()
  rr:=callGitStatusHandler(t,s,http.MethodGet,"/api/git/status?view=compare-tree&ref="+ref,"")
  if rr.Code!=http.StatusOK{t.Fatalf("Git tree %s HTTP %d: %s",ref,rr.Code,rr.Body.String())}
  var result tree
  if err:=json.Unmarshal(rr.Body.Bytes(),&result);err!=nil{t.Fatal(err)}
  if result.Commit==""||result.RepoID!="."||result.Ref!=ref{t.Fatalf("invalid tree metadata %+v",result)}
  return result
 }
 left:=fetch("refs/heads/feature/left")
 right:=fetch("refs/heads/feature/right")
 if left.Commit==right.Commit{t.Fatal("distinct branches yielded identical commit")}
 maps:=func(entries []gitTreeCompareEntry)map[string]gitTreeCompareEntry{
  result:=map[string]gitTreeCompareEntry{};for _,e:=range entries{result[e.Path]=e};return result
 }
 a,b:=maps(left.Entries),maps(right.Entries)
 if a["nested"].Kind!="dir"||b["nested"].Kind!="dir"{t.Fatalf("subfolders missing from Git snapshot: %v %v",a,b)}
 if a["nested/changed name.txt"].OID==b["nested/changed name.txt"].OID {t.Fatal("different blob contents were marked identical")}
 if a["nested/changed name.txt"].Mode!="100644"||b["nested/changed name.txt"].Kind!="file"{t.Fatal("Git file metadata lost")}
 if _,exists:=a["right-only.txt"];exists {t.Fatal("right-only file incorrectly appeared on left")}
 if _,exists:=b["left-only.txt"];exists {t.Fatal("left-only file incorrectly appeared on right")}
 if _,exists:=a["left-only.txt"];!exists{t.Fatal("left-only file missing")}
 if _,exists:=b["right-only.txt"];!exists{t.Fatal("right-only file missing")}
 if strings.TrimSpace(gitQuickRun(t,ws,"rev-parse","--abbrev-ref","HEAD"))!=branchBefore{t.Fatal("compare unexpectedly checked out a branch")}
 if got:=gitQuickRun(t,ws,"status","--porcelain");got!=statusBefore{t.Fatalf("compare mutated worktree: %q → %q",statusBefore,got)}
}

func TestGitCompareTreeRejectsUnknownOrArbitraryRevisions(t *testing.T){
 _,s,_:=setupGitQuickRepo(t)
 for _,ref:=range []string{"refs/heads/not-found","HEAD~1","--all","refs/remotes/origin/HEAD","refs/heads/a..b"}{
  rr:=callGitStatusHandler(t,s,http.MethodGet,"/api/git/status?view=compare-tree&ref="+ref,"")
  if rr.Code==http.StatusOK{t.Fatalf("unexpectedly accepted compare ref %q: %s",ref,rr.Body.String())}
 }
}

func TestParseGitTreeCompareEntriesPreservesTabAndNewlineNames(t *testing.T){
 oid:=strings.Repeat("a",40)
 raw:="100644 blob "+oid+"\tname\twith-tab.txt\x00"+
  "040000 tree "+oid+"\tdirectory\x00"+
  "100644 blob "+oid+"\tdirectory/new\nline.txt\x00"
 entries,err:=parseGitTreeCompareEntries(raw)
 if err!=nil{t.Fatal(err)}
 if len(entries)!=3||entries[0].Path!="name\twith-tab.txt"||entries[1].Kind!="dir"||entries[2].Path!="directory/new\nline.txt"{
  t.Fatalf("Git tree parser damaged paths: %+v",entries)
 }
}
