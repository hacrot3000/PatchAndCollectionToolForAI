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

func TestGitDiffRealTaskDeckHostReadOnlyAndSHAApprovedClient(t *testing.T){
 s,session,client,agent:=remoteDispatchFixture(t)
 if err:=exec.Command("git","init","-q",s.Workspace).Run();err!=nil{
  t.Skipf("Git not available: %v",err)
 }
 path:=filepath.Join(s.Workspace,"worktree-diff.txt")
 if err:=os.WriteFile(path,[]byte("before\n"),0600);err!=nil{t.Fatal(err)}
 for _,args:=range [][]string{
  {"-C",s.Workspace,"add","--","worktree-diff.txt"},
  {"-C",s.Workspace,"-c","user.name=TaskDeck CI","-c",
   "user.email=ci@example.invalid","commit","-qm","test"},
 }{
  if result,err:=exec.Command("git",args...).CombinedOutput();err!=nil{
   t.Fatalf("prepare Git fixture %v: %v, %s",args,err,result)
  }
 }
 if err:=os.WriteFile(path,[]byte("after\n"),0600);err!=nil{t.Fatal(err)}
 encrypted,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,
  Operation:remotetransport.RemoteActionGitDiff,
  Path:"worktree-diff.txt",
 })
 if err!=nil{t.Fatal(err)}
 if response,err:=s.executeRemoteE2EEPing(context.Background(),nil,agent,encrypted);
  err==nil||len(response)!=0{t.Fatal("compromised PHP bypassed TaskDeck Git principal")}
 sealed,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,encrypted)
 if err!=nil{t.Fatal(err)}
 diff,err:=remotetransport.OpenRemoteGitDiffResult(client,sealed,remoteDispatchTestID)
 if err!=nil||!strings.Contains(diff,"-before")||!strings.Contains(diff,"+after"){
  t.Fatalf("fixed read-only Git diff unavailable: %q %v",diff,err)
 }
 if content,err:=os.ReadFile(path);err!=nil||!bytes.Equal(content,[]byte("after\n")){
  t.Fatal("read-only remote Git diff modified worktree")
 }
 if _,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,encrypted);err==nil{
  t.Fatal("replayed E2EE Git diff accepted")
 }
}

func TestGitDiffRejectsUnsafeFilesAndOversizeWithoutTruncation(t *testing.T){
 s,session,client,agent:=remoteDispatchFixture(t)
 if err:=exec.Command("git","init","-q",s.Workspace).Run();err!=nil{
  t.Skipf("Git not available: %v",err)
 }
 outside:=filepath.Join(t.TempDir(),"outside")
 if err:=os.WriteFile(outside,[]byte("secret"),0600);err!=nil{t.Fatal(err)}
 if err:=os.Symlink(outside,filepath.Join(s.Workspace,"external-link"));err!=nil{t.Fatal(err)}
 sealed,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionGitDiff,
  Path:"external-link",
 })
 if err!=nil{t.Fatal(err)}
 if response,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,sealed);
  err==nil||len(response)>0{t.Fatal("Git diff accepted workspace-escaping symlink")}
 var output cappedRemoteGitDiffOutput
 if _,err:=output.Write([]byte(strings.Repeat("X",remotetransport.RemoteGitDiffMaxBytes)));err!=nil{t.Fatal(err)}
 if _,err:=output.Write([]byte("Y"));err==nil{t.Fatal("oversized Git diff was silently truncated")}
}
