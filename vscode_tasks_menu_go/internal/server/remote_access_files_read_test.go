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

func TestRemoteFilesSelectedAllowlistedRootAndStaleSnapshot(t *testing.T){
 s,session,_:=remoteFileReadFixture(t)
 t.Setenv("TASKDECK_REMOTE_FILES_SELECT","1")
 t.Setenv("TASKDECK_REMOTE_FILES_SELECT_NAMES","selected.txt")
 if err:=os.WriteFile(filepath.Join(s.Workspace,"selected.txt"),
  []byte(strings.Repeat("chosen by index, not path\n",12)),0600);err!=nil{t.Fatal(err)}
 ctx:=context.Background()
 if _,err:=s.readRemoteFileTextSelected(ctx,session,1,remoteDispatchTestID);
  err==nil{t.Fatal("selected file read before root E2EE listing")}
 listing,err:=s.readRemoteFilesRootPage(ctx,session,true)
 if err!=nil{t.Fatal(err)}
 selectedIndex:=0
 for i,entry:=range listing.Items{
  if entry=="F:selected.txt"{selectedIndex=i+1}
 }
 if selectedIndex==0{t.Fatalf("selected host file not in first root page: %+v",listing)}
 page,err:=s.readRemoteFileTextSelected(ctx,session,selectedIndex,remoteDispatchTestID)
 if err!=nil||page.Name!="selected.txt"||!page.More{
  t.Fatalf("allowlisted selection denied: %+v %v",page,err)
 }
 // Changing the root listing, even without touching the selected file,
 // invalidates every continuation of a selected source snapshot.
 if err:=os.WriteFile(filepath.Join(s.Workspace,"aaa.txt"),[]byte("new"),0600);err!=nil{t.Fatal(err)}
 if _,err:=s.readRemoteFileTextPage(ctx,session,false,remoteDispatchTestID);
  err==nil{t.Fatal("root changed and existing selected-file cursor survived")}
 if _,err:=s.readRemoteFileTextPage(ctx,session,false,remoteDispatchTestID);
  err==nil{t.Fatal("stale selected cursor was reusable")}
 if _,err:=s.readRemoteFileTextSelected(ctx,session,selectedIndex,remoteDispatchTestID);
  err==nil{t.Fatal("old root list index was accepted after insertion")}
}
func TestRemoteFilesSelectedRequiresHostExplicitAllowlist(t *testing.T){
 s,session,_:=remoteFileReadFixture(t)
 t.Setenv("TASKDECK_REMOTE_FILES_SELECT","1")
 if err:=os.WriteFile(filepath.Join(s.Workspace,"approved.txt"),[]byte("public"),0600);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(s.Workspace,"private.txt"),[]byte("private"),0600);err!=nil{t.Fatal(err)}
 listing,err:=s.readRemoteFilesRootPage(context.Background(),session,true)
 if err!=nil{t.Fatal(err)}
 approvedIndex,privateIndex:=0,0
 for i,entry:=range listing.Items{
  if entry=="F:approved.txt"{approvedIndex=i+1}
  if entry=="F:private.txt"{privateIndex=i+1}
 }
 if approvedIndex==0||privateIndex==0{t.Fatalf("bad root fixture: %+v",listing)}
 t.Setenv("TASKDECK_REMOTE_FILES_SELECT_NAMES","approved.txt")
 if _,err:=s.readRemoteFileTextSelected(context.Background(),session,privateIndex,remoteDispatchTestID);
  err==nil{t.Fatal("non-approved file content disclosed")}
 good,err:=s.readRemoteFileTextSelected(context.Background(),session,approvedIndex,remoteDispatchTestID)
 if err!=nil||good.Text!="public"{
  t.Fatalf("allowlisted file read failed: %+v %v",good,err)
 }
 for _,names:=range []string{
  "",".env","approved.txt,../../etc/passwd","approved.txt,approved.txt",
  "approved.txt,private.txt,private.txt",
 }{
  t.Setenv("TASKDECK_REMOTE_FILES_SELECT_NAMES",names)
  if _,err:=s.readRemoteFileTextSelected(context.Background(),session,approvedIndex,remoteDispatchTestID);
   err==nil{t.Fatalf("invalid host allowlist accepted: %q",names)}
 }
 t.Setenv("TASKDECK_REMOTE_FILES_SELECT_NAMES","approved.txt")
 t.Setenv("TASKDECK_REMOTE_FILES_SELECT","0")
 if _,err:=s.readRemoteFileTextSelected(context.Background(),session,approvedIndex,remoteDispatchTestID);
  err==nil{t.Fatal("selected read bypassed host opt-in")}
}
func TestRemoteFilesSelectedNeverGuessesUnissuedPage(t *testing.T){
 s,session,_:=remoteFileReadFixture(t)
 t.Setenv("TASKDECK_REMOTE_FILES_SELECT","1")
 t.Setenv("TASKDECK_REMOTE_FILES_SELECT_NAMES","file09.txt")
 for i:=0;i<10;i++{
  filename:="file"+string(rune('0'+i/10))+string(rune('0'+i%10))+".txt"
  if err:=os.WriteFile(filepath.Join(s.Workspace,filename),[]byte("safe"),0600);err!=nil{t.Fatal(err)}
 }
 first,err:=s.readRemoteFilesRootPage(context.Background(),session,true)
 if err!=nil||!first.More||len(first.Items)!=8{t.Fatalf("expected root pagination: %+v %v",first,err)}
 if _,err:=s.readRemoteFileTextSelected(context.Background(),session,10,remoteDispatchTestID);
  err==nil{t.Fatal("unissued file index could be guessed")}
 second,err:=s.readRemoteFilesRootPage(context.Background(),session,false)
 if err!=nil||second.More||len(second.Items)!=2{t.Fatalf("second root page: %+v %v",second,err)}
 page,err:=s.readRemoteFileTextSelected(context.Background(),session,10,remoteDispatchTestID)
 if err!=nil||page.Name!="file09.txt"{t.Fatalf("approved issued index denied: %+v %v",page,err)}
}
func TestRemoteFilesSelectedE2EEPrincipalAndAudit(t *testing.T){
 if runtime.GOOS=="windows"{t.Skip("Unix-only Files selection")}
 s,session,client,agent:=remoteDispatchFixture(t)
 t.Setenv("TASKDECK_REMOTE_FILES_ROOT","1")
 t.Setenv("TASKDECK_REMOTE_FILES_READ","1")
 t.Setenv("TASKDECK_REMOTE_FILES_SELECT","1")
 t.Setenv("TASKDECK_REMOTE_FILES_SELECT_NAMES","notes.txt")
 if err:=os.WriteFile(filepath.Join(s.Workspace,"notes.txt"),[]byte("approved"),0600);err!=nil{t.Fatal(err)}
 first,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionFilesRoot,
 })
 if err!=nil{t.Fatal(err)}
 frame,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,first)
 if err!=nil{t.Fatal(err)}
 list,err:=remotetransport.OpenRemoteFilesRoot(client,frame,remoteDispatchTestID)
 if err!=nil{t.Fatal(err)}
 index:=0
 for i,v:=range list.Items{if v=="F:notes.txt"{index=i+1}}
 if index==0{t.Fatalf("expected notes file in issued E2EE root %+v",list)}
 selectFrame,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionFilesReadSelect,
  FileIndex:index,
 })
 if err!=nil{t.Fatal(err)}
 reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,selectFrame)
 if err!=nil{t.Fatalf("E2EE selected Files read denied: %v",err)}
 result,err:=remotetransport.OpenRemoteFilesReadPage(client,reply,remoteDispatchTestID)
 if err!=nil||result.Name!="notes.txt"||result.Text!="approved"{
  t.Fatalf("typed selected file result: %+v %v",result,err)
 }
 project,err:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
 if err!=nil{t.Fatal(err)}
 audits,err:=s.Identity.ListAudit(context.Background(),identity.AuditQuery{
  ProjectID:project.ID,Action:"remote.action.files_read_select",Limit:5,
 })
 if err!=nil||len(audits)==0||audits[0].UserID==nil||
  *audits[0].UserID!=session.userID||audits[0].ResourceType!="files"{
  t.Fatalf("selected Files read missing local authenticated audit: %+v %v",audits,err)
 }
}
