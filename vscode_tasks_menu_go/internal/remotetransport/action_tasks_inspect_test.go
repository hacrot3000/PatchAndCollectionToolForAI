package remotetransport

import (
 "errors"
 "testing"
)

func TestTaskInspectTypedE2EERequestResponse(t *testing.T){
 client,agent,_:=participants(t)
 req,err:=SealRemoteAction(client,RemoteAction{
  RequestID:testActionID,Operation:RemoteActionTasksInspect,TaskID:123,
 })
 if err!=nil{t.Fatal(err)}
 action,err:=OpenRemoteAction(agent,req)
 if err!=nil||action.TaskID!=123||action.Operation!=RemoteActionTasksInspect{
  t.Fatalf("failed to bind selected task ID: %+v %v",action,err)
 }
 frame,err:=SealRemoteActionTasksInspectResult(agent,testActionID,
  RemoteTaskInspect{TaskID:123,Eligible:true})
 if err!=nil{t.Fatal(err)}
 result,err:=OpenRemoteActionTasksInspectResult(client,frame,testActionID,123)
 if err!=nil||!result.Eligible||result.TaskID!=123{t.Fatalf("%+v %v",result,err)}
}
func TestTaskInspectRejectsWrongOrMissingSelection(t *testing.T){
 for _,action:=range []RemoteAction{
  {RequestID:testActionID,Operation:RemoteActionTasksInspect},
  {RequestID:testActionID,Operation:RemoteActionTasksInspect,TaskID:-1},
  {RequestID:testActionID,Operation:RemoteActionTasksInspect,TaskID:2147483648},
  {RequestID:testActionID,Operation:RemoteActionSessionPing,TaskID:1},
 }{
  client,_,_:=participants(t)
  if _,err:=SealRemoteAction(client,action);!errors.Is(err,ErrInvalidRemoteAction){
   t.Fatalf("invalid selection allowed: %+v %v",action,err)
  }
 }
}
func TestTaskInspectWrongIDPoisonsAuthenticatedReply(t *testing.T){
 client,agent,_:=participants(t)
 frame,err:=SealRemoteActionTasksInspectResult(agent,testActionID,
  RemoteTaskInspect{TaskID:4,Eligible:true})
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteActionTasksInspectResult(client,frame,testActionID,5);err==nil{
  t.Fatal("TaskDeck allowed wrong selected task ID")
 }
 if client.Established(){t.Fatal("mismatched result did not revoke E2EE channel")}
}

func TestRemoteTaskInspectSinglePickStringRoundTrip(t *testing.T){
 client,agent,_:=participants(t)
 expected:=RemoteTaskInspect{
  TaskID:123,Eligible:true,InputID:"target",
  Choices:[]string{"main","gateway"},
 }
 frame,err:=SealRemoteActionTasksInspectResult(agent,testActionID,expected)
 if err!=nil{t.Fatal(err)}
 actual,err:=OpenRemoteActionTasksInspectResult(client,frame,testActionID,123)
 if err!=nil||actual.InputID!="target"||len(actual.Choices)!=2||
  actual.Choices[0]!="main"||actual.Choices[1]!="gateway"{
  t.Fatalf("fixed pick options did not survive E2EE: %+v %v",actual,err)
 }
}
func TestRemoteTaskInspectRejectsUnsafePickMetadata(t *testing.T){
 for _,item:=range []RemoteTaskInspect{
  {TaskID:2,Eligible:true,InputID:"bad/id",Choices:[]string{"main"}},
  {TaskID:2,Eligible:true,InputID:"target",Choices:[]string{}},
  {TaskID:2,Eligible:true,InputID:"target",Choices:[]string{"a","a"}},
  {TaskID:2,Eligible:true,InputID:"target",Choices:[]string{"a\n"}},
  {TaskID:2,Eligible:true,InputID:"target",Choices:[]string{"<script>"}},
 }{
  _,agent,_:=participants(t)
  if _,err:=SealRemoteActionTasksInspectResult(agent,testActionID,item);err==nil{
   t.Fatalf("untrusted choice metadata was accepted: %+v",item)
  }
 }
}
