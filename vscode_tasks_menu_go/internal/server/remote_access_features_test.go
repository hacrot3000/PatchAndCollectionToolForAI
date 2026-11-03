package server

import (
 "context"
 "errors"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func TestRemoteFeaturesHostOptInAndLocalUserAudit(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_PROJECT_INFO","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","0")
 t.Setenv("TASKDECK_REMOTE_GIT_STATUS","1")
 s,session,client,agent:=remoteDispatchFixture(t)
 firstID:=remoteDispatchTestID
 first,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:firstID,Operation:remotetransport.RemoteActionSessionFeatures,
 })
 if err!=nil{t.Fatal(err)}
 sealed,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,first)
 if err!=nil{t.Fatal(err)}
 info,err:=remotetransport.OpenRemoteActionFeaturesResult(client,sealed,firstID)
 if err!=nil||!info.ProjectInfo||info.TasksPreview||!info.GitStatus{
  t.Fatalf("host incorrectly exposed opt-in/RBAC state: %+v %v",info,err)
 }
 // Discovery is advisory only. The host must not cache flags: toggling
 // an opt-in between two authenticated requests is reflected immediately.
 t.Setenv("TASKDECK_REMOTE_PROJECT_INFO","0")
 t.Setenv("TASKDECK_REMOTE_GIT_STATUS","0")
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 id:="bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
 second,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:id,Operation:remotetransport.RemoteActionSessionFeatures,
 })
 if err!=nil{t.Fatal(err)}
 sealed,err=s.executeRemoteE2EEPing(context.Background(),session,agent,second)
 if err!=nil{t.Fatal(err)}
 info,err=remotetransport.OpenRemoteActionFeaturesResult(client,sealed,id)
 if err!=nil||info.ProjectInfo||!info.TasksPreview||info.GitStatus{
  t.Fatalf("stale feature cache survived opt-in change: %+v %v",info,err)
 }
 project,err:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
 if err!=nil{t.Fatal(err)}
 events,err:=s.Identity.ListAudit(context.Background(),identity.AuditQuery{
  ProjectID:project.ID,Action:"remote.action.session_features",Limit:5,
 })
 if err!=nil||len(events)!=2||events[0].UserID==nil||
  *events[0].UserID!=session.userID{
  t.Fatalf("E2EE feature probes lack host TaskDeck user audit: %+v %v",events,err)
 }
}

func TestRemoteFeaturesRequireApprovedClientAndProject(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_PROJECT_INFO","1")
 s,session,client,agent:=remoteDispatchFixture(t)
 session.clientProofVerified=false
 req,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionSessionFeatures,
 })
 if err!=nil{t.Fatal(err)}
 frame,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req)
 if !errors.Is(err,remotetransport.ErrRemoteActionDenied)||len(frame)!=0 {
  t.Fatal("unapproved client read TaskDeck host capability flags")
 }
}

func TestRemoteFeaturesFailClosedOnCrossProject(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_PROJECT_INFO","1")
 s,session,client,agent:=remoteDispatchFixture(t)
 session.projectKey="forged-other-project"
 req,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionSessionFeatures,
 })
 if err!=nil{t.Fatal(err)}
 frame,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req)
 if !errors.Is(err,remotetransport.ErrRemoteActionDenied)||len(frame)!=0 {
  t.Fatal("cross-project feature discovery bypassed local TaskDeck identity")
 }
}
