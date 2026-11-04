package main

import (
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/remoterelay"
)

func TestNativeGUIManagedTaskReservesCancelAndVerificationRounds(t *testing.T){
 g,remote:=newTestNativeGUI()
 // One unrelated read already used, leaving exactly the budget needed
 // for start review + execution + one read + cancel + verification.
 g.used=1
 status,review:=guiRequestTest(t,g,guiRequest{
  Operation:"task_review",Mode:"start",TaskID:1,
 },g.origin,g.key)
 if status!=200||!review.OK||review.Confirm!="START TASK 1"||g.used!=2{
  t.Fatalf("managed review could not reserve future Cancel/Status: %+v",review)
 }
 status,start:=guiRequestTest(t,g,guiRequest{
  Operation:"task_execute",Mode:"start",TaskID:1,Confirmation:review.Confirm,
 },g.origin,g.key)
 handle:=strings.Repeat("a",32)
 if status!=200||!start.OK||g.used!=3||g.managedHandle!=handle{
  t.Fatalf("host-issued managed handle was not pinned: %+v",start)
 }
 // A 2-round Git preview would consume the emergency budget at once.
 before:=len(remote.calls)
 status,denied:=guiRequestTest(t,g,guiRequest{
  Operation:"git_review",Path:"main.go",
 },g.origin,g.key)
 if status!=400||denied.OK||len(remote.calls)!=before||g.used!=3{
  t.Fatalf("two-round Git read spent Cancel+Status budget: %+v",denied)
 }
 // An ordinary read may use the third remaining round.
 status,view:=guiRequestTest(t,g,guiRequest{Operation:"git_status"},g.origin,g.key)
 if status!=200||!view.OK||g.used!=4{t.Fatal("safe read rejected before reserve boundary")}
 before=len(remote.calls)
 for _,req:=range []guiRequest{
  {Operation:"git_status"},
  {Operation:"task_status",Handle:handle},
  {Operation:"task_output",Handle:handle,Offset:0},
  {Operation:"task_cancel",Handle:strings.Repeat("b",32),
   Confirmation:"CANCEL TASK "+strings.Repeat("b",12)},
 }{
  s,v:=guiRequestTest(t,g,req,g.origin,g.key)
  if s!=400||v.OK||g.used!=4||len(remote.calls)!=before{
   t.Fatalf("unreserved action admitted while task needs Cancel+Status: %+v %+v",req,v)
  }
 }
 status,cancel:=guiRequestTest(t,g,guiRequest{
  Operation:"task_cancel",Handle:handle,
  Confirmation:"CANCEL TASK "+handle[:12],
 },g.origin,g.key)
 if status!=200||!cancel.OK||g.used!=5||!g.managedCancelRequested{
  t.Fatalf("host-approved Cancel denied by emergency quota: %+v",cancel)
 }
 before=len(remote.calls)
 status,denied=guiRequestTest(t,g,guiRequest{Operation:"git_status"},g.origin,g.key)
 if status!=400||denied.OK||len(remote.calls)!=before{
  t.Fatal("last status-verification round was spent on Git")
 }
 status,verified:=guiRequestTest(t,g,guiRequest{
  Operation:"task_status",Handle:handle,
 },g.origin,g.key)
 if status!=200||!verified.OK||g.used!=remoterelay.MaxAppPingRounds||
  !strings.Contains(verified.Text,"TASK_STATE stopped")||
  g.managedHandle!=""||g.managedCancelRequested{
  t.Fatalf("authenticated task-stop status was not released: %+v",verified)
 }
 // Exact E2EE budget remains bounded even after task completion.
 status,denied=guiRequestTest(t,g,guiRequest{Operation:"git_status"},g.origin,g.key)
 if status!=400||denied.OK||g.used!=remoterelay.MaxAppPingRounds{
  t.Fatal("task completion expanded session E2EE round quota")
 }
}

func TestNativeGUIManagedTaskStartCannotExhaustCancelQuota(t *testing.T){
 g,remote:=newTestNativeGUI()
 g.used=3
 status,denied:=guiRequestTest(t,g,guiRequest{
  Operation:"task_review",TaskID:1,Mode:"start",
 },g.origin,g.key)
 if status!=400||denied.OK||g.used!=3||len(remote.calls)!=0{
  t.Fatalf("unsafe managed review consumed E2EE round: %+v",denied)
 }
 // Synchronous run review is still permitted with only two rounds left:
 // it does not leave a managed process requiring a second Cancel request.
 status,review:=guiRequestTest(t,g,guiRequest{
  Operation:"task_review",TaskID:1,Mode:"run",
 },g.origin,g.key)
 if status!=200||!review.OK||g.used!=4||len(remote.calls)!=1{
  t.Fatalf("safe synchronous run review refused: %+v",review)
 }
}

func TestNativeGUIManagedReceiptRejectsSpoofedStatusAndHandle(t *testing.T){
 handle:=strings.Repeat("a",32)
 valid:="TASK_HANDLE "+handle+
  "\nTASK_STATE running\nTASK_EXIT_CODE -1\n-- TASK OUTPUT --\n"
 h,state,ok:=nativeManagedTaskReceipt(valid)
 if !ok||h!=handle||state!="running"{t.Fatal("valid canonical receipt refused")}
 for _,broken:=range []string{
  "TASK_HANDLE "+strings.Repeat("b",31)+"\nTASK_STATE running\nTASK_EXIT_CODE -1\n-- TASK OUTPUT --\n",
  strings.Replace(valid,"TASK_EXIT_CODE -1","TASK_EXIT_CODE 0",1),
  strings.Replace(valid,"TASK_STATE running","TASK_STATE rooted",1),
  strings.Replace(valid,"TASK_HANDLE "," TASK_HANDLE ",1),
  strings.Replace(valid,"-- TASK OUTPUT --","-- EXECUTE --",1),
  valid+"injected output",
  valid+"TASK_HANDLE "+strings.Repeat("b",32),
 }{
  if _,_,ok:=nativeManagedTaskReceipt(broken);ok{
   t.Fatalf("forged managed status admitted: %q",broken)
  }
 }
}


func TestNativeGUIRejectedSecondReviewInvalidatesFirstConfirmation(t *testing.T){
 g,remote:=newTestNativeGUI()
 status,review:=guiRequestTest(t,g,guiRequest{
  Operation:"task_review",Mode:"run",TaskID:1,
 },g.origin,g.key)
 if status!=200||!review.OK||review.Confirm!="RUN TASK 1"{t.Fatalf("initial review: %+v",review)}
 status,denied:=guiRequestTest(t,g,guiRequest{
  Operation:"task_review",Mode:"not-allowed",TaskID:1,
 },g.origin,g.key)
 if status!=400||denied.OK{t.Fatalf("invalid replacement review accepted: %+v",denied)}
 status,denied=guiRequestTest(t,g,guiRequest{
  Operation:"task_execute",Mode:"run",TaskID:1,Confirmation:review.Confirm,
 },g.origin,g.key)
 if status!=400||denied.OK||len(remote.calls)!=1{
  t.Fatalf("old approval survived rejected review: %+v calls=%v",denied,remote.calls)
 }
}

func TestNativeGUIManagedReceiptOmittedTerminalOutputIsExplicitAndStrict(t *testing.T){
 handle:=strings.Repeat("b",32)
 valid:="TASK_HANDLE "+handle+"\nTASK_STATE exited\nTASK_EXIT_CODE 0\n"+
  "TASK_OUTPUT_OMITTED yes\n-- TASK OUTPUT --\n"
 if h,state,ok:=nativeManagedTaskReceipt(valid);!ok||h!=handle||state!="exited"{
  t.Fatal("host-authenticated omitted terminal receipt rejected")
 }
 for _,bad:=range []string{
  strings.Replace(valid,"TASK_OUTPUT_OMITTED yes","TASK_OUTPUT_OMITTED no",1),
  strings.Replace(valid,"TASK_STATE exited","TASK_STATE running",1),
  valid+"partial unverified stdout",
  strings.Replace(valid,"TASK_EXIT_CODE 0","TASK_EXIT_CODE 999",1),
  strings.Replace(valid,"TASK_EXIT_CODE 0","TASK_EXIT_CODE +0",1),
 }{
  if _,_,ok:=nativeManagedTaskReceipt(bad);ok{
   t.Fatalf("invalid managed omission/status accepted: %q",bad)
  }
 }
}
