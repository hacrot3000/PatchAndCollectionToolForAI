package server

import (
 "context"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// This fixture uses a LOCAL bare remote only inside regression tests.
// Production HTTPS Push refuses local/file transports and never accepts
// such a URL from PHP, native GUI, the Git repo or remote action records.
func remotePushGitFixture(t *testing.T)(root,remote,base string){
 t.Helper()
 dir:=t.TempDir()
 remote=filepath.Join(dir,"upstream.git")
 if out,e:=exec.Command("git","init","--bare","-q",remote).CombinedOutput();e!=nil{
  t.Fatalf("init bare: %v %s",e,out)
 }
 root=filepath.Join(dir,"workspace")
 if out,e:=exec.Command("git","init","-q","-b","main",root).CombinedOutput();e!=nil{
  t.Fatalf("init work: %v %s",e,out)
 }
 remotePullTestGit(t,root,"-c","user.name=Remote Git Test",
  "-c","user.email=test@localhost.invalid",
  "commit","--allow-empty","-q","-m","initial")
 base=remotePullTestGit(t,root,"rev-parse","HEAD")
 remotePullTestGit(t,root,"remote","add","origin",remote)
 remotePullTestGit(t,root,"push","-q","origin","main:main")
 return root,remote,base
}

func TestRemoteGitPushUsesNormalNonForcingRealGit(t *testing.T){
 root,remote,old:=remotePushGitFixture(t)
 remotePullTestGit(t,root,"-c","user.name=Remote Git Test",
  "-c","user.email=test@localhost.invalid",
  "commit","--allow-empty","-q","-m","next")
 target:=remotePullTestGit(t,root,"rev-parse","HEAD")
 args:=remoteGitHTTPSPushArgs(root,remote,"/bin/true","main",target,true)
 joined:=strings.Join(args," ")
 if !strings.Contains(joined,"--no-force")||
  strings.Contains(joined,"--force-with-lease")||
  strings.Contains(joined,"--force ")||
  strings.Contains(joined,"--force-if-includes")||
  strings.Contains(joined,"--all")||
  strings.Contains(joined,"--mirror"){
  t.Fatalf("Push arguments allowed history rewrite: %s",joined)
 }
 cmd:=exec.Command("git",args...)
 cmd.Env=isolatedRemoteGitEnvironment()
 if output,e:=cmd.CombinedOutput();e!=nil{
  t.Fatalf("ordinary non-force Git push failed: %v %s",e,output)
 }
 if got:=strings.TrimSpace(string(mustPushGit(t,
  "git","--git-dir",remote,"rev-parse","refs/heads/main")));got!=target{
  t.Fatal("Push did not deliver intended branch tip")
 }
 remotePullTestGit(t,root,"reset","-q","--hard",old)
 remotePullTestGit(t,root,"-c","user.name=Remote Git Test",
  "-c","user.email=test@localhost.invalid",
  "commit","--allow-empty","-q","-m","divergent")
 divergent:=remotePullTestGit(t,root,"rev-parse","HEAD")
 cmd=exec.Command("git",remoteGitHTTPSPushArgs(
  root,remote,"/bin/true","main",divergent,true)...)
 cmd.Env=isolatedRemoteGitEnvironment()
 if output,e:=cmd.CombinedOutput();e==nil{
  t.Fatalf("DANGER: non-fast-forward branch rewrite succeeded: %s",output)
 }
 if got:=strings.TrimSpace(string(mustPushGit(t,
  "git","--git-dir",remote,"rev-parse","refs/heads/main")));got!=target{
  t.Fatal("non-force Push changed remote history")
 }
}

func mustPushGit(t *testing.T,command string,args ...string)[]byte{
 t.Helper()
 out,e:=exec.Command(command,args...).CombinedOutput()
 if e!=nil{t.Fatalf("Git command failed: %v %s",e,out)}
 return out
}

func TestRemoteGitPushHelperCannotBeSelectedFromProjectOrCustomPath(t *testing.T){
 root:=t.TempDir()
 for _,path:=range []string{
  "", "/tmp/git-credential-manager",
  filepath.Join(root,"git-credential-manager"),
  "/usr/bin/false",
 }{
  t.Setenv("TASKDECK_REMOTE_GIT_PUSH_HELPER",path)
  if name,ok:=remoteGitHTTPSPushHelper(root);ok{
   t.Fatalf("host Push trusted arbitrary executable %q",name)
  }
 }
}

func TestRemoteGitPushReviewDeniedWithNoHostCredentialHelper(t *testing.T){
 root,_:=fetchTestRepo(t)
 t.Setenv("TASKDECK_REMOTE_GIT_PUSH_HELPER","")
 if r,e:=remoteGitHTTPSPushReview(context.Background(),root);
  e==nil||len(r)!=0{
  t.Fatal("Push review accepted missing local host-only credential manager")
 }
}

func TestRemoteGitPushApplyHostFlagsOffAndPHPCannotBypassPrincipal(t *testing.T){
 for _,tc:=range []struct{name,write,push string}{
  {"both-off","",""},
  {"gitwrite-only","1",""},
  {"push-only","","1"},
 }{
  t.Run(tc.name,func(t *testing.T){
   t.Setenv("TASKDECK_REMOTE_GIT_WRITE",tc.write)
   t.Setenv("TASKDECK_REMOTE_GIT_PUSH_HTTPS",tc.push)
   t.Setenv("TASKDECK_REMOTE_GIT_PUSH_HELPER","")
   s,owner,client,agent:=remoteDispatchFixtureWithRole(t,remotetransport.RoleAdmin)
   id:="0123456789abcdef0123456789abcdef"
   frame,e:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
    RequestID:id,Operation:remotetransport.RemoteActionGitPushApply,
    ExpectedSHA256:strings.Repeat("a",64),
   })
   if e!=nil{t.Fatal(e)}
   if out,e:=s.executeRemoteE2EEPing(context.Background(),nil,agent,frame);
    e==nil||len(out)!=0{t.Fatal("PHP-only channel bypassed TaskDeck password")}
   if out,e:=s.executeRemoteE2EEPing(context.Background(),owner,agent,frame);
    e==nil||len(out)!=0{t.Fatal("host-disabled Push unexpectedly allowed")}
  })
 }
}
