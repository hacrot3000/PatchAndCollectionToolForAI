//go:build linux || darwin

package server

import (
 "bufio"
 "context"
 "errors"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
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
 if root==""||(mode!="denied"&&mode!="allowed"&&mode!="hold"){
  t.Fatal("invalid helper setup")
 }
 release,err:=reserveRemoteWorkspaceExecution(context.Background(),root)
 if mode=="hold"{
  if err!=nil||release==nil{t.Fatalf("child lock failed: %v",err)}
  // The parent may observe this marker only AFTER the OS lock is held.
  // Kill the child without Unlock to exercise kernel crash cleanup.
  _,_=os.Stdout.WriteString("WORKSPACE_LOCK_HELD\n")
  for{time.Sleep(time.Second)}
 }
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


func TestRemoteWorkspaceOSLockReleasedWhenDaemonCrashes(t *testing.T){
 root:=t.TempDir()
 ctx,cancel:=context.WithTimeout(context.Background(),10*time.Second)
 defer cancel()
 cmd:=exec.CommandContext(ctx,os.Args[0],
  "-test.run=^TestRemoteWorkspaceOSLockSubprocessHelper$",
  "-test.count=1")
 cmd.Env=append(os.Environ(),
  "TASKDECK_TEST_WORKSPACE_FLOCK_HELPER=1",
  "TASKDECK_TEST_WORKSPACE_FLOCK_ROOT="+root,
  "TASKDECK_TEST_WORKSPACE_FLOCK_MODE=hold")
 pipe,err:=cmd.StdoutPipe()
 if err!=nil{t.Fatal(err)}
 if err:=cmd.Start();err!=nil{t.Fatal(err)}
 defer func(){
  if cmd.Process!=nil{_=cmd.Process.Kill()}
  _=cmd.Wait()
 }()
 marker,err:=bufio.NewReader(pipe).ReadString('\n')
 if err!=nil||strings.TrimSpace(marker)!="WORKSPACE_LOCK_HELD"{
  t.Fatalf("separate Agent never held its workspace lock: %q %v",marker,err)
 }
 _,err=reserveRemoteWorkspaceExecution(context.Background(),root)
 if !errors.Is(err,remotetransport.ErrRemoteActionDenied){
  t.Fatalf("parent bypassed live child's directory lock: %v",err)
 }
 if err:=cmd.Process.Kill();err!=nil{t.Fatalf("cannot kill child Agent: %v",err)}
 // Reap the process before asserting that all of its open descriptors
 // (and hence advisory locks) are closed by the OS, not by our code.
 if err:=cmd.Wait();err==nil{
  t.Fatal("helper unexpectedly exited normally instead of being killed")
 }
 release,err:=reserveRemoteWorkspaceExecution(context.Background(),root)
 if err!=nil{t.Fatalf("dead Agent left a stale OS lock: %v",err)}
 release()
}
