package server

import (
 "context"
 "fmt"
 "os"
 "path/filepath"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func fixtureLongTaskMenu(t *testing.T,root string,count int){
 t.Helper()
 var entries []string
 for i:=0;i<count;i++{
  entries=append(entries,fmt.Sprintf(
   "{\"label\":\"Task %03d\",\"command\":\"echo SECRET_NEVER_EXPOSED\"}",i))
 }
 writeRemoteTasksPreviewFixture(t,root,"{\"tasks\":["+strings.Join(entries,",")+"]}")
}

func remoteTaskMenuAction(t *testing.T,s *Server,session *remoteTaskDeckSession,
 client,agent *remotetransport.Channel,id,operation string)(
 remotetransport.RemoteTasksPreview,error,
){
 t.Helper()
 frame,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:id,Operation:operation,
 })
 if err!=nil{t.Fatal(err)}
 reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,frame)
 if err!=nil{return remotetransport.RemoteTasksPreview{},err}
 return remotetransport.OpenRemoteActionTasksPreviewResult(client,reply,id)
}

func TestRemoteTaskPaginationRealHostRBACAndAudit(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 s,session,client,agent:=remoteDispatchFixture(t)
 fixtureLongTaskMenu(t,s.Workspace,56)
 pages:=[]remotetransport.RemoteTasksPreview{}
 for index,op:=range []string{
  remotetransport.RemoteActionTasksPreview,
  remotetransport.RemoteActionTasksPreviewNext,
  remotetransport.RemoteActionTasksPreviewNext,
 }{
  id:=fmt.Sprintf("%032x",index+1)
  page,err:=remoteTaskMenuAction(t,s,session,client,agent,id,op)
  if err!=nil{t.Fatalf("authorized host task page %d failed: %v",index,err)}
  if page.Total!=56||len(page.Labels)==0{
   t.Fatalf("page %d dropped valid task labels: %+v",index,page)
  }
  pages=append(pages,page)
 }
 all:=[]string{}
 for _,p:=range pages {all=append(all,p.Labels...)}
 if len(all)!=56 {t.Fatalf("paging skipped/duplicated labels: got %d, want 56",len(all))}
 for i,label:=range all{
  want:=fmt.Sprintf("Task %03d",i)
  if label!=want {t.Fatalf("wrong page cursor at %d: %q, expected %q",i,label,want)}
  if strings.Contains(label,"SECRET_NEVER_EXPOSED"){t.Fatal("command leaked into task label")}
 }
 project,err:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
 if err!=nil{t.Fatal(err)}
 events,err:=s.Identity.ListAudit(context.Background(),identity.AuditQuery{
  ProjectID:project.ID,Action:"remote.action.tasks_preview_next",Limit:5,
 })
 if err!=nil||len(events)!=2||events[0].UserID==nil||
  *events[0].UserID!=session.userID{
  t.Fatalf("next-page action lacks local TaskDeck principal audit: %+v %v",events,err)
 }
}

func TestRemoteTaskNextRequiresInitialReadAndStableSource(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 s,session,client,agent:=remoteDispatchFixture(t)
 fixtureLongTaskMenu(t,s.Workspace,40)
 // No PHP-supplied offset can bootstrap the host-owned pagination cursor.
 next,err:=remoteTaskMenuAction(t,s,session,client,agent,
  strings.Repeat("a",32),remotetransport.RemoteActionTasksPreviewNext)
 if err==nil||len(next.Labels)>0 {t.Fatal("next page without authorized initial preview accepted")}
 // Denied operation may trigger channel retirement in a real Relay;
 // use a new native E2EE session to prove source-change refusal.
 s,session,client,agent=remoteDispatchFixture(t)
 fixtureLongTaskMenu(t,s.Workspace,40)
 first,err:=remoteTaskMenuAction(t,s,session,client,agent,
  strings.Repeat("b",32),remotetransport.RemoteActionTasksPreview)
 if err!=nil||len(first.Labels)==0{t.Fatalf("initial task page failed: %v",err)}
 path:=filepath.Join(s.Workspace,".vscode","tasks.json")
 data,err:=os.ReadFile(path)
 if err!=nil{t.Fatal(err)}
 data=append(data,byte(' '))
 if err:=os.WriteFile(path,data,0600);err!=nil{t.Fatal(err)}
 next,err=remoteTaskMenuAction(t,s,session,client,agent,
  strings.Repeat("c",32),remotetransport.RemoteActionTasksPreviewNext)
 if err==nil||len(next.Labels)>0{
  t.Fatal("changed host task source was silently mixed with previous page")
 }
}

func TestRemoteTaskPagingDeniedWhenHostRevokesOptIn(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 s,session,client,agent:=remoteDispatchFixture(t)
 fixtureLongTaskMenu(t,s.Workspace,40)
 first,err:=remoteTaskMenuAction(t,s,session,client,agent,
  strings.Repeat("d",32),remotetransport.RemoteActionTasksPreview)
 if err!=nil||len(first.Labels)==0{t.Fatalf("preview failed: %v",err)}
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","0")
 page,err:=remoteTaskMenuAction(t,s,session,client,agent,
  strings.Repeat("e",32),remotetransport.RemoteActionTasksPreviewNext)
 if err==nil||len(page.Labels)>0{
  t.Fatal("host allowed next page after removing tasks.preview opt-in")
 }
}

func TestRemoteTaskNextRejectsSwitchedWorkspaceEvenWithSameTaskBytes(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 s,session,client,agent:=remoteDispatchFixture(t)
 fixtureLongTaskMenu(t,s.Workspace,44)
 first,err:=remoteTaskMenuAction(t,s,session,client,agent,
  strings.Repeat("1",32),remotetransport.RemoteActionTasksPreview)
 if err!=nil||len(first.Labels)==0 {t.Fatalf("initial page failed: %v",err)}
 source,err:=os.ReadFile(filepath.Join(s.Workspace,".vscode","tasks.json"))
 if err!=nil{t.Fatal(err)}
 other:=t.TempDir()
 writeRemoteTasksPreviewFixture(t,other,string(source))
 s.Workspace=other
 _,_,_,err=s.readRemoteTaskPage(context.Background(),session,
  session.taskPageCursor,&session.taskPageHash,strings.Repeat("2",32))
 if err==nil {
  t.Fatal("task paging continued in another canonical workspace")
 }
}
