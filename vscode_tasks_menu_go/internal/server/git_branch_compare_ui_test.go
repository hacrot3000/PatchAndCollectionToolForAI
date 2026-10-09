package server

import (
 "strings"
 "testing"

 webassets "bletonfc/vscode_tasks_menu/web"
)

func TestGitBranchCompareReusesDirectoryCompareTabAndReadOnlyFileDiff(t *testing.T) {
 jsBytes,err:=webassets.Files.ReadFile("featuremods/directorycompare.js")
 if err!=nil {t.Fatal(err)}
 js:=string(jsBytes)
 for _,required:=range []string{
  "function sourceGit(repoID,ref)",
  "if(source.kind==='git'){",
  "view:'compare-tree',repo:source.repoID,ref:source.ref",
  "source.sha=data.commit",
  "left.oid&&right.oid?(left.oid===right.oid&&left.mode===right.mode?'same':'changed')",
  "if(!entry)return node",
  ".dircmp-name[data-status=\"left-only\"],.dircmp-name[data-status=\"right-only\"]{color:#ff777c",
  "gitCommitSource(src.repoID,entry.rel,src.sha,{allowMissing:true",
  "session.left.kind==='git'?'Git branch file diff",
  "session.left.kind!=='git'&&session.right.kind!=='git'",
  "button.textContent='Diff ↗'",
  "async function openGitBranches(repoID,leftRef,rightRef)",
  "return open(sourceGit(repoID,leftRef),sourceGit(repoID,rightRef))",
  "TaskMenuDirectoryCompare={open,openGitBranches",
  "if((left.kind==='git')!==(right.kind==='git'))",
  "if(left.kind==='git'&&left.repoID!==right.repoID)",
  "return api.analyzeTexts(",
  "const files=session.rows.filter(row=>",
 }{
  if !strings.Contains(js,required){t.Errorf("Git Folder Compare missing contract %q",required)}
 }
}

func TestGitPanelUsesTwoExplicitBranchSelectorsWithoutCheckout(t *testing.T) {
 jsBytes,err:=webassets.Files.ReadFile("featuremods/gitstatus.js")
 if err!=nil {t.Fatal(err)}
 js:=string(jsBytes)
 for _,required:=range []string{
  "loadCompare((branch.remote?'refs/remotes/':'refs/heads/')+branch.name)",
  "async function loadCompare(base='')",
  "const repoID=activeRepoID;",
  "const leftSelect=document.createElement('select'),rightSelect=document.createElement('select')",
  "choices=[{ref:'HEAD',name:'HEAD · current commit'}]",
  "refs/heads/",
  "refs/remotes/",
  "leftSelect.setAttribute('aria-label','Left Git branch')",
  "rightSelect.setAttribute('aria-label','Right Git branch')",
  "rightSelect.value='HEAD'",
  "swap.onclick=()=>{const temp=leftSelect.value;",
  "actionButton('Open folder compare ↗'",
  "compare.openGitBranches(repoID,left,right)",
  "if(opened!==false)panel.classList.remove('visible')",
  "Read-only Git snapshots:",
 }{
  if !strings.Contains(js,required){t.Errorf("Git compare panel missing contract %q",required)}
 }
 start:=strings.Index(js,"async function loadCompare(base=''){")
 end:=strings.Index(js[start:],"async function loadCurrentView(){")
 if start<0||end<0 {t.Fatal("Git comparison handler missing")}
 section:=js[start:start+end]
 for _,bad:=range []string{"checkout","action('merge'","action('switch'","action('push'","gitView('compare'"} {
  if strings.Contains(section,bad)&&bad!="checkout"{t.Errorf("Git branch compare must remain read-only; found %q",bad)}
 }
 if strings.Contains(section,"window.prompt("){t.Fatal("Git branch Compare must not use typed prompt")}
}
