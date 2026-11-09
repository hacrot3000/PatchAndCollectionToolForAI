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

 
func TestRemoteLongTaskWorkflowStepEscapedOutputFitsE2EEEnvelope(t *testing.T){
 for _,char:=range []string{"<",">","&","\"","\\","\n","\t"}{
  client,agent,_:=participants(t)
  value:=RemoteLongTaskProgress{
   TaskID:2147483647,Status:"completed",ExitCode:255,
   Output:strings.Repeat(char,64),Truncated:true,More:true,
   Step:4,StepTotal:4,Attempt:2,AttemptTotal:2,StepStatus:"completed",
   StepHistory:"CCCC",
   StepDetails:"255:90000|255:90000|255:90000|255:90000",
  }
  frame,err:=SealRemoteLongTaskProgress(agent,testActionID,value)
  if err!=nil{t.Fatalf("escaped workflow output %q overflowed E2EE: %v",char,err)}
  got,err:=OpenRemoteLongTaskProgress(client,frame,testActionID,2147483647)
  if err!=nil||got.Output!=value.Output||got.Step!=4||got.Attempt!=2||
   got.StepHistory!="CCCC"||got.StepDetails!=value.StepDetails{
   t.Fatalf("escaped workflow response corrupted: %+v %v",got,err)
  }
 }
}

func TestRemoteLongTaskWorkflowHistoryStrictE2EEContract(t *testing.T){
 good:=RemoteLongTaskProgress{
  TaskID:17,Status:"completed",ExitCode:0,
  Step:3,StepTotal:3,Attempt:1,AttemptTotal:1,
  StepStatus:"completed",StepHistory:"CCC",Output:"visible",
 }
 client,agent,_:=participants(t)
 frame,err:=SealRemoteLongTaskProgress(agent,testActionID,good)
 if err!=nil{t.Fatal(err)}
 actual,err:=OpenRemoteLongTaskProgress(client,frame,testActionID,17)
 if err!=nil||actual.StepHistory!="CCC"{
  t.Fatalf("workflow summary not authenticated: %+v %v",actual,err)
 }
 for _,history:=range []string{
  "CC","CCCC","CC-", "CFC","CCR","CXX","CCC\x1b",
 }{
  invalid:=good
  invalid.StepHistory=history
  _,ch,_:=participants(t)
  if _,err:=SealRemoteLongTaskProgress(ch,testActionID,invalid);err==nil{
   t.Fatalf("unsafe workflow history allowed: %q",history)
  }
 }
 invalid:=good
 invalid.StepHistory="C--"
 invalid.Step=2
 invalid.StepStatus="running"
 _,ch,_:=participants(t)
 if _,err:=SealRemoteLongTaskProgress(ch,testActionID,invalid);err==nil{
  t.Fatal("history must agree with current step state")
 }
 // Accept the five older authenticated metadata fields without summary.
 good.StepHistory=""
 client,agent,_=participants(t)
 frame,err=SealRemoteLongTaskProgress(agent,testActionID,good)
 if err!=nil{t.Fatal(err)}
 actual,err=OpenRemoteLongTaskProgress(client,frame,testActionID,17)
 if err!=nil||actual.StepHistory!=""{
  t.Fatalf("prior protocol metadata ceased working: %+v %v",actual,err)
 }
}

 
func TestRemoteLongTaskWorkflowHistoryRejectsMalformedAuthenticatedFrames(t *testing.T){
 for _,metadata:=range []string{
  `"step_history":"CX-"`,
  `"step_history":"C--"`,
  `"step_history":null`,
  `"step_history":"CCC","step_history":"CCC"`,
 }{
  client,agent,_:=participants(t)
  jsonMessage:=`{"type":"`+RemoteLongTaskProgressProtocol+
   `","request_id":"`+testActionID+
   `","task_id":14,"status":"completed","exit_code":0,
   "output":"","truncated":false,"more":false,
   "remote_control_available":false,"step":3,"step_total":3,
   "attempt":1,"attempt_total":1,"step_status":"completed",`+
   metadata+`}`
  frame,err:=agent.Seal([]byte(jsonMessage))
  if err!=nil{t.Fatal(err)}
  if _,err:=OpenRemoteLongTaskProgress(client,frame,testActionID,14);err==nil{
   t.Fatalf("malformed host step history accepted: %s",metadata)
  }
  if client.Established(){t.Fatal("invalid step history failed to close E2EE channel")}
 }
}

 
func TestRemoteLongTaskStepDetailsBoundedAuthenticatedAndBackwardCompatible(t *testing.T){
 base:=RemoteLongTaskProgress{
  TaskID:31,Status:"completed",ExitCode:0,
  Step:3,StepTotal:3,Attempt:1,AttemptTotal:1,
  StepStatus:"completed",StepHistory:"CCC",
  StepDetails:"0:5|0:100|0:90000",Output:"done",
 }
 client,agent,_:=participants(t)
 frame,err:=SealRemoteLongTaskProgress(agent,testActionID,base)
 if err!=nil{t.Fatal(err)}
 got,err:=OpenRemoteLongTaskProgress(client,frame,testActionID,31)
 if err!=nil||got.StepDetails!=base.StepDetails{
  t.Fatalf("authenticated detailed result was corrupted: %+v %v",got,err)
 }
 for _,invalid:=range []string{
  "0:1|0:2","0:1|0:2|0:3|0:4","0:1|0:2|-",
  "01:2|0:2|0:3","0:01|0:2|0:3",
  "0:1|0:2|1:3","0:1|0:2|0:90001",
  "0:1|0:2|-1:90001","0:1|0:2|0:-1",
  "0:1|0:2|0:0;echo injected",
 }{
  altered:=base;altered.StepDetails=invalid
  _,agent,_:=participants(t)
  if _,err:=SealRemoteLongTaskProgress(agent,testActionID,altered);err==nil{
   t.Fatalf("invalid step result detail accepted: %q",invalid)
  }
 }
 noDetails:=base
 noDetails.StepDetails=""
 client,agent,_=participants(t)
 frame,err=SealRemoteLongTaskProgress(agent,testActionID,noDetails)
 if err!=nil{t.Fatal(err)}
 got,err=OpenRemoteLongTaskProgress(client,frame,testActionID,31)
 if err!=nil||got.StepDetails!=""{
  t.Fatalf("older workflow frame became incompatible: %+v %v",got,err)
 }
}
