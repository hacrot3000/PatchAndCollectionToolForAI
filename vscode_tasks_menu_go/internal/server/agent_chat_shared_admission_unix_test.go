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

const sharedCapacityChildEnv="TASKDECK_SHARED_PROVIDER_CAPACITY_CHILD"
const sharedCapacityHoldMarker="TASKDECK_SHARED_PROVIDER_CAPACITY_HELD"

func TestAgentSharedCapacityAcrossDaemons(t *testing.T){
 mode:=os.Getenv(sharedCapacityChildEnv)
 if mode!=""{
  owner:=os.Getenv("TASKDECK_SHARED_PROVIDER_CAPACITY_OWNER")
  switch mode{
  case "try":
   ctx,cancel:=context.WithTimeout(context.Background(),200*time.Millisecond)
   defer cancel()
   release,ok:=acquireAgentProviderSlot(ctx,owner)
   if ok{
    fmt.Println("TASKDECK_SHARED_PROVIDER_CAPACITY_ADMITTED")
    release()
   }else{
    fmt.Println("TASKDECK_SHARED_PROVIDER_CAPACITY_REFUSED")
   }
  case "hold":
   ctx,cancel:=context.WithTimeout(context.Background(),5*time.Second)
   defer cancel()
   release,ok:=acquireAgentProviderSlot(ctx,owner)
   if !ok{t.Fatal("child could not acquire its own capacity slot")}
   defer release()
   fmt.Println(sharedCapacityHoldMarker)
   time.Sleep(30*time.Second) // parent must kill before release
   t.Fatal("parent failed to kill capacity holder")
  default:t.Fatalf("invalid child mode %q",mode)
  }
  return
 }
 t.Setenv("XDG_CONFIG_HOME",t.TempDir())
 resetAgentAdmissionsForTest()
 t.Cleanup(resetAgentAdmissionsForTest)
 runTry:=func(owner string,want bool){
  t.Helper()
  ctx,cancel:=context.WithTimeout(context.Background(),6*time.Second)
  defer cancel()
  cmd:=exec.CommandContext(ctx,os.Args[0],"-test.run=^TestAgentSharedCapacityAcrossDaemons$")
  cmd.Env=append(os.Environ(),
   sharedCapacityChildEnv+"=try",
   "TASKDECK_SHARED_PROVIDER_CAPACITY_OWNER="+owner)
  out,err:=cmd.CombinedOutput()
  if err!=nil{t.Fatalf("capacity child failed: %v\n%s",err,out)}
  accepted:=strings.Contains(string(out),"TASKDECK_SHARED_PROVIDER_CAPACITY_ADMITTED")
  refused:=strings.Contains(string(out),"TASKDECK_SHARED_PROVIDER_CAPACITY_REFUSED")
  if accepted==refused||accepted!=want{
   t.Fatalf("capacity child owner=%s admission=%t want=%t output=%s",owner,accepted,want,out)
  }
 }

 var global []func()
 for i:=0;i<agentMaxConcurrentGlobal;i++{
  ctx,cancel:=context.WithTimeout(context.Background(),2*time.Second)
  release,ok:=acquireAgentProviderSlot(ctx,fmt.Sprintf("workspace-%d:local",i))
  cancel()
  if !ok{t.Fatalf("global slot %d refused below capacity",i)}
  global=append(global,release)
 }
 runTry("unrelated-workspace:other",false)
 global[0]()
 runTry("unrelated-workspace:other",true)
 for _,release:=range global[1:]{release()}
 if _,total,_:=agentAdmissionState();total!=0{
  t.Fatalf("global quota leaked %d in-process slots",total)
 }

 // Enforce per-owner cap independently of global availability.
 owner:="fixed-workspace:local"
 var held []func()
 for i:=0;i<agentMaxConcurrentPerOwner;i++{
  ctx,cancel:=context.WithTimeout(context.Background(),2*time.Second)
  release,ok:=acquireAgentProviderSlot(ctx,owner)
  cancel()
  if !ok{t.Fatalf("owner slot %d refused below capacity",i)}
  held=append(held,release)
 }
 runTry(owner,false)
 runTry("another-workspace:local",true)
 held[0]()
 runTry(owner,true)
 held[1]()
 if _,total,_:=agentAdmissionState();total!=0{
  t.Fatalf("per-owner refusal leaked %d active in-process slots",total)
 }

 // A real killed daemon must release both global and per-owner leases. A
 // durable ProviderRun SENDING/UNKNOWN is deliberately NOT touched here.
 ctx,cancel:=context.WithTimeout(context.Background(),12*time.Second)
 defer cancel()
 cmd:=exec.CommandContext(ctx,os.Args[0],"-test.run=^TestAgentSharedCapacityAcrossDaemons$")
 cmd.Env=append(os.Environ(),
  sharedCapacityChildEnv+"=hold",
  "TASKDECK_SHARED_PROVIDER_CAPACITY_OWNER=killed-daemon:local")
 pipe,err:=cmd.StdoutPipe();if err!=nil{t.Fatal(err)}
 cmd.Stderr=os.Stderr
 if err=cmd.Start();err!=nil{t.Fatal(err)}
 defer func(){
  if cmd.ProcessState==nil{
   _=cmd.Process.Kill()
   _=cmd.Wait()
  }
 }()
 scanner:=bufio.NewScanner(pipe)
 if !scanner.Scan()||strings.TrimSpace(scanner.Text())!=sharedCapacityHoldMarker{
  t.Fatalf("killed holder never acquired capacity: %v",scanner.Err())
 }
 var others []func()
 for i:=0;i<agentMaxConcurrentGlobal-1;i++{
  wait,cancel:=context.WithTimeout(context.Background(),2*time.Second)
  release,ok:=acquireAgentProviderSlot(wait,fmt.Sprintf("other-%d:local",i))
  cancel()
  if !ok{t.Fatal("child-held global slot consumed more than one position")}
  others=append(others,release)
 }
 runTry("overflow:local",false)
 if err=cmd.Process.Kill();err!=nil{t.Fatal(err)}
 if err=cmd.Wait();err==nil||ctx.Err()!=nil{
  t.Fatalf("expected SIGKILL rather than normal/timeout exit: %v %v",err,ctx.Err())
 }
 runTry("overflow:local",true)
 for _,release:=range others{release()}
 if _,total,_:=agentAdmissionState();total!=0{
  t.Fatalf("child death/release leaked local slots: %d",total)
 }
}

func TestAgentSharedCapacityFailsClosedOnUnsafeLease(t *testing.T){
 t.Setenv("XDG_CONFIG_HOME",t.TempDir())
 resetAgentAdmissionsForTest()
 t.Cleanup(resetAgentAdmissionsForTest)
 dir,err:=agentSharedCapacityDir()
 if err!=nil{t.Fatal(err)}
 bad:=filepath.Join(dir,"global-0.lock")
 // Fail closed instead of skipping a corrupted/aliased capacity slot and
 // admitting a fifth turn beyond the global limit.
 if err=os.Symlink(filepath.Join(dir,"not-a-slot"),bad);err!=nil{
  t.Skipf("symlinks not supported: %v",err)
 }
 ctx,cancel:=context.WithTimeout(context.Background(),120*time.Millisecond)
 release,ok:=acquireAgentProviderSlot(ctx,"owner-A")
 cancel()
 if ok{release();t.Fatal("symlinked global capacity lock was allowed")}
 if err=os.Remove(bad);err!=nil{t.Fatal(err)}
 if err=os.WriteFile(bad,[]byte("unsafe"),0644);err!=nil{t.Fatal(err)}
 if err=os.Chmod(bad,0644);err!=nil{t.Fatal(err)}
 ctx,cancel=context.WithTimeout(context.Background(),120*time.Millisecond)
 release,ok=acquireAgentProviderSlot(ctx,"owner-A")
 cancel()
 if ok{release();t.Fatal("public capacity file was accepted")}
 if err=os.Remove(bad);err!=nil{t.Fatal(err)}
 if err=os.Chmod(dir,0755);err!=nil{t.Fatal(err)}
 ctx,cancel=context.WithTimeout(context.Background(),120*time.Millisecond)
 release,ok=acquireAgentProviderSlot(ctx,"owner-A")
 cancel()
 if ok{release();t.Fatal("insecure private capacity directory was accepted")}
 if err=os.Chmod(dir,0700);err!=nil{t.Fatal(err)}
 ctx,cancel=context.WithTimeout(context.Background(),time.Second)
 release,ok=acquireAgentProviderSlot(ctx,"owner-A")
 cancel()
 if !ok{t.Fatal("healthy capacity store failed after unsafe states repaired")}
 release()
 if _,total,_:=agentAdmissionState();total!=0{
  t.Fatalf("unsafe refusal or release leaked %d local slots",total)
 }
}
