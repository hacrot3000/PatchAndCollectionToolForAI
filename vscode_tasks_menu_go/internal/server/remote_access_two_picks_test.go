package server

import (
 "context"
 "os"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
 "bletonfc/vscode_tasks_menu/internal/tasks"
)

const remoteTwoPickSource="{\"inputs\":[{\"id\":\"region\",\"type\":\"pickString\",\"options\":[\"us\",\"eu\"]},{\"id\":\"mode\",\"type\":\"pickString\",\"options\":[\"debug\",\"release\",\"preview\"]}],\"tasks\":[{\"label\":\"Two picks\",\"type\":\"process\",\"command\":\"/bin/echo\",\"args\":[\"${input:region}\",\"${input:mode}\"]}]}"

func TestRemoteTwoPickSnapshotAndRowMajorSelection(t *testing.T){
 workspace,_,list,digest:=remoteTaskSnapshotFixture(t,remoteTwoPickSource)
 if len(list)!=1{t.Fatal("expected one task")}
 inspection,err:=inspectRemoteTaskSelection(context.Background(),workspace,digest,list[0].ID)
 if err!=nil||inspection.InputID!="region"||inspection.InputID2!="mode"||
  len(inspection.Choices)!=2||len(inspection.Choices2)!=3{
  t.Fatalf("missing second host-owned input metadata: %+v %v",inspection,err)
 }
 spec,_,err:=prepareRemoteTaskExecutionChoice(context.Background(),workspace,digest,list[0].ID,5,false)
 if err!=nil||len(spec.Args)!=2||spec.Args[0]!="eu"||spec.Args[1]!="release"{
  t.Fatalf("combined selection 5 did not resolve both pickString inputs: %+v %v",spec,err)
 }
 for _,choice:=range []int{0,7,36}{
  if _,_,err:=prepareRemoteTaskExecutionChoice(context.Background(),workspace,digest,list[0].ID,choice,false);err==nil{
   t.Fatalf("out-of-range two-input selection %d accepted",choice)
  }
 }
}

func TestRemoteTwoPickAuthenticatedE2EERun(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_RUN","1")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 id:=previewOneRemoteRunTask(t,s,session,client,agent,remoteTwoPickSource)
 inspectID:="33333333333333333333333333333333"
 request,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:inspectID,Operation:remotetransport.RemoteActionTasksInspect,TaskID:id,
 })
 if err!=nil{t.Fatal(err)}
 frame,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,request)
 if err!=nil{t.Fatal(err)}
 info,err:=remotetransport.OpenRemoteActionTasksInspectResult(client,frame,inspectID,id)
 if err!=nil||info.InputID2!="mode"||len(info.Choices2)!=3{
  t.Fatalf("second pick metadata lost in E2EE reply: %+v %v",info,err)
 }
 runID:="44444444444444444444444444444444"
 run,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:runID,Operation:remotetransport.RemoteActionTasksRun,TaskID:id,Choice:5,
 })
 if err!=nil{t.Fatal(err)}
 resultFrame,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,run)
 if err!=nil{t.Fatal(err)}
 result,err:=remotetransport.OpenRemoteActionTaskRunResult(client,resultFrame,runID,id)
 if err!=nil||result.ExitCode!=0||result.Output!="eu release\n"{
  t.Fatalf("host returned wrong two-input command output: %+v %v",result,err)
 }
}

func TestRemoteTwoPickRejectsChangedSecondInputBeforeRun(t *testing.T){
 workspace,path,list,digest:=remoteTaskSnapshotFixture(t,remoteTwoPickSource)
 if len(list)!=1{t.Fatal("task fixture invalid")}
 original,err:=os.ReadFile(path)
 if err!=nil{t.Fatal(err)}
 changed:=strings.Replace(string(original),"preview","modified",1)
 if changed==string(original){t.Fatal("second choices were not changed")}
 if err:=os.WriteFile(path,[]byte(changed),0600);err!=nil{t.Fatal(err)}
 reloaded,err:=tasks.Load(workspace)
 if err!=nil||len(reloaded)!=1||reloaded[0].ID!=list[0].ID{
  t.Fatalf("test no longer checks global input source commitment: %v",err)
 }
 if _,_,err:=prepareRemoteTaskExecutionChoice(context.Background(),workspace,digest,list[0].ID,5,false);err==nil{
  t.Fatal("stale second-input definition reused old authenticated selection")
 }
}
