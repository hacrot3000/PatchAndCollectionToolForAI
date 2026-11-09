package server

import(
 "context"
 "os"
 "path/filepath"
 "strings"
 "testing"
)

// All three host-approved branch operations run git status before
// deciding whether a mutation is safe. Git can invoke filter.clean
// through gitattributes during status despite disabled fsmonitor/hooks.
// Enforce a fail-closed executable-filter preflight on BOTH read-only
// reviews and mutating calls.
func TestRemoteGitBranchOperationsRejectExecutableFilters(t *testing.T){
 t.Run("branch-create",func(t *testing.T){
  s,session,git,_,_,_:=setupRemoteGitSwitch(t)
  t.Setenv("TASKDECK_REMOTE_GIT_BRANCH_CREATE","1")
  t.Setenv("TASKDECK_REMOTE_GIT_BRANCH_CREATE_NAME","new-safe-branch")
  installRemoteGitBranchFilterFixture(t,s.Workspace,git)
  before:=git("rev-parse","HEAD")
  assertRemoteGitBranchFiltersDenied(t,s.Workspace,git,func(){
   if _,err:=s.executeRemoteGitBranchCreate(context.Background(),session);err==nil{
    t.Fatal("host branch creation accepted executable filter")
   }
   if git("rev-parse","HEAD")!=before{
    t.Fatal("rejected branch-create changed HEAD")
   }
  })
 })
 t.Run("same-head-switch",func(t *testing.T){
  s,session,git,_,current,_:=setupRemoteGitSwitch(t)
  t.Setenv("TASKDECK_REMOTE_GIT_SWITCH","1")
  installRemoteGitBranchFilterFixture(t,s.Workspace,git)
  // The original review branch predates attributes commit; repin its
  // tip without checking it out, preserving the same-HEAD prerequisite.
  git("branch","-f","review","HEAD")
  assertRemoteGitBranchFiltersDenied(t,s.Workspace,git,func(){
   if _,err:=s.readRemoteGitSwitchReview(context.Background(),session);err==nil{
    t.Fatal("host switch preview admitted executable filter")
   }
   if _,err:=s.executeRemoteGitSwitchSameHead(context.Background(),session);err==nil{
    t.Fatal("host switch mutation admitted executable filter")
   }
   if got:=git("symbolic-ref","--short","HEAD");got!=current{
    t.Fatal("filter-denied switch moved the active branch")
   }
  })
 })
 t.Run("branch-delete",func(t *testing.T){
  s,session,git,current,_:=setupRemoteGitBranchDelete(t)
  installRemoteGitBranchFilterFixture(t,s.Workspace,git)
  git("branch","-f","taskdeck/safe-target","HEAD")
  target:=filepath.Join(s.Workspace,".git","refs","heads","taskdeck","safe-target")
  assertRemoteGitBranchFiltersDenied(t,s.Workspace,git,func(){
   if _,err:=s.readRemoteGitBranchDeleteReview(context.Background(),session);err==nil{
    t.Fatal("branch-delete review admitted executable filter")
   }
   if _,err:=s.executeRemoteGitBranchDelete(context.Background(),session);err==nil{
    t.Fatal("branch-delete mutation admitted executable filter")
   }
   if _,err:=os.Stat(target);err!=nil{
    t.Fatalf("branch ref removed despite filter denial: %v",err)
   }
   if got:=git("symbolic-ref","--short","HEAD");got!=current{
    t.Fatal("filter-denied delete moved the active branch")
   }
  })
 })
}

func installRemoteGitBranchFilterFixture(t *testing.T,root string,git func(...string)string){
 t.Helper()
 attrs:=filepath.Join(root,".gitattributes")
 if err:=os.WriteFile(attrs,[]byte("tracked.txt filter=remote-test\n"),0600);err!=nil{
  t.Fatal(err)
 }
 git("add",".gitattributes")
 git("commit","-m","Track filter attributes before enabling driver")
}

// Only test-owned, deterministic repository config; no untrusted command
// actually gets invoked by TaskDeck. A marker would prove any inadvertent
// execution of the configured shell filter.
func assertRemoteGitBranchFiltersDenied(t *testing.T,root string,
 git func(...string)string,exercise func()){
 t.Helper()
 marker:=filepath.Join(t.TempDir(),"filter-was-executed")
 for _,kind:=range []string{"clean","process","smudge"}{
  t.Run(kind,func(t *testing.T){
   key:="filter.remote-test."+kind
   git("config",key,"sh -c 'printf FILTER-RAN > "+marker+"; cat'")
   defer git("config","--unset",key)
   exercise()
   if _,err:=os.Stat(marker);err==nil{
    t.Fatalf("remote Git branch operation launched filter.%s",kind)
   }else if !os.IsNotExist(err){t.Fatal(err)}
  })
 }
 if contents,err:=os.ReadFile(filepath.Join(root,".gitattributes"));err!=nil||
  !strings.Contains(string(contents),"filter=remote-test"){
  t.Fatalf("filter fixture unexpectedly changed: %q %v",contents,err)
 }
}
