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
