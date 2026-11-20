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

func directoryFixture(t *testing.T)(*Server,*remoteTaskDeckSession,string){
 t.Helper()
 if runtime.GOOS=="windows"{t.Skip("Unix-only directory listing")}
 s,session,_,_:=remoteDispatchFixture(t)
 t.Setenv("TASKDECK_REMOTE_FILES_ROOT","1")
 t.Setenv("TASKDECK_REMOTE_FILES_DIRS","1")
 t.Setenv("TASKDECK_REMOTE_FILES_DIR_NAMES","docs")
 target:=filepath.Join(s.Workspace,"docs")
 if err:=os.Mkdir(target,0700);err!=nil{t.Fatal(err)}
 return s,session,target
}
func issuedDirectoryIndex(t *testing.T,s *Server,session *remoteTaskDeckSession,name string)int{
 t.Helper()
 ctx:=context.Background()
 i:=0
 first:=true
 for {
  page,err:=s.readRemoteFilesRootPage(ctx,session,first)
  if err!=nil{t.Fatal(err)}
  for j,entry:=range page.Items{
   if entry=="D:"+name{i+=j+1;return i}
  }
  i+=len(page.Items)
  if !page.More{t.Fatalf("root missing folder %q",name)}
  first=false
 }
}

func TestRemoteFilesDirSelectedOneLevelPagesAndStaleDenial(t *testing.T){
 s,session,folder:=directoryFixture(t)
 for i:=0;i<11;i++{
  name:="item"+string(rune('a'+i))+".txt"
  if err:=os.WriteFile(filepath.Join(folder,name),[]byte("private content"),0600);err!=nil{t.Fatal(err)}
 }
 if err:=os.Mkdir(filepath.Join(folder,"zeta"),0700);err!=nil{t.Fatal(err)}
 if err:=os.Symlink("/etc/passwd",filepath.Join(folder,"link"));err!=nil{t.Fatal(err)}
 // This action never publishes names before they were authorized in root.
 if _,err:=s.readRemoteDirFilesPage(context.Background(),session,true,1);err==nil{
  t.Fatal("guessed a directory index before first root E2EE response")
 }
 index:=issuedDirectoryIndex(t,s,session,"docs")
 if _,err:=s.readRemoteDirFilesPage(context.Background(),session,false,0);err==nil{
  t.Fatal("uninitialized selected directory cursor")
 }
 page,err:=s.readRemoteDirFilesPage(context.Background(),session,true,index)
 if err!=nil||len(page.Items)!=8||!page.More{
  t.Fatalf("first selected directory page: %+v %v",page,err)
 }
 var all []string
 all=append(all,page.Items...)
 for page.More{
  page,err=s.readRemoteDirFilesPage(context.Background(),session,false,0)
  if err!=nil{t.Fatalf("next directory page: %v",err)}
  all=append(all,page.Items...)
 }
 if len(all)!=12||all[len(all)-1]!="D:zeta"{
  t.Fatalf("unexpected nested files: %+v",all)
 }
 for _,item:=range all{
  if strings.Contains(item,"link")||strings.Contains(item,"/etc")||
   strings.Contains(item,"private content"){t.Fatal("followed symlink or sent content")}
 }
 if _,err:=s.readRemoteDirFilesPage(context.Background(),session,false,0);err==nil{
  t.Fatal("replayed final directory page")
 }
 first,err:=s.readRemoteDirFilesPage(context.Background(),session,true,index)
 if err!=nil||!first.More{t.Fatal("could not restart directory list")}
 if err:=os.WriteFile(filepath.Join(folder,"new-file"),[]byte("x"),0600);err!=nil{t.Fatal(err)}
 if _,err:=s.readRemoteDirFilesPage(context.Background(),session,false,0);err==nil{
  t.Fatal("folder modified during paging but continuation succeeded")
 }
 if _,err:=s.readRemoteDirFilesPage(context.Background(),session,false,0);err==nil{
  t.Fatal("stale directory cursor was reusable")
 }
}

func TestRemoteFilesDirStrictHostAllowlistAndSymlinks(t *testing.T){
 s,session,folder:=directoryFixture(t)
 if err:=os.WriteFile(filepath.Join(folder,"visible.txt"),[]byte("content"),0600);err!=nil{t.Fatal(err)}
 index:=issuedDirectoryIndex(t,s,session,"docs")
 t.Setenv("TASKDECK_REMOTE_FILES_DIRS","0")
 if _,err:=s.readRemoteDirFilesPage(context.Background(),session,true,index);err==nil{
  t.Fatal("directory browse bypassed host opt-in")
 }
 t.Setenv("TASKDECK_REMOTE_FILES_DIRS","1")
 for _,v:=range []string{"","docs,docs","../docs","docs,/etc","docs,.secrets"}{
  t.Setenv("TASKDECK_REMOTE_FILES_DIR_NAMES",v)
  if _,err:=s.readRemoteDirFilesPage(context.Background(),session,true,index);err==nil{
   t.Fatalf("invalid directory host allowlist accepted %q",v)
  }
 }
 t.Setenv("TASKDECK_REMOTE_FILES_DIR_NAMES","docs")
 result,err:=s.readRemoteDirFilesPage(context.Background(),session,true,index)
 if err!=nil||len(result.Items)!=1||result.Items[0]!="F:visible.txt"{
  t.Fatalf("approved folder failed: %+v %v",result,err)
 }
 // A symlinked folder must never reveal a directory outside workspace.
 outside:=t.TempDir()
 if err:=os.WriteFile(filepath.Join(outside,"leak.txt"),[]byte("secret"),0600);err!=nil{t.Fatal(err)}
 if err:=os.RemoveAll(folder);err!=nil{t.Fatal(err)}
 if err:=os.Symlink(outside,folder);err!=nil{t.Fatal(err)}
 if _,err:=s.readRemoteDirFilesPage(context.Background(),session,true,index);err==nil{
  t.Fatal("symlink folder escaped workspace")
 }
}
func TestRemoteFilesDirRootDigestAndUnissuedIndex(t *testing.T){
 s,session,_:=directoryFixture(t)
 for i:=0;i<9;i++{
  name:="dir"+string(rune('a'+i))
  if err:=os.Mkdir(filepath.Join(s.Workspace,name),0700);err!=nil{t.Fatal(err)}
 }
 t.Setenv("TASKDECK_REMOTE_FILES_DIR_NAMES","docs,dirh")
 first,err:=s.readRemoteFilesRootPage(context.Background(),session,true)
 if err!=nil||!first.More{t.Fatalf("expected multiple root pages: %+v %v",first,err)}
 // Total index 9 hasn't been disclosed yet in first eight names.
 if _,err:=s.readRemoteDirFilesPage(context.Background(),session,true,9);err==nil{
  t.Fatal("guessed a future page directory")
 }
 if err:=os.Mkdir(filepath.Join(s.Workspace,"aaa-new"),0700);err!=nil{t.Fatal(err)}
 if _,err:=s.readRemoteDirFilesPage(context.Background(),session,true,1);err==nil{
  t.Fatal("stale root directory listing allowed index selection")
 }
 if _,err:=s.readRemoteFilesRootPage(context.Background(),session,false);err==nil{
  t.Fatal("directory root pagination continued after stale root")
 }
}
func TestRemoteFilesDirE2EEPrincipalAudit(t *testing.T){
 if runtime.GOOS=="windows"{t.Skip("Unix-only directory listing")}
 s,session,client,agent:=remoteDispatchFixture(t)
 t.Setenv("TASKDECK_REMOTE_FILES_ROOT","1")
 t.Setenv("TASKDECK_REMOTE_FILES_DIRS","1")
 t.Setenv("TASKDECK_REMOTE_FILES_DIR_NAMES","docs")
 dir:=filepath.Join(s.Workspace,"docs")
 if err:=os.Mkdir(dir,0700);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(dir,"notes.txt"),[]byte("not disclosed"),0600);err!=nil{t.Fatal(err)}
 makeRequest:=func(op string,index int)([]byte,error){
  t.Helper()
  frame,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
   RequestID:remoteDispatchTestID,Operation:op,FileIndex:index,
  })
  if err!=nil{t.Fatal(err)}
  return s.executeRemoteE2EEPing(context.Background(),session,agent,frame)
 }
 if _,err:=makeRequest(remotetransport.RemoteActionFilesDirSelect,1);err==nil{
  t.Fatal("unissued directory read crossed E2EE")
 }
 // Recreate authenticated fixture after a deliberate rejected operation
 // only when necessary; this implementation retains channel on denial.
 frame,err:=makeRequest(remotetransport.RemoteActionFilesRoot,0)
 if err!=nil{t.Fatalf("E2EE root list failed: %v",err)}
 first,err:=remotetransport.OpenRemoteFilesRoot(client,frame,remoteDispatchTestID)
 if err!=nil{t.Fatal(err)}
 index:=0
 for i,n:=range first.Items{if n=="D:docs"{index=i+1}}
 if index==0{t.Fatalf("missing root directory: %+v",first)}
 frame,err=makeRequest(remotetransport.RemoteActionFilesDirSelect,index)
 if err!=nil{t.Fatal(err)}
 page,err:=remotetransport.OpenRemoteFilesRoot(client,frame,remoteDispatchTestID)
 if err!=nil||len(page.Items)!=1||page.Items[0]!="F:notes.txt"{
  t.Fatalf("E2EE directory reply mismatched: %+v %v",page,err)
 }
 project,err:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
 if err!=nil{t.Fatal(err)}
 audits,err:=s.Identity.ListAudit(context.Background(),identity.AuditQuery{
  ProjectID:project.ID,Action:"remote.action.files_dir_select",Limit:5,
 })
 if err!=nil||len(audits)==0||audits[0].UserID==nil||
  *audits[0].UserID!=session.userID||audits[0].ResourceType!="files"{
  t.Fatalf("local directory audit missing: %+v %v",audits,err)
 }
}
