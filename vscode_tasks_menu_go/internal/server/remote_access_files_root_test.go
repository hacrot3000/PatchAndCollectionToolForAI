package server

import (
 "context"
 "os"
 "path/filepath"
 "runtime"
 "strings"
 "testing"

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
