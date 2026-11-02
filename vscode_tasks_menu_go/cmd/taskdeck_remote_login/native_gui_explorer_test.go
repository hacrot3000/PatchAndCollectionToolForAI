package main

import (
 "strings"
 "testing"
)

func TestNativeGUIExplorerAssetsUseAuthenticatedTextOnlySelection(t *testing.T){
 html,err:=trustedGUIAssets.ReadFile("native_gui.html")
 if err!=nil{t.Fatal(err)}
 js,err:=trustedGUIAssets.ReadFile("native_gui.js")
 if err!=nil{t.Fatal(err)}
 css,err:=trustedGUIAssets.ReadFile("native_gui.css")
 if err!=nil{t.Fatal(err)}
 for _,fragment:=range []string{
  `id="workspace-entries"`,`id="explorer-location"`,
  `id="folder-up"`,`data-op="files_list"`,
 }{
  if !strings.Contains(string(html),fragment){
   t.Errorf("native explorer markup lacks %q",fragment)
  }
 }
 for _,fragment:=range []string{
  `function renderExplorer(directory,listing)`,
  `new TextEncoder().encode(listing).length>192`,
  `button.textContent=`,
  `target.replaceChildren()`,
  `document.createElement("button")`,
  `"explorer-location"`,
  `$("git-path").value=item.path`,
 }{
  if !strings.Contains(string(js),fragment){
   t.Errorf("native explorer source lacks %q",fragment)
  }
 }
 for _,unsafe:=range []string{
  ".innerHTML=", "insertAdjacentHTML", "document.write(",
  "eval(", "new Function(", "window.open(", "location.href=",
 }{
  if strings.Contains(string(js),unsafe){
   t.Errorf("native explorer allows unsafe browser execution primitive: %q",unsafe)
  }
 }
 if !strings.Contains(string(css),".explorer-entries button"){
  t.Fatal("native explorer missing bounded visible click targets")
 }
}
