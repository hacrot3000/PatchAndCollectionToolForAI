package broker

import (
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/tasks"
)

func TestRemoteManagedTaskUsesLocalOffsetReaderWithoutBrokerHTTP(t *testing.T){
 compat:=NewCompatibilityService(nil)
 defer compat.Close()
 task:=tasks.Execution{
  TaskID:7,Label:"local-only remote managed test",
  Command:"/bin/echo",Args:[]string{"OFFSET_PROOF"},
  Cwd:t.TempDir(),Env:[]string{"PATH=/usr/bin:/bin"},
  SessionKind:tasks.SessionKindTask,OwnerUserID:"operator",ProjectID:"project",
 }
 if _,err:=compat.StartRemoteManagedProcess(tasks.Execution{
  Command:"/bin/echo",Cwd:task.Cwd,OwnerUserID:"operator",
  ProjectID:"project",SessionKind:tasks.SessionKindTask,
 });err==nil{
  t.Fatal("unbound managed process admitted")
 }
 meta,err:=compat.StartRemoteManagedProcess(task)
 if err!=nil{t.Fatal(err)}
 if !compat.local(meta.ID){t.Fatal("managed process unexpectedly routed to broker")}
 deadline:=time.Now().Add(2*time.Second)
 for time.Now().Before(deadline){
  state,ok:=compat.Metadata(meta.ID)
  if ok&&state.Status=="exited"{break}
  time.Sleep(15*time.Millisecond)
 }
 var total strings.Builder
 var cursor int64
 for i:=0;i<10;i++{
  window,err:=compat.ReadOutputWindow(meta.ID,cursor,128)
  if err!=nil{t.Fatal(err)}
  if window.Offset!=cursor||window.Next<cursor{t.Fatal("lost output cursor")}
  total.Write(window.Data)
  cursor=window.Next
  if cursor==window.Total{break}
 }
 if !strings.Contains(total.String(),"OFFSET_PROOF"){t.Fatal("local output lost task stdout")}
 if _,err:=compat.ReadOutputWindow("unknown-broker-session",0,128);err==nil{
  t.Fatal("remote output API crossed into unaudited broker session")
 }
}
