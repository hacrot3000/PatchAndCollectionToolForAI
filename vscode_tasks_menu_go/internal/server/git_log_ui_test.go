package server

import (
 "strings"
 "testing"
 webassets "bletonfc/vscode_tasks_menu/web"
)

func TestGitLogFilterAndCommitDiffUIContracts(t *testing.T){
 loader,err:=webassets.Files.ReadFile("featuremods/next.js");if err!=nil{t.Fatal(err)}
 src:=string(loader)
 before:=strings.Index(src,"/featuremods/gitlog.js")
 after:=strings.Index(src,"/featuremods/gitstatus.js")
 if before<0||after<=before{t.Fatal("Git Log module must load before the Git panel")}
 panel,err:=webassets.Files.ReadFile("featuremods/gitstatus.js");if err!=nil{t.Fatal(err)}
 if !strings.Contains(string(panel),"TaskDeckGitLog")||!strings.Contains(string(panel),"async function loadLog()"){
  t.Fatal("Git Log tab must delegate to the Log module")
 }
 ui,err:=webassets.Files.ReadFile("featuremods/gitlog.js");if err!=nil{t.Fatal(err)}
 text:=string(ui)
 for _,needle:=range []string{
  "input.type='search'", "search.oninput=", "String(query||'').toLowerCase()",
  "limit:String(PAGE)", "skip:String(skip)", "Load older commits", "Refresh log",
  "actionButton('Show Diff'", "gitView('commit-files'", "openGitCommitFileDiffBetween",
  "dirCompare.openGitCommit(repoID,parent,commit.sha)", "choose merge parent",
  "renderGitGraphCommitDetail",
 }{
  if !strings.Contains(text,needle){t.Errorf("Git Log UI missing %q",needle)}
 }
 tree,err:=webassets.Files.ReadFile("featuremods/directorycompare.js");if err!=nil{t.Fatal(err)}
 if !strings.Contains(string(tree),"preferChangedOnly")||!strings.Contains(string(tree),"async function openGitCommit("){
  t.Fatal("Commit compare must use changed-only Directory Compare")
 }
}
