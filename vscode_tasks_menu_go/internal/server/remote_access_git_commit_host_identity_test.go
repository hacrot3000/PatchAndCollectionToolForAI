package server

import(
 "context"
 "errors"
 "os"
 "os/exec"
 "path/filepath"
 "runtime"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func TestRemoteGitCommitHostIdentityValidation(t *testing.T){
 cases:=[]struct{name,author,email string;valid,explicit bool}{
  {"no override","","",true,false},
  {"Vietnamese host name","Dương Trương","duong+agent@example.invalid",true,true},
  {"missing email","Local Admin","",false,false},
  {"missing name","","admin@example.invalid",false,false},
  {"control injection","Evil\nName","admin@example.invalid",false,false},
  {"trailing whitespace","Local Admin ","admin@example.invalid",false,false},
  {"domain traversal","Local Admin","a@example..invalid",false,false},
  {"bad domain label","Local Admin","a@-example.invalid",false,false},
 }
 for _,tc:=range cases{
  t.Run(tc.name,func(t *testing.T){
   t.Setenv("TASKDECK_REMOTE_GIT_AUTHOR_NAME",tc.author)
   t.Setenv("TASKDECK_REMOTE_GIT_AUTHOR_EMAIL",tc.email)
   name,email,explicit,valid:=remoteGitCommitHostIdentity()
   if valid!=tc.valid||explicit!=tc.explicit{
    t.Fatalf("identity validity mismatch: explicit=%v valid=%v",explicit,valid)
   }
   if valid&&explicit&&(name!=tc.author||email!=tc.email){
    t.Fatalf("validated host identity not preserved")
   }
  })
 }
}

func TestRemoteGitCommitHostAuthorWithoutGlobalConfig(t *testing.T){
 requireGit(t)
 if runtime.GOOS=="windows"{t.Skip("remote Git process execution is Unix-only")}
 t.Setenv("TASKDECK_REMOTE_GIT_COMMIT","1")
 t.Setenv("TASKDECK_REMOTE_GIT_AUTHOR_NAME","TaskDeck Remote")
 t.Setenv("TASKDECK_REMOTE_GIT_AUTHOR_EMAIL","remote@example.invalid")
 s,session,_,_:=remoteTerminalAdminFixture(t)
 gitInitRemoteTest(t,s.Workspace)
 run:=func(args ...string)string{
  t.Helper()
  cmd:=exec.Command("git",append([]string{"-C",s.Workspace},args...)...)
  out,err:=cmd.CombinedOutput()
  if err!=nil{t.Fatalf("git %v failed: %v: %s",args,err,out)}
  return strings.TrimSpace(string(out))
 }
 path:=filepath.Join(s.Workspace,"tracked.txt")
 if err:=os.WriteFile(path,[]byte("before\n"),0600);err!=nil{t.Fatal(err)}
 run("add","tracked.txt")
 run("-c","user.name=Bootstrap","-c","user.email=bootstrap@example.invalid",
  "commit","-qm","baseline")
 // The test repository deliberately has NO local identity; remote Commit
 // also refuses global Git config and inherited Git author overrides.
 checkLocal:=exec.Command("git","-C",s.Workspace,"config","--local","--get","user.name")
 if out,err:=checkLocal.CombinedOutput();err==nil{
  t.Fatalf("fixture unexpectedly configured repository-local author: %q",out)
 }
 if err:=os.WriteFile(path,[]byte("after\n"),0600);err!=nil{t.Fatal(err)}
 run("add","tracked.txt")
 before:=run("rev-parse","HEAD")
 result,err:=s.executeRemoteGitCommit(context.Background(),session,"Host-configured author")
 if err!=nil||result.Before!=before||result.After==before{
  t.Fatalf("host-configured author failed: %+v %v",result,err)
 }
 if got:=run("log","-1","--format=%an <%ae>");got!="TaskDeck Remote <remote@example.invalid>"{
  t.Fatalf("commit author did not match agent configuration: %q",got)
 }
}

type remoteGitCommitIdentityAuditDrift struct{ identity.Store; email string }
func(h remoteGitCommitIdentityAuditDrift)AppendAudit(ctx context.Context,
 event identity.AuditEvent)error{
 err:=h.Store.AppendAudit(ctx,event)
 if err==nil&&event.Action=="remote.action.git_commit"&&event.Result=="attempted"{
  return os.Setenv("TASKDECK_REMOTE_GIT_AUTHOR_EMAIL",h.email)
 }
 return err
}

func TestRemoteGitCommitPostAuditAuthorDriftRejectsMutation(t *testing.T){
 requireGit(t)
 if runtime.GOOS=="windows"{t.Skip("remote Git process execution is Unix-only")}
 t.Setenv("TASKDECK_REMOTE_GIT_COMMIT","1")
 t.Setenv("TASKDECK_REMOTE_GIT_AUTHOR_NAME","Approved Agent")
 t.Setenv("TASKDECK_REMOTE_GIT_AUTHOR_EMAIL","original@example.invalid")
 s,session,_,_:=remoteTerminalAdminFixture(t)
 gitInitRemoteTest(t,s.Workspace)
 run:=func(args ...string)string{
  t.Helper()
  cmd:=exec.Command("git",append([]string{"-C",s.Workspace},args...)...)
  out,err:=cmd.CombinedOutput()
  if err!=nil{t.Fatalf("git %v failed: %v %s",args,err,out)}
  return strings.TrimSpace(string(out))
 }
 path:=filepath.Join(s.Workspace,"tracked.txt")
 if err:=os.WriteFile(path,[]byte("first\n"),0600);err!=nil{t.Fatal(err)}
 run("add","tracked.txt")
 run("-c","user.name=Bootstrap","-c","user.email=bootstrap@example.invalid",
  "commit","-qm","baseline")
 before:=run("rev-parse","HEAD")
 if err:=os.WriteFile(path,[]byte("second\n"),0600);err!=nil{t.Fatal(err)}
 run("add","tracked.txt")
 s.Identity=remoteGitCommitIdentityAuditDrift{
  Store:s.Identity,email:"changed@example.invalid",
 }
 _,err:=s.executeRemoteGitCommit(context.Background(),session,"Must be denied")
 if !errors.Is(err,remotetransport.ErrRemoteActionDenied){
  t.Fatalf("post-audit identity mutation accepted: %v",err)
 }
 if after:=run("rev-parse","HEAD");after!=before{
  t.Fatalf("post-audit identity drift moved HEAD: %s",after)
 }
}
