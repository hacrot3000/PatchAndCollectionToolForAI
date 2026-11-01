package server

import (
 "bytes"
 "context"
 "crypto/sha256"
 "encoding/hex"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func gitStageTestRequest(t *testing.T, client *remotetransport.Channel, source []byte)[]byte{
 t.Helper()
 sha:=sha256.Sum256(source)
 frame,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionGitStage,
  Path:"source.txt",ExpectedSHA256:hex.EncodeToString(sha[:]),
 })
 if err!=nil{t.Fatal(err)}
 return frame
}

func TestGitStageHostDefaultDisabledAndDoesNotMutateIndex(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_GIT_WRITE","")
 t.Setenv("TASKDECK_REMOTE_GIT_STAGE","")
 s,session,client,agent:=remoteDispatchFixtureWithRole(t,remotetransport.RoleAdmin)
 if err:=exec.Command("git","init","-q",s.Workspace).Run();err!=nil{t.Skipf("git unavailable: %v",err)}
 original:=[]byte("content\n")
 if err:=os.WriteFile(filepath.Join(s.Workspace,"source.txt"),original,0600);err!=nil{t.Fatal(err)}
 request:=gitStageTestRequest(t,client,original)
 if reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,request);err==nil||len(reply)!=0{
  t.Fatal("Git stage admitted while both host opt-ins disabled")
 }
 if _,err:=os.Stat(filepath.Join(s.Workspace,".git","index"));!os.IsNotExist(err){
  t.Fatalf("Git index created despite host flags disabled: %v",err)
 }
}

func TestGitStageHostRequiresExactCASAndNoPHPLoginBypass(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_GIT_WRITE","1")
 t.Setenv("TASKDECK_REMOTE_GIT_STAGE","1")
 s,session,client,agent:=remoteDispatchFixtureWithRole(t,remotetransport.RoleAdmin)
 if err:=exec.Command("git","init","-q",s.Workspace).Run();err!=nil{t.Skipf("git unavailable: %v",err)}
 original:=[]byte("content\n")
 if err:=os.WriteFile(filepath.Join(s.Workspace,"source.txt"),original,0600);err!=nil{t.Fatal(err)}
 request:=gitStageTestRequest(t,client,original)
 if reply,err:=s.executeRemoteE2EEPing(context.Background(),nil,agent,request);err==nil||len(reply)!=0{
  t.Fatal("compromised PHP portal performed stage without TaskDeck login")
 }
 // File changes after user preview: host must refuse stale SHA-256.
 if err:=os.WriteFile(filepath.Join(s.Workspace,"source.txt"),[]byte("tampered\n"),0600);err!=nil{t.Fatal(err)}
 if reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,request);err==nil||len(reply)!=0{
  t.Fatal("stale Git stage hash accepted")
 }
 if _,err:=os.Stat(filepath.Join(s.Workspace,".git","index"));!os.IsNotExist(err){
  t.Fatalf("stale stage created Git index: %v",err)
 }
}

func TestGitStageHostStagesPinnedBlobWithoutRepositoryCleanFilter(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_GIT_WRITE","1")
 t.Setenv("TASKDECK_REMOTE_GIT_STAGE","1")
 s,session,client,agent:=remoteDispatchFixtureWithRole(t,remotetransport.RoleAdmin)
 if err:=exec.Command("git","init","-q",s.Workspace).Run();err!=nil{t.Skipf("git unavailable: %v",err)}
 original:=[]byte("unchanged plaintext\n")
 if err:=os.WriteFile(filepath.Join(s.Workspace,"source.txt"),original,0600);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(s.Workspace,".gitattributes"),[]byte("*.txt filter=evil\n"),0600);err!=nil{t.Fatal(err)}
 marker:=filepath.Join(s.Workspace,"FILTER_TRIGGERED")
 command:="sh -c 'touch "+marker+"; cat'"
 if output,err:=exec.Command("git","-C",s.Workspace,"config","filter.evil.clean",command).CombinedOutput();err!=nil{
  t.Fatalf("filter fixture %s: %v",output,err)
 }
 request:=gitStageTestRequest(t,client,original)
 sealed,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,request)
 if err!=nil{t.Fatal(err)}
 oid,err:=remotetransport.OpenRemoteGitStageResult(client,sealed,remoteDispatchTestID)
 if err!=nil||len(oid)!=40{t.Fatalf("no authenticated Git object ID: %q %v",oid,err)}
 if _,err:=os.Stat(marker);!os.IsNotExist(err){
  t.Fatalf("repository clean filter was executed by remote Git stage: %v",err)
 }
 staged,err:=exec.Command("git","-C",s.Workspace,"show",":source.txt").Output()
 if err!=nil||!bytes.Equal(staged,original){
  t.Fatalf("Git index differs from authenticated preview: %q %v",staged,err)
 }
 actual,err:=os.ReadFile(filepath.Join(s.Workspace,"source.txt"))
 if err!=nil||!bytes.Equal(actual,original){t.Fatal("Git stage altered working tree")}
 if _,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,request);err==nil{
  t.Fatal("replayed Git stage action was accepted")
 }
}

func TestGitStageHostRejectsSymlinkEscape(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_GIT_WRITE","1")
 t.Setenv("TASKDECK_REMOTE_GIT_STAGE","1")
 s,session,client,agent:=remoteDispatchFixtureWithRole(t,remotetransport.RoleAdmin)
 if err:=exec.Command("git","init","-q",s.Workspace).Run();err!=nil{t.Skipf("git unavailable: %v",err)}
 outside:=filepath.Join(t.TempDir(),"private")
 data:=[]byte("sensitive\n")
 if err:=os.WriteFile(outside,data,0600);err!=nil{t.Fatal(err)}
 if err:=os.Symlink(outside,filepath.Join(s.Workspace,"source.txt"));err!=nil{t.Fatal(err)}
 request:=gitStageTestRequest(t,client,data)
 if reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,request);err==nil||len(reply)!=0{
  t.Fatal("Git stage permitted symlink escape")
 }
}
