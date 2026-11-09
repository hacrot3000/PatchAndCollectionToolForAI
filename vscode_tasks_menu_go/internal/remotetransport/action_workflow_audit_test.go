package remotetransport

import(
 "context"
 "encoding/json"
 "errors"
 "strings"
 "testing"
 "time"
)

func TestRemoteWorkflowAuditBoundedStrictE2EE(t *testing.T){
 now:=time.Now().UTC().Unix()
 report:=RemoteWorkflowAudit{Items:[]RemoteWorkflowAuditEntry{
  {RootTaskID:2147483647,TaskID:2147483647,Step:4,Total:4,
   Attempt:2,Status:"timed_out",ExitCode:-1,Timestamp:now},
  {RootTaskID:8,TaskID:7,Step:2,Total:3,Attempt:1,
   Status:"completed",ExitCode:0,Timestamp:now-1},
 }}
 client,agent,_:=participants(t)
 frame,err:=SealRemoteWorkflowAudit(agent,testActionID,report)
 if err!=nil{t.Fatal(err)}
 got,err:=OpenRemoteWorkflowAudit(client,frame,testActionID)
 if err!=nil||len(got.Items)!=2||got.Items[0]!=report.Items[0]||
  got.Items[1]!=report.Items[1]{
  t.Fatalf("authenticated step report corrupted: %+v %v",got,err)
 }
 for _,bad:=range []RemoteWorkflowAudit{
  {Items:append(append([]RemoteWorkflowAuditEntry{},report.Items...),report.Items[0])},
  {Items:[]RemoteWorkflowAuditEntry{{RootTaskID:0,TaskID:1,Step:1,Total:2,
   Attempt:1,Status:"failed",ExitCode:1,Timestamp:now}}},
  {Items:[]RemoteWorkflowAuditEntry{{RootTaskID:1,TaskID:1,Step:1,Total:2,
   Attempt:1,Status:"attempted",ExitCode:-1,Timestamp:now}}},
  {Items:[]RemoteWorkflowAuditEntry{{RootTaskID:1,TaskID:1,Step:1,Total:2,
   Attempt:1,Status:"completed",ExitCode:1,Timestamp:now}}},
  {Items:[]RemoteWorkflowAuditEntry{{RootTaskID:1,TaskID:1,Step:3,Total:2,
   Attempt:1,Status:"failed",ExitCode:1,Timestamp:now}}},
  {Items:[]RemoteWorkflowAuditEntry{{RootTaskID:1,TaskID:1,Step:1,Total:2,
   Attempt:3,Status:"failed",ExitCode:1,Timestamp:now}}},
  {Items:[]RemoteWorkflowAuditEntry{
   {RootTaskID:1,TaskID:1,Step:1,Total:2,Attempt:1,
    Status:"failed",ExitCode:1,Timestamp:now-1},
   {RootTaskID:1,TaskID:2,Step:2,Total:2,Attempt:1,
    Status:"failed",ExitCode:1,Timestamp:now},
  }},
 }{
  _,a,_:=participants(t)
  if _,err:=SealRemoteWorkflowAudit(a,testActionID,bad);
   !errors.Is(err,ErrInvalidRemoteAction){
   t.Fatalf("invalid audit report was sealed: %+v %v",bad,err)
  }
 }
 data,err:=json.Marshal(remoteWorkflowAuditWire{
  Type:RemoteWorkflowAuditProtocol,RequestID:testActionID,
  Items:report.Items,RemoteControlAvailable:false,
 })
 if err!=nil{t.Fatal(err)}
 source:=string(data)
 for name,raw:=range map[string]string{
  "extra":strings.Replace(source,`"attempt":2`,`"attempt":2,"output":"secret"`,1),
  "duplicate":strings.Replace(source,`"step":4`,`"step":4,"step":2`,1),
  "null-code":strings.Replace(source,`"exit_code":-1`,`"exit_code":null`,1),
  "negative-zero":strings.Replace(source,`"exit_code":-1`,`"exit_code":-0`,1),
  "null-items":strings.Replace(source,string(data),`{"type":"`+
   RemoteWorkflowAuditProtocol+`","request_id":"`+testActionID+
   `","items":null,"remote_control_available":false}`,1),
  "forged-control":strings.Replace(source,`"remote_control_available":false`,
   `"remote_control_available":true`,1),
 }{
  t.Run(name,func(t *testing.T){
   client,agent,_:=participants(t)
   frame,err:=agent.Seal([]byte(raw))
   if err!=nil{t.Fatal(err)}
   if _,err:=OpenRemoteWorkflowAudit(client,frame,testActionID);
    !errors.Is(err,ErrInvalidRemoteAction){
    t.Fatalf("malformed E2EE report admitted: %s (%v)",raw,err)
   }
   if client.Established(){t.Fatal("malformed E2EE audit failed to retire channel")}
  })
 }
}
func TestRemoteWorkflowAuditNoCallerSelectedTaskOrProject(t *testing.T){
 client,agent,_:=participants(t)
 exchange:=pingExchangeFunc(func(_ context.Context,frame []byte)([]byte,error){
  request,err:=OpenRemoteAction(agent,frame)
  if err!=nil||request.Operation!=RemoteActionTasksWorkflowAuditStart||
   request.TaskID!=0||request.FileIndex!=0||request.Text!=""||
   request.Message!=""||request.Snapshot!=""||request.Choice!=0{
   return nil,ErrInvalidRemoteAction
  }
  return SealRemoteWorkflowAudit(agent,request.RequestID,RemoteWorkflowAudit{
   Items:[]RemoteWorkflowAuditEntry{{RootTaskID:2,TaskID:1,
    Step:1,Total:2,Attempt:1,Status:"completed",ExitCode:0,
    Timestamp:time.Now().UTC().Unix()}},
  })
 })
 p,err:=NewPingClient(client,exchange)
 if err!=nil{t.Fatal(err)}
 defer p.Close()
 result,err:=p.WorkflowAudit(context.Background())
 if err!=nil||len(result.Items)!=1||result.Items[0].RootTaskID!=2{
  t.Fatalf("native principal audit client: %+v %v",result,err)
 }
 for _,bad:=range []RemoteAction{
  {RequestID:testActionID,Operation:RemoteActionTasksWorkflowAudit,TaskID:3},
  {RequestID:testActionID,Operation:RemoteActionTasksWorkflowAudit,Text:"other user"},
  {RequestID:testActionID,Operation:RemoteActionTasksWorkflowAudit,Message:"latest"},
  {RequestID:testActionID,Operation:RemoteActionTasksWorkflowAudit,FileIndex:2},
 }{
  c,_,_:=participants(t)
  if _,err:=SealRemoteAction(c,bad);!errors.Is(err,ErrInvalidRemoteAction){
   t.Fatalf("caller-selected workflow audit query accepted: %+v",bad)
  }
 }
}

 
func TestRemoteWorkflowAuditMorePageStrictAndCompatible(t *testing.T){
 now:=time.Now().UTC().Unix()
 entry:=RemoteWorkflowAuditEntry{RootTaskID:3,TaskID:2,
  Step:1,Total:2,Attempt:1,Status:"completed",ExitCode:0,Timestamp:now}
 row:=RemoteWorkflowAudit{Items:[]RemoteWorkflowAuditEntry{entry,entry},More:true}
 client,agent,_:=participants(t)
 frame,err:=SealRemoteWorkflowAudit(agent,testActionID,row)
 if err!=nil{t.Fatal(err)}
 result,err:=OpenRemoteWorkflowAudit(client,frame,testActionID)
 if err!=nil||!result.More||len(result.Items)!=2{
  t.Fatalf("host next-page hint not authenticated: %+v %v",result,err)
 }
 for _,bad:=range []RemoteWorkflowAudit{
  {Items:[]RemoteWorkflowAuditEntry{entry},More:true},
  {Items:[]RemoteWorkflowAuditEntry{},More:true},
 }{
  _,agent,_:=participants(t)
  if _,err:=SealRemoteWorkflowAudit(agent,testActionID,bad);err==nil{
   t.Fatalf("partial page with more was sealed: %+v",bad)
  }
 }
 c,agent,_:=participants(t)
 legacy:=remoteWorkflowAuditWire{
  Type:RemoteWorkflowAuditProtocol,RequestID:testActionID,
  Items:[]RemoteWorkflowAuditEntry{entry},RemoteControlAvailable:false,
 }
 clear,err:=json.Marshal(legacy)
 if err!=nil{t.Fatal(err)}
 frame,err=agent.Seal(clear)
 if err!=nil{t.Fatal(err)}
 actual,err:=OpenRemoteWorkflowAudit(c,frame,testActionID)
 if err!=nil||actual.More||len(actual.Items)!=1{
  t.Fatalf("previous wire response no longer compatible: %+v %v",actual,err)
 }
 for _,rawMore:=range []string{"null",`"true"`,"0"}{
  c,agent,_:=participants(t)
  altered:=strings.Replace(string(clear),`"remote_control_available":false`,
   `"remote_control_available":false,"more":`+rawMore,1)
  frame,err:=agent.Seal([]byte(altered))
  if err!=nil{t.Fatal(err)}
  if _,err:=OpenRemoteWorkflowAudit(c,frame,testActionID);err==nil{
   t.Fatalf("bad more boolean %s admitted",rawMore)
  }
 }
}

func TestRemoteWorkflowAuditStartAndNextForbidCallerSelectors(t *testing.T){
 for _,op:=range []string{RemoteActionTasksWorkflowAuditStart,RemoteActionTasksWorkflowAuditNext}{
  for _,bad:=range []RemoteAction{
   {RequestID:testActionID,Operation:op,TaskID:3},
   {RequestID:testActionID,Operation:op,Text:"user"},
   {RequestID:testActionID,Operation:op,Message:"offset"},
   {RequestID:testActionID,Operation:op,FileIndex:1},
  }{
   client,_,_:=participants(t)
   if _,err:=SealRemoteAction(client,bad);err==nil{
    t.Fatalf("untrusted selector accepted: %+v",bad)
   }
  }
 }
}

func TestRemoteWorkflowRunIdentifierBoundedE2EEOptionalField(t *testing.T){
 now:=time.Now().UTC().Unix()
 good:=RemoteWorkflowAudit{Items:[]RemoteWorkflowAuditEntry{
  {RootTaskID:2147483647,TaskID:2147483647,Step:4,Total:4,
   Attempt:2,Status:"timed_out",ExitCode:-1,Timestamp:now,
   RunID:"0123456789abcdef"},
  {RootTaskID:2147483647,TaskID:2147483647,Step:4,Total:4,
   Attempt:2,Status:"timed_out",ExitCode:-1,Timestamp:now-1,
   RunID:"fedcba9876543210"},
 },More:true}
 client,agent,_:=participants(t)
 frame,err:=SealRemoteWorkflowAudit(agent,testActionID,good)
 if err!=nil{t.Fatalf("max-sized run-correlated E2EE page did not fit: %v",err)}
 decoded,err:=OpenRemoteWorkflowAudit(client,frame,testActionID)
 if err!=nil||!decoded.More||
  decoded.Items[0].RunID!=good.Items[0].RunID||
  decoded.Items[1].RunID!=good.Items[1].RunID{
  t.Fatalf("run IDs lost in authenticated page: %+v %v",decoded,err)
 }
 for _,candidate:=range []string{
  "0123456789abcde","0123456789abcdef0","ABCDEF1234567890",
  "0123456789abcd/g","0123456789abcd:g",
  "0123456789abcd\u0000",
 }{
  bad:=good
  bad.Items=append([]RemoteWorkflowAuditEntry{},good.Items...)
  bad.Items[0].RunID=candidate
  _,agent,_:=participants(t)
  if _,err:=SealRemoteWorkflowAudit(agent,testActionID,bad);err==nil{
   t.Fatalf("invalid run correlation accepted: %q",candidate)
  }
 }
}
