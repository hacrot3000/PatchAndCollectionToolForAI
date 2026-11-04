package server

import (
 "context"
 "os"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func TestRemoteInspectRequiresIssuedSnapshotAndTaskDeckAuthorization(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 workspace,source,list,_:=remoteTaskSnapshotFixture(t,
  `{"tasks":[{"label":"Inspect","type":"process","command":"echo","args":["ok"]}]}`)
 s,session,client,agent:=remoteDispatchFixture(t)
 s.Workspace=workspace
 inspect:=func(id int)(remotetransport.RemoteTaskInspect,error){
  req,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
   RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionTasksInspect,
   TaskID:id,
  })
  if err!=nil{return remotetransport.RemoteTaskInspect{},err}
  reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req)
  if err!=nil{return remotetransport.RemoteTaskInspect{},err}
  return remotetransport.OpenRemoteActionTasksInspectResult(client,reply,
   remoteDispatchTestID,id)
 }
 if _,err:=inspect(list[0].ID);err==nil{
  t.Fatal("unissued Task.ID must not pass host inspection")
 }
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
 result,err:=inspect(list[0].ID)
 if err!=nil||!result.Eligible{t.Fatalf("host inspect denied unchanged task: %+v %v",result,err)}
 // Read-only preflight MUST NOT launch the command. Task has no executable
 // side effects, and its definition is never returned to PHP or client.
 if _,err:=inspect(list[0].ID+1);err==nil{
  t.Fatal("guessed task ID should be rejected")
 }
 _=source
 _=os.ErrNotExist
}
