package server

import (
 "context"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "testing"
 "time"
)

func scheduleGitTestRun(t *testing.T,dir string,args ...string)string{
 t.Helper()
 cmd:=exec.Command("git",args...)
 cmd.Dir=dir
 cmd.Env=append(os.Environ(),"GIT_TERMINAL_PROMPT=0","LC_ALL=C")
 data,err:=cmd.CombinedOutput()
 if err!=nil{t.Fatalf("git %v in %s: %v\n%s",args,dir,err,string(data))}
 return strings.TrimSpace(string(data))
}

func scheduleGitTestSetup(t *testing.T)(*Server,string,string){
 t.Helper()
 if _,err:=exec.LookPath("git");err!=nil{t.Skip("git is not installed")}
 root:=t.TempDir()
 remote:=filepath.Join(root,"remote.git")
 checkout:=filepath.Join(root,"checkout")
 source:=filepath.Join(root,"upstream")
 scheduleGitTestRun(t,root,"init","--bare",remote)
 if err:=os.Mkdir(checkout,0700);err!=nil{t.Fatal(err)}
 scheduleGitTestRun(t,checkout,"init")
 scheduleGitTestRun(t,checkout,"checkout","-b","main")
 scheduleGitTestRun(t,checkout,"config","user.email","schedule@example.invalid")
 scheduleGitTestRun(t,checkout,"config","user.name","Schedule test")
 if err:=os.WriteFile(filepath.Join(checkout,"input.txt"),[]byte("one\n"),0600);err!=nil{t.Fatal(err)}
 scheduleGitTestRun(t,checkout,"add","input.txt")
 scheduleGitTestRun(t,checkout,"commit","-m","initial")
 scheduleGitTestRun(t,checkout,"remote","add","origin",remote)
 scheduleGitTestRun(t,checkout,"push","-u","origin","main")
 scheduleGitTestRun(t,root,"clone","-b","main",remote,source)
 scheduleGitTestRun(t,source,"config","user.email","schedule@example.invalid")
 scheduleGitTestRun(t,source,"config","user.name","Schedule test")
 return &Server{Workspace:root},checkout,source
}
func scheduleGitTestPush(t *testing.T,source,text string){
 t.Helper()
 if err:=os.WriteFile(filepath.Join(source,"input.txt"),[]byte(text+"\n"),0600);err!=nil{t.Fatal(err)}
 scheduleGitTestRun(t,source,"add","input.txt")
 scheduleGitTestRun(t,source,"commit","-m",text)
 scheduleGitTestRun(t,source,"push","origin","main")
}

func TestScheduleRemoteGitObservationAndSafePull(t *testing.T){
 s,checkout,source:=scheduleGitTestSetup(t)
 ctx,cancel:=context.WithTimeout(context.Background(),25*time.Second);defer cancel()
 cond:=scheduleCondition{ID:"git",Type:"git",GitPath:"checkout",GitRef:"HEAD",GitCondition:"remote_change",GitRemote:"origin",GitAutoPull:true}
 observe:=func()(string,bool){
  t.Helper()
  v,matched,err:=s.scheduleObservation(ctx,scheduleDefinition{Type:"git",GitPath:cond.GitPath,GitRef:cond.GitRef,GitCondition:cond.GitCondition,GitRemote:cond.GitRemote,GitAutoPull:cond.GitAutoPull})
  if err!=nil{t.Fatalf("remote observation: sha=%q matched=%v err=%v",v,matched,err)}
  return v,matched
 }
 initial,firstDiff:=observe()
 if firstDiff{t.Fatal("freshly synced checkout should not be treated as an outstanding remote update")}
 if local:=scheduleGitTestRun(t,checkout,"rev-parse","HEAD");local!=initial{t.Fatalf("baseline remote does not equal local HEAD")}
 scheduleGitTestPush(t,source,"two")
 updated,ahead:=observe()
 if !ahead{t.Fatal("already-ahead remote should be detected immediately, even before any local fetch")}
 if updated==initial{t.Fatal("remote update not observed")}
 if local:=scheduleGitTestRun(t,checkout,"rev-parse","HEAD");local!=initial{t.Fatal("remote observation unexpectedly fetched/pulled the checkout")}
 msg,err:=s.scheduleGitPull(ctx,cond)
 if err!=nil{t.Fatalf("pull: %v",err)}
 if !strings.Contains(msg,updated){t.Fatalf("pull message must include new SHA: %q",msg)}
 if local:=scheduleGitTestRun(t,checkout,"rev-parse","HEAD");local!=updated{t.Fatal("fast-forward did not move HEAD")}
 if _,remaining:=observe();remaining{t.Fatal("pull should clear remote/local mismatch")}
 // A dirty index/worktree must be retained, with no attempt to reset it.
 scheduleGitTestPush(t,source,"three")
 if err:=os.WriteFile(filepath.Join(checkout,"input.txt"),[]byte("LOCAL EDIT\n"),0600);err!=nil{t.Fatal(err)}
 if _,err=s.scheduleGitPull(ctx,cond);err==nil||!strings.Contains(err.Error(),"local changes"){t.Fatalf("dirty checkout should block auto-pull: %v",err)}
 if local:=scheduleGitTestRun(t,checkout,"rev-parse","HEAD");local!=updated{t.Fatal("dirty pull moved HEAD")}
 scheduleGitTestRun(t,checkout,"checkout","--","input.txt")
 // A diverged local commit must never be rebased/reset or auto-merged.
 if err:=os.WriteFile(filepath.Join(checkout,"local-only.txt"),[]byte("local\n"),0600);err!=nil{t.Fatal(err)}
 scheduleGitTestRun(t,checkout,"add","local-only.txt")
 scheduleGitTestRun(t,checkout,"commit","-m","local change")
 diverged:=scheduleGitTestRun(t,checkout,"rev-parse","HEAD")
 if _,err=s.scheduleGitPull(ctx,cond);err==nil||!strings.Contains(err.Error(),"non-fast-forward"){t.Fatalf("divergence should block auto-pull: %v",err)}
 if local:=scheduleGitTestRun(t,checkout,"rev-parse","HEAD");local!=diverged{t.Fatal("divergence unexpectedly modified local HEAD")}
}
func TestScheduleRemoteGitValidation(t *testing.T){
 s,checkout,_:=scheduleGitTestSetup(t)
 ctx,cancel:=context.WithTimeout(context.Background(),10*time.Second);defer cancel()
 for _,tc:=range []struct{remote,ref string}{
  {"-evil","main"},{"missing","main"},{"origin","-evil"},{"origin","refs/heads/not a branch"},
 }{
  if _,err:=scheduleGitRemoteHead(ctx,checkout,tc.remote,tc.ref);err==nil{t.Fatalf("unsafe or absent remote/ref accepted: %+v",tc)}
 }
 base:=scheduleDefinition{ID:"a",Name:"Git remote",Enabled:true,Type:"git",GitPath:"checkout",GitRef:"HEAD",GitRemote:"origin",GitCondition:"remote_change",GitAutoPull:true,IntervalSeconds:60,Target:"command",Command:"echo ok",RetainDays:7,RetainRuns:10}
 if err:=s.validateSchedule(base,scheduleStore{});err!=nil{t.Fatalf("valid legacy single trigger: %v",err)}
 base.ConditionGroup=&scheduleConditionGroup{Operator:"or",Conditions:[]scheduleCondition{
  {ID:"a",Type:"git",GitPath:"checkout",GitRef:"HEAD",GitRemote:"origin",GitCondition:"remote_change",GitAutoPull:true,IntervalSeconds:60},
  {ID:"b",Type:"cron",Cron:"* * * * *",Timezone:"Local"},
 }}
 base.Type="multi"
 if err:=s.validateSchedule(base,scheduleStore{});err!=nil{t.Fatalf("valid multi trigger: %v",err)}
 base.ConditionGroup.Conditions[0].GitCondition="change"
 if err:=s.validateSchedule(base,scheduleStore{});err==nil||!strings.Contains(err.Error(),"auto-pull"){t.Fatalf("auto pull on local-only trigger should be rejected: %v",err)}
 base.ConditionGroup=nil;base.Type="git";base.GitCondition="dirty"
 if err:=s.validateSchedule(base,scheduleStore{});err==nil||!strings.Contains(err.Error(),"auto-pull"){t.Fatalf("legacy auto pull on local-only trigger should be rejected: %v",err)}
}
