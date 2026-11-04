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
 // Git can auto-fetch missing objects from a partial clone during ostensibly
 // read-only object traversal. All host Git commands must forbid that path.
 if !strings.Contains(strings.Join(isolatedRemoteGitEnvironment(),"\n"),
  "GIT_NO_LAZY_FETCH=1"){t.Fatal("Git partial-clone lazy network fetch not disabled")}
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

func TestRemoteReadOnlyGitPullReviewRequiresAuthenticatedHostPrincipal(t *testing.T){
 s,owner,client,agent:=remoteDispatchFixtureWithRole(t,remotetransport.RoleAdmin)
 root:=s.Workspace
 init:=exec.Command("git","init","-q","-b","main",root)
 if output,err:=init.CombinedOutput();err!=nil{t.Fatalf("git init: %v %s",err,output)}
 remotePullTestGit(t,root,"add",".vscode/tasks.json")
 remotePullTestGit(t,root,"-c","user.name=TaskDeck test",
  "-c","user.email=test@localhost.invalid","commit","-q","-m","test")
 oid:=remotePullTestGit(t,root,"rev-parse","HEAD")
 remotePullTestGit(t,root,"config","branch.main.remote","origin")
 remotePullTestGit(t,root,"config","branch.main.merge","refs/heads/main")
 remotePullTestGit(t,root,"remote","add","origin","https://unreachable.invalid/repository.git")
 remotePullTestGit(t,root,"update-ref","refs/remotes/origin/main",oid)
 request,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionGitPullReview,
 })
 if err!=nil{t.Fatal(err)}
 // PHP cannot supply a TaskDeck password principal merely by possessing
 // a valid Portal ticket and a well-formed encrypted browser frame.
 if response,err:=s.executeRemoteE2EEPing(context.Background(),nil,agent,request);
  err==nil||len(response)!=0{
  t.Fatal("PHP-only caller bypassed local login for Pull preview")
 }
 response,err:=s.executeRemoteE2EEPing(context.Background(),owner,agent,request)
 if err!=nil{t.Fatalf("valid local Pull preview denied: %v",err)}
 report,err:=remotetransport.OpenRemoteGitPullReviewResult(client,response,remoteDispatchTestID)
 if err!=nil||!strings.Contains(report,"STATUS up-to-date\n")||
  !strings.Contains(report,"NETWORK none\n"){
  t.Fatalf("invalid authenticated Pull preview: %q %v",report,err)
 }
 if actual:=remotePullTestGit(t,root,"rev-parse","HEAD");actual!=oid{
  t.Fatal("E2EE Pull preview unexpectedly updated local HEAD")
 }
 if _,err:=s.executeRemoteE2EEPing(context.Background(),owner,agent,request);err==nil{
  t.Fatal("replayed Pull preflight E2EE action")
 }
}
