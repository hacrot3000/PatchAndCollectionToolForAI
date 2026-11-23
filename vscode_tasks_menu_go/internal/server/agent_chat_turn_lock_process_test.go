//go:build linux || darwin

package server

import (
 "bufio"
 "context"
 "fmt"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "testing"
 "time"
)

const agentTurnLeaseTestEnv = "TASKDECK_AGENT_TURN_LEASE_CHILD"
const agentTurnLeaseMarker = "TASKDECK_CHILD_TURN_LOCK_ACQUIRED"

func TestAgentTurnCrossProcessLease(t *testing.T){
 mode:=os.Getenv(agentTurnLeaseTestEnv)
 if mode!=""{
  id:=os.Getenv("TASKDECK_AGENT_TURN_LEASE_ID")
  workspace:=os.Getenv("TASKDECK_AGENT_TURN_LEASE_WORKSPACE")
  release,ok:=lockAgentTurn(workspace,"local",id)
  switch mode{
  case "try":
   if ok{
    fmt.Println(agentTurnLeaseMarker)
    release()
   }else{
    fmt.Println("TASKDECK_CHILD_TURN_LOCK_BUSY")
   }
   return
  case "hold":
   if !ok{t.Fatal("child could not acquire OS turn lock")}
   defer release()
   fmt.Println(agentTurnLeaseMarker)
   time.Sleep(30*time.Second) // parent must SIGKILL this process
   t.Fatal("parent did not terminate the holding child")
  default: t.Fatal("invalid test mode")
  }
  return
 }
 t.Setenv("XDG_CONFIG_HOME",t.TempDir())
 workspace:=filepath.Join(t.TempDir(),"project")
 if err:=os.MkdirAll(workspace,0700);err!=nil{t.Fatal(err)}
 settings,err:=aiPrivatePath(workspace)
 if err!=nil{t.Fatal(err)}
 dir:=filepath.Join(filepath.Dir(settings),"agent-sessions-"+aiWorkspaceHash(workspace))
 if err:=os.MkdirAll(dir,0700);err!=nil{t.Fatal(err)}
 const id="1234567890abcdef1234567890abcdef"
 childEnv:=func(mode string)[]string{
  return append(os.Environ(),
   agentTurnLeaseTestEnv+"="+mode,
   "TASKDECK_AGENT_TURN_LEASE_ID="+id,
   "TASKDECK_AGENT_TURN_LEASE_WORKSPACE="+workspace,
  )
 }
 runTry:=func()string{
  t.Helper()
  ctx,cancel:=context.WithTimeout(context.Background(),8*time.Second)
  defer cancel()
  cmd:=exec.CommandContext(ctx,os.Args[0],"-test.run=^TestAgentTurnCrossProcessLease$")
  cmd.Env=childEnv("try")
  data,e:=cmd.CombinedOutput()
  if e!=nil{t.Fatalf("child try failed: %v\n%s",e,data)}
  return string(data)
 }
 ownerRelease,ok:=lockAgentTurn(workspace,"local",id)
 if !ok{t.Fatal("parent cannot acquire new session lease")}
 if release,ok:=lockAgentTurn(workspace,"other-owner",id);ok{
  release()
  t.Fatal("same session accepted duplicate in-process turn")
 }
 if result:=runTry();!strings.Contains(result,"TASKDECK_CHILD_TURN_LOCK_BUSY"){
  t.Fatalf("second daemon entered an active Agent turn: %s",result)
 }
 // A separate conversation must stay independent for parallel user tabs.
 if release,ok:=lockAgentTurn(workspace,"local","aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa");!ok{
  t.Fatal("session lease incorrectly blocks unrelated conversations")
 }else{release()}
 ownerRelease()
 if result:=runTry();!strings.Contains(result,agentTurnLeaseMarker){
  t.Fatalf("lock was not released for another process: %s",result)
 }

 ctx,cancel:=context.WithTimeout(context.Background(),12*time.Second)
 defer cancel()
 cmd:=exec.CommandContext(ctx,os.Args[0],"-test.run=^TestAgentTurnCrossProcessLease$")
 cmd.Env=childEnv("hold")
 pipe,err:=cmd.StdoutPipe()
 if err!=nil{t.Fatal(err)}
 cmd.Stderr=os.Stderr
 if err:=cmd.Start();err!=nil{t.Fatal(err)}
 defer func(){
  if cmd.ProcessState==nil{
   _=cmd.Process.Kill()
   _=cmd.Wait()
  }
 }()
 scanner:=bufio.NewScanner(pipe)
 if !scanner.Scan()||strings.TrimSpace(scanner.Text())!=agentTurnLeaseMarker{
  t.Fatalf("holder did not reach lock barrier: err=%v",scanner.Err())
 }
 if release,ok:=lockAgentTurn(workspace,"local",id);ok{
  release()
  t.Fatal("second daemon entered while child held a turn lease")
 }
 if err:=cmd.Process.Kill();err!=nil{t.Fatal(err)}
 if err:=cmd.Wait();err==nil||ctx.Err()!=nil{
  t.Fatalf("expected forced process kill, not normal exit/timeout: err=%v ctx=%v",err,ctx.Err())
 }
 if release,ok:=lockAgentTurn(workspace,"local",id);!ok{
  t.Fatal("flock did not auto-release after a killed Agent daemon")
 }else{release()}
}

func TestAgentTurnLeaseFailsClosedOnUnsafeState(t *testing.T){
 t.Setenv("XDG_CONFIG_HOME",t.TempDir())
 workspace:=filepath.Join(t.TempDir(),"workspace")
 if err:=os.MkdirAll(workspace,0700);err!=nil{t.Fatal(err)}
 id:="bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
 if release,ok:=lockAgentTurn(workspace,"local",id);ok{
  release()
  t.Fatal("missing private Agent store must fail closed")
 }
 settings,err:=aiPrivatePath(workspace)
 if err!=nil{t.Fatal(err)}
 dir:=filepath.Join(filepath.Dir(settings),"agent-sessions-"+aiWorkspaceHash(workspace))
 if err=os.MkdirAll(dir,0700);err!=nil{t.Fatal(err)}
 path:=filepath.Join(dir,".agent-turn-"+id+".lock")
 if err=os.WriteFile(path,[]byte("untrusted"),0644);err!=nil{t.Fatal(err)}
 if err=os.Chmod(path,0644);err!=nil{t.Fatal(err)}
 if release,ok:=lockAgentTurn(workspace,"local",id);ok{
  release()
  t.Fatal("world-readable turn lock must fail closed")
 }
 if err=os.Remove(path);err!=nil{t.Fatal(err)}
 if err=os.Symlink(filepath.Join(dir,"untrusted"),path);err!=nil{t.Skipf("symlinks unavailable: %v",err)}
 if release,ok:=lockAgentTurn(workspace,"local",id);ok{
  release()
  t.Fatal("symlinked turn lock must fail closed")
 }
}
