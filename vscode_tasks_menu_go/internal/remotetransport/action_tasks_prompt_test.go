package remotetransport

import (
 "bytes"
 "testing"
)

func TestTaskPromptTokenE2EERequestAndInspectRoundTrip(t *testing.T){
 client,agent,_:=participants(t)
 req,err:=SealRemoteAction(client,RemoteAction{
  RequestID:testActionID,Operation:RemoteActionTasksRun,
  TaskID:345,Text:"release-1.2.3",
 })
 if err!=nil{t.Fatal(err)}
 if bytes.Contains(req,[]byte("release-1.2.3")){t.Fatal("encrypted record leaked text")}
 received,err:=OpenRemoteAction(agent,req)
 if err!=nil||received.TaskID!=345||received.Choice!=0||
  received.Text!="release-1.2.3"{
  t.Fatalf("prompt token lost E2EE binding: %+v %v",received,err)
 }
 inspectFrame,err:=SealRemoteActionTasksInspectResult(agent,testActionID,
  RemoteTaskInspect{TaskID:345,Eligible:true,InputID:"version",InputType:"promptString"})
 if err!=nil{t.Fatal(err)}
 info,err:=OpenRemoteActionTasksInspectResult(client,inspectFrame,testActionID,345)
 if err!=nil||info.InputType!="promptString"||info.InputID!="version"||len(info.Choices)!=0{
  t.Fatalf("prompt metadata not authenticated: %+v %v",info,err)
 }
}

func TestTaskPromptTokenRejectsCodeInjectionAndCrossOperationFields(t *testing.T){
 bad:=[]string{";reboot","../etc"," hello","hello world",
  "one\nline","$(date)","foo/bar","-rf","ábc","v1|rm","",
  string(bytes.Repeat([]byte{'A'},65))}
 for _,value:=range bad {
  if ValidRemoteTaskPromptValue(value){
   t.Fatalf("unsafe prompt token was accepted: %q",value)
  }
  if value==""{continue}
  client,_,_:=participants(t)
  if _,err:=SealRemoteAction(client,RemoteAction{
   RequestID:testActionID,Operation:RemoteActionTasksRun,
   TaskID:123,Text:value,
  });err==nil{t.Fatalf("unsafe value was sealed: %q",value)}
 }
 for _,action:=range []RemoteAction{
  {RequestID:testActionID,Operation:RemoteActionTasksInspect,TaskID:123,Text:"abc"},
  {RequestID:testActionID,Operation:RemoteActionSessionPing,Text:"abc"},
  {RequestID:testActionID,Operation:RemoteActionTasksRun,TaskID:123,Choice:1,Text:"abc"},
 }{
  client,_,_:=participants(t)
  if _,err:=SealRemoteAction(client,action);err==nil{
   t.Fatalf("text escaped operation/choice binding: %+v",action)
  }
 }
}

func TestTaskPromptInspectRejectsUnknownAndMixedInputs(t *testing.T){
 for _,value:=range []RemoteTaskInspect{
  {TaskID:7,Eligible:true,InputID:"version",InputType:"promptString",
   Choices:[]string{"unexpected"}},
  {TaskID:7,Eligible:true,InputID:"bad/id",InputType:"promptString"},
  {TaskID:7,Eligible:true,InputID:"version",InputType:"shell"},
 }{
  _,agent,_:=participants(t)
  if _,err:=SealRemoteActionTasksInspectResult(agent,testActionID,value);err==nil{
   t.Fatalf("unsafe prompt metadata allowed: %+v",value)
  }
 }
}
