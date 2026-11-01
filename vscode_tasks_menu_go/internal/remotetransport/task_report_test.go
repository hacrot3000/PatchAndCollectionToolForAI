package remotetransport

import (
 "bytes"
 "strings"
 "testing"
)

func TestRemoteTaskReportAuthenticatedExitCodeAndTranscript(t *testing.T){
 const id="0123456789abcdef0123456789abcdef"
 client,host,_:=participants(t)
 packet,err:=SealRemoteAction(client,RemoteAction{
  RequestID:id,Operation:RemoteActionTaskRunReport,TaskID:17,
 })
 if err!=nil{t.Fatal(err)}
 if bytes.Contains(packet,[]byte("task.run_report")){t.Fatal("relay saw plaintext task ID")}
 action,err:=OpenRemoteAction(host,packet)
 if err!=nil||action.Operation!=RemoteActionTaskRunReport||action.TaskID!=17{
  t.Fatalf("wrong report action %+v %v",action,err)
 }
 value:=RemoteTaskReport{Status:"exited",ExitCode:7,Output:"fake text EXIT 0\r\n"+strings.Repeat("Z",640)}
 sealed,err:=SealRemoteTaskReportResult(host,id,value)
 if err!=nil{t.Fatal(err)}
 if bytes.Contains(sealed,[]byte("fake text")){t.Fatal("relay saw task output")}
 opened,err:=OpenRemoteTaskReportResult(client,sealed,id)
 if err!=nil||opened!=value{t.Fatalf("false exit report %+v %v",opened,err)}
 if _,err:=OpenRemoteTaskReportResult(client,sealed,id);err==nil{
  t.Fatal("replayed task report")
 }
}

func TestRemoteTaskReportRejectsWrongDomainAndInvalidExitStatus(t *testing.T){
 const id="0123456789abcdef0123456789abcdef"
 for _,v:=range []RemoteTaskReport{
  {Status:"running",ExitCode:0,Output:"x"},
  {Status:"stopped",ExitCode:0,Output:"x"},
  {Status:"exited",ExitCode:-1,Output:"x"},
  {Status:"exited",ExitCode:256,Output:"x"},
  {Status:"exited",ExitCode:1,Output:strings.Repeat("x",RemoteTaskReportMaxOutput+1)},
  {Status:"exited",ExitCode:1,Output:string([]byte{0xff})},
 }{
  _,host,_:=participants(t)
  if b,err:=SealRemoteTaskReportResult(host,id,v);err==nil||len(b)!=0{
   t.Fatalf("unbounded/forged process report was sealed: %+v",v)
  }
 }
 client,host,_:=participants(t)
 wire,err:=SealRemoteTaskRunTextResult(host,id,[]byte("EXIT 0"))
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteTaskReportResult(client,wire,id);err==nil||client.Established(){
  t.Fatal("old unstructured task output spoofed authoritative exit status")
 }
}

func TestRemoteTaskReportRejectsInjectedRequestKeys(t *testing.T){
 const id="0123456789abcdef0123456789abcdef"
 client,host,_:=participants(t)
 forged:=[]byte(`{"type":"taskdeck-e2ee-action-v1","request_id":"`+id+`","operation":"task.run_report","task_id":1,"shell":"rm -rf /"}`)
 packet,err:=client.Seal(forged)
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteAction(host,packet);err==nil||host.Established(){
  t.Fatal("untrusted shell key accepted in task report")
 }
}
