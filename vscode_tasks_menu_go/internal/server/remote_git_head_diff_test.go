package server

import (
 "bytes"
 "context"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func TestGitHeadDiffHostCombinesStagedAndUnstagedWithoutWriting(t *testing.T){
 s,session,client,agent:=remoteDispatchFixture(t)
 if err:=exec.Command("git","init","-q",s.Workspace).Run();err!=nil{
  t.Skipf("Git is unavailable: %v",err)
 }
 path:=filepath.Join(s.Workspace,"with spaces.txt")
 for _,step:=range [][]string{
  {"-C",s.Workspace,"add","--","with spaces.txt"},
  {"-C",s.Workspace,"-c","user.name=TaskDeck CI","-c",
   "user.email=ci@example.invalid","commit","-qm","baseline"},
 }{
  // File creation precedes first staging below.
  if err:=os.WriteFile(path,[]byte("base\n"),0600);err!=nil{t.Fatal(err)}
  if result,err:=exec.Command("git",step...).CombinedOutput();err!=nil{
   t.Fatalf("Git fixture: %v: %v, %s",step,err,result)
  }
 }
 if err:=os.WriteFile(path,[]byte("staged\n"),0600);err!=nil{t.Fatal(err)}
 if result,err:=exec.Command("git","-C",s.Workspace,"add","--","with spaces.txt").CombinedOutput();err!=nil{
  t.Fatalf("stage fixture: %v %s",err,result)
 }
 if err:=os.WriteFile(path,[]byte("worktree\n"),0600);err!=nil{t.Fatal(err)}
 req,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,
  Operation:remotetransport.RemoteActionGitHeadDiff,Path:"with spaces.txt",
 })
 if err!=nil{t.Fatal(err)}
 if reply,err:=s.executeRemoteE2EEPing(context.Background(),nil,agent,req);err==nil||len(reply)!=0{
  t.Fatal("compromised PHP-only ticket admitted full Git HEAD diff")
 }
 sealed,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req)
 if err!=nil{t.Fatal(err)}
 diff,err:=remotetransport.OpenRemoteGitHeadDiffResult(client,sealed,remoteDispatchTestID)
 if err!=nil||!strings.Contains(diff,"-base")||!strings.Contains(diff,"+worktree")||
  strings.Contains(diff,"+staged"){
  t.Fatalf("did not return HEAD-vs-worktree, or returned intermediate staged diff: %q %v",diff,err)
 }
 data,err:=os.ReadFile(path)
 if err!=nil||!bytes.Equal(data,[]byte("worktree\n")){
  t.Fatalf("read-only action changed working file: %q %v",data,err)
 }
 index,err:=exec.Command("git","-C",s.Workspace,"show",":with spaces.txt").Output()
 if err!=nil||!bytes.Equal(index,[]byte("staged\n")){
  t.Fatalf("read-only action modified Git index: %q %v",index,err)
 }
 if _,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req);err==nil {
  t.Fatal("replayed HEAD diff E2EE record accepted")
 }
}

func TestGitHeadDiffHostRejectsEscapingSymlink(t *testing.T){
 s,session,client,agent:=remoteDispatchFixture(t)
 if err:=exec.Command("git","init","-q",s.Workspace).Run();err!=nil{
  t.Skipf("Git is unavailable: %v",err)
 }
 outside:=filepath.Join(t.TempDir(),"secret")
 if err:=os.WriteFile(outside,[]byte("outside"),0600);err!=nil{t.Fatal(err)}
 if err:=os.Symlink(outside,filepath.Join(s.Workspace,"external"));err!=nil{t.Fatal(err)}
 frame,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionGitHeadDiff,Path:"external",
 })
 if err!=nil{t.Fatal(err)}
 if response,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,frame);err==nil||len(response)!=0 {
  t.Fatal("Git HEAD diff accepted workspace escape symlink")
 }
}
