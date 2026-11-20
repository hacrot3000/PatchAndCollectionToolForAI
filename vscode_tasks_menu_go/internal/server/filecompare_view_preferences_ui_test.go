package server

import (
  "strings"
  "testing"

  webassets "bletonfc/vscode_tasks_menu/web"
)

func TestFileCompareBrowserViewPreferences(t *testing.T) {
  data, err := webassets.Files.ReadFile("featuremods/filecompare.js")
  if err != nil { t.Fatal(err) }
  js := string(data)
  for _, want := range []string{
    "const compareViewPreferenceKey='taskmenu.filecompare.viewPreferences.v1'",
    "localStorage.getItem(compareViewPreferenceKey)",
    "localStorage.setItem(compareViewPreferenceKey",
    "let contentMode=savedCompareViewPreferences?.contentMode??'context'",
    "let showUnimportant=savedCompareViewPreferences?.showUnimportant??true",
    "viewUnimportantButton.disabled=contentMode==='all'",
    "if(contentMode==='all')return model.rows.map((_,index)=>index);",
    "showUnimportant||row.importance!=='unimportant'",
    "viewDiffButton.onclick=()=>{contentMode='diff';saveCompareViewPreferences();render();}",
    "viewContextButton.onclick=()=>{contentMode='context';saveCompareViewPreferences();render();}",
    "viewUnimportantButton.onclick=()=>{if(contentMode==='all')return;showUnimportant=!showUnimportant;saveCompareViewPreferences();render();}",
  } {
    if !strings.Contains(js,want) { t.Errorf("File Compare browser preference contract missing %q",want) }
  }
  restore := strings.SplitN(js,"async function restoreCompareState(saved){",2)
  if len(restore)!=2 { t.Fatal("missing compare tab restore") }
  body := strings.SplitN(restore[1],"globalThis.TaskMenuFileCompare=",2)[0]
  for _, stale := range []string{"contentMode=saved.","showUnimportant=saved.","viewMode=saved."} {
    if strings.Contains(body,stale) { t.Errorf("snapshot must not override browser filter preferences: %s",stale) }
  }
}
