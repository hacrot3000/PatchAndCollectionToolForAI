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
func TestRemoteFilesRootStrictOptInAndScope(t *testing.T){
 if runtime.GOOS=="windows"{t.Skip("Unix-only Files preview")}
 s,_,_,_:=remoteDispatchFixture(t)
 root:=s.Workspace
 if err:=os.WriteFile(filepath.Join(root,"visible.txt"),[]byte("private contents"),0600);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(root,".secret"),[]byte("SECRET"),0600);err!=nil{t.Fatal(err)}
 if err:=os.Mkdir(filepath.Join(root,"docs"),0700);err!=nil{t.Fatal(err)}
 if err:=os.Symlink(filepath.Join(root,"visible.txt"),filepath.Join(root,"symlink.txt"));err!=nil{t.Fatal(err)}
 t.Setenv("TASKDECK_REMOTE_FILES_ROOT","0")
 if _,err:=s.readRemoteFilesRoot(context.Background());err==nil{t.Fatal("host opt-in bypassed")}
 t.Setenv("TASKDECK_REMOTE_FILES_ROOT","1")
 listing,err:=s.readRemoteFilesRoot(context.Background())
 if err!=nil{t.Fatal(err)}
 if !remotetransport.ValidRemoteFilesRoot(listing){t.Fatalf("invalid listing: %+v",listing)}
 joined:=strings.Join(listing.Items,",")
 if !strings.Contains(joined,"D:docs")||!strings.Contains(joined,"F:visible.txt")||
  strings.Contains(joined,"secret")||strings.Contains(joined,"symlink")||
  strings.Contains(joined,"private contents"){
  t.Fatalf("unauthorized names/data in root preview: %q",joined)
 }
 // Changing the configured workspace to a nested dir never walks upward.
 nested:=filepath.Join(root,"docs")
 s.Workspace=nested
 empty,err:=s.readRemoteFilesRoot(context.Background())
 if err!=nil||len(empty.Items)!=0{t.Fatalf("nested root escaped: %+v %v",empty,err)}
}
func TestRemoteFilesRootBoundedDirectory(t *testing.T){
 if runtime.GOOS=="windows"{t.Skip("Unix-only Files preview")}
 s,_,_,_:=remoteDispatchFixture(t)
 t.Setenv("TASKDECK_REMOTE_FILES_ROOT","1")
 for i:=0;i<12;i++{
  name:="item"+string(rune('a'+i))
  if err:=os.WriteFile(filepath.Join(s.Workspace,name),[]byte("x"),0600);err!=nil{t.Fatal(err)}
 }
 result,err:=s.readRemoteFilesRoot(context.Background())
 if err!=nil||len(result.Items)!=remotetransport.MaxRemoteFilesRootItems||!result.More{
  t.Fatalf("unbounded preview: %+v %v",result,err)
 }
}

func TestRemoteFilesRootPagedSnapshotAndStaleDenial(t *testing.T){
 if runtime.GOOS=="windows"{t.Skip("Unix-only Files preview")}
 s,session,_,_:=remoteDispatchFixture(t)
 s.Workspace=t.TempDir()
 t.Setenv("TASKDECK_REMOTE_FILES_ROOT","1")
 for i:=0;i<17;i++{
  name:="file"+string(rune('a'+i))
  if err:=os.WriteFile(filepath.Join(s.Workspace,name),[]byte("safe"),0600);err!=nil{t.Fatal(err)}
 }
 page,err:=s.readRemoteFilesRootPage(context.Background(),session,false)
 if err==nil{t.Fatalf("next page before first read accepted: %+v",page)}
 first,err:=s.readRemoteFilesRootPage(context.Background(),session,true)
 if err!=nil||len(first.Items)!=8||!first.More{t.Fatalf("first page: %+v %v",first,err)}
 second,err:=s.readRemoteFilesRootPage(context.Background(),session,false)
 if err!=nil||len(second.Items)!=8||!second.More{t.Fatalf("second page: %+v %v",second,err)}
 third,err:=s.readRemoteFilesRootPage(context.Background(),session,false)
 if err!=nil||len(third.Items)!=1||third.More{t.Fatalf("last page: %+v %v",third,err)}
 if _,err:=s.readRemoteFilesRootPage(context.Background(),session,false);err==nil{
  t.Fatal("terminal page replay accepted")
 }
 names:=append(append(first.Items,second.Items...),third.Items...)
 if len(names)!=17||names[0]!="F:filea"||names[16]!="F:fileq"{
  t.Fatalf("bad bounded page order: %v",names)
 }
 first,err=s.readRemoteFilesRootPage(context.Background(),session,true)
 if err!=nil||!first.More{t.Fatalf("restart: %+v %v",first,err)}
 if err:=os.WriteFile(filepath.Join(s.Workspace,"filenew"),[]byte("X"),0600);err!=nil{t.Fatal(err)}
 if _,err:=s.readRemoteFilesRootPage(context.Background(),session,false);err==nil{
  t.Fatal("changed root listing accepted stale page")
 }
 if _,err:=s.readRemoteFilesRootPage(context.Background(),session,false);err==nil{
  t.Fatal("stale page token remained usable")
 }
}

func TestRemoteFilesRootPagedE2EEAndAudit(t *testing.T){
 if runtime.GOOS=="windows"{t.Skip("Unix-only Files preview")}
 s,session,client,agent:=remoteDispatchFixture(t)
 t.Setenv("TASKDECK_REMOTE_FILES_ROOT","1")
 for i:=0;i<14;i++{
  name:="root"+string(rune('a'+i))
  if err:=os.WriteFile(filepath.Join(s.Workspace,name),[]byte("x"),0600);err!=nil{t.Fatal(err)}
 }
 request:=func(op string)remotetransport.RemoteFilesRoot{
  t.Helper()
  frame,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
   RequestID:remoteDispatchTestID,Operation:op,
  })
  if err!=nil{t.Fatal(err)}
  sealed,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,frame)
  if err!=nil{t.Fatalf("%s rejected: %v",op,err)}
  result,err:=remotetransport.OpenRemoteFilesRoot(client,sealed,remoteDispatchTestID)
  if err!=nil{t.Fatalf("%s result not authenticated: %v",op,err)}
  return result
 }
 first:=request(remotetransport.RemoteActionFilesRoot)
 if len(first.Items)!=8||!first.More{t.Fatalf("first page failed: %+v",first)}
 next:=request(remotetransport.RemoteActionFilesRootNext)
 if len(next.Items)==0||first.Items[7][2:]>=next.Items[0][2:]{
  t.Fatalf("next page order failed: %+v %+v",first,next)
 }
 project,err:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
 if err!=nil{t.Fatal(err)}
 audits,err:=s.Identity.ListAudit(context.Background(),identity.AuditQuery{
  ProjectID:project.ID,Action:"remote.action.files_root_next",Limit:5,
 })
 if err!=nil||len(audits)==0||audits[0].UserID==nil||
  *audits[0].UserID!=session.userID||audits[0].ResourceType!="files"{
  t.Fatalf("next page missing TaskDeck-local audit: %+v %v",audits,err)
 }
}

func TestRemoteFilesRootRejectsSwappedWorkspaceSymlink(t *testing.T){
 if runtime.GOOS=="windows"{t.Skip("Unix-only no-follow workspace root")}
 s,_,_,_:=remoteDispatchFixture(t)
 t.Setenv("TASKDECK_REMOTE_FILES_ROOT","1")
 base:=t.TempDir()
 root:=filepath.Join(base,"workspace")
 original:=filepath.Join(base,"workspace-original")
 if err:=os.Mkdir(root,0700);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(root,"allowed.txt"),[]byte("ok"),0600);err!=nil{t.Fatal(err)}
 outside:=t.TempDir()
 if err:=os.WriteFile(filepath.Join(outside,"private.txt"),
  []byte("do not disclose"),0600);err!=nil{t.Fatal(err)}
 s.Workspace=root
 first,err:=s.readRemoteFilesRoot(context.Background())
 if err!=nil||len(first.Items)!=1||first.Items[0]!="F:allowed.txt"{
  t.Fatalf("real workspace refused: %+v %v",first,err)
 }
 if err:=os.Rename(root,original);err!=nil{t.Fatal(err)}
 if err:=os.Symlink(outside,root);err!=nil{t.Fatal(err)}
 if _,err:=s.readRemoteFilesRoot(context.Background());err==nil{
  t.Fatal("swapped symlink workspace root disclosed outside directory")
 }
 if err:=os.Remove(root);err!=nil{t.Fatal(err)}
 if err:=os.Rename(original,root);err!=nil{t.Fatal(err)}
 restored,err:=s.readRemoteFilesRoot(context.Background())
 if err!=nil||len(restored.Items)!=1||restored.Items[0]!="F:allowed.txt"{
  t.Fatalf("restoring original root failed: %+v %v",restored,err)
 }
}
