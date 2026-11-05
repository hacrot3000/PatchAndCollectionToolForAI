package server

import (
 "context"
 "encoding/json"
 "os"
 "path/filepath"
 "reflect"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
 "bletonfc/vscode_tasks_menu/internal/tasks"
)

func TestRemoteTaskChildEnvironmentDoesNotExposePrivateDaemonHomeOrPATH(t *testing.T) {
 t.Setenv("HOME","/private/taskdeck-identity-and-keys")
 t.Setenv("PATH","/private/attacker-bin:/bin")
 t.Setenv("LANG","PRIVATE_DAEMON_SETTING")
 expected:=[]string{"PATH=/usr/bin:/bin","HOME=/nonexistent","LANG=C","LC_ALL=C","TERM=dumb"}
 if got:=isolatedRemoteTaskEnvironment();!reflect.DeepEqual(got,expected) {
  t.Fatalf("remote child environment inherited daemon values: %q",got)
 }
}

func TestRemoteTaskCannotRunOwnerApprovedWorkspaceExecutable(t *testing.T) {
 t.Setenv("TASKDECK_REMOTE_TASK_RUN","1")
 s,principal,client,agent:=remoteDispatchFixtureWithRole(t,remotetransport.RoleOperator)
 executable:=filepath.Join(s.Workspace,"remote-mutable-executable")
 marker:=filepath.Join(s.Workspace,"execution-marker")
 contents:="#!/bin/sh\necho attacked > "+marker+"\n"
 if err:=os.WriteFile(executable,[]byte(contents),0700);err!=nil{t.Fatal(err)}
 body,err:=json.Marshal(map[string]any{
  "version":"2.0.0",
  "tasks":[]map[string]any{{
   "label":"workspace-controlled process",
   "type":"process",
   "command":executable,
   "taskdeckRemoteAllowed":true,
  }},
 })
 if err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(s.Workspace,".vscode","tasks.json"),body,0600);err!=nil{t.Fatal(err)}
 loaded,err:=tasks.Load(s.Workspace)
 if err!=nil||len(loaded)!=1{t.Fatalf("invalid task fixture: %v",err)}
 approved,err:=tasks.RemoteApprovalToken(loaded[0])
 if err!=nil{t.Fatal(err)}
 t.Setenv("TASKDECK_REMOTE_APPROVED_TASKS",approved)
 req,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionTaskRun,
  TaskID:loaded[0].ID,
 })
 if err!=nil{t.Fatal(err)}
 if response,err:=s.executeRemoteE2EEPing(context.Background(),principal,agent,req);
  err==nil||len(response)!=0 {
  t.Fatal("remote actor ran a mutable workspace executable despite full task SHA approval")
 }
 if _,err:=os.Stat(marker);!os.IsNotExist(err) {
  t.Fatalf("workspace executable was invoked: %v",err)
 }
}
