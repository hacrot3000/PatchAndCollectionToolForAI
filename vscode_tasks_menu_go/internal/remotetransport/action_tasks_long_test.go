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
  {TaskID:1,Status:"started",ExitCode:-1,Output:"untrusted"},
  {TaskID:1,Status:"cancel_requested",ExitCode:-1,Output:"untrusted"},
  {TaskID:1,Status:"cleared",ExitCode:-1,Output:"untrusted"},
  {TaskID:1,Status:"started",ExitCode:-1,Truncated:true},
  {TaskID:1,Status:"cancel_requested",ExitCode:-1,Truncated:true},
  {TaskID:1,Status:"cleared",ExitCode:-1,Truncated:true},
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
   StepDetails:"0:90000|0:90000|0:90000|0:90000",
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
  "CC","CCCC","CC-", "FCC","CCR","CXX","CCC\x1b",
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

 
func TestRemoteLongTaskConditionalCleanupAllowsOnlyImmediateFailedPredecessor(t *testing.T){
 good:=RemoteLongTaskProgress{
  TaskID:44,Status:"failed",ExitCode:1,Output:"cleanup complete",
  Step:2,StepTotal:2,Attempt:1,AttemptTotal:1,
  StepStatus:"completed",StepHistory:"FC",
  StepDetails:"1:5|0:8",
 }
 client,agent,_:=participants(t)
 sealed,err:=SealRemoteLongTaskProgress(agent,testActionID,good)
 if err!=nil{t.Fatalf("host cleanup result not sealable: %v",err)}
 result,err:=OpenRemoteLongTaskProgress(client,sealed,testActionID,44)
 if err!=nil||result.Status!="failed"||result.StepHistory!="FC"{
  t.Fatalf("authenticated cleanup result lost: %+v %v",result,err)
 }
 three:=good
 three.Step=3;three.StepTotal=3
 three.StepHistory="CFC";three.StepDetails="0:1|1:2|0:3"
 client,agent,_=participants(t)
 sealed,err=SealRemoteLongTaskProgress(agent,testActionID,three)
 if err!=nil{t.Fatal(err)}
 result,err=OpenRemoteLongTaskProgress(client,sealed,testActionID,44)
 if err!=nil||result.StepHistory!="CFC"{t.Fatalf("3-step final cleanup: %+v %v",result,err)}
 for _,candidate:=range []RemoteLongTaskProgress{
  {TaskID:44,Status:"failed",ExitCode:1,Step:2,StepTotal:3,
   Attempt:1,AttemptTotal:1,StepStatus:"completed",StepHistory:"FC-",
   StepDetails:"1:2|0:3|-"},
  {TaskID:44,Status:"failed",ExitCode:1,Step:3,StepTotal:3,
   Attempt:1,AttemptTotal:1,StepStatus:"completed",StepHistory:"FCC",
   StepDetails:"1:2|0:3|0:4"},
  {TaskID:44,Status:"failed",ExitCode:1,Step:3,StepTotal:3,
   Attempt:1,AttemptTotal:1,StepStatus:"completed",StepHistory:"F-C",
   StepDetails:"1:2|-|0:4"},
  {TaskID:44,Status:"failed",ExitCode:1,Step:4,StepTotal:4,
   Attempt:1,AttemptTotal:1,StepStatus:"completed",StepHistory:"CFCC",
   StepDetails:"0:1|1:2|0:3|0:4"},
 }{
  _,agent,_:=participants(t)
  if _,err:=SealRemoteLongTaskProgress(agent,testActionID,candidate);err==nil{
   t.Fatalf("unsafe failed step prefix allowed: %+v",candidate)
  }
 }
}

 
func TestRemoteLongTaskFailureOnlyFinalSkippedState(t *testing.T){
 good:=RemoteLongTaskProgress{
  TaskID:21,Status:"completed",ExitCode:0,Output:"skipped",
  Step:2,StepTotal:2,Attempt:1,AttemptTotal:1,
  StepStatus:"skipped",StepHistory:"CS",StepDetails:"0:13|-",
 }
 client,agent,_:=participants(t)
 frame,err:=SealRemoteLongTaskProgress(agent,testActionID,good)
 if err!=nil{t.Fatalf("skip metadata cannot be sealed: %v",err)}
 got,err:=OpenRemoteLongTaskProgress(client,frame,testActionID,21)
 if err!=nil||got.Status!="completed"||got.StepHistory!="CS"||
  got.StepStatus!="skipped"||got.StepDetails!="0:13|-"{ 
  t.Fatalf("skip E2EE metadata corrupted: %+v %v",got,err)
 }
 transitional:=good
 transitional.Status="running"
 transitional.ExitCode=-1
 client,agent,_=participants(t)
 frame,err=SealRemoteLongTaskProgress(agent,testActionID,transitional)
 if err!=nil{t.Fatalf("final skip race must not poison E2EE: %v",err)}
 got,err=OpenRemoteLongTaskProgress(client,frame,testActionID,21)
 if err!=nil||got.Status!="running"||got.StepStatus!="skipped"{
  t.Fatalf("in-flight final skip transition lost: %+v %v",got,err)
 }
 triple:=good
 triple.Step=3;triple.StepTotal=3
 triple.StepHistory="CCS"
 triple.StepDetails="0:5|0:10|-"
 client,agent,_=participants(t)
 frame,err=SealRemoteLongTaskProgress(agent,testActionID,triple)
 if err!=nil{t.Fatal(err)}
 got,err=OpenRemoteLongTaskProgress(client,frame,testActionID,21)
 if err!=nil||got.StepHistory!="CCS"{
  t.Fatalf("three-step skipped root rejected: %+v %v",got,err)
 }
 for _,mutate:=range []struct{name string; change func(*RemoteLongTaskProgress)}{
  {"failed-predecessor",func(p *RemoteLongTaskProgress){p.StepHistory="FS";p.StepDetails="1:2|-"}},
  {"faked-exit",func(p *RemoteLongTaskProgress){p.StepDetails="0:13|0:0"}},
  {"nonfinal",func(p *RemoteLongTaskProgress){p.Step=1;p.StepHistory="S-";p.StepDetails="-|-"}},
  {"failed-aggregate",func(p *RemoteLongTaskProgress){p.Status="failed";p.ExitCode=1}},
  {"no-history",func(p *RemoteLongTaskProgress){p.StepHistory="";p.StepDetails=""}},
  {"unexpected-run",func(p *RemoteLongTaskProgress){p.StepStatus="running"}},
 }{
  t.Run(mutate.name,func(t *testing.T){
   value:=good
   mutate.change(&value)
   _,agent,_:=participants(t)
   if _,err:=SealRemoteLongTaskProgress(agent,testActionID,value);err==nil{
    t.Fatalf("unsafe E2EE skipped step accepted: %+v",value)
   }
  })
 }
}


func TestRemoteLongTaskMalformedAckFrameRevokesE2EE(t *testing.T){
 for _,status:=range []string{"started","cancel_requested","cleared"}{
  for _,variant:=range []string{"output","truncated"}{
   t.Run(status+"/"+variant,func(t *testing.T){
    client,agent,_:=participants(t)
    message:=`{"type":"`+RemoteLongTaskProgressProtocol+
     `","request_id":"`+testActionID+
     `","task_id":41,"status":"`+status+`","exit_code":-1,
      "output":"","truncated":false,"more":false,
      "remote_control_available":false}`
    if variant=="output"{
     message=strings.Replace(message,`"output":""`,`"output":"injected"`,1)
    }else{
     message=strings.Replace(message,`"truncated":false`,`"truncated":true`,1)
    }
    frame,err:=agent.Seal([]byte(message))
    if err!=nil{t.Fatal(err)}
    if _,err:=OpenRemoteLongTaskProgress(client,frame,testActionID,41);err==nil{
     t.Fatalf("impossible %s ACK %s was accepted",status,variant)
    }
    if client.Established(){t.Fatal("invalid task ACK did not revoke E2EE")}
   })
  }
 }
}

func TestRemoteLongTaskParallelForkAuthenticatedHistory(t *testing.T){
 good:=RemoteLongTaskProgress{
  TaskID:77,Status:"running",ExitCode:-1,
  Step:2,StepTotal:3,Attempt:1,AttemptTotal:1,
  StepStatus:"running",StepHistory:"RR-",StepDetails:"-|-|-",
 }
 if !validLongTaskProgress(good){t.Fatal("two concurrent native children refused by E2EE codec")}
 client,agent,_:=participants(t)
 frame,err:=SealRemoteLongTaskProgress(agent,testActionID,good)
 if err!=nil{t.Fatal(err)}
 restored,err:=OpenRemoteLongTaskProgress(client,frame,testActionID,77)
 if err!=nil||restored.StepHistory!="RR-"||restored.Step!=2{
  t.Fatalf("parallel E2EE progress lost: %+v %v",restored,err)
 }
 // After one sibling exits, the other may still be active, but neither
 // is a valid completed root. Final failure may contain two terminal
 // dependency outcomes without any executed root.
 for _,value:=range []RemoteLongTaskProgress{
  {TaskID:77,Status:"running",ExitCode:-1,Step:1,StepTotal:3,
   Attempt:1,AttemptTotal:1,StepStatus:"completed",
   StepHistory:"CR-",StepDetails:"0:20|-|-"},
  {TaskID:77,Status:"failed",ExitCode:1,Step:2,StepTotal:3,
   Attempt:1,AttemptTotal:1,StepStatus:"cancelled",
   StepHistory:"FX-",StepDetails:"1:10|-1:20|-"},
 }{
  if !validLongTaskProgress(value){t.Fatalf("valid fork transition denied: %+v",value)}
 }
 for name,change:=range map[string]func(*RemoteLongTaskProgress){
  "wrong total":func(x *RemoteLongTaskProgress){x.StepTotal=4},
  "future root running":func(x *RemoteLongTaskProgress){x.StepHistory="RRR"},
  "invalid code":func(x *RemoteLongTaskProgress){x.StepHistory="R?-"},
  "bad step":func(x *RemoteLongTaskProgress){x.Step=3},
  "retry not supported":func(x *RemoteLongTaskProgress){x.AttemptTotal=2},
  "status mismatch":func(x *RemoteLongTaskProgress){x.StepStatus="completed"},
  "early completed aggregate":func(x *RemoteLongTaskProgress){
   x.Status="completed";x.ExitCode=0},
  "terminal with live child":func(x *RemoteLongTaskProgress){
   x.Status="failed";x.ExitCode=1},
 }{
  t.Run(name,func(t *testing.T){
   bad:=good;change(&bad)
   if validLongTaskProgress(bad){
    t.Fatalf("unsafe fork E2EE status accepted: %+v",bad)
   }
  })
 }
}
