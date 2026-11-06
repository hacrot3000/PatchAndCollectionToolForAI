package server

import (
 "context"
 "os"
 "path/filepath"
 "runtime"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func remoteFileReadFixture(t *testing.T)(*Server,*remoteTaskDeckSession,string){
 t.Helper()
 if runtime.GOOS=="windows"{t.Skip("Unix-only file read")}
 s,session,_,_:=remoteDispatchFixture(t)
 t.Setenv("TASKDECK_REMOTE_FILES_ROOT","1")
 t.Setenv("TASKDECK_REMOTE_FILES_READ","1")
 t.Setenv("TASKDECK_REMOTE_FILES_READ_NAME","notes.txt")
 file:=filepath.Join(s.Workspace,"notes.txt")
 return s,session,file
}
func TestRemoteFileReadPagedImmutableHostSelectedText(t *testing.T){
 s,session,file:=remoteFileReadFixture(t)
 original:=strings.Repeat("Safe file content\n",70)
 if err:=os.WriteFile(file,[]byte(original),0600);err!=nil{t.Fatal(err)}
 first,err:=s.readRemoteFileTextPage(context.Background(),session,false,remoteDispatchTestID)
 if err==nil{t.Fatalf("page continuation accepted without first: %+v",first)}
 first,err=s.readRemoteFileTextPage(context.Background(),session,true,remoteDispatchTestID)
 if err!=nil||first.Name!="notes.txt"||first.Page!=1||!first.More{
  t.Fatalf("invalid first text page: %+v %v",first,err)
 }
 var captured strings.Builder
 captured.WriteString(first.Text)
 current:=first
 for current.More{
  current,err=s.readRemoteFileTextPage(context.Background(),session,false,remoteDispatchTestID)
  if err!=nil{t.Fatalf("page continuation: %v",err)}
  captured.WriteString(current.Text)
  if current.Page>16{t.Fatal("unbounded page count")}
 }
 if captured.String()!=original{t.Fatalf("incomplete file text: %q",captured.String())}
 if _,err:=s.readRemoteFileTextPage(context.Background(),session,false,remoteDispatchTestID);
  err==nil{t.Fatal("replayed terminal page accepted")}
 _,err=s.readRemoteFileTextPage(context.Background(),session,true,remoteDispatchTestID)
 if err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(file,[]byte("modified while paginating\n"),0600);err!=nil{t.Fatal(err)}
 if _,err:=s.readRemoteFileTextPage(context.Background(),session,false,remoteDispatchTestID);
  err==nil{t.Fatal("changed file allowed stale E2EE continuation")}
 if _,err:=s.readRemoteFileTextPage(context.Background(),session,false,remoteDispatchTestID);
  err==nil{t.Fatal("stale page cursor remained reusable")}
}
func TestRemoteFileReadRejectsUnapprovedSymlinkAndBinary(t *testing.T){
 s,session,file:=remoteFileReadFixture(t)
 t.Setenv("TASKDECK_REMOTE_FILES_READ","0")
 if _,err:=s.readRemoteFileTextPage(context.Background(),session,true,remoteDispatchTestID);
  err==nil{t.Fatal("host opt-in bypassed")}
 t.Setenv("TASKDECK_REMOTE_FILES_READ","1")
 t.Setenv("TASKDECK_REMOTE_FILES_READ_NAME","../../etc/passwd")
 if _,err:=s.readRemoteFileTextPage(context.Background(),session,true,remoteDispatchTestID);
  err==nil{t.Fatal("user or host path traversal accepted")}
 t.Setenv("TASKDECK_REMOTE_FILES_READ_NAME","notes.txt")
 if err:=os.WriteFile(file,[]byte("before\nunsafe\x00after"),0600);err!=nil{t.Fatal(err)}
 if _,err:=s.readRemoteFileTextPage(context.Background(),session,true,remoteDispatchTestID);
  err==nil{t.Fatal("binary data partially disclosed")}
 if err:=os.WriteFile(file,[]byte(strings.Repeat("a",2049)),0600);err!=nil{t.Fatal(err)}
 if _,err:=s.readRemoteFileTextPage(context.Background(),session,true,remoteDispatchTestID);
  err==nil{t.Fatal("oversized file exposed partial content")}
 if err:=os.Remove(file);err!=nil{t.Fatal(err)}
 outside:=filepath.Join(t.TempDir(),"secret.txt")
 if err:=os.WriteFile(outside,[]byte("PRIVATE"),0600);err!=nil{t.Fatal(err)}
 if err:=os.Symlink(outside,file);err!=nil{t.Fatal(err)}
 if _,err:=s.readRemoteFileTextPage(context.Background(),session,true,remoteDispatchTestID);
  err==nil{t.Fatal("symlink escaped workspace")}
 nested:=filepath.Join(s.Workspace,"nested")
 if err:=os.MkdirAll(nested,0700);err!=nil{t.Fatal(err)}
 s.Workspace=nested
 if _,err:=s.readRemoteFileTextPage(context.Background(),session,true,remoteDispatchTestID);
  err==nil{t.Fatal("read escaped nested workspace to parent")}
}
func TestRemoteFileReadE2EEAuditAndNoCallerPath(t *testing.T){
 if runtime.GOOS=="windows"{t.Skip("Unix-only file read")}
 s,session,client,agent:=remoteDispatchFixture(t)
 t.Setenv("TASKDECK_REMOTE_FILES_ROOT","1")
 t.Setenv("TASKDECK_REMOTE_FILES_READ","1")
 t.Setenv("TASKDECK_REMOTE_FILES_READ_NAME","notes.txt")
 if err:=os.WriteFile(filepath.Join(s.Workspace,"notes.txt"),[]byte("Host approved\n"),0600);err!=nil{t.Fatal(err)}
 frame,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionFilesReadStart,
 })
 if err!=nil{t.Fatal(err)}
 encrypted,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,frame)
 if err!=nil{t.Fatalf("authorized text preview denied: %v",err)}
 page,err:=remotetransport.OpenRemoteFilesReadPage(client,encrypted,remoteDispatchTestID)
 if err!=nil||page.Name!="notes.txt"||page.Text!="Host approved\n"{
  t.Fatalf("E2EE file read mismatched: %+v %v",page,err)
 }
 project,err:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
 if err!=nil{t.Fatal(err)}
 audit,err:=s.Identity.ListAudit(context.Background(),identity.AuditQuery{
  ProjectID:project.ID,Action:"remote.action.files_read_start",Limit:5,
 })
 if err!=nil||len(audit)==0||audit[0].UserID==nil||
  *audit[0].UserID!=session.userID||audit[0].ResourceType!="files"{
  t.Fatalf("file read lacks TaskDeck-local audit: %+v %v",audit,err)
 }
}
