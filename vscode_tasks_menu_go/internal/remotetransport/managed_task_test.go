package remotetransport

import (
 "bytes"
 "strings"
 "testing"
)

func TestRemoteManagedTaskStrictRequestsAndEncryptedResult(t *testing.T){
 const id="0123456789abcdef0123456789abcdef"
 handle:=strings.Repeat("a",32)
 for _,a:=range []RemoteAction{
  {RequestID:id,Operation:RemoteActionTaskManagedStart,TaskID:7},
  {RequestID:id,Operation:RemoteActionTaskManagedStatus,Path:handle},
  {RequestID:id,Operation:RemoteActionTaskManagedCancel,Path:handle},
 }{
  client,host,_:=participants(t)
  frame,err:=SealRemoteAction(client,a)
  if err!=nil{t.Fatal(err)}
  if bytes.Contains(frame,[]byte(handle))||bytes.Contains(frame,[]byte(a.Operation)){
   t.Fatal("Relay saw managed task operation or handle")
  }
  opened,err:=OpenRemoteAction(host,frame)
  if err!=nil||opened.Operation!=a.Operation||opened.TaskID!=a.TaskID||opened.Path!=a.Path{
   t.Fatalf("managed action lost E2EE fields: %+v %v",opened,err)
  }
 }
 client,host,_:=participants(t)
 report:=RemoteManagedTaskResult{
  Handle:handle,State:"exited",ExitCode:9,Output:"Process says EXIT 0\n"+strings.Repeat("x",620),
 }
 frame,err:=SealRemoteManagedTaskResult(host,id,report)
 if err!=nil{t.Fatal(err)}
 if bytes.Contains(frame,[]byte("Process says")){t.Fatal("Relay saw stdout")}
 got,err:=OpenRemoteManagedTaskResult(client,frame,id)
 if err!=nil||got!=report{t.Fatalf("wrong E2EE TaskDeck process status: %+v %v",got,err)}
 if _,err:=OpenRemoteManagedTaskResult(client,frame,id);err==nil{t.Fatal("replay accepted")}
}
func TestRemoteManagedTaskRejectsForgedResultDomainAndInvalidState(t *testing.T){
 const id="0123456789abcdef0123456789abcdef"
 handle:=strings.Repeat("a",32)
 for _,r:=range []RemoteManagedTaskResult{
  {Handle:handle,State:"running",ExitCode:0},
  {Handle:handle,State:"running",ExitCode:-1,Output:"secret"},
  {Handle:handle,State:"exited",ExitCode:-1},
  {Handle:handle,State:"exited",ExitCode:256},
  {Handle:handle,State:"cancel_requested",ExitCode:0},
  {Handle:"bad",State:"running",ExitCode:-1},
  {Handle:handle,State:"exited",ExitCode:0,Output:strings.Repeat("x",RemoteManagedTaskMaxOutput+1)},
  {Handle:handle,State:"exited",ExitCode:0,Output:string([]byte{0xff})},
  {Handle:handle,State:"running",ExitCode:-1,OutputOmitted:true},
  {Handle:handle,State:"cancel_requested",ExitCode:-1,OutputOmitted:true},
  {Handle:handle,State:"exited",ExitCode:0,Output:"partial",OutputOmitted:true},
  {Handle:handle,State:"stopped",ExitCode:-1,Output:"tail",OutputOmitted:true},
 }{
  _,host,_:=participants(t)
  if frame,err:=SealRemoteManagedTaskResult(host,id,r);err==nil||len(frame)>0{
   t.Fatalf("invalid report accepted: %+v",r)
  }
 }
 client,host,_:=participants(t)
 other,err:=SealRemoteTaskReportResult(host,id,RemoteTaskReport{Status:"exited",ExitCode:0,Output:"done"})
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteManagedTaskResult(client,other,id);err==nil||client.Established(){
  t.Fatal("task.run_report receipt was confused with managed process status")
 }
}
func TestRemoteManagedTaskRejectsArbitraryPIDAndCommandInjection(t *testing.T){
 const id="0123456789abcdef0123456789abcdef"
 handle:=strings.Repeat("f",32)
 for _,action:=range []RemoteAction{
  {RequestID:id,Operation:RemoteActionTaskManagedStart,TaskID:7,Content:"id"},
  {RequestID:id,Operation:RemoteActionTaskManagedStart,TaskID:0},
  {RequestID:id,Operation:RemoteActionTaskManagedStatus,Path:"12345"},
  {RequestID:id,Operation:RemoteActionTaskManagedCancel,Path:"/proc/1"},
  {RequestID:id,Operation:RemoteActionTaskManagedCancel,Path:handle,Content:"kill -9 1"},
  {RequestID:id,Operation:RemoteActionTaskManagedCancel,Path:handle,TaskID:2},
 }{
  client,_,_:=participants(t)
  if frame,err:=SealRemoteAction(client,action);err==nil||len(frame)>0{
   t.Fatalf("untrusted process argument accepted: %+v",action)
  }
 }
 client,host,_:=participants(t)
 forged:=[]byte(`{"type":"taskdeck-e2ee-action-v1","request_id":"`+id+`","operation":"task.managed_cancel","path":"`+handle+`","pid":1}`)
 frame,err:=client.Seal(forged)
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteAction(host,frame);err==nil||host.Established(){
  t.Fatal("injected PID field accepted")
 }
}

func TestRemoteManagedTaskOmittedOutputExplicitEncryptedReceipt(t *testing.T){
 const id="1123456789abcdef0123456789abcdef"
 handle:=strings.Repeat("a",32)
 for _,tc:=range []RemoteManagedTaskResult{
  {Handle:handle,State:"exited",ExitCode:0,OutputOmitted:true},
  {Handle:handle,State:"exited",ExitCode:11,OutputOmitted:true},
  {Handle:handle,State:"stopped",ExitCode:-1,OutputOmitted:true},
 }{
  client,host,_:=participants(t)
  frame,err:=SealRemoteManagedTaskResult(host,id,tc)
  if err!=nil{t.Fatal(err)}
  if bytes.Contains(frame,[]byte("output_omitted")){t.Fatal("omission metadata visible to relay")}
  got,err:=OpenRemoteManagedTaskResult(client,frame,id)
  if err!=nil||got!=tc{t.Fatalf("typed encrypted omission lost: got %+v, error %v",got,err)}
 }
 // A malicious peer that has an established E2EE channel but sends a
 // forged inconsistent terminal receipt is rejected and the channel closed.
 client,host,_:=participants(t)
 forged:=[]byte(`{"type":"taskdeck-e2ee-managed-task-result-v1",
 "request_id":"`+id+`","ok":true,"handle":"`+handle+`",
 "state":"exited","exit_code":0,"output":"secret",
 "output_omitted":true,"remote_control_available":false}`)
 frame,err:=host.Seal(forged)
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteManagedTaskResult(client,frame,id);err==nil||client.Established(){
  t.Fatal("forged partial stdout was misrepresented as omitted")
 }
}


func TestRemoteManagedTaskLegacyExactResultCompatibility(t *testing.T){
 const id="2123456789abcdef0123456789abcdef"
 handle:=strings.Repeat("b",32)
 // Existing Agent binaries send the original strict envelope, which does
 // not include output_omitted. Mixed-version native clients must keep
 // accepting a normal result without ever accepting extra untrusted keys.
 client,host,_:=participants(t)
 legacy:=[]byte(`{"type":"taskdeck-e2ee-managed-task-result-v1",
 "request_id":"`+id+`","ok":true,"handle":"`+handle+`",
 "state":"exited","exit_code":0,"output":"complete",
 "remote_control_available":false}`)
 frame,err:=host.Seal(legacy)
 if err!=nil{t.Fatal(err)}
 got,err:=OpenRemoteManagedTaskResult(client,frame,id)
 if err!=nil||got.Output!="complete"||got.OutputOmitted||got.State!="exited"{
  t.Fatalf("legacy exact result failed compatibility: %+v %v",got,err)
 }
 client2,host2,_:=participants(t)
 forged:=[]byte(`{"type":"taskdeck-e2ee-managed-task-result-v1",
 "request_id":"`+id+`","ok":true,"handle":"`+handle+`",
 "state":"exited","exit_code":0,"output":"complete",
 "output_omitted":"maybe","remote_control_available":false}`)
 bad,err:=host2.Seal(forged)
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteManagedTaskResult(client2,bad,id);err==nil||client2.Established(){
  t.Fatal("unexpected omission type bypassed strict result schema")
 }
}
