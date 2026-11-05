package server

import (
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

const fetchTestURL="https://github.com/public-fixture/test-project.git"

func fetchTestRepo(t *testing.T)(root,head string){
 t.Helper()
 root,head,_=remotePullChangesFixture(t)
 remotePullTestGit(t,root,"config","remote.origin.url",fetchTestURL)
 t.Setenv("TASKDECK_REMOTE_GIT_FETCH_URL",fetchTestURL)
 return root,head
}

func TestRemoteGitFetchURLAllowsOnlyCredentiallessPublicHTTPS(t *testing.T){
 for _,ok:=range []string{
  "https://github.com/example/project.git",
  "https://gitlab.com/group/subproject/project.git",
  "https://bitbucket.org/workspace/myrepo",
 }{
  host,accepted:=remoteGitTrustedFetchURL(ok)
  if !accepted||host==""{t.Fatalf("canonical public HTTPS rejected: %q",ok)}
 }
 for _,bad:=range []string{
  "", "https://127.0.0.1/repo.git", "https://localhost/a/b",
  "http://github.com/o/r.git", "git@github.com:o/r.git",
  "ssh://github.com/o/r.git", "file:///tmp/remote.git",
  "ext::sh -c 'touch /tmp/evil'",
  "https://evil.github.com/o/r.git",
  "https://github.com.evil.invalid/o/r.git",
  "https://user:secret@github.com/o/r.git",
  "https://github.com:443/o/r.git",
  "https://github.com/o/r.git?access_token=x",
  "https://github.com/o/r.git#fragment",
  "https://github.com/o/../r.git",
  "https://github.com/o/%2e%2e/r.git",
  "https://github.com/o/r.git/extra/",
  "https://github.com/o/r.git\nAuthorization: leaked",
 }{
  if host,accepted:=remoteGitTrustedFetchURL(bad);accepted||host!=""{
   t.Fatalf("unsafe Git Fetch URL admitted: %q",bad)
  }
 }
}

func TestRemoteGitFetchReadOnlyReviewIsPinnedAndHasNoNetwork(t *testing.T){
 root,old:=fetchTestRepo(t)
 configBefore,err:=os.ReadFile(filepath.Join(root,".git","config"))
 if err!=nil{t.Fatal(err)}
 report,err:=remoteGitHTTPSFetchReview(context.Background(),root)
 if err!=nil||!remotetransport.ValidRemoteGitFetchReviewReport(string(report)){
  t.Fatalf("clean pinned public HTTPS review denied: %q %v",report,err)
 }
 digest:=sha256.Sum256([]byte(fetchTestURL))
 if !strings.Contains(string(report),"HTTPS_HOST github.com\n")||
  !strings.Contains(string(report),"URL_SHA256 "+hex.EncodeToString(digest[:])+"\n"){
  t.Fatalf("invalid host pin digest in Fetch preview: %q",report)
 }
 if strings.Contains(string(report),fetchTestURL)||
  strings.Contains(string(report),"token")||
  strings.Contains(string(report),"password"){
  t.Fatal("Host URL/credential unexpectedly exposed in preview")
 }
 if got:=remotePullTestGit(t,root,"rev-parse","HEAD");got!=old{
  t.Fatal("read-only Fetch preview advanced checked out HEAD")
 }
 after,err:=os.ReadFile(filepath.Join(root,".git","config"))
 if err!=nil||string(after)!=string(configBefore){
  t.Fatal("Fetch review changed Git config")
 }
}

func TestRemoteGitFetchDeniedForRepoTransportConfigAndHostMismatch(t *testing.T){
 for _,tc:=range []struct{name,key,value string}{
  {"url-insteadOf","url.https://evil.invalid/.insteadOf","https://github.com/"},
  {"http-extraheader","http.extraheader","Authorization: Basic secret"},
  {"http-proxy","http.proxy","http://127.0.0.1:3128"},
  {"TLS-disable","http.sslVerify","false"},
  {"credential-helper","credential.helper","/bin/sh"},
  {"include","include.path","/etc/passwd"},
  {"custom-uploadpack","remote.origin.uploadpack","touch /tmp/marker"},
  {"local-worktree","core.worktree","/tmp/external"},
  {"origin-changed","remote.origin.url","https://github.com/different/repo.git"},
 }{
  t.Run(tc.name,func(t *testing.T){
   root,old:=fetchTestRepo(t)
   remotePullTestGit(t,root,"config",tc.key,tc.value)
   if report,err:=remoteGitHTTPSFetchReview(context.Background(),root);
    err==nil||len(report)!=0{
    t.Fatal("unsafe Git transport configuration admitted for HTTPS Fetch")
   }
   // A hostile core.worktree can prevent Git from finding this repo.
   // Read the original loose checked-out ref without trusting Git config.
   head,e:=os.ReadFile(filepath.Join(root,".git","refs","heads","main"))
   if e!=nil||strings.TrimSpace(string(head))!=old{
    t.Fatal("unsafe Fetch preflight changed local HEAD")
   }
  })
 }
}

func TestRemoteGitFetchDeniedOnMissingOrUnpinnedURL(t *testing.T){
 for _,raw:=range []string{
  "", "https://evil.invalid/org/repo.git",
  "http://github.com/owner/repo.git",
  "https://user:password@github.com/owner/repo.git",
 }{
  t.Run(raw,func(t *testing.T){
   root,_:=fetchTestRepo(t)
   t.Setenv("TASKDECK_REMOTE_GIT_FETCH_URL",raw)
   if output,err:=remoteGitHTTPSFetchReview(context.Background(),root);
    err==nil||len(output)!=0{
    t.Fatal("missing or unauthorized HTTPS URL admitted")
   }
  })
 }
}

func TestRemoteGitFetchHostGatesNeverAllowPHPSideApproval(t *testing.T){
 for _,tc:=range []struct{name,write,https string}{
  {"default-off","",""},
  {"write-only","1",""},
  {"HTTPS-only","","1"},
 }{
  t.Run(tc.name,func(t *testing.T){
   t.Setenv("TASKDECK_REMOTE_GIT_WRITE",tc.write)
   t.Setenv("TASKDECK_REMOTE_GIT_FETCH_HTTPS",tc.https)
   t.Setenv("TASKDECK_REMOTE_GIT_FETCH_URL",fetchTestURL)
   s,owner,client,agent:=remoteDispatchFixtureWithRole(t,remotetransport.RoleAdmin)
   root:=s.Workspace
   init:=exec.Command("git","init","-q","-b","main",root)
   if out,err:=init.CombinedOutput();err!=nil{t.Fatalf("git init: %v %s",err,out)}
   remotePullTestGit(t,root,"add",".vscode/tasks.json")
   remotePullTestGit(t,root,"-c","user.name=TaskDeck test",
    "-c","user.email=test@localhost.invalid","commit","-q","-m","baseline")
   head:=remotePullTestGit(t,root,"rev-parse","HEAD")
   remotePullTestGit(t,root,"remote","add","origin",fetchTestURL)
   remotePullTestGit(t,root,"config","branch.main.remote","origin")
   remotePullTestGit(t,root,"config","branch.main.merge","refs/heads/main")
   remotePullTestGit(t,root,"update-ref","refs/remotes/origin/main",head)
   id:="0123456789abcdef0123456789abcdef"
   preflight,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
    RequestID:id,Operation:remotetransport.RemoteActionGitFetchReview,
   })
   if err!=nil{t.Fatal(err)}
   if result,err:=s.executeRemoteE2EEPing(context.Background(),nil,agent,preflight);
    err==nil||len(result)!=0{
    t.Fatal("PHP-only caller bypassed TaskDeck principal to review HTTPS Fetch")
   }
   result,err:=s.executeRemoteE2EEPing(context.Background(),owner,agent,preflight)
   if err!=nil{t.Fatalf("admin Fetch review denied: %v",err)}
   report,err:=remotetransport.OpenRemoteGitFetchReviewResult(client,result,id)
   if err!=nil{t.Fatal(err)}
   sha:=sha256.Sum256([]byte(report))
   apply,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
    RequestID:"1123456789abcdef0123456789abcdef",
    Operation:remotetransport.RemoteActionGitFetchApply,
    ExpectedSHA256:hex.EncodeToString(sha[:]),
   })
   if err!=nil{t.Fatal(err)}
   if response,err:=s.executeRemoteE2EEPing(context.Background(),owner,agent,apply);
    err==nil||len(response)!=0{
    t.Fatal("host-disabled network Fetch reached Git")
   }
   if current:=remotePullTestGit(t,root,"rev-parse","HEAD");current!=head{
    t.Fatal("disabled HTTPS Fetch changed HEAD")
   }
  })
 }
}


func TestRemoteGitFetchRejectsStaleDigestBeforeNetwork(t *testing.T){
 root,old:=fetchTestRepo(t)
 review,err:=remoteGitHTTPSFetchReview(context.Background(),root)
 if err!=nil{t.Fatal(err)}
 digest:=sha256.Sum256(review)
 wrong:=strings.Repeat("0",64)
 if hex.EncodeToString(digest[:])==wrong{t.Fatal("unexpected matching digest")}
 if fetched,e:=remoteGitHTTPSFetchApply(context.Background(),root,wrong);
  e==nil||fetched!=""{
  t.Fatal("unreviewed HTTPS Fetch unexpectedly attempted network")
 }
 // The approved review also includes the exact local Git config snapshot;
 // updating origin or adding a credential helper must invalidate it.
 remotePullTestGit(t,root,"config","user.email","different@localhost.invalid")
 if fetched,e:=remoteGitHTTPSFetchApply(context.Background(),root,
  hex.EncodeToString(digest[:]));e==nil||fetched!=""{
  t.Fatal("stale host Git configuration did not invalidate Fetch approval")
 }
 loose,e:=os.ReadFile(filepath.Join(root,".git","refs","heads","main"))
 if e!=nil||strings.TrimSpace(string(loose))!=old{
  t.Fatal("denied Fetch advanced local branch")
 }
}

func TestRemoteGitFetchReadOnlyReviewNeverAllowsDirtyWorkspace(t *testing.T){
 root,_:=fetchTestRepo(t)
 if err:=os.WriteFile(filepath.Join(root,"untracked_secret"),[]byte("private"),0600);err!=nil{t.Fatal(err)}
 if report,e:=remoteGitHTTPSFetchReview(context.Background(),root);
  e==nil||len(report)!=0{
  t.Fatal("dirty workspace accepted a network Fetch review")
 }
}
