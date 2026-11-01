package server

import (
 "context"
 "fmt"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func readManagedTaskOutputTest(t *testing.T,s *Server,owner *remoteTaskDeckSession,
 client,host *remotetransport.Channel,handle string,offset int64,n int)(
 remotetransport.RemoteManagedOutput,error){
 t.Helper()
 id:=fmt.Sprintf("%032x",n)
 frame,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:id,Operation:remotetransport.RemoteActionTaskManagedOutput,
  Path:handle,Offset:offset,
 })
 if err!=nil{t.Fatal(err)}
 result,err:=s.executeRemoteE2EEPing(context.Background(),owner,host,frame)
 if err!=nil{return remotetransport.RemoteManagedOutput{},err}
 return remotetransport.OpenRemoteManagedOutput(client,result,id)
}
func TestRemoteManagedOutputWindowsReconstructLargerThanFrame(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASK_RUN","1")
 t.Setenv("TASKDECK_REMOTE_TASK_MANAGE","1")
 s,owner,client,agent:=remoteDispatchFixtureWithRole(t,remotetransport.RoleOperator)
 defer owner.Close()
 payload:=strings.Repeat("F",2800)
 id:=managedTaskFixture(t,s,"/bin/echo",[]string{payload})
 started,err:=managedTaskTestRound(t,s,owner,client,agent,
  remotetransport.RemoteActionTaskManagedStart,id,"",1)
 if err!=nil{t.Fatal(err)}
 first,err:=readManagedTaskOutputTest(t,s,owner,client,agent,started.Handle,0,2)
 if err!=nil||first.Next<=0||first.Total<=first.Next||first.Offset!=0{
  t.Fatalf("first output window invalid: %+v %v",first,err)
 }
 second,err:=readManagedTaskOutputTest(t,s,owner,client,agent,started.Handle,first.Next,3)
 if err!=nil||second.Offset!=first.Next||second.Next<=first.Next{
  t.Fatalf("output window lost byte offset: %+v %v",second,err)
 }
 combined:=first.Data+second.Data
 if !strings.Contains(combined,payload){
  t.Fatalf("typed output windows dropped or changed task data: got len %d",len(combined))
 }
 if first.EOF{t.Fatal("first truncated window falsely announced EOF")}
 // A final completion receipt intentionally retires this task. Native
 // callers must read cursor windows before consuming terminal task-status.
 for n:=4;n<9;n++{
  receipt,err:=managedTaskTestRound(t,s,owner,client,agent,
   remotetransport.RemoteActionTaskManagedStatus,0,started.Handle,n)
  if err!=nil{t.Fatal(err)}
  if receipt.State=="exited"{break}
  time.Sleep(25*time.Millisecond)
 }
 if _,err:=readManagedTaskOutputTest(t,s,owner,client,agent,started.Handle,0,9);err==nil{
  t.Fatal("retired task handle allowed reading process output")
 }
}
func TestRemoteManagedOutputCannotInspectOtherProcessOrBypassHostFlag(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASK_RUN","1")
 t.Setenv("TASKDECK_REMOTE_TASK_MANAGE","1")
 s,owner,client,agent:=remoteDispatchFixtureWithRole(t,remotetransport.RoleOperator)
 defer owner.Close()
 taskID:=managedTaskFixture(t,s,"/bin/sleep",[]string{"5"})
 started,err:=managedTaskTestRound(t,s,owner,client,agent,
  remotetransport.RemoteActionTaskManagedStart,taskID,"",1)
 if err!=nil{t.Fatal(err)}
 wrong:=strings.Repeat("e",32)
 if wrong==started.Handle{wrong=strings.Repeat("f",32)}
 if _,err:=readManagedTaskOutputTest(t,s,owner,client,agent,wrong,0,2);err==nil{
  t.Fatal("forged handle disclosed another process's PTY output")
 }
 t.Setenv("TASKDECK_REMOTE_TASK_MANAGE","")
 if _,err:=readManagedTaskOutputTest(t,s,owner,client,agent,started.Handle,0,3);err==nil{
  t.Fatal("host disable still allowed process output")
 }
}
