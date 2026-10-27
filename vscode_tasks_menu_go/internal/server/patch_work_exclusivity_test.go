package server

import (
 "encoding/json"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/session"
)

func TestPatchSelectedWorkKindRejectsMixedKinds(t *testing.T) {
 raw:=map[string]any{"items":[]map[string]any{
  {"index":1,"kind":"PATCH"},{"index":2,"kind":"COLLECT"},{"index":3,"kind":"PATCH"},
 }}
 encoded,_:=json.Marshal(raw)
 state:=session.ProtocolState{Prompt:encoded}
 for _,tc:=range []struct{
  selected []int
  kind string
  fails bool
 }{
  {[]int{1,3},"PATCH",false},
  {[]int{2},"COLLECT",false},
  {[]int{1,2},"",true},
  {[]int{1,2,3},"",true},
  {[]int{},"",true},
 }{
  actual,err:=patchSelectedWorkKind(state,tc.selected)
  if tc.fails {
   if err==nil {t.Fatalf("accepted unsafe selection %v",tc.selected)}
   if len(tc.selected)>1&&!strings.Contains(err.Error(),"must not"){t.Fatalf("unexpected guard error: %v",err)}
  }else if err!=nil||actual!=tc.kind {t.Fatalf("selected=%v: kind=%q err=%v",tc.selected,actual,err)}
 }
}
