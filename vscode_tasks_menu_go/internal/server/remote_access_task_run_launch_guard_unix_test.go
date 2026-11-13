//go:build linux || darwin

package server

import(
 "context"
 "os"
 "path/filepath"
 "strconv"
 "testing"
)

func shortLaunchApproval(t *testing.T, source string)(
 *Server,*remoteTaskDeckSession,int,remoteParallelPrepared,string){
 t.Helper()
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_RUN","1")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 id:=previewOneRemoteRunTask(t,s,session,client,agent,source)
 spec,_,err:=prepareRemoteTaskExecutionInput(
  context.Background(),s.Workspace,session.taskPageHash,id,0,false,"")
 if err!=nil{t.Fatal(err)}
 root,err:=filepath.EvalSymlinks(s.Workspace)
 if err!=nil{t.Fatal(err)}
 root,err=filepath.Abs(root)
 if err!=nil{t.Fatal(err)}
 cwd,err:=filepath.EvalSymlinks(spec.Cwd)
 if err!=nil{t.Fatal(err)}
 cwd,err=filepath.Abs(cwd)
 if err!=nil{t.Fatal(err)}
 exe,err:=remoteShortResolvedExecutable(spec.Command)
 if err!=nil{t.Fatal(err)}
 approved:=remoteParallelPrepared{
  execution:spec,executable:exe,resolvedCwd:cwd,
 }
 if _,err:=s.revalidateRemoteShortTaskAtLaunch(
  context.Background(),session,id,0,"",approved,root);err!=nil{
  t.Fatalf("unchanged approved task was refused: %v",err)
 }
 return s,session,id,approved,root
}
func TestRemoteShortLaunchRejectsPostAuditSourceChange(t *testing.T){
 s,session,id,approved,root:=shortLaunchApproval(t,
  `{"tasks":[{"label":"Safe","type":"process","command":"/bin/echo","args":["safe"]}]}`)
 path:=filepath.Join(s.Workspace,".vscode","tasks.json")
 data,err:=os.ReadFile(path)
 if err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(path,append(data,' '),0600);err!=nil{t.Fatal(err)}
 if _,err:=s.revalidateRemoteShortTaskAtLaunch(
  context.Background(),session,id,0,"",approved,root);err==nil{
  t.Fatal("stale source accepted at process launch")
 }
}
func TestRemoteShortLaunchRejectsExecutableRetarget(t *testing.T){
 link:=filepath.Join(t.TempDir(),"short-exe")
 if err:=os.Symlink("/bin/echo",link);err!=nil{t.Fatal(err)}
 source:=`{"tasks":[{"label":"Safe","type":"process","command":`+
  strconv.Quote(link)+`,"args":["safe"]}]}`
 s,session,id,approved,root:=shortLaunchApproval(t,source)
 if err:=os.Remove(link);err!=nil{t.Fatal(err)}
 if err:=os.Symlink("/bin/cat",link);err!=nil{t.Fatal(err)}
 if _,err:=s.revalidateRemoteShortTaskAtLaunch(
  context.Background(),session,id,0,"",approved,root);err==nil{
  t.Fatal("late executable symlink retarget accepted")
 }
}
func TestRemoteShortLaunchRejectsEnvironmentAndOptOut(t *testing.T){
 t.Setenv("TASKDECK_SHORT_LAUNCH_GUARD","before")
 s,session,id,approved,root:=shortLaunchApproval(t,
  `{"tasks":[{"label":"Safe","type":"process","command":"/bin/echo","args":["safe"]}]}`)
 t.Setenv("TASKDECK_SHORT_LAUNCH_GUARD","after")
 if _,err:=s.revalidateRemoteShortTaskAtLaunch(
  context.Background(),session,id,0,"",approved,root);err==nil{
  t.Fatal("late inherited environment change accepted")
 }
 t.Setenv("TASKDECK_REMOTE_TASKS_RUN","0")
 if _,err:=s.revalidateRemoteShortTaskAtLaunch(
  context.Background(),session,id,0,"",approved,root);err==nil{
  t.Fatal("host opt-out accepted at process launch")
 }
}
