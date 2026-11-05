package server

import (
 "context"
 "encoding/json"
 "os"
 "path/filepath"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
 "bletonfc/vscode_tasks_menu/internal/session"
 "bletonfc/vscode_tasks_menu/internal/tasks"
)

func prepareRemoteReportFailureTask(t *testing.T,s *Server)int{
 t.Helper()
 payload:=`{"version":"2.0.0","tasks":[
  {"label":"safe exit failure","type":"process","command":"/bin/false",
   "taskdeckRemoteAllowed":true}]}`
 if err:=os.WriteFile(filepath.Join(s.Workspace,".vscode","tasks.json"),[]byte(payload),0600);err!=nil{t.Fatal(err)}
 items,err:=tasks.Load(s.Workspace)
 if err!=nil||len(items)!=1{t.Fatalf("invalid task fixture: %v",err)}
 token,err:=tasks.RemoteApprovalToken(items[0])
 if err!=nil{t.Fatal(err)}
 t.Setenv("TASKDECK_REMOTE_APPROVED_TASKS",token)
 return items[0].ID
}

func TestTaskReportE2EEContainsRealProcessExitCodeSuccessAndFailure(t *testing.T){
 for _,tc:=range []struct{name string;exit int}{
  {"success",0},{"failure",1},
 }{
  t.Run(tc.name,func(t *testing.T){
   t.Setenv("TASKDECK_REMOTE_TASK_RUN","1")
   s,owner,client,agent:=remoteDispatchFixtureWithRole(t,remotetransport.RoleOperator)
   taskID:=0
   if tc.exit==0{taskID,_=writeRemoteTaskFixtures(t,s)}else{
    taskID=prepareRemoteReportFailureTask(t,s)
   }
   packet,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
    RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionTaskRunReport,
    TaskID:taskID,
   })
   if err!=nil{t.Fatal(err)}
   reply,err:=s.executeRemoteE2EEPing(context.Background(),owner,agent,packet)
   if err!=nil{t.Fatalf("approved short task was denied: %v",err)}
   report,err:=remotetransport.OpenRemoteTaskReportResult(client,reply,remoteDispatchTestID)
   if err!=nil||report.ExitCode!=tc.exit||report.Status!="exited"{
    t.Fatalf("wrong authoritative host exit status: %+v %v",report,err)
   }
   if tc.exit==0&&!strings.Contains(report.Output,"TASKDECK_APPROVED_TASK_PASS"){
    t.Fatal("missing authenticated task stdout")
   }
   if _,err:=s.executeRemoteE2EEPing(context.Background(),owner,agent,packet);err==nil{
    t.Fatal("replayed task execution")
   }
  })
 }
}

func TestTaskReportDeniedWithoutHostOptInOrTaskDeckPrincipal(t *testing.T){
 for _,tc:=range []struct{name string;flag string;role remotetransport.Role;noPrincipal bool}{
  {"no-host-optin","",remotetransport.RoleOperator,false},
  {"viewer","1",remotetransport.RoleViewer,false},
  {"php-only","1",remotetransport.RoleOperator,true},
 }{
  t.Run(tc.name,func(t *testing.T){
   t.Setenv("TASKDECK_REMOTE_TASK_RUN",tc.flag)
   s,owner,client,agent:=remoteDispatchFixtureWithRole(t,tc.role)
   taskID,_:=writeRemoteTaskFixtures(t,s)
   packet,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
    RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionTaskRunReport,
    TaskID:taskID,
   })
   if err!=nil{t.Fatal(err)}
   if tc.noPrincipal{owner=nil}
   reply,err:=s.executeRemoteE2EEPing(context.Background(),owner,agent,packet)
   if err==nil||len(reply)!=0{t.Fatal("unauthorized task report executed or leaked status")}
  })
 }
}

func TestTaskReportRejectsApprovalChangedAndInjectedCommand(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASK_RUN","1")
 s,owner,client,agent:=remoteDispatchFixtureWithRole(t,remotetransport.RoleOperator)
 taskID,_:=writeRemoteTaskFixtures(t,s)
 path:=filepath.Join(s.Workspace,".vscode","tasks.json")
 original,err:=os.ReadFile(path)
 if err!=nil{t.Fatal(err)}
 var tasksJSON map[string]any
 if err:=json.Unmarshal(original,&tasksJSON);err!=nil{t.Fatal(err)}
 taskList:=tasksJSON["tasks"].([]any)
 taskList[0].(map[string]any)["command"]="/bin/false"
 changed,err:=json.Marshal(tasksJSON)
 if err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(path,changed,0600);err!=nil{t.Fatal(err)}
 frame,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionTaskRunReport,
  TaskID:taskID,
 })
 if err!=nil{t.Fatal(err)}
 reply,err:=s.executeRemoteE2EEPing(context.Background(),owner,agent,frame)
 if err==nil||len(reply)!=0{t.Fatal("modified host task bypassed full task approval digest")}
}


func TestTaskReportOversizedOrEvictedLogKeepsVerifiedExitWithoutPartialText(t *testing.T){
 for _,tc:=range []struct{name string;scrollback int;payload int}{
  {"oversized",0,remotetransport.RemoteTaskReportMaxOutput+300},
  {"evicted",128,900},
 }{
  t.Run(tc.name,func(t *testing.T){
   t.Setenv("TASKDECK_REMOTE_TASK_RUN","1")
   s,owner,client,agent:=remoteDispatchFixtureWithRole(t,remotetransport.RoleOperator)
   if tc.scrollback>0{s.Sessions=session.NewManager(tc.scrollback)}
   taskID:=managedTaskFixture(t,s,"/bin/echo",[]string{strings.Repeat("L",tc.payload)})
   req,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
    RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionTaskRunReport,
    TaskID:taskID,
   })
   if err!=nil{t.Fatal(err)}
   encrypted,err:=s.executeRemoteE2EEPing(context.Background(),owner,agent,req)
   if err!=nil{t.Fatalf("approved report should preserve host exit: %v",err)}
   result,err:=remotetransport.OpenRemoteTaskReportResult(client,encrypted,remoteDispatchTestID)
   if err!=nil||result.Status!="exited"||result.ExitCode!=0||
    !result.OutputOmitted||result.Output!=""{
    t.Fatalf("sync Run incorrectly lost/trimmed exit metadata: %+v %v",result,err)
   }
   if _,err:=s.executeRemoteE2EEPing(context.Background(),owner,agent,req);err==nil{
    t.Fatal("run request replayed after incomplete output")
   }
  })
 }
}
