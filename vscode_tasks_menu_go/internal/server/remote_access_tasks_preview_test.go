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

const fixtureTaskLabels = `{"tasks":[
 {"label":"Build app","menuLabel":"Build app","type":"shell","command":"echo never-expose-secret"},
 {"label":"Unit tests","command":"go test ./..."},
 {"label":"Deploy","command":"echo should-not-expose"},
 {"label":"Fourth task","command":"false"}
]}`

func writeRemoteTasksPreviewFixture(t *testing.T,root,contents string){
 t.Helper()
 dir:=filepath.Join(root,".vscode")
 if err:=os.MkdirAll(dir,0700);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(dir,"tasks.json"),[]byte(contents),0600);err!=nil{
  t.Fatal(err)
 }
}

func TestRemoteTasksPreviewLocalTaskDeckE2EEAndAudit(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 s,session,client,agent:=remoteDispatchFixture(t)
 writeRemoteTasksPreviewFixture(t,s.Workspace,fixtureTaskLabels)
 req,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionTasksPreview,
 })
 if err!=nil{t.Fatal(err)}
 reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req)
 if err!=nil{t.Fatal(err)}
 preview,err:=remotetransport.OpenRemoteActionTasksPreviewResult(client,reply,remoteDispatchTestID)
 if err!=nil||preview.Total!=4||len(preview.Labels)!=3||
  preview.Labels[0]!="Build app" || preview.Labels[2]!="Deploy" {
  t.Fatalf("wrong authenticated TaskDeck tasks preview: %+v %v",preview,err)
 }
 for _,label:=range preview.Labels{
  if strings.Contains(label,"secret")||strings.Contains(label,"go test")||
   strings.Contains(label,"false")||strings.Contains(label,"should-not-expose"){
   t.Fatal("TaskDeck task command leaked into read-only preview")
  }
 }
 project,err:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
 if err!=nil{t.Fatal(err)}
 audits,err:=s.Identity.ListAudit(context.Background(),identity.AuditQuery{
  ProjectID:project.ID,Action:"remote.action.tasks_preview",Limit:5,
 })
 if err!=nil||len(audits)!=1||audits[0].UserID==nil||
  *audits[0].UserID!=session.userID {
  t.Fatalf("remote tasks.preview not audited against local TaskDeck user: %+v %v",audits,err)
 }
 if _,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req);err==nil{
  t.Fatal("replayed E2EE task preview gained TaskDeck authority")
 }
}

func TestRemoteTasksPreviewRequiresSeparateHostOptIn(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","0")
 s,session,client,agent:=remoteDispatchFixture(t)
 writeRemoteTasksPreviewFixture(t,s.Workspace,fixtureTaskLabels)
 req,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionTasksPreview,
 })
 if err!=nil{t.Fatal(err)}
 reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req)
 if !errors.Is(err,remotetransport.ErrRemoteActionDenied)||len(reply)!=0 {
  t.Fatal("TaskDeck exposed tasks without independent host-only opt-in")
 }
}

func TestRemoteTasksPreviewFailsClosedOnSymlinkOrOversizedFile(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 s,session,_,_:=remoteDispatchFixture(t)
 writeRemoteTasksPreviewFixture(t,s.Workspace,fixtureTaskLabels)
 path:=filepath.Join(s.Workspace,".vscode","tasks.json")
 if err:=os.Remove(path);err!=nil{t.Fatal(err)}
 external:=filepath.Join(t.TempDir(),"outside-tasks.json")
 if err:=os.WriteFile(external,[]byte(fixtureTaskLabels),0600);err!=nil{t.Fatal(err)}
 if err:=os.Symlink(external,path);err!=nil{t.Fatal(err)}
 if _,err:=s.readRemoteTasksPreview(context.Background(),session);err==nil{
  t.Fatal("TaskDeck followed a tasks.json symlink outside the workspace")
 }
 if err:=os.Remove(path);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(path,[]byte(strings.Repeat("X",maxRemoteTasksPreviewSourceBytes+1)),0600);err!=nil{
  t.Fatal(err)
 }
 if _,err:=s.readRemoteTasksPreview(context.Background(),session);err==nil{
  t.Fatal("remote preview read an oversized tasks.json")
 }
}

func TestRemoteTasksPreviewScopeAndLabelSanitization(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 s,session,_,_:=remoteDispatchFixture(t)
 writeRemoteTasksPreviewFixture(t,s.Workspace,fixtureTaskLabels)
 session.projectKey="forged-other-project"
 if _,err:=s.readRemoteTasksPreview(context.Background(),session);err==nil{
  t.Fatal("cross-project task enumeration accepted")
 }
 for _,label:=range []string{"hello\ncommand","invisible\u202eoverride","bad\x00name"}{
  if safeRemoteTaskPreviewLabel(label)!=""{
   t.Fatal("unsafe task label survived sanitizer")
  }
 }
 long:="Đây là một tên tác vụ dài đủ để cần cắt ngắn nhằm giữ an toàn"
 bounded:=safeRemoteTaskPreviewLabel(long)
 if bounded==""||len(bounded)>remotetransport.MaxRemoteTaskLabelBytes{
  t.Fatalf("UTF-8 task label truncation failed: %q",bounded)
 }
}

func TestRemoteTasksPreviewEmptyWorkspaceIsReadOnlySuccess(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 s,session,_,_:=remoteDispatchFixture(t)
 // An empty workspace is a valid GUI state and must not invalidate a
 // verified user's E2EE session merely because tasks.json is not set up.
 dir:=filepath.Join(s.Workspace,".vscode")
 _=os.Remove(filepath.Join(dir,"tasks.json"))
 preview,err:=s.readRemoteTasksPreview(context.Background(),session)
 if err!=nil||preview.Total!=0||preview.Labels==nil||len(preview.Labels)!=0{
  t.Fatalf("empty TaskDeck workspace failed safe summary: %+v %v",preview,err)
 }
}
