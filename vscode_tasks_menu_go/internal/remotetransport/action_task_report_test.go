package remotetransport

import(
 "context"
 "encoding/json"
 "errors"
 "strings"
 "testing"
 "time"
)

func TestRemoteTaskReportE2EEStrictAndBounded(t *testing.T){
 now:=time.Now().UTC().Unix()
 rows:=RemoteTaskReport{Items:[]RemoteTaskReportEntry{
  {TaskID:2147483647,Status:"failed",ExitCode:7,DurationMS:120,Timestamp:now},
  {TaskID:2,Status:"completed",ExitCode:0,DurationMS:8,Timestamp:now-1},
  {TaskID:3,Status:"timed_out",ExitCode:-1,DurationMS:90000,Timestamp:now-2},
 }}
 c,a,_:=participants(t)
 frame,err:=SealRemoteTaskReport(a,testActionID,rows)
 if err!=nil{t.Fatal(err)}
 result,err:=OpenRemoteTaskReport(c,frame,testActionID)
 if err!=nil||len(result.Items)!=3||result.Items[0]!=rows.Items[0]{
  t.Fatalf("bounded authenticated report failed: %+v %v",result,err)
 }
 for _,bad:=range []RemoteTaskReport{
  {Items:[]RemoteTaskReportEntry{{TaskID:0,Status:"completed",Timestamp:now}}},
  {Items:[]RemoteTaskReportEntry{{TaskID:1,Status:"running",Timestamp:now}}},
  {Items:[]RemoteTaskReportEntry{{TaskID:1,Status:"completed",ExitCode:256,Timestamp:now}}},
  {Items:[]RemoteTaskReportEntry{{TaskID:1,Status:"failed",DurationMS:-1,Timestamp:now}}},
  {Items:[]RemoteTaskReportEntry{{TaskID:1,Status:"completed",Timestamp:now+10000}}},
  {Items:append(append([]RemoteTaskReportEntry{},rows.Items...),rows.Items[0])},
  {Items:[]RemoteTaskReportEntry{
   {TaskID:1,Status:"completed",Timestamp:now-10},
   {TaskID:2,Status:"completed",Timestamp:now},
  }},
 }{
  c,_,_:=participants(t)
  if _,err:=SealRemoteTaskReport(c,testActionID,bad);
   !errors.Is(err,ErrInvalidRemoteAction){t.Fatalf("bad report accepted: %+v %v",bad,err)}
 }
 wire:=remoteTaskReportWire{
  Type:RemoteTaskReportProtocol,RequestID:testActionID,
  Items:rows.Items,RemoteControlAvailable:false,
 }
 clear,err:=json.Marshal(wire)
 if err!=nil{t.Fatal(err)}
 for name,raw:=range map[string]string{
  "extra":strings.Replace(string(clear),"}",`,"path":"/etc/passwd"}`,1),
  "duplicate":strings.Replace(string(clear),`"task_id":2147483647`,`"task_id":2147483647,"task_id":1`,1),
  "forged-control":strings.Replace(string(clear),`"remote_control_available":false`,`"remote_control_available":true`,1),
  "secret":strings.Replace(string(clear),`"status":"failed"`,`"status":"failed","output":"TOP SECRET"`,1),
 }{
  t.Run(name,func(t *testing.T){
   c,a,_:=participants(t)
   frame,err:=a.Seal([]byte(raw))
   if err!=nil{t.Fatal(err)}
   if _,err:=OpenRemoteTaskReport(c,frame,testActionID);
    !errors.Is(err,ErrInvalidRemoteAction){t.Fatalf("forged report accepted: %v",err)}
   if c.Established(){t.Fatal("malformed E2EE report did not retire channel")}
  })
 }
}
func TestRemoteTaskReportNativeClientNoCallerArguments(t *testing.T){
 c,a,_:=participants(t)
 exchange:=pingExchangeFunc(func(ctx context.Context,frame []byte)([]byte,error){
  request,err:=OpenRemoteAction(a,frame)
  if err!=nil||request.Operation!=RemoteActionTasksReport||
   request.TaskID!=0||request.FileIndex!=0||request.Text!=""||
   request.Message!=""||request.Snapshot!=""||request.Choice!=0{
   return nil,ErrInvalidRemoteAction
  }
  return SealRemoteTaskReport(a,request.RequestID,RemoteTaskReport{
   Items:[]RemoteTaskReportEntry{{
    TaskID:1,Status:"completed",Timestamp:time.Now().UTC().Unix(),
   }},
  })
 })
 client,err:=NewPingClient(c,exchange)
 if err!=nil{t.Fatal(err)}
 defer client.Close()
 report,err:=client.TasksReport(context.Background())
 if err!=nil||len(report.Items)!=1||report.Items[0].TaskID!=1{
  t.Fatalf("native report: %+v %v",report,err)
 }
 for _,bad:=range []RemoteAction{
  {RequestID:testActionID,Operation:RemoteActionTasksReport,TaskID:5},
  {RequestID:testActionID,Operation:RemoteActionTasksReport,Text:"other user"},
  {RequestID:testActionID,Operation:RemoteActionTasksReport,Message:"where audit"},
  {RequestID:testActionID,Operation:RemoteActionTasksReport,FileIndex:3},
 }{
  c,_,_:=participants(t)
  if _,err:=SealRemoteAction(c,bad);!errors.Is(err,ErrInvalidRemoteAction){
   t.Fatalf("accepted caller report query: %+v",bad)
  }
 }
}
