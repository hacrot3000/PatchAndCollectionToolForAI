package remotetransport

import (
 "strings"
 "testing"
)

func TestRemoteLongTaskTypedE2EERoundTripAndTaskIDBinding(t *testing.T){
 client,agent,_:=participants(t)
 for _,operation:=range []string{
  RemoteActionTasksStart,RemoteActionTasksPoll,
  RemoteActionTasksCancel,RemoteActionTasksClear,
 }{
  request,err:=SealRemoteAction(client,RemoteAction{
   RequestID:testActionID,Operation:operation,TaskID:234,
  })
  if err!=nil{t.Fatalf("seal %q: %v",operation,err)}
  selected,err:=OpenRemoteAction(agent,request)
  if err!=nil||selected.TaskID!=234||selected.Operation!=operation{
   t.Fatalf("wrong long-task action: %+v %v",selected,err)
  }
  frame,err:=SealRemoteLongTaskProgress(agent,testActionID,
   RemoteLongTaskProgress{
    TaskID:234,Status:"running",ExitCode:-1,Output:"status\n",More:true,
   })
  if err!=nil{t.Fatal(err)}
  actual,err:=OpenRemoteLongTaskProgress(client,frame,testActionID,234)
  if err!=nil||actual.Output!="status\n"||actual.Status!="running"||!actual.More{
   t.Fatalf("progress did not roundtrip: %+v %v",actual,err)
  }
 }
}

func TestRemoteLongTaskProgressWrongIDPoisonsChannel(t *testing.T){
 client,agent,_:=participants(t)
 frame,err:=SealRemoteLongTaskProgress(agent,testActionID,
  RemoteLongTaskProgress{TaskID:12,Status:"completed",ExitCode:0,Output:"safe"})
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteLongTaskProgress(client,frame,testActionID,13);err==nil{
  t.Fatal("forged task ID was accepted")
 }
 if client.Established(){t.Fatal("mismatched long-task result did not revoke E2EE")}
}

func TestRemoteLongTaskProgressRejectsUnsafeOutputAndState(t *testing.T){
 for _,progress:=range []RemoteLongTaskProgress{
  {TaskID:0,Status:"started",ExitCode:-1},
  {TaskID:1,Status:"running",ExitCode:0},
  {TaskID:1,Status:"ghost",ExitCode:-1},
  {TaskID:1,Status:"completed",ExitCode:0,Output:"\x1b[31m"},
  {TaskID:1,Status:"completed",ExitCode:0,Output:string(make([]byte,161))},
  {TaskID:1,Status:"completed",ExitCode:300},
  {TaskID:1,Status:"started",ExitCode:-1,More:true},
  {TaskID:1,Status:"cancel_requested",ExitCode:-1,More:true},
  {TaskID:1,Status:"cleared",ExitCode:-1,More:true},
 }{
  _,agent,_:=participants(t)
  if _,err:=SealRemoteLongTaskProgress(agent,testActionID,progress);err==nil{
   t.Fatalf("invalid long-task progress was sealed: %+v",progress)
  }
 }
}

func TestRemoteLongTaskInputOnlyAllowedForStart(t *testing.T){
 for _,operation:=range []string{
  RemoteActionTasksPoll,RemoteActionTasksCancel,RemoteActionTasksClear,
 }{
  client,_,_:=participants(t)
  if _,err:=SealRemoteAction(client,RemoteAction{
   RequestID:testActionID,Operation:operation,TaskID:1,Text:"release",
  });err==nil{
   t.Fatalf("client supplied input to %s",operation)
  }
  client,_,_=participants(t)
  if _,err:=SealRemoteAction(client,RemoteAction{
   RequestID:testActionID,Operation:operation,TaskID:1,Choice:1,
  });err==nil{
   t.Fatalf("client supplied choice to %s",operation)
  }
 }
 client,agent,_:=participants(t)
 request,err:=SealRemoteAction(client,RemoteAction{
  RequestID:testActionID,Operation:RemoteActionTasksStart,
  TaskID:7,Text:"release-2",
 })
 if err!=nil{t.Fatal(err)}
 action,err:=OpenRemoteAction(agent,request)
 if err!=nil||action.Text!="release-2"{t.Fatalf("typed input not supported for start: %+v %v",action,err)}
}

func TestRemoteLongTaskEscapedOutputFitsFixedE2EEEnvelope(t *testing.T){
 for _,character:=range []string{"<",">","&","\"","\\","\n","\t"}{
  client,agent,_:=participants(t)
  output:=strings.Repeat(character,MaxRemoteLongTaskOutputBytes)
  frame,err:=SealRemoteLongTaskProgress(agent,testActionID,
   RemoteLongTaskProgress{
    TaskID:2147483647,Status:"completed",ExitCode:255,
    Output:output,More:true,Truncated:true,
   })
  if err!=nil{
   t.Fatalf("allowed escaped output exceeded AEAD record: %q: %v",character,err)
  }
  got,err:=OpenRemoteLongTaskProgress(client,frame,testActionID,2147483647)
  if err!=nil||got.Output!=output||!got.More||!got.Truncated{
   t.Fatalf("E2EE codec corrupted escaped output %q: %+v %v",character,got,err)
  }
 }
}

 
func TestRemoteLongTaskWorkflowStepTypedE2EERoundTrip(t *testing.T){
 for _,state:=range []string{"running","completed","failed","cancelled","timed_out"}{
  client,agent,_:=participants(t)
  input:=RemoteLongTaskProgress{
   TaskID:23,Status:"running",ExitCode:-1,Output:"step",
   Step:2,StepTotal:4,Attempt:2,AttemptTotal:2,StepStatus:state,
  }
  frame,err:=SealRemoteLongTaskProgress(agent,testActionID,input)
  if err!=nil{t.Fatalf("workflow progress %q not sealed: %v",state,err)}
  result,err:=OpenRemoteLongTaskProgress(client,frame,testActionID,23)
  if err!=nil||result.Step!=2||result.StepTotal!=4||
   result.Attempt!=2||result.AttemptTotal!=2||result.StepStatus!=state{
   t.Fatalf("authenticated workflow fields lost: %+v %v",result,err)
  }
 }
}

func TestRemoteLongTaskRejectsPartialOrOutOfBoundsWorkflowMetadata(t *testing.T){
 invalid:=[]RemoteLongTaskProgress{
  {TaskID:3,Status:"running",ExitCode:-1,Step:1},
  {TaskID:3,Status:"running",ExitCode:-1,Step:1,StepTotal:5,
   Attempt:1,AttemptTotal:1,StepStatus:"running"},
  {TaskID:3,Status:"running",ExitCode:-1,Step:2,StepTotal:1,
   Attempt:1,AttemptTotal:1,StepStatus:"running"},
  {TaskID:3,Status:"running",ExitCode:-1,Step:1,StepTotal:2,
   Attempt:3,AttemptTotal:2,StepStatus:"running"},
  {TaskID:3,Status:"running",ExitCode:-1,Step:1,StepTotal:2,
   Attempt:1,AttemptTotal:1,StepStatus:"unknown"},
  {TaskID:3,Status:"started",ExitCode:-1,Step:1,StepTotal:2,
   Attempt:1,AttemptTotal:1,StepStatus:"running"},
  {TaskID:3,Status:"running",ExitCode:-1,Step:1,StepTotal:2,
   Attempt:1,AttemptTotal:1,StepStatus:"running",Output:strings.Repeat("X",65)},
 }
 for _,value:=range invalid{
  _,agent,_:=participants(t)
  if _,err:=SealRemoteLongTaskProgress(agent,testActionID,value);err==nil{
   t.Fatalf("unsafe workflow metadata accepted: %+v",value)
  }
 }
 for _,jsonSource:=range []string{
  `{"type":"taskdeck-e2ee-task-progress-v1","request_id":"`+testActionID+
   `","task_id":3,"status":"running","exit_code":-1,"output":"",
     "truncated":false,"more":false,"remote_control_available":false,
     "step":1}`,
  `{"type":"taskdeck-e2ee-task-progress-v1","request_id":"`+testActionID+
   `","task_id":3,"status":"running","exit_code":-1,"output":"",
     "truncated":false,"more":false,"remote_control_available":false,
     "step":1,"step":2}`,
 }{
  client,agent,_:=participants(t)
  frame,err:=agent.Seal([]byte(jsonSource))
  if err!=nil{t.Fatal(err)}
  if _,err:=OpenRemoteLongTaskProgress(client,frame,testActionID,3);err==nil{
   t.Fatalf("malformed metadata not denied: %s",jsonSource)
  }
  if client.Established(){t.Fatal("malformed workflow metadata did not revoke channel")}
 }
}
