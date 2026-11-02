package main

import (
 "crypto/sha256"
 "encoding/hex"
 "strings"
 "testing"
 "time"
)

func guiSHA256(s string)string{
 h:=sha256.Sum256([]byte(s))
 return hex.EncodeToString(h[:])
}

func TestNativeGUIFileEditRequiresReviewedE2EEPreviewAndExactCAS(t *testing.T){
 g,f:=newTestNativeGUI()
 path:="src/hello.txt"
 status,rev:=guiRequestTest(t,g,guiRequest{Operation:"file_edit_review",Path:path},g.origin,g.key)
 if status!=200||!rev.OK||rev.Used!=1||rev.Max!=6||rev.Text==""||
  rev.Confirm!="EDIT "+path+" "+guiSHA256(rev.Text)[:12]{
  t.Fatalf("invalid authenticated edit review: %d %+v",status,rev)
 }
 next:="first line\nsecond line with Vietnamese UTF-8: tiếng Việt\n"
 status,done:=guiRequestTest(t,g,guiRequest{Operation:"file_edit_execute",
  Path:path,Content:next,Confirmation:rev.Confirm},g.origin,g.key)
 expected:="file:replace:"+path+":"+guiSHA256(next)+":"+guiSHA256(rev.Text)
 if status!=200||!done.OK||done.Used!=2||len(f.calls)!=2||f.calls[1]!=expected{
  t.Fatalf("unreviewed or wrong CAS file mutation: %d %+v %v",status,done,f.calls)
 }
 status,done=guiRequestTest(t,g,guiRequest{Operation:"file_edit_execute",
  Path:path,Content:"replay",Confirmation:rev.Confirm},g.origin,g.key)
 if status!=400||done.OK||len(f.calls)!=2{
  t.Fatal("replayed file edit confirmation mutated workspace")
 }
}

func TestNativeGUIFileEditValidationBeforeE2EE(t *testing.T){
 g,f:=newTestNativeGUI()
 invalid:=[]guiRequest{
  {Operation:"file_edit_execute",Path:"source.txt",Content:"write",Confirmation:"EDIT source.txt"},
  {Operation:"file_edit_review",Path:".git/config"},
  {Operation:"file_edit_review",Path:".GIT/config"},
  {Operation:"file_edit_review",Path:"src/.hidden"},
  {Operation:"file_edit_review",Path:"../secrets"},
  {Operation:"file_edit_review",Path:"/etc/passwd"},
  {Operation:"file_edit_review",Path:"a/../b"},
  {Operation:"file_edit_review",Path:"file.txt",Content:"unreviewed"},
  {Operation:"file_edit_review",Path:"file.txt",Message:"extra"},
  {Operation:"file_edit_review",Path:"file.txt",Mode:"overwrite"},
  {Operation:"git_status",Content:"secret"},
  {Operation:"task_review",TaskID:1,Mode:"run",Content:"secret"},
 }
 for _,req:=range invalid{
  status,res:=guiRequestTest(t,g,req,g.origin,g.key)
  if status!=400||res.OK||res.Used!=0||len(f.calls)!=0{
   t.Fatalf("invalid file edit bypassed local authorization: %+v %d %+v %v",
    req,status,res,f.calls)
  }
 }
 status,res:=guiRequestTest(t,g,guiRequest{
  Operation:"file_edit_review",Path:"source.txt",
 },"https://hacked-php.invalid",g.key)
 if status!=403||res.OK||len(f.calls)!=0{
  t.Fatal("compromised PHP origin started file mutation")
 }
}

func TestNativeGUIFileEditOneUseWrongOrExpiredConfirmation(t *testing.T){
 g,f:=newTestNativeGUI()
 _,rev:=guiRequestTest(t,g,guiRequest{Operation:"file_edit_review",
  Path:"report.txt"},g.origin,g.key)
 status,res:=guiRequestTest(t,g,guiRequest{Operation:"file_edit_execute",
  Path:"report.txt",Content:"new",Confirmation:"EDIT report.txt fake"},
  g.origin,g.key)
 if status!=400||res.OK||len(f.calls)!=1{
  t.Fatal("wrong file review confirmation caused a write")
 }
 status,res=guiRequestTest(t,g,guiRequest{Operation:"file_edit_execute",
  Path:"report.txt",Content:"new",Confirmation:rev.Confirm},g.origin,g.key)
 if status!=400||res.OK||len(f.calls)!=1{
  t.Fatal("invalid confirmation failed to consume one-use review")
 }
 _,rev=guiRequestTest(t,g,guiRequest{Operation:"file_edit_review",
  Path:"report.txt"},g.origin,g.key)
 g.mu.Lock()
 g.pendingFile.expiry=time.Now().Add(-time.Second)
 g.mu.Unlock()
 status,res=guiRequestTest(t,g,guiRequest{Operation:"file_edit_execute",
  Path:"report.txt",Content:"new",Confirmation:rev.Confirm},g.origin,g.key)
 if status!=400||res.OK||len(f.calls)!=2{
  t.Fatal("expired review mutated a file")
 }
}

func TestNativeGUIFileEditRefusesOversizedInvalidAndUnchangedText(t *testing.T){
 for _,variant:=range []struct{name,text string}{
  {"large",strings.Repeat("x",4097)},
  {"nul","x\x00y"},
  {"unchanged","HOST-E2EE: view:notes.txt"},
 }{
  t.Run(variant.name,func(t *testing.T){
   g,f:=newTestNativeGUI()
   _,rev:=guiRequestTest(t,g,guiRequest{Operation:"file_edit_review",
    Path:"notes.txt"},g.origin,g.key)
   status,res:=guiRequestTest(t,g,guiRequest{Operation:"file_edit_execute",
    Path:"notes.txt",Content:variant.text,Confirmation:rev.Confirm},g.origin,g.key)
   if status!=400||res.OK||len(f.calls)!=1{
    t.Fatalf("unsafe content wrote to remote host: %+v %d %+v %v",
     variant,status,res,f.calls)
   }
  })
 }
}

func TestNativeGUIFileEditRejectsMalformedAuthenticatedPreview(t *testing.T){
 for _,bad:=range []string{strings.Repeat("A",4097),"bad\x00input",string([]byte{0xff,0xfe})}{
  g,f:=newTestNativeGUI()
  f.previewOverride=bad
  status,res:=guiRequestTest(t,g,guiRequest{Operation:"file_edit_review",
   Path:"notes.txt"},g.origin,g.key)
  if status!=400||res.OK||res.Confirm!=""||res.Used!=1{
   t.Fatalf("malformed preview passed verification: %d %+v",status,res)
  }
  status,res=guiRequestTest(t,g,guiRequest{Operation:"file_edit_execute",
   Path:"notes.txt",Content:"changed",Confirmation:"EDIT notes.txt fake"},g.origin,g.key)
  if status!=400||res.OK||len(f.calls)!=1{
   t.Fatal("malformed file preview granted edit capability")
  }
 }
}

func TestNativeGUIFileEditQuotaRequiresBothRounds(t *testing.T){
 g,f:=newTestNativeGUI()
 for i:=0;i<5;i++{guiRequestTest(t,g,guiRequest{Operation:"git_status"},g.origin,g.key)}
 status,res:=guiRequestTest(t,g,guiRequest{Operation:"file_edit_review",
  Path:"notes.txt"},g.origin,g.key)
 if status!=400||res.OK||res.Used!=5||len(f.calls)!=5{
  t.Fatal("file edit review consumed last round without ability to write")
 }
}
