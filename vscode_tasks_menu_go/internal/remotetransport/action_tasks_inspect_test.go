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
