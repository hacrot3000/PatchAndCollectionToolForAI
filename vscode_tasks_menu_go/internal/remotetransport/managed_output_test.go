package remotetransport

import (
 "bytes"
 "strings"
 "testing"
)

func TestRemoteManagedOutputStrictE2EERequestAndResult(t *testing.T){
 id:="0123456789abcdef0123456789abcdef"
 handle:=strings.Repeat("b",32)
 client,host,_:=participants(t)
 action:=RemoteAction{RequestID:id,Operation:RemoteActionTaskManagedOutput,Path:handle,Offset:2048}
 frame,err:=SealRemoteAction(client,action)
 if err!=nil{t.Fatal(err)}
 if bytes.Contains(frame,[]byte(handle))||bytes.Contains(frame,[]byte("task.managed_output")){
  t.Fatal("untrusted relay saw managed output handle")
 }
 got,err:=OpenRemoteAction(host,frame)
 if err!=nil||got.Path!=handle||got.Offset!=2048||got.Operation!=RemoteActionTaskManagedOutput{
  t.Fatalf("wrong task window request %+v %v",got,err)
 }
 receipt:=RemoteManagedOutput{
  Handle:handle,Offset:2048,Next:2055,Total:2055,
  State:"exited",EOF:true,Data:"abcdefg",
 }
 encrypted,err:=SealRemoteManagedOutput(host,id,receipt)
 if err!=nil{t.Fatal(err)}
 if bytes.Contains(encrypted,[]byte("abcdefg")){t.Fatal("task stdout plaintext leaked")}
 decoded,err:=OpenRemoteManagedOutput(client,encrypted,id)
 if err!=nil||decoded!=receipt{t.Fatalf("invalid output cursor %+v %v",decoded,err)}
 if _,err:=OpenRemoteManagedOutput(client,encrypted,id);err==nil{t.Fatal("E2EE replay accepted")}
}
func TestManagedOutputRejectsMalformedCursorAndForeignReply(t *testing.T){
 id:="0123456789abcdef0123456789abcdef"
 h:=strings.Repeat("b",32)
 for _,action:=range []RemoteAction{
  {RequestID:id,Operation:RemoteActionTaskManagedOutput,Path:"pid:1234",Offset:0},
  {RequestID:id,Operation:RemoteActionTaskManagedOutput,Path:h,Offset:-1},
  {RequestID:id,Operation:RemoteActionTaskManagedOutput,Path:h,Offset:RemoteManagedOutputMaxOffset+1},
  {RequestID:id,Operation:RemoteActionTaskManagedOutput,Path:h,Offset:0,Content:"kill -9 1"},
  {RequestID:id,Operation:RemoteActionTaskManagedOutput,Path:h,TaskID:17},
 }{
  client,_,_:=participants(t)
  if frame,err:=SealRemoteAction(client,action);err==nil||len(frame)>0{
   t.Fatalf("unsafe output request accepted %+v",action)
  }
 }
 for _,v:=range []RemoteManagedOutput{
  {Handle:h,Offset:0,Next:3,Total:4,State:"running",Data:"ab"},
  {Handle:h,Offset:0,Next:2,Total:1,State:"running",Data:"ab"},
  {Handle:h,Offset:0,Next:2,Total:2,State:"exited",EOF:false,Data:"ab"},
  {Handle:h,Offset:0,Next:2,Total:3,State:"running",EOF:true,Data:"ab"},
  {Handle:h,Offset:0,Next:0,Total:0,State:"unknown"},
  {Handle:h,Offset:0,Next:2049,Total:2049,State:"running",Data:strings.Repeat("x",2049)},
 }{
  _,host,_:=participants(t)
  if b,err:=SealRemoteManagedOutput(host,id,v);err==nil||len(b)>0{
   t.Fatalf("invalid cursor receipt accepted %+v",v)
  }
 }
 client,host,_:=participants(t)
 other,err:=SealRemoteManagedTaskResult(host,id,RemoteManagedTaskResult{Handle:h,State:"running",ExitCode:-1})
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteManagedOutput(client,other,id);err==nil||client.Established(){
  t.Fatal("task status spoofed output window")
 }
}
