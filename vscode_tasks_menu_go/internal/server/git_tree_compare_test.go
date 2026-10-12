package server

import (
 "fmt"
 "os/exec"
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

func TestParseGitTreeDiffChangesSupportsLargeChangedSetsAndSpecialPaths(t *testing.T) {
	a:=strings.Repeat("a",40)
	b:=strings.Repeat("b",40)
	// 12,000 changes exceed the old 10,000-entry tree limit.
	var raw strings.Builder
	for i:=0;i<12000;i++ {
		path:=fmt.Sprintf("src/deep/file-%05d.txt",i)
		if i==11 {path="src/tab\twith\nnewline.txt"}
		raw.WriteString(":100644 100644 "+a+" "+b+" M\x00"+path+"\x00")
	}
	changes,err:=parseGitTreeDiffChanges(raw.String())
	if err!=nil||len(changes)!=12000 {t.Fatalf("large branch diff: %d entries, %v",len(changes),err)}
	if changes[11].Path!="src/tab\twith\nnewline.txt"||changes[11].Left.OID!=a||changes[11].Right.OID!=b {
		t.Fatalf("invalid special-path metadata: %+v",changes[11])
	}
	added,err:=parseGitTreeDiffChanges(":000000 100644 "+strings.Repeat("0",40)+" "+a+" A\x00added.txt\x00")
	if err!=nil||len(added)!=1||added[0].Left!=nil||added[0].Right==nil {t.Fatalf("add: %+v %v",added,err)}
	deleted,err:=parseGitTreeDiffChanges(":100644 000000 "+a+" "+strings.Repeat("0",40)+" D\x00deleted.txt\x00")
	if err!=nil||len(deleted)!=1||deleted[0].Left==nil||deleted[0].Right!=nil {t.Fatalf("delete: %+v %v",deleted,err)}
	for _,invalid:=range []string{
		":100644 100644 "+a+" "+b+" M\x00unterminated",
		":100644 100644 "+a+" "+b+" R100\x00renamed.txt\x00",
		":100644 100644 "+a+" "+b+" M\x00\x00",
		":000000 000000 "+strings.Repeat("0",40)+" "+strings.Repeat("0",40)+" M\x00unknown.txt\x00",
	} {
		if _,err:=parseGitTreeDiffChanges(invalid);err==nil {t.Fatalf("accepted malformed diff %q",invalid)}
	}
}

func TestGitCompareTreeDiffHandlesMoreThanTenThousandUnchangedFiles(t *testing.T) {
	ws,s,_:=setupGitQuickRepo(t)
	oidA:=strings.TrimSpace(gitQuickRun(t,ws,"hash-object","-w","--stdin"))
	if len(oidA)!=40 {t.Fatalf("missing empty blob SHA: %q",oidA)}
	file:=filepath.Join(ws,"changed-payload.txt")
	if err:=os.WriteFile(file,[]byte("different"),0o600);err!=nil{t.Fatal(err)}
	oidB:=strings.TrimSpace(gitQuickRun(t,ws,"hash-object","-w",file))
	buildTree:=func(changed string)string{
		t.Helper()
		var input strings.Builder
		for i:=0;i<10005;i++{
			oid:=oidA
			if i==10004 {oid=changed}
			fmt.Fprintf(&input,"100644 blob %s\titem-%05d.txt\x00",oid,i)
		}
		cmd:=exec.Command("git","-C",ws,"mktree","-z")
		cmd.Stdin=strings.NewReader(input.String())
		output,err:=cmd.CombinedOutput()
		if err!=nil{t.Fatalf("mktree failed: %v %s",err,output)}
		return strings.TrimSpace(string(output))
	}
	leftTree,rightTree:=buildTree(oidA),buildTree(oidB)
	if leftTree==rightTree{t.Fatal("test snapshots have identical trees")}
	for _,pair:=range []struct{name,tree string}{{"bulk-left",leftTree},{"bulk-right",rightTree}}{
		commit:=strings.TrimSpace(gitQuickRun(t,ws,"commit-tree",pair.tree,"-m",pair.name))
		gitQuickRun(t,ws,"branch",pair.name,commit)
	}
	// The legacy snapshot limit is reproducible, while optimized branch diff
	// must only return the actual changed file.
	legacy:=callGitStatusHandler(t,s,http.MethodGet,"/api/git/status?view=compare-tree&ref=bulk-left","")
	if legacy.Code!=http.StatusRequestEntityTooLarge {t.Fatalf("test did not exceed legacy cap: %d %s",legacy.Code,legacy.Body.String())}
	rr:=callGitStatusHandler(t,s,http.MethodGet,"/api/git/status?view=compare-tree-diff&left=bulk-left&right=bulk-right","")
	if rr.Code!=http.StatusOK{t.Fatalf("large Git diff HTTP %d: %s",rr.Code,rr.Body.String())}
	var result struct{
		RepoID string `json:"repo_id"`
		LeftCommit string `json:"left_commit"`
		RightCommit string `json:"right_commit"`
		ChangedOnly bool `json:"changed_only"`
		Entries []gitTreeDiffChange `json:"entries"`
	}
	if err:=json.Unmarshal(rr.Body.Bytes(),&result);err!=nil{t.Fatal(err)}
	if result.RepoID!="."||!result.ChangedOnly||result.LeftCommit==result.RightCommit||
		len(result.Entries)!=1||result.Entries[0].Path!="item-10004.txt"||
		result.Entries[0].Left==nil||result.Entries[0].Right==nil {t.Fatalf("invalid large-tree diff response: %+v",result)}
	if got:=gitQuickRun(t,ws,"status","--porcelain");got!="" {t.Fatalf("diff mutated working tree: %q",got)}
	for _,ref:=range []string{"HEAD~1","--all","refs/remotes/origin/HEAD","nonexistent"}{
		r:=callGitStatusHandler(t,s,http.MethodGet,"/api/git/status?view=compare-tree-diff&left="+ref+"&right=bulk-right","")
		if r.Code==http.StatusOK{t.Fatalf("unsafe ref %q accepted",ref)}
	}
}
