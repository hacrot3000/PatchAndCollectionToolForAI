package remotetransport

import (
 "bytes"
 "strings"
 "testing"
)

func TestLargeLineEditTypedE2EEAndReceiptDomainIsolation(t *testing.T){
 const id="0123456789abcdef0123456789abcdef"
 digest:=strings.Repeat("a",64)
 c,h,_:=participants(t)
 req,err:=SealRemoteAction(c,RemoteAction{
  RequestID:id,Operation:RemoteActionProjectFileEditLine,Path:"source.txt",
  Offset:412,Content:"replacement ✓",ExpectedSHA256:digest})
 if err!=nil{t.Fatal(err)}
 for _,v:=range []string{"source.txt","replacement",digest,"412"}{
  if bytes.Contains(req,[]byte(v)){t.Fatal("untrusted relay saw plaintext edit")}
 }
 action,err:=OpenRemoteAction(h,req)
 if err!=nil||action.Path!="source.txt"||action.Offset!=412||
  action.Content!="replacement ✓"||action.ExpectedSHA256!=digest{
  t.Fatalf("invalid decoded E2EE line edit: %+v %v",action,err)
 }
 receipt:=strings.Repeat("b",64)
 response,err:=SealRemoteLargeLineEditResult(h,id,receipt)
 if err!=nil{t.Fatal(err)}
 got,err:=OpenRemoteLargeLineEditResult(c,response,id)
 if err!=nil||got!=receipt{t.Fatal("wrong line-edit receipt")}
 c,h,_=participants(t)
 response,err=SealRemoteLargeLineEditResult(h,id,receipt)
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteFileReplaceResult(c,response,id);err==nil||c.Established(){
  t.Fatal("large-line receipt spoofed full-file replacement")
 }
}

func TestLargeLineEditRejectsInvalidActionAndForgedFields(t *testing.T){
 const id="0123456789abcdef0123456789abcdef"
 digest:=strings.Repeat("a",64)
 for _,a:=range []RemoteAction{
  {Operation:RemoteActionProjectFileEditLine,Path:"../outside",Offset:1,Content:"ok",ExpectedSHA256:digest},
  {Operation:RemoteActionProjectFileEditLine,Path:".git/config",Offset:1,Content:"ok",ExpectedSHA256:digest},
  {Operation:RemoteActionProjectFileEditLine,Path:"source.txt",Offset:0,Content:"ok",ExpectedSHA256:digest},
  {Operation:RemoteActionProjectFileEditLine,Path:"source.txt",Offset:2<<20+1,Content:"ok",ExpectedSHA256:digest},
  {Operation:RemoteActionProjectFileEditLine,Path:"source.txt",Offset:1,Content:"x\ny",ExpectedSHA256:digest},
  {Operation:RemoteActionProjectFileEditLine,Path:"source.txt",Offset:1,Content:"\x1b[31m",ExpectedSHA256:digest},
  {Operation:RemoteActionProjectFileEditLine,Path:"source.txt",Offset:1,Content:strings.Repeat("x",513),ExpectedSHA256:digest},
  {Operation:RemoteActionProjectFileEditLine,Path:"source.txt",Offset:1,Content:"ok",ExpectedSHA256:"wrong"},
  {Operation:RemoteActionProjectFileEditLine,Path:"source.txt",Offset:1,Content:"ok",ExpectedSHA256:digest,TaskID:2},
 }{
  c,_,_:=participants(t)
  a.RequestID=id
  if frame,err:=SealRemoteAction(c,a);err==nil||len(frame)>0{
   t.Fatalf("accepted unsafe line edit %+v",a)
  }
 }
 c,h,_:=participants(t)
 forged:=[]byte(`{"type":"taskdeck-e2ee-action-v1","request_id":"`+id+`","operation":"project.file.edit_line","path":"source.txt","content":"safe","expected_sha256":"`+digest+`","offset":3,"command":"rm -rf /"}`)
 packet,err:=c.Seal(forged)
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteAction(h,packet);err==nil||h.Established(){
  t.Fatal("injected command key was accepted")
 }
}
