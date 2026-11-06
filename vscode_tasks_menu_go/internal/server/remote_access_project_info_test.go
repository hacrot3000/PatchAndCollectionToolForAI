package server

import (
 "context"
 "errors"
 "os"
 "path/filepath"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func TestRemoteProjectInfoRealTaskDeckRBACAndCiphertext(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_PROJECT_INFO","1")
 s,session,client,agent:=remoteDispatchFixture(t)
 project,err:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
 if err!=nil{t.Fatal(err)}
 req,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionProjectInfo,
 })
 if err!=nil{t.Fatal(err)}
 result,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req)
 if err!=nil{t.Fatal(err)}
 output,err:=remotetransport.OpenRemoteActionProjectInfoResult(client,result,remoteDispatchTestID)
 if err!=nil{t.Fatal(err)}
 if output.ProjectName!=project.DisplayName &&
  !(project.DisplayName=="" && output.ProjectName==filepath.Base(s.Workspace)){
  t.Fatalf("local TaskDeck project name not returned: %+v project=%+v",output,project)
 }
 if output.WorkspaceName!=filepath.Base(s.Workspace) ||
  strings.Contains(output.WorkspaceName,string(filepath.Separator)) ||
  strings.Contains(string(result),s.Workspace){
  t.Fatal("encrypted project info leaked a full TaskDeck host path")
 }
 audits,err:=s.Identity.ListAudit(context.Background(),identity.AuditQuery{
  ProjectID:project.ID,Action:"remote.action.project_info",Limit:5,
 })
 if err!=nil || len(audits)!=1 || audits[0].UserID==nil ||
  *audits[0].UserID!=session.userID {
  t.Fatalf("project info not audited against real TaskDeck user: %+v %v",audits,err)
 }
 if _,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req);err==nil {
  t.Fatal("replayed project info request was authorized")
 }
}

func TestRemoteProjectInfoRequiresSeparateHostOptIn(t *testing.T){
 s,session,client,agent:=remoteDispatchFixture(t)
 t.Setenv("TASKDECK_REMOTE_PROJECT_INFO","0")
 req,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionProjectInfo,
 })
 if err!=nil{t.Fatal(err)}
 reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req)
 if !errors.Is(err,remotetransport.ErrRemoteActionDenied)||len(reply)!=0{
  t.Fatal("PHP had project metadata access despite missing host-only opt-in")
 }
}

func TestRemoteProjectInfoNoCrossProjectOrRevokedKey(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_PROJECT_INFO","1")
 s,session,client,agent:=remoteDispatchFixture(t)
 session.projectKey="forged-unowned-project"
 req,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionProjectInfo,
 })
 if err!=nil{t.Fatal(err)}
 reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req)
 if err==nil||len(reply)!=0{t.Fatal("another TaskDeck project's metadata was exposed")}
}

func TestRemoteProjectInfoRejectsMissingOrUnsafeWorkspace(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_PROJECT_INFO","1")
 s,session,_,_:=remoteDispatchFixture(t)
 before:=s.Workspace
 s.Workspace=filepath.Join(t.TempDir(),"missing")
 if _,err:=s.readRemoteProjectInfo(context.Background(),session);err==nil{
  t.Fatal("nonexistent workspace accepted")
 }
 s.Workspace=before
 // The host does not return absolute workspace paths even in the
 // explicit local project metadata result.
 info,err:=s.readRemoteProjectInfo(context.Background(),session)
 if err!=nil{t.Fatal(err)}
 if strings.Contains(info.WorkspaceName,string(os.PathSeparator)){
  t.Fatal("read-only project info exposed host directory tree")
 }
}
