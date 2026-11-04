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

func remotePullChangesFixture(t *testing.T)(root,old,target string){
 t.Helper()
 root,_=remotePullTestRepo(t)
 if err:=os.WriteFile(filepath.Join(root,"a.txt"),[]byte("first\n"),0600);err!=nil{t.Fatal(err)}
 remotePullTestGit(t,root,"add","a.txt")
 remotePullTestGit(t,root,"-c","user.name=TaskDeck test",
  "-c","user.email=test@localhost.invalid","commit","-q","-m","baseline with a file")
 old=remotePullTestGit(t,root,"rev-parse","HEAD")
 remotePullTestGit(t,root,"update-ref","refs/remotes/origin/main",old)
 if err:=os.WriteFile(filepath.Join(root,"a.txt"),[]byte("second\n"),0600);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(root,"b.txt"),[]byte("new\n"),0600);err!=nil{t.Fatal(err)}
 remotePullTestGit(t,root,"add","a.txt","b.txt")
 remotePullTestGit(t,root,"-c","user.name=TaskDeck test",
  "-c","user.email=test@localhost.invalid","commit","-q","-m","upstream update files")
 target=remotePullTestGit(t,root,"rev-parse","HEAD")
 remotePullTestGit(t,root,"update-ref","refs/remotes/origin/main",target)
 remotePullTestGit(t,root,"reset","--hard",old)
 return root,old,target
}

func TestRemoteGitPullChangedPathsPreviewsExactFastForwardWithoutMutation(t *testing.T){
 root,old,target:=remotePullChangesFixture(t)
 indexBefore,err:=os.ReadFile(filepath.Join(root,".git","index"))
 if err!=nil{t.Fatal(err)}
 preview,err:=remoteGitPullChangesReview(context.Background(),root)
 if err!=nil||!remotetransport.ValidRemoteGitPullChangesReport(string(preview)){
  t.Fatalf("full trusted Pull preview failed: %q %v",preview,err)
 }
 for _,part:=range []string{
  "HEAD "+old,"UPSTREAM_HEAD "+target,"FILES 2",
  "M a.txt\nA b.txt\n","NETWORK none\n",
 }{
  if !strings.Contains(string(preview),part){t.Fatalf("missing item %q from %q",part,preview)}
 }
 if got:=remotePullTestGit(t,root,"rev-parse","HEAD");got!=old{
  t.Fatal("read-only changed paths preview advanced HEAD")
 }
 now,err:=os.ReadFile(filepath.Join(root,".git","index"))
 if err!=nil||string(now)!=string(indexBefore){t.Fatal("Git index changed during preview")}
 if _,err:=os.Stat(filepath.Join(root,"b.txt"));!os.IsNotExist(err){
  t.Fatal("read-only change preview checked out upstream's new file")
 }
}

func TestRemoteGitPullChangedPathsRejectsDirtyAndStaleUpstream(t *testing.T){
 root,old,target:=remotePullChangesFixture(t)
 if err:=os.WriteFile(filepath.Join(root,"user-untracked"),[]byte("leave me"),0600);err!=nil{t.Fatal(err)}
 if r,err:=remoteGitPullChangesReview(context.Background(),root);err==nil||len(r)!=0{
  t.Fatal("dirty checkout exposed ready Pull manifest")
 }
 if err:=os.Remove(filepath.Join(root,"user-untracked"));err!=nil{t.Fatal(err)}
 remotePullTestGit(t,root,"update-ref","refs/remotes/origin/main",old,target)
 if r,err:=remoteGitPullChangesReview(context.Background(),root);err==nil||len(r)!=0{
  t.Fatal("up-to-date upstream represented as changed-file Pull")
 }
}

func TestRemoteGitPullChangedPathsRejectsUnsafeNamesAndTooManyFiles(t *testing.T){
 for _,tc:=range []struct{name string;paths []string}{
  {"unsafe-name",[]string{"name with spaces.txt"}},
  {"hidden",[]string{".private.txt"}},
  {"over-limit",func()[]string{
   var names []string
   for i:=0;i<remotetransport.RemoteGitPullChangesMaxFiles+1;i++{
    names=append(names,strings.Repeat("x",i+1)+".txt")
   }
   return names
  }()},
 }{
  t.Run(tc.name,func(t *testing.T){
   root,old:=remotePullTestRepo(t)
   for _,path:=range tc.paths{
    if err:=os.WriteFile(filepath.Join(root,path),[]byte("content"),0600);err!=nil{t.Fatal(err)}
   }
   remotePullTestGit(t,root,"add",".")
   remotePullTestGit(t,root,"-c","user.name=TaskDeck test",
    "-c","user.email=test@localhost.invalid","commit","-q","-m","upstream changed paths")
   target:=remotePullTestGit(t,root,"rev-parse","HEAD")
   remotePullTestGit(t,root,"update-ref","refs/remotes/origin/main",target)
   remotePullTestGit(t,root,"reset","--hard",old)
   if out,err:=remoteGitPullChangesReview(context.Background(),root);err==nil||len(out)!=0{
    t.Fatal("unsafe/too many changed paths emitted a partial trusted manifest")
   }
  })
 }
}

func TestRemoteGitPullChangesAuthenticatedHostRequiresLocalPrincipal(t *testing.T){
 s,owner,client,agent:=remoteDispatchFixtureWithRole(t,remotetransport.RoleAdmin)
 root:=s.Workspace
 init:=exec.Command("git","init","-q","-b","main",root)
 if out,err:=init.CombinedOutput();err!=nil{t.Fatalf("git init: %v %s",err,out)}
 remotePullTestGit(t,root,"add",".vscode/tasks.json")
 remotePullTestGit(t,root,"-c","user.name=TaskDeck test",
  "-c","user.email=test@localhost.invalid","commit","-q","-m","baseline")
 old:=remotePullTestGit(t,root,"rev-parse","HEAD")
 remotePullTestGit(t,root,"remote","add","origin","https://unreachable.invalid/repository.git")
 remotePullTestGit(t,root,"config","branch.main.remote","origin")
 remotePullTestGit(t,root,"config","branch.main.merge","refs/heads/main")
 if err:=os.WriteFile(filepath.Join(root,"visible.txt"),[]byte("upstream"),0600);err!=nil{t.Fatal(err)}
 remotePullTestGit(t,root,"add","visible.txt")
 remotePullTestGit(t,root,"-c","user.name=TaskDeck test",
  "-c","user.email=test@localhost.invalid","commit","-q","-m","future file")
 target:=remotePullTestGit(t,root,"rev-parse","HEAD")
 remotePullTestGit(t,root,"update-ref","refs/remotes/origin/main",target)
 remotePullTestGit(t,root,"reset","--hard",old)
 id:="0123456789abcdef0123456789abcdef"
 req,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:id,Operation:remotetransport.RemoteActionGitPullChangesReview,
 })
 if err!=nil{t.Fatal(err)}
 if encrypted,err:=s.executeRemoteE2EEPing(context.Background(),nil,agent,req);
  err==nil||len(encrypted)!=0{
  t.Fatal("PHP ticket alone bypassed TaskDeck login for changed paths")
 }
 reply,err:=s.executeRemoteE2EEPing(context.Background(),owner,agent,req)
 if err!=nil{t.Fatalf("host changed-path review denied: %v",err)}
 manifest,err:=remotetransport.OpenRemoteGitPullChangesResult(client,reply,id)
 if err!=nil||!strings.Contains(manifest,"A visible.txt\n"){
  t.Fatalf("authenticated changed-file result invalid: %q %v",manifest,err)
 }
 if current:=remotePullTestGit(t,root,"rev-parse","HEAD");current!=old{
  t.Fatal("E2EE read-only Pull review advanced HEAD")
 }
 if _,err:=s.executeRemoteE2EEPing(context.Background(),owner,agent,req);err==nil{
  t.Fatal("replayed changed-path read accepted")
 }
}
