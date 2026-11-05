package main

import (
 "testing"
)

func TestNativeGUIReadActionInvalidatesFileEditReview(t *testing.T){
 g,f:=newTestNativeGUI()
 _,review:=guiRequestTest(t,g,guiRequest{Operation:"file_edit_review",Path:"docs/notice.txt"},g.origin,g.key)
 if !review.OK||review.Confirm==""{t.Fatal("missing approved file read")}
 status,read:=guiRequestTest(t,g,guiRequest{Operation:"git_status"},g.origin,g.key)
 if status!=200||!read.OK{t.Fatal("unrelated read must remain allowed")}
 status,res:=guiRequestTest(t,g,guiRequest{Operation:"file_edit_execute",
  Path:"docs/notice.txt",Content:"overwrite",Confirmation:review.Confirm},g.origin,g.key)
 if status!=400||res.OK||len(f.calls)!=2{
  t.Fatalf("stale file edit confirmation survived unrelated action: %d %+v %v",status,res,f.calls)
 }
}

func TestNativeGUIReadActionInvalidatesGitWriteReview(t *testing.T){
 g,f:=newTestNativeGUI()
 _,review:=guiRequestTest(t,g,guiRequest{Operation:"git_write_review",Mode:"stage",Path:"src/main.go"},g.origin,g.key)
 if !review.OK||review.Confirm==""{t.Fatal("missing reviewed stage action")}
 status,_:=guiRequestTest(t,g,guiRequest{Operation:"file_view",Path:"src/main.go"},g.origin,g.key)
 if status!=200{t.Fatal("file preview should still work")}
 status,res:=guiRequestTest(t,g,guiRequest{Operation:"git_write_execute",
  Mode:"stage",Path:"src/main.go",Confirmation:review.Confirm},g.origin,g.key)
 if status!=400||res.OK||len(f.calls)!=2{
  t.Fatalf("stale Git index modification authorized: %d %+v %v",status,res,f.calls)
 }
}

func TestNativeGUIReadActionInvalidatesLargeLineReview(t *testing.T){
 g,f:=newTestNativeGUI()
 f.linePageOverride=guiLinePage(1,"current")
 _,review:=guiRequestTest(t,g,guiRequest{Operation:"line_write_review",
  Path:"large.txt",Mode:"edit",Offset:1},g.origin,g.key)
 if !review.OK{t.Fatal("missing SHA-pinned large line review")}
 status,_:=guiRequestTest(t,g,guiRequest{Operation:"git_status"},g.origin,g.key)
 if status!=200{t.Fatal("unrelated metadata read failed")}
 status,res:=guiRequestTest(t,g,guiRequest{Operation:"line_write_execute",
  Path:"large.txt",Mode:"edit",Offset:1,Content:"changed",
  Confirmation:review.Confirm},g.origin,g.key)
 if status!=400||res.OK||len(f.calls)!=2{
  t.Fatalf("stale one-line mutation authorized: %d %+v %v",status,res,f.calls)
 }
}

func TestNativeGUIReadActionInvalidatesReviewedTaskExecution(t *testing.T){
 g,f:=newTestNativeGUI()
 _,review:=guiRequestTest(t,g,guiRequest{Operation:"task_review",Mode:"run",TaskID:1},g.origin,g.key)
 if !review.OK{t.Fatal("missing trusted approved task review")}
 status,_:=guiRequestTest(t,g,guiRequest{Operation:"git_status"},g.origin,g.key)
 if status!=200{t.Fatal("read should still work")}
 status,res:=guiRequestTest(t,g,guiRequest{Operation:"task_execute",
  Mode:"run",TaskID:1,Confirmation:review.Confirm},g.origin,g.key)
 if status!=400||res.OK||len(f.calls)!=2{
  t.Fatalf("stale approved task executed after intervening action: %d %+v %v",status,res,f.calls)
 }
}
