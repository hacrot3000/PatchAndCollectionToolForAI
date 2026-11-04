package server

import (
 "context"
 "encoding/hex"
 "fmt"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
 "bletonfc/vscode_tasks_menu/internal/tasks"
)

func managedTaskFixture(t *testing.T,s *Server,command string,args []string)int{
 t.Helper()
 payload:=fmt.Sprintf(`{"version":"2.0.0","tasks":[{"label":"remote managed test",
 "type":"process","command":%q,"args":[%s],"taskdeckRemoteAllowed":true}]}`,
  command,func()string{
   var pieces []string
   for _,arg:=range args{pieces=append(pieces,fmt.Sprintf("%q",arg))}
   return strings.Join(pieces,",")
  }())
 if err:=os.WriteFile(filepath.Join(s.Workspace,".vscode","tasks.json"),[]byte(payload),0600);err!=nil{t.Fatal(err)}
 items,err:=tasks.Load(s.Workspace)
 if err!=nil||len(items)!=1{t.Fatalf("invalid TaskDeck task fixture: %v",err)}
 token,err:=tasks.RemoteApprovalToken(items[0])
 if err!=nil{t.Fatal(err)}
 t.Setenv("TASKDECK_REMOTE_APPROVED_TASKS",token)
 return items[0].ID
}
func sealManagedTestRequest(t *testing.T,c *remotetransport.Channel,
 op string,taskID int,handle string,n int)[]byte{
 t.Helper()
 requestID:=fmt.Sprintf("%032x",n)
 request,err:=remotetransport.SealRemoteAction(c,remotetransport.RemoteAction{
  RequestID:requestID,Operation:op,TaskID:taskID,Path:handle,
 })
 if err!=nil{t.Fatal(err)}
 return request
}
func managedTaskTestRound(t *testing.T,s *Server,owner *remoteTaskDeckSession,
 client,agent *remotetransport.Channel,op string,taskID int,handle string,n int)(
 remotetransport.RemoteManagedTaskResult,error){
 t.Helper()
 req:=sealManagedTestRequest(t,client,op,taskID,handle,n)
 encrypted,err:=s.executeRemoteE2EEPing(context.Background(),owner,agent,req)
 if err!=nil{return remotetransport.RemoteManagedTaskResult{},err}
 result,err:=remotetransport.OpenRemoteManagedTaskResult(client,encrypted,fmt.Sprintf("%032x",n))
 return result,err
}
func TestRemoteManagedTaskStartsPollsCompletionAndReleasesSlot(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASK_RUN","1")
 t.Setenv("TASKDECK_REMOTE_TASK_MANAGE","1")
 s,owner,client,agent:=remoteDispatchFixtureWithRole(t,remotetransport.RoleOperator)
 defer owner.Close()
 taskID:=managedTaskFixture(t,s,"/bin/echo",[]string{"MANAGED_TASK_SUCCESS"})
 started,err:=managedTaskTestRound(t,s,owner,client,agent,
  remotetransport.RemoteActionTaskManagedStart,taskID,"",1)
 if err!=nil||!remotetransport.ValidManagedTaskHandle(started.Handle)||
  started.State!="running"||started.ExitCode!=-1{
  t.Fatalf("invalid TaskDeck start receipt %+v %v",started,err)
 }
 if again,err:=managedTaskTestRound(t,s,owner,client,agent,
  remotetransport.RemoteActionTaskManagedStart,taskID,"",2);err==nil||again.Handle!=""{
  t.Fatal("same session launched second task while first result unclaimed")
 }
 var finished remotetransport.RemoteManagedTaskResult
 for i:=3;i<9;i++{
  finished,err=managedTaskTestRound(t,s,owner,client,agent,
   remotetransport.RemoteActionTaskManagedStatus,0,started.Handle,i)
  if err!=nil{t.Fatal(err)}
  if finished.State=="exited"{break}
  time.Sleep(75*time.Millisecond)
 }
 if finished.State!="exited"||finished.ExitCode!=0||
  !strings.Contains(finished.Output,"MANAGED_TASK_SUCCESS"){
  t.Fatalf("host completion metadata incorrect: %+v",finished)
 }
 if owner.managedTask!=nil{t.Fatal("terminal receipt did not release task slot")}
}

func TestRemoteManagedTaskCancellationStopsOnlyItsOwnProcess(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASK_RUN","1")
 t.Setenv("TASKDECK_REMOTE_TASK_MANAGE","1")
 s,owner,client,agent:=remoteDispatchFixtureWithRole(t,remotetransport.RoleOperator)
 defer owner.Close()
 taskID:=managedTaskFixture(t,s,"/bin/sleep",[]string{"5"})
 started,err:=managedTaskTestRound(t,s,owner,client,agent,
  remotetransport.RemoteActionTaskManagedStart,taskID,"",1)
 if err!=nil{t.Fatal(err)}
 if owner.managedTask==nil{t.Fatal("TaskDeck did not retain live task")}
 wrongHandle:=strings.Repeat("f",32)
 if wrongHandle==started.Handle{wrongHandle=strings.Repeat("e",32)}
 // This is a well-formed but unauthorized task handle. It cannot name any
 // arbitrary PID, task from another login, or the Portal's remote session.
 if reply,err:=managedTaskTestRound(t,s,owner,client,agent,
  remotetransport.RemoteActionTaskManagedCancel,0,wrongHandle,2);err==nil||reply.Handle!=""{
  t.Fatal("forged handle canceled or inspected TaskDeck process")
 }
 acknowledged,err:=managedTaskTestRound(t,s,owner,client,agent,
  remotetransport.RemoteActionTaskManagedCancel,0,started.Handle,3)
 if err!=nil||acknowledged.State!="cancel_requested"{
  t.Fatalf("cancel did not acknowledge bounded host-owned task: %+v %v",acknowledged,err)
 }
 var done remotetransport.RemoteManagedTaskResult
 for i:=4;i<10;i++{
  done,err=managedTaskTestRound(t,s,owner,client,agent,
   remotetransport.RemoteActionTaskManagedStatus,0,started.Handle,i)
  if err!=nil{t.Fatal(err)}
  if done.State=="stopped"||done.State=="exited"{break}
  time.Sleep(80*time.Millisecond)
 }
 if done.State!="stopped"{
  t.Fatalf("cancel misrepresented completion: %+v",done)
 }
}

func TestRemoteManagedTaskRequiresIndependentOptInAndRealPrincipal(t *testing.T){
 for _,tc:=range []struct{name,flag string;role remotetransport.Role;phpOnly bool}{
  {"disabled","",remotetransport.RoleOperator,false},
  {"viewer","1",remotetransport.RoleViewer,false},
  {"php-only","1",remotetransport.RoleOperator,true},
 }{
  t.Run(tc.name,func(t *testing.T){
   t.Setenv("TASKDECK_REMOTE_TASK_RUN","1")
   t.Setenv("TASKDECK_REMOTE_TASK_MANAGE",tc.flag)
   s,owner,client,agent:=remoteDispatchFixtureWithRole(t,tc.role)
   defer owner.Close()
   id:=managedTaskFixture(t,s,"/bin/echo",[]string{"NEVER_RUN"})
   if tc.phpOnly{owner=nil}
   result,err:=managedTaskTestRound(t,s,owner,client,agent,
    remotetransport.RemoteActionTaskManagedStart,id,"",1)
   if err==nil||result.Handle!=""{
    t.Fatal("unauthorized Portal/role started managed TaskDeck child")
   }
  })
 }
}
func TestRemoteManagedTaskSessionCloseStopsChildWithoutRelayHelp(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASK_RUN","1")
 t.Setenv("TASKDECK_REMOTE_TASK_MANAGE","1")
 s,owner,client,agent:=remoteDispatchFixtureWithRole(t,remotetransport.RoleOperator)
 id:=managedTaskFixture(t,s,"/bin/sleep",[]string{"5"})
 started,err:=managedTaskTestRound(t,s,owner,client,agent,
  remotetransport.RemoteActionTaskManagedStart,id,"",1)
 if err!=nil{t.Fatal(err)}
 if _,err:=hex.DecodeString(started.Handle);err!=nil{t.Fatal(err)}
 entry:=owner.managedTask
 if entry==nil{t.Fatal("missing TaskDeck owned process")}
 owner.Close() // PHP/Relay cannot revive this principal.
 deadline:=time.Now().Add(2*time.Second)
 for time.Now().Before(deadline){
  state,ok:=s.Sessions.Metadata(entry.sessionID)
  if ok&&state.Status!="running"{return}
  time.Sleep(25*time.Millisecond)
 }
 t.Fatal("closing E2EE TaskDeck principal left managed process running")
}

func TestRemoteManagedTaskLargeLogReturnsExplicitOmissionWithoutBlockingSlot(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASK_RUN","1")
 t.Setenv("TASKDECK_REMOTE_TASK_MANAGE","1")
 s,owner,client,agent:=remoteDispatchFixtureWithRole(t,remotetransport.RoleOperator)
 defer owner.Close()
 payload:=strings.Repeat("L",remotetransport.RemoteManagedTaskMaxOutput+200)
 id:=managedTaskFixture(t,s,"/bin/echo",[]string{payload})
 started,err:=managedTaskTestRound(t,s,owner,client,agent,
  remotetransport.RemoteActionTaskManagedStart,id,"",1)
 if err!=nil{t.Fatal(err)}
 // The host must keep a task handle only until the first final receipt,
 // not forever because its output exceeds the single-message cap.
 deadline:=time.Now().Add(3*time.Second)
 for time.Now().Before(deadline){
  meta,ok:=s.Sessions.Metadata(owner.managedTask.sessionID)
  if ok&&meta.Status=="exited"{break}
  time.Sleep(25*time.Millisecond)
 }
 result,err:=managedTaskTestRound(t,s,owner,client,agent,
  remotetransport.RemoteActionTaskManagedStatus,0,started.Handle,2)
 if err!=nil{t.Fatal(err)}
 if result.State!="exited"||result.ExitCode!=0||!result.OutputOmitted||
  result.Output!=""{
  t.Fatalf("oversize task gave wrong or silently clipped final receipt: %+v",result)
 }
 if owner.managedTask!=nil{t.Fatal("oversize receipt kept the slot permanently occupied")}
 // The old handle is never reusable; there are no implicit output downloads,
 // task restarts or privilege escalation after the terminal receipt.
 if _,err:=readManagedTaskOutputTest(t,s,owner,client,agent,started.Handle,0,3);err==nil{
  t.Fatal("retired oversized task handle still exposed process output")
 }
 again,err:=managedTaskTestRound(t,s,owner,client,agent,
  remotetransport.RemoteActionTaskManagedStart,id,"",4)
 if err!=nil||again.State!="running"||again.Handle==started.Handle{
  t.Fatalf("next independently authorized task could not start: %+v %v",again,err)
 }
}
