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
  {Status:"exited",ExitCode:1,Output:"partial-tail",OutputOmitted:true},
  {Status:"running",ExitCode:-1,OutputOmitted:true},
  {Status:"stopped",ExitCode:0,OutputOmitted:true},
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

func TestRemoteTaskReportOmittedOutputE2EEAndExactLegacySchema(t *testing.T){
 const id="1123456789abcdef0123456789abcdef"
 for _,code:=range []int{0,9,255}{
  client,host,_:=participants(t)
  expected:=RemoteTaskReport{Status:"exited",ExitCode:code,OutputOmitted:true}
  frame,err:=SealRemoteTaskReportResult(host,id,expected)
  if err!=nil{t.Fatal(err)}
  if bytes.Contains(frame,[]byte("output_omitted")){t.Fatal("Relay saw typed output omission")}
  actual,err:=OpenRemoteTaskReportResult(client,frame,id)
  if err!=nil||actual!=expected{t.Fatalf("typed omission not preserved %+v %v",actual,err)}
 }
 // Existing binary clients emit the original exact wire schema when
 // the entire transcript is available. No implicit migration is needed.
 client,host,_:=participants(t)
 old:=[]byte(`{"type":"taskdeck-e2ee-task-run-report-result-v1",
 "request_id":"`+id+`","ok":true,"status":"exited","exit_code":0,
 "output":"complete","remote_control_available":false}`)
 frame,err:=host.Seal(old)
 if err!=nil{t.Fatal(err)}
 got,err:=OpenRemoteTaskReportResult(client,frame,id)
 if err!=nil||got.Output!="complete"||got.OutputOmitted{
  t.Fatalf("old exact task report denied: %+v %v",got,err)
 }
 // A body with explicit omission must be rejected even when protected by
 // a valid E2EE peer key (e.g., a corrupt Agent implementation).
 client2,host2,_:=participants(t)
 forged:=[]byte(`{"type":"taskdeck-e2ee-task-run-report-result-v1",
 "request_id":"`+id+`","ok":true,"status":"exited","exit_code":0,
 "output":"truncated","output_omitted":true,"remote_control_available":false}`)
 bad,err:=host2.Seal(forged)
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteTaskReportResult(client2,bad,id);err==nil||client2.Established(){
  t.Fatal("partial output fraud with omission flag accepted")
 }
}
