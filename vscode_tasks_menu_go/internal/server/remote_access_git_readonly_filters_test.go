package server

import(
 "context"
 "os"
 "os/exec"
 "path/filepath"
 "runtime"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// A plain git status/diff may run filter.clean from .gitattributes; this is
// an executable command, even when Git hooks and external diffs are off.
// Read-only E2EE roles must not be able to trigger it.
func TestRemoteGitDiffStatusIndexRejectExecutableFilters(t *testing.T){
 requireGit(t)
 if runtime.GOOS=="windows"{t.Skip("Unix-only Git process runner")}
 s,session,_,_:=remoteDispatchFixture(t)
 gitInitRemoteTest(t,s.Workspace)
 git:=func(args ...string)string{
  t.Helper()
  cmd:=exec.Command("git",append([]string{"-C",s.Workspace},args...)...)
  output,err:=cmd.CombinedOutput()
  if err!=nil{t.Fatalf("git %v failed: %v %s",args,err,output)}
  return strings.TrimSpace(string(output))
 }
 git("config","user.name","TaskDeck Filter Test")
 git("config","user.email","filters@example.invalid")
 if err:=os.WriteFile(filepath.Join(s.Workspace,".gitattributes"),
  []byte("tracked.txt filter=untrusted\n"),0600);err!=nil{t.Fatal(err)}
 file:=filepath.Join(s.Workspace,"tracked.txt")
 if err:=os.WriteFile(file,[]byte("original\n"),0600);err!=nil{t.Fatal(err)}
 git("add","tracked.txt",".gitattributes",".vscode/tasks.json")
 git("commit","-m","baseline")
 if err:=os.WriteFile(file,[]byte("changed\n"),0600);err!=nil{t.Fatal(err)}
 for _,flag:=range []string{
  "TASKDECK_REMOTE_GIT_STATUS","TASKDECK_REMOTE_GIT_DIFF",
  "TASKDECK_REMOTE_GIT_PATCH","TASKDECK_REMOTE_GIT_STAGE_TRACKED",
  "TASKDECK_REMOTE_GIT_UNSTAGE_ALL",
 }{t.Setenv(flag,"1")}
 before:=git("rev-parse","HEAD")
 marker:=filepath.Join(t.TempDir(),"filter-executed")
 for _,kind:=range []string{"clean","smudge","process"}{
  t.Run(kind,func(t *testing.T){
   key:="filter.untrusted."+kind
   git("config",key,"sh -c 'printf EXECUTED > "+marker+"; cat'")
   defer git("config","--unset",key)
   if _,err:=s.readRemoteGitStatus(context.Background());err==nil{
    t.Fatal("read-only Git status admitted executable filter")
   }
   if _,err:=s.readRemoteGitDiff(context.Background());err==nil{
    t.Fatal("read-only Git diff admitted executable filter")
   }
   if _,err:=s.readRemoteGitPatchPage(context.Background(),session,
    true,remoteDispatchTestID);err==nil{
    t.Fatal("read-only Git patch admitted executable filter")
   }
   if _,err:=s.executeRemoteGitIndex(context.Background(),session,
    remotetransport.RemoteActionGitStageTracked);err==nil{
    t.Fatal("stage-tracked index admitted executable filter")
   }
   if _,err:=s.executeRemoteGitIndex(context.Background(),session,
    remotetransport.RemoteActionGitUnstageAll);err==nil{
    t.Fatal("unstage-all index admitted executable filter")
   }
   if _,err:=os.Stat(marker);err==nil{
    t.Fatal("read-only or index preflight executed repository filter")
   }else if !os.IsNotExist(err){t.Fatal(err)}
   if got:=git("rev-parse","HEAD");got!=before{
    t.Fatal("filter-denied operation moved HEAD")
   }
  })
 }
}

func TestRemoteGitPullReviewRejectsExecutableFilterConfig(t *testing.T){
 s,_,_,_,git,_,head:=setupRemoteGitPullReview(t)
 t.Setenv("TASKDECK_REMOTE_GIT_PULL_REVIEW","1")
 marker:=filepath.Join(t.TempDir(),"pull-review-filter-executed")
 for _,kind:=range []string{"clean","smudge","process"}{
  t.Run(kind,func(t *testing.T){
   key:="filter.untrusted."+kind
   git("config",key,"sh -c 'printf EXECUTED > "+marker+"; cat'")
   defer git("config","--unset",key)
   if _,err:=s.readRemoteGitPullReview(context.Background());err==nil{
    t.Fatal("read-only pull review admitted executable filter config")
   }
   if _,err:=os.Stat(marker);err==nil{
    t.Fatal("pull review executed external filter helper")
   }else if !os.IsNotExist(err){t.Fatal(err)}
   if got:=git("rev-parse","HEAD");got!=head{
    t.Fatal("filter-denied pull review modified HEAD")
   }
  })
 }
}
