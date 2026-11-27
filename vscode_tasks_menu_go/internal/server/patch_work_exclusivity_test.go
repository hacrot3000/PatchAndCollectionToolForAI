package server

import (
 "encoding/json"
 "context"
 "errors"
 "time"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/session"
)

func TestPatchSelectedWorkKindRejectsMixedKinds(t *testing.T) {
 raw:=map[string]any{"items":[]map[string]any{
  {"index":1,"kind":"PATCH"},{"index":2,"kind":"COLLECT"},{"index":3,"kind":"PATCH"},{"index":4,"kind":"COLLECT INVALID"},
 }}
 encoded,_:=json.Marshal(raw)
 state:=session.ProtocolState{Prompt:encoded}
 for _,tc:=range []struct{
  selected []int
  kind string
  fails bool
 }{
  {[]int{1,3},"PATCH",false},
  {[]int{4},"",true},
  {[]int{2},"COLLECT",false},
  {[]int{1,2},"",true},
  {[]int{1,2,3},"",true},
  {[]int{},"",true},
 }{
  actual,err:=patchSelectedWorkKind(state,tc.selected)
  if tc.fails {
   if err==nil {t.Fatalf("accepted unsafe selection %v",tc.selected)}
   if len(tc.selected)>1&&!strings.Contains(err.Error(),"must not"){t.Fatalf("unexpected guard error: %v",err)}
   if len(tc.selected)==1&&tc.selected[0]==4&&!strings.Contains(err.Error(),"diagnostic handoff"){t.Fatalf("invalid COLLECT must identify handoff: %v",err)}
  }else if err!=nil||actual!=tc.kind {t.Fatalf("selected=%v: kind=%q err=%v",tc.selected,actual,err)}
 }
}

func TestAgentNativeAdmissionWaitsOnlyOnProvenConflict(t *testing.T){
 ctx,cancel:=context.WithTimeout(agentNativeWaitContext(context.Background(),"/test","local","wait"),3*time.Second)
 defer cancel()
 attempts:=0
 err:=waitAgentNativeAdmission(ctx,func()error{
  attempts++
  if attempts==1{return &patchWorkBusyError{"PATCH","COLLECT"}}
  if got:=agentNativeWaitSnapshot("/test","local","wait");got!="waiting_for_patch"{t.Fatalf("wait not visible: %q",got)}
  return nil
 })
 if err!=nil||attempts!=2||agentNativeWaitSnapshot("/test","local","wait")!=""{t.Fatalf("unexpected wait result: %d %v",attempts,err)}
 attempts=0
 realFailure:=errors.New("native launch failed")
 if err=waitAgentNativeAdmission(ctx,func()error{attempts++;return realFailure});!errors.Is(err,realFailure)||attempts!=1{t.Fatal("real failure retried")}
 stop,cancelStop:=context.WithCancel(ctx)
 attempts=0
 err=waitAgentNativeAdmission(stop,func()error{attempts++;cancelStop();return &patchWorkBusyError{"COLLECT","PATCH"}})
 if !errors.Is(err,context.Canceled)||attempts!=1{t.Fatal("cancelled wait started another attempt")}
}
