//go:build linux || darwin

package server

import (
 "context"
 "os"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
 "bletonfc/vscode_tasks_menu/internal/tasks"
)

const remotePromptStringFixture = "{\"inputs\":[{\"id\":\"version\",\"type\":\"promptString\",\"description\":\"Version\"}],\"tasks\":[{\"label\":\"Print version\",\"type\":\"process\",\"command\":\"/usr/bin/printf\",\"args\":[\"%s\",\"${input:version}\"]}]}"

func TestRemoteTaskPromptStringAuthenticatedRun(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_RUN","1")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 taskID:=previewOneRemoteRunTask(t,s,session,client,agent,remotePromptStringFixture)
 inspectID:="33333333333333333333333333333333"
 request,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:inspectID,Operation:remotetransport.RemoteActionTasksInspect,TaskID:taskID,
 })
 if err!=nil{t.Fatal(err)}
 frame,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,request)
 if err!=nil{t.Fatal(err)}
 info,err:=remotetransport.OpenRemoteActionTasksInspectResult(client,frame,inspectID,taskID)
 if err!=nil||info.InputID!="version"||info.InputType!="promptString"||
  len(info.Choices)!=0{
  t.Fatalf("prompt input metadata not authenticated: %+v %v",info,err)
 }
 runID:="44444444444444444444444444444444"
 run,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:runID,Operation:remotetransport.RemoteActionTasksRun,
  TaskID:taskID,Text:"release-1.2",
 })
 if err!=nil{t.Fatal(err)}
 resultFrame,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,run)
 if err!=nil{t.Fatal(err)}
 result,err:=remotetransport.OpenRemoteActionTaskRunResult(client,resultFrame,runID,taskID)
 if err!=nil||result.ExitCode!=0||result.Output!="release-1.2"{
  t.Fatalf("authenticated process argv substitution failed: %+v %v",result,err)
 }
}

func TestRemoteTaskPromptStringSourceDigestAndInputValidation(t *testing.T){
 workspace,path,list,digest:=remoteTaskSnapshotFixture(t,remotePromptStringFixture)
 if len(list)!=1{t.Fatal("expected host task")}
 candidate,_,err:=prepareRemoteTaskExecutionInput(context.Background(),
  workspace,digest,list[0].ID,0,false,"release-1.2")
 if err!=nil||len(candidate.Args)!=2||candidate.Args[1]!="release-1.2"{
  t.Fatalf("verified process argv was not prepared: %+v %v",candidate,err)
 }
 for _,value:=range []string{"bad;rm","../../etc","-rf","two words"}{
  if _,_,err:=prepareRemoteTaskExecutionInput(context.Background(),
   workspace,digest,list[0].ID,0,false,value);err==nil{
   t.Fatalf("untrusted free-form shell value was accepted: %q",value)
  }
 }
 before,err:=os.ReadFile(path)
 if err!=nil{t.Fatal(err)}
 after:=strings.Replace(string(before),"Version","Revised Version",1)
 if err:=os.WriteFile(path,[]byte(after),0600);err!=nil{t.Fatal(err)}
 changed,err:=tasks.Load(workspace)
 if err!=nil||len(changed)!=1||changed[0].ID!=list[0].ID{
  t.Fatalf("test expected unchanged stable ID after global input edit: %v",err)
 }
 if _,_,err:=prepareRemoteTaskExecutionInput(context.Background(),
  workspace,digest,list[0].ID,0,false,"release-1.2");err==nil{
  t.Fatal("changed global inputs bypassed host snapshot verification")
 }
}

func TestRemoteTaskPromptStringRejectsExecutableShellEnvAndMultipleInputs(t *testing.T){
 for _,tc:=range []struct{name,source string}{
  {"shell type","{\"inputs\":[{\"id\":\"version\",\"type\":\"promptString\",\"description\":\"Version\"}],\"tasks\":[{\"label\":\"Unsafe shell\",\"type\":\"shell\",\"command\":\"echo ${input:version}\"}]}"},
  {"shell executable","{\"inputs\":[{\"id\":\"version\",\"type\":\"promptString\",\"description\":\"Version\"}],\"tasks\":[{\"label\":\"Unsafe sh\",\"type\":\"process\",\"command\":\"/bin/sh\",\"args\":[\"-c\",\"echo ${input:version}\"]}]}"},
  {"input command","{\"inputs\":[{\"id\":\"version\",\"type\":\"promptString\",\"description\":\"Version\"}],\"tasks\":[{\"label\":\"Unsafe command\",\"type\":\"process\",\"command\":\"${input:version}\",\"args\":[\"ok\"]}]}"},
  {"input cwd","{\"inputs\":[{\"id\":\"version\",\"type\":\"promptString\",\"description\":\"Version\"}],\"tasks\":[{\"label\":\"Unsafe cwd\",\"type\":\"process\",\"command\":\"/usr/bin/printf\",\"args\":[\"ok\"],\"options\":{\"cwd\":\"${input:version}\"}}]}"},
  {"input env","{\"inputs\":[{\"id\":\"version\",\"type\":\"promptString\",\"description\":\"Version\"}],\"tasks\":[{\"label\":\"Unsafe env\",\"type\":\"process\",\"command\":\"/usr/bin/printf\",\"args\":[\"ok\"],\"options\":{\"env\":{\"LD_PRELOAD\":\"${input:version}\"}}}]}"},
  {"two inputs","{\"inputs\":[{\"id\":\"version\",\"type\":\"promptString\",\"description\":\"Version\"},{\"id\":\"target\",\"type\":\"promptString\"}],\"tasks\":[{\"label\":\"Unsafe multi\",\"type\":\"process\",\"command\":\"/usr/bin/printf\",\"args\":[\"${input:version}\",\"${input:target}\"]}]}"},
 }{
  t.Run(tc.name,func(t *testing.T){
   workspace,_,list,digest:=remoteTaskSnapshotFixture(t,tc.source)
   if len(list)!=1{t.Fatal("expected task")}
   if _,_,err:=prepareRemoteTaskExecutionInput(context.Background(),
    workspace,digest,list[0].ID,0,false,"release");err==nil{
    t.Fatal("unreviewed dynamic prompt task was remotely executable")
   }
  })
 }
}
