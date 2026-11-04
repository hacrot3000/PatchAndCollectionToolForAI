package server

import (
 "context"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
 "bletonfc/vscode_tasks_menu/internal/tasks"
)

const remoteInspectFixtureSource=`{"tasks":[{"label":"Inspect","type":"process","command":"echo","args":["ok"]}]}`

func TestRemoteInspectRequiresIssuedSnapshotAndTaskDeckAuthorization(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 // An unauthorized selected ID may terminate the actual E2EE session.
 // Never reuse such a session to test later authorized operations.
 t.Run("unissued task cannot be inspected",func(t *testing.T){
  s,session,client,agent:=remoteDispatchFixture(t)
  writeRemoteTasksPreviewFixture(t,s.Workspace,remoteInspectFixtureSource)
  list,err:=tasks.Load(s.Workspace)
  if err!=nil||len(list)!=1{t.Fatalf("invalid host task: %v",err)}
  req,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
   RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionTasksInspect,
   TaskID:list[0].ID,
  })
  if err!=nil{t.Fatal(err)}
  if reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req);
   err==nil||len(reply)!=0{
   t.Fatal("unissued task selection bypassed host-owned snapshot")
  }
 })
 t.Run("published task survives exact-source revalidation",func(t *testing.T){
  s,session,client,agent:=remoteDispatchFixture(t)
  // Retain the original TaskDeck workspace: it binds the owner-approved
  // client private-key registry and real project authorization.
  writeRemoteTasksPreviewFixture(t,s.Workspace,remoteInspectFixtureSource)
  list,err:=tasks.Load(s.Workspace)
  if err!=nil||len(list)!=1{t.Fatalf("invalid task fixture: %v",err)}
  req,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
   RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionTasksPreview,
  })
  if err!=nil{t.Fatal(err)}
  sealed,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req)
  if err!=nil{t.Fatal(err)}
  page,err:=remotetransport.OpenRemoteActionTasksPreviewResult(client,sealed,remoteDispatchTestID)
  if err!=nil||len(page.IDs)!=1||page.IDs[0]!=list[0].ID{
   t.Fatalf("missing host-issued stable ID: %+v %v",page,err)
  }
  inspect:=func(taskID int)(remotetransport.RemoteTaskInspect,error){
   req,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
    RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionTasksInspect,
    TaskID:taskID,
   })
   if err!=nil{return remotetransport.RemoteTaskInspect{},err}
   sealed,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req)
   if err!=nil{return remotetransport.RemoteTaskInspect{},err}
   return remotetransport.OpenRemoteActionTasksInspectResult(
    client,sealed,remoteDispatchTestID,taskID)
  }
  result,err:=inspect(list[0].ID)
  if err!=nil||!result.Eligible||result.TaskID!=list[0].ID{
   t.Fatalf("unchanged host-selected task rejected: %+v %v",result,err)
  }
  if _,err:=inspect(list[0].ID+1);err==nil{
   t.Fatal("unpublished guessed Task.ID was admitted")
  }
 })
}
