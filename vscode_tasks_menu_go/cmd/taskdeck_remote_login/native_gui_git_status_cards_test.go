package main

import (
 "strings"
 "testing"
)

func TestNativeGUIGitStatusCardsRemainReadOnlyAndUntrusted(t *testing.T){
 html,err:=trustedGUIAssets.ReadFile("native_gui.html")
 if err!=nil{t.Fatal(err)}
 js,err:=trustedGUIAssets.ReadFile("native_gui.js")
 if err!=nil{t.Fatal(err)}
 source:=string(js)
 for _,match:=range []string{
  `id="git-status-files"`,`id="git-status-count"`,
  `data-op="git_status"`,`data-op="git_write_review"`,
  `id="git-path"`,
 }{
  if !strings.Contains(string(html),match){
   t.Errorf("missing Git status selection UI %q",match)
  }
 }
 for _,match:=range []string{
  `function renderGitStatus(report)`,
  `new TextEncoder().encode(report).length>4096`,
  `lines.length>65`,
  `path.includes("..")`,
  `path.includes(" -> ")`,
  `!safeWorkspacePath(path)`,
  `button.textContent=entry.status+" · "+entry.path`,
  `$("git-path").value=entry.path`,
  `gitReview=null`,
  `$("git-approval").classList.add("hidden")`,
  `if(result&&operation==="git_status"){renderGitStatus(result.text||"");}`,
 }{
  if !strings.Contains(source,match){
   t.Errorf("Git status cards missing safety check %q",match)
  }
 }
 for _,bad:=range []string{"innerHTML=", "document.write(","eval(","new Function(", "insertAdjacentHTML"}{
  if strings.Contains(source,bad){
   t.Errorf("Git cards parse untrusted source with dynamic code or HTML: %q",bad)
  }
 }
}
