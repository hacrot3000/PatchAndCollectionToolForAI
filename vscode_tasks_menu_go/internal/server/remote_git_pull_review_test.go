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

func remotePullTestGit(t *testing.T,repo string,args ...string)string{
 t.Helper()
 command:=exec.Command("git",append([]string{"-C",repo},args...)...)
 out,err:=command.CombinedOutput()
 if err!=nil{t.Fatalf("git %v: %v %s",args,err,out)}
 return strings.TrimSpace(string(out))
}
func remotePullTestRepo(t *testing.T)(string,string){
 t.Helper()
 root:=t.TempDir()
 init:=exec.Command("git","init","-q","-b","main",root)
 if out,err:=init.CombinedOutput();err!=nil{t.Fatalf("git init: %v %s",err,out)}
 remotePullTestGit(t,root,"-c","user.name=TaskDeck test",
  "-c","user.email=test@localhost.invalid",
  "commit","-q","--allow-empty","-m","initial")
 head:=remotePullTestGit(t,root,"rev-parse","HEAD")
 remotePullTestGit(t,root,"config","branch.main.remote","origin")
 remotePullTestGit(t,root,"config","branch.main.merge","refs/heads/main")
 // Deliberately invalid URL: the preflight MUST use already-fetched
 // refs/remotes/* only and NEVER attempt Git network access.
 remotePullTestGit(t,root,"remote","add","origin","https://unreachable.invalid/repository.git")
 remotePullTestGit(t,root,"update-ref","refs/remotes/origin/main",head)
 return root,head
}
func TestRemoteReadOnlyGitPullReviewVerifiesLocalFastForwardWithoutNetwork(t *testing.T){
 root,old:=remotePullTestRepo(t)
 ctx:=context.Background()
 base,err:=remoteGitPullReview(ctx,root)
 if err!=nil||!remotetransport.ValidRemoteGitPullReviewReport(string(base))||
  !strings.Contains(string(base),"STATUS up-to-date\n")||
  !strings.Contains(string(base),"NETWORK none\n"){
  t.Fatalf("same-HEAD offline review failed %q %v",base,err)
 }
 remotePullTestGit(t,root,"-c","user.name=TaskDeck test",
  "-c","user.email=test@localhost.invalid",
  "commit","-q","--allow-empty","-m","remote tip")
 target:=remotePullTestGit(t,root,"rev-parse","HEAD")
 remotePullTestGit(t,root,"update-ref","refs/remotes/origin/main",target)
 remotePullTestGit(t,root,"reset","--hard",old)
 report,err:=remoteGitPullReview(ctx,root)
 if err!=nil||!remotetransport.ValidRemoteGitPullReviewReport(string(report))||
  !strings.Contains(string(report),"STATUS fast-forward\n")||
  !strings.Contains(string(report),"UPSTREAM_HEAD "+target+"\n"){
  t.Fatalf("legitimate already-fetched FF was denied: %q %v",report,err)
 }
 if actual:=remotePullTestGit(t,root,"rev-parse","HEAD");actual!=old{
  t.Fatal("read-only Pull review unexpectedly modified HEAD")
 }
 if actual:=remotePullTestGit(t,root,"rev-parse","refs/remotes/origin/main");actual!=target{
  t.Fatal("read-only Pull review modified remote tracking")
 }
 // A dirty workspace must not be labelled ready for a fast-forward.
 if err:=os.WriteFile(filepath.Join(root,"untracked.txt"),[]byte("local edit"),0600);err!=nil{t.Fatal(err)}
 if report,err:=remoteGitPullReview(ctx,root);err==nil||len(report)!=0{
  t.Fatal("dirty worktree admitted Pull review")
 }
 if err:=os.Remove(filepath.Join(root,"untracked.txt"));err!=nil{t.Fatal(err)}
 // A diverged local history is not fast-forward even with origin/main
 // configured and a plausible local tracking ref.
 remotePullTestGit(t,root,"-c","user.name=TaskDeck test",
  "-c","user.email=test@localhost.invalid",
  "commit","-q","--allow-empty","-m","local split")
 if report,err:=remoteGitPullReview(ctx,root);err==nil||len(report)!=0{
  t.Fatal("diverged local branch accepted Pull preview")
 }
}
func TestRemoteReadOnlyGitPullReviewRejectsMissingUpstreamAndExecutableFilter(t *testing.T){
 root,_:=remotePullTestRepo(t)
 remotePullTestGit(t,root,"config","--unset","branch.main.merge")
 if report,err:=remoteGitPullReview(context.Background(),root);err==nil||len(report)!=0{
  t.Fatal("missing upstream fabricated a Pull target")
 }
 remotePullTestGit(t,root,"config","branch.main.merge","refs/heads/main")
 remotePullTestGit(t,root,"config","filter.hijack.clean","/bin/false")
 if report,err:=remoteGitPullReview(context.Background(),root);err==nil||len(report)!=0{
  t.Fatal("executable Git clean filter accepted in remote preflight")
 }
}
