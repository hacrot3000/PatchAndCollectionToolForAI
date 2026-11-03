package server

import (
 "context"
 "os/exec"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func TestRemoteGitTrackingRefsOfflineWithVerifiedCommitOID(t *testing.T){
 root:=initRemoteGitTestRepo(t)
 cmd:=exec.Command("git","-C",root,"-c","user.name=Test","-c","user.email=test@example.invalid","commit","--allow-empty","-qm","fixture")
 if out,err:=cmd.CombinedOutput();err!=nil{t.Fatalf("commit fixture: %v %s",err,out)}
 oidBytes,err:=exec.Command("git","-C",root,"rev-parse","HEAD").Output()
 if err!=nil{t.Fatal(err)}
 oid:=strings.TrimSpace(string(oidBytes))
 for _,ref:=range []string{"refs/remotes/origin/main","refs/remotes/origin/feature/test","refs/remotes/backup/release"}{
  if out,err:=exec.Command("git","-C",root,"update-ref",ref,oid).CombinedOutput();err!=nil{
   t.Fatalf("create local remote tracking ref: %s %v",out,err)
  }
 }
 result,err:=remoteGitMetadata(context.Background(),root,remotetransport.RemoteActionGitTrackingRefs)
 if err!=nil{t.Fatal(err)}
 want:="GIT_TRACKING_V1\nbackup/release "+oid+"\norigin/feature/test "+oid+"\norigin/main "+oid+"\n"
 if string(result)!=want{t.Fatalf("bad tracking refs: got %q want %q",result,want)}
 if strings.Contains(string(result),"https://")||strings.Contains(string(result),"token"){
  t.Fatal("tracking refs response contains credential URL")
 }
}
func TestRemoteGitTrackingRefsEmptyAndRejectBadNames(t *testing.T){
 root:=initRemoteGitTestRepo(t)
 result,err:=remoteGitMetadata(context.Background(),root,remotetransport.RemoteActionGitTrackingRefs)
 if err!=nil||string(result)!="GIT_TRACKING_V1\n"{t.Fatalf("empty refs: %q %v",result,err)}
 for _,name:=range []string{"origin/main","origin/feature/test","Backup/release_1"}{
  if !validRemoteTrackingName(name){t.Fatalf("safe ref rejected: %s",name)}
 }
 for _,name:=range []string{"","main","-origin/main","origin/.secret","origin/hi lock","origin/main/..","origin/a.lock","origin/a//b","origin/ü"}{
  if validRemoteTrackingName(name){t.Fatalf("unsafe tracking ref name accepted: %q",name)}
 }
 for _,oid:=range []string{"","1234",strings.Repeat("f",40)+"0",strings.Repeat("A",40),"aaaa?"+strings.Repeat("b",35)}{
  if validRemoteTrackingOID(oid){t.Fatalf("invalid object ID accepted: %q",oid)}
 }
}
func TestRemoteGitTrackingRefsRejectOver32RefsWithoutPartialResults(t *testing.T){
 root:=initRemoteGitTestRepo(t)
 cmd:=exec.Command("git","-C",root,"-c","user.name=Test","-c","user.email=test@example.invalid","commit","--allow-empty","-qm","fixture")
 if out,err:=cmd.CombinedOutput();err!=nil{t.Fatalf("commit fixture: %v %s",err,out)}
 oidBytes,err:=exec.Command("git","-C",root,"rev-parse","HEAD").Output()
 if err!=nil{t.Fatal(err)}
 oid:=strings.TrimSpace(string(oidBytes))
 for i:=0;i<33;i++{
  ref:="refs/remotes/origin/b"+string(rune('a'+i/26))+string(rune('a'+i%26))
  if out,err:=exec.Command("git","-C",root,"update-ref",ref,oid).CombinedOutput();err!=nil{
   t.Fatalf("create ref: %v %s",err,out)
  }
 }
 result,err:=remoteGitMetadata(context.Background(),root,remotetransport.RemoteActionGitTrackingRefs)
 if err==nil||len(result)!=0{t.Fatalf("33 refs partially accepted: %q %v",result,err)}
}
