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
 // Wait for the host task to finish without consuming the E2EE terminal
 // status receipt (which otherwise retires the managed task handle).
 deadline:=time.Now().Add(3*time.Second)
 for time.Now().Before(deadline){
  meta,ok:=s.Sessions.Metadata(owner.managedTask.sessionID)
  if ok&&meta.Status=="exited"{break}
  time.Sleep(25*time.Millisecond)
 }
 var total int64=-1
 var cursor int64
 var combined strings.Builder
 var reachedEOF bool
 for n:=2;n<=6;n++{
  window,readErr:=readManagedTaskOutputTest(t,s,owner,client,agent,started.Handle,cursor,n)
  if readErr!=nil{t.Fatal(readErr)}
  if window.Offset!=cursor||window.Next<=cursor||
   (total>=0&&window.Total!=total){
   t.Fatalf("wrong monotonic terminal cursor: %+v",window)
  }
  if n==2&&window.EOF{t.Fatal("first window falsely claimed full output")}
  total=window.Total
  cursor=window.Next
  combined.WriteString(window.Data)
  if window.EOF{reachedEOF=true;break}
 }
 if !reachedEOF||cursor!=total||!strings.Contains(combined.String(),payload){
  t.Fatalf("full managed output was lost: cursor=%d total=%d bytes=%d",
   cursor,total,combined.Len())
 }
 // The old single-frame final status must not silently present a tail as a
 // complete transcript if the output exceeds its 3200-byte limit.
 if total>remotetransport.RemoteManagedTaskMaxOutput {
  if _,err:=managedTaskTestRound(t,s,owner,client,agent,
   remotetransport.RemoteActionTaskManagedStatus,0,started.Handle,7);err==nil{
   t.Fatal("final status silently truncated large task output")
  }
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
