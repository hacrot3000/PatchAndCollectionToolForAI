package main

import (
 "fmt"
 "strings"
 "testing"
 "time"
)

func guiLinePage(start int64,source string)string {
 return "FILE_SHA256 "+strings.Repeat("a",64)+
  fmt.Sprintf("\nSTART_LINE %d\nNEXT_LINE 0\nTOTAL_LINES %d\n-- COMPLETE SOURCE LINES --\n%d: %s\n",
   start,start,start,source)
}

func TestNativeGUILargeLineReviewAndCASWrites(t *testing.T){
 for _,tc:=range []struct{mode,content,operation string}{
  {"edit","tiếng Việt","large-edit"},
  {"insert","insert before","large-insert"},
  {"delete","","large-delete"},
 }{
  t.Run(tc.mode,func(t *testing.T){
   g,f:=newTestNativeGUI()
   f.linePageOverride=guiLinePage(17,"original source")
   code,review:=guiRequestTest(t,g,guiRequest{
    Operation:"line_write_review",Path:"src/file.txt",
    Mode:tc.mode,Offset:17,
   },g.origin,g.key)
   if code!=200||!review.OK||review.Used!=1||review.Max!=6||
    review.Text!=f.linePageOverride||review.Line!="original source"||
    !strings.HasSuffix(review.Confirm," "+strings.Repeat("a",12))||
    len(f.calls)!=1||f.calls[0]!="lines:src/file.txt"{
    t.Fatalf("invalid authenticated page review: %d %+v %v",code,review,f.calls)
   }
   code,done:=guiRequestTest(t,g,guiRequest{
    Operation:"line_write_execute",Path:"src/file.txt",
    Mode:tc.mode,Offset:17,Content:tc.content,Confirmation:review.Confirm,
   },g.origin,g.key)
   expected:=tc.operation+":src/file.txt:17:"
   if tc.mode=="delete"{expected+=strings.Repeat("a",64)
   }else{expected+=tc.content+":"+strings.Repeat("a",64)}
   if code!=200||!done.OK||done.Used!=2||
    len(f.calls)!=2||f.calls[1]!=expected{
    t.Fatalf("unsafe or missing exact-host CAS write: %d %+v %v wanted %s",
     code,done,f.calls,expected)
   }
   code,replay:=guiRequestTest(t,g,guiRequest{
    Operation:"line_write_execute",Path:"src/file.txt",
    Mode:tc.mode,Offset:17,Content:tc.content,Confirmation:review.Confirm,
   },g.origin,g.key)
   if code!=400||replay.OK||len(f.calls)!=2{
    t.Fatalf("one-use review replay executed: %d %+v",code,replay)
   }
  })
 }
}

func TestNativeGUILargeLineRejectsPathTraversalAndUnsupportedRequests(t *testing.T){
 g,f:=newTestNativeGUI()
 invalid:=[]guiRequest{
  {Operation:"line_write_execute",Path:"src/code.go",Mode:"edit",Offset:2,
   Content:"bypass",Confirmation:"EDIT LINE 2 aaaa"},
  {Operation:"line_write_review",Path:".git/config",Mode:"edit",Offset:2},
  {Operation:"line_write_review",Path:"src/.private",Mode:"edit",Offset:2},
  {Operation:"line_write_review",Path:"src/../secret",Mode:"edit",Offset:2},
  {Operation:"line_write_review",Path:"/etc/passwd",Mode:"edit",Offset:2},
  {Operation:"line_write_review",Path:"src/file",Mode:"shell",Offset:2},
  {Operation:"line_write_review",Path:"src/file",Mode:"edit",Offset:0},
  {Operation:"line_write_review",Path:"src/file",Mode:"edit",Offset:2097153},
  {Operation:"line_write_review",Path:"src/file",Mode:"edit",Offset:2,Content:"stolen"},
  {Operation:"line_write_review",Path:"src/file",Mode:"edit",Offset:2,Literal:"x"},
  {Operation:"line_write_review",Path:"src/file",Mode:"edit",Offset:2,Message:"git push"},
  {Operation:"git_status",Content:"secret"},
 }
 for _,req:=range invalid{
  code,res:=guiRequestTest(t,g,req,g.origin,g.key)
  if code!=400||res.OK||res.Used!=0||len(f.calls)!=0{
   t.Fatalf("invalid line operation reached host: %+v %d %+v %v",req,code,res,f.calls)
  }
 }
 code,res:=guiRequestTest(t,g,guiRequest{
  Operation:"line_write_review",Path:"src/code.go",Mode:"delete",Offset:1,
 },"https://hacked-php.invalid",g.key)
 if code!=403||res.OK||len(f.calls)!=0{
  t.Fatal("compromised PHP origin could access authenticated source")
 }
}

func TestNativeGUILargeLineFailsClosedOnBadOrExpiredReview(t *testing.T){
 for _,source:=range []string{
  "FILE_SHA256 bad\nSTART_LINE 1\nNEXT_LINE 0\nTOTAL_LINES 1\n-- COMPLETE SOURCE LINES --\n1: x\n",
  guiLinePage(1,"bad\x00text"),
  guiLinePage(1,strings.Repeat("L",550)),
  strings.Repeat("Q",4097),
  strings.Replace(guiLinePage(1,"a"),"NEXT_LINE 0","NEXT_LINE 2",1),
 }{
  g,f:=newTestNativeGUI()
  f.linePageOverride=source
  code,res:=guiRequestTest(t,g,guiRequest{
   Operation:"line_write_review",Path:"file.txt",Offset:1,Mode:"edit",
  },g.origin,g.key)
  if code!=400||res.OK||res.Confirm!=""||res.Used!=1||len(f.calls)!=1{
   t.Fatalf("accepted forged/truncated page: %d %+v",code,res)
  }
 }
 g,f:=newTestNativeGUI()
 f.linePageOverride=guiLinePage(1,"old")
 _,review:=guiRequestTest(t,g,guiRequest{
  Operation:"line_write_review",Path:"file.txt",Offset:1,Mode:"edit",
 },g.origin,g.key)
 code,res:=guiRequestTest(t,g,guiRequest{
  Operation:"line_write_execute",Path:"file.txt",Offset:1,Mode:"insert",
  Content:"new",Confirmation:review.Confirm,
 },g.origin,g.key)
 if code!=400||res.OK||len(f.calls)!=1{
  t.Fatal("switching reviewed mode caused write")
 }
 _,review=guiRequestTest(t,g,guiRequest{
  Operation:"line_write_review",Path:"file.txt",Offset:1,Mode:"edit",
 },g.origin,g.key)
 code,res=guiRequestTest(t,g,guiRequest{
  Operation:"line_write_execute",Path:"file.txt",Offset:1,Mode:"edit",
  Content:"old",Confirmation:review.Confirm,
 },g.origin,g.key)
 if code!=400||res.OK||len(f.calls)!=2{
  t.Fatal("unchanged edit wrote anyway")
 }
 _,review=guiRequestTest(t,g,guiRequest{
  Operation:"line_write_review",Path:"file.txt",Offset:1,Mode:"delete",
 },g.origin,g.key)
 code,res=guiRequestTest(t,g,guiRequest{
  Operation:"line_write_execute",Path:"file.txt",Offset:1,Mode:"delete",
  Content:"not-empty",Confirmation:review.Confirm,
 },g.origin,g.key)
 if code!=400||res.OK||len(f.calls)!=3{
  t.Fatal("delete-smuggled content wrote anyway")
 }
 _,review=guiRequestTest(t,g,guiRequest{
  Operation:"line_write_review",Path:"file.txt",Offset:1,Mode:"edit",
 },g.origin,g.key)
 g.mu.Lock()
 g.pendingLine.expiry=time.Now().Add(-time.Second)
 g.mu.Unlock()
 code,res=guiRequestTest(t,g,guiRequest{
  Operation:"line_write_execute",Path:"file.txt",Offset:1,Mode:"edit",
  Content:"updated",Confirmation:review.Confirm,
 },g.origin,g.key)
 if code!=400||res.OK||len(f.calls)!=4{
  t.Fatal("expired review allowed mutation")
 }
}

func TestNativeGUILargeLineReservesRoundQuota(t *testing.T){
 g,f:=newTestNativeGUI()
 for i:=0;i<5;i++{guiRequestTest(t,g,guiRequest{
  Operation:"git_status",
 },g.origin,g.key)}
 code,res:=guiRequestTest(t,g,guiRequest{
  Operation:"line_write_review",Path:"file.txt",Mode:"edit",Offset:1,
 },g.origin,g.key)
 if code!=400||res.OK||res.Used!=5||len(f.calls)!=5{
  t.Fatalf("review consumed final E2EE slot: %d %+v %v",code,res,f.calls)
 }
}
