//go:build linux || darwin

package server

import (
 "context"
 "errors"
 "os"
 "os/exec"
 "path/filepath"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// Spawn this SAME compiled test executable as a different process. A
// goroutine-only test cannot prove that two TaskDeck daemons exclude each
// other, since they already share the in-process workspace map.
func TestRemoteWorkspaceOSLockSubprocessHelper(t *testing.T){
 if os.Getenv("TASKDECK_TEST_WORKSPACE_FLOCK_HELPER")!="1"{return}
 root:=os.Getenv("TASKDECK_TEST_WORKSPACE_FLOCK_ROOT")
 mode:=os.Getenv("TASKDECK_TEST_WORKSPACE_FLOCK_MODE")
 if root==""||(mode!="denied"&&mode!="allowed"){
  t.Fatal("invalid helper setup")
 }
 release,err:=reserveRemoteWorkspaceExecution(context.Background(),root)
 if mode=="denied"{
  if !errors.Is(err,remotetransport.ErrRemoteActionDenied){
   if release!=nil{release()}
   t.Fatalf("another TaskDeck process acquired active workspace: %v",err)
  }
  return
 }
 if err!=nil||release==nil{
  t.Fatalf("workspace was not released after prior process exited: %v",err)
 }
 release()
}

func TestRemoteWorkspaceOSLockAcrossIndependentProcesses(t *testing.T){
 root:=t.TempDir()
 alias:=filepath.Join(t.TempDir(),"alias")
 if err:=os.Symlink(root,alias);err!=nil{
  t.Skipf("filesystem cannot create alias: %v",err)
 }
 run:=func(path,mode string){
  t.Helper()
  cmd:=exec.Command(os.Args[0],
   "-test.run=^TestRemoteWorkspaceOSLockSubprocessHelper$",
   "-test.count=1")
  cmd.Env=append(os.Environ(),
   "TASKDECK_TEST_WORKSPACE_FLOCK_HELPER=1",
   "TASKDECK_TEST_WORKSPACE_FLOCK_ROOT="+path,
   "TASKDECK_TEST_WORKSPACE_FLOCK_MODE="+mode)
  started:=time.Now()
  output,err:=cmd.CombinedOutput()
  if err!=nil{
   t.Fatalf("subprocess mode %s: %v; output: %s",mode,err,output)
  }
  if time.Since(started)>8*time.Second{
   t.Fatalf("OS workspace reservation took too long (%s)",mode)
  }
 }
 release,err:=reserveRemoteWorkspaceExecution(context.Background(),root)
 if err!=nil{t.Fatalf("cannot reserve parent process workspace: %v",err)}
 run(root,"denied")
 run(alias,"denied") // canonical symlink alias must share inode lock
 release()
 run(alias,"allowed")
 run(root,"allowed")
 // Must be automatically released even if the process that held the
 // descriptor crashes; it never creates a stale .lock file.
 if _,err:=os.Stat(filepath.Join(root,".taskdeck-workspace.lock"));
  !os.IsNotExist(err){
  t.Fatal("workspace lease unexpectedly created a project lock file")
 }
}
