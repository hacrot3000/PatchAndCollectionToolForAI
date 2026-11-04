package server

import (
 "context"
 "crypto/ed25519"
 "crypto/rand"
 "crypto/sha256"
 "encoding/hex"
 "fmt"
 "os"
 "path/filepath"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/config"
 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
 "bletonfc/vscode_tasks_menu/internal/tasks"
)

// A real operator-signed Portal ticket and approved native client-key are
// required. Never fake elevation by changing session.role after login.
func remoteTaskOperatorFixture(t *testing.T)(*Server,*remoteTaskDeckSession,*remotetransport.Channel,*remotetransport.Channel){
 t.Helper()
 pub,priv,err:=ed25519.GenerateKey(rand.Reader)
 if err!=nil{t.Fatal(err)}
 fingerprint:=sha256.Sum256(pub)
 device:=hex.EncodeToString(fingerprint[:])
 browser,err:=remotetransport.NewEphemeral()
 if err!=nil{t.Fatal(err)}
 hello,agent,err:=remotetransport.ClientHello(priv,device,
  strings.Repeat("a",32),strings.Repeat("b",32),browser.Public)
 if err!=nil{t.Fatal(err)}
 client,err:=remotetransport.AcceptHello(browser,device,
  strings.Repeat("a",32),strings.Repeat("b",32),hello)
 if err!=nil{t.Fatal(err)}
 first,err:=client.Seal([]byte("authenticated-client-possession"))
 if err!=nil{t.Fatal(err)}
 if _,err:=agent.Open(first);err!=nil{t.Fatal(err)}
 s:=sharedLoginTestServer(t)
 s.Config.RemoteAccessEnabled=true
 s.Config.Protocol=config.ProtocolHTTPS
 grant:=testRemoteSignedGrant(t,agent,remotetransport.RoleOperator)
 session,err:=s.authenticateRemoteTaskDeckCredentials(context.Background(),agent,grant,
  "alice","private-admin-password")
 if err!=nil{t.Fatal(err)}
 bindTestRemoteOwnerClient(t,s,session)
 t.Cleanup(session.Close)
 t.Cleanup(client.Close)
 allowTestLivePortal(session)
 return s,session,client,agent
}

func previewOneRemoteRunTask(t *testing.T,s *Server,session *remoteTaskDeckSession,
 client,agent *remotetransport.Channel,source string) int{
 t.Helper()
 writeRemoteTasksPreviewFixture(t,s.Workspace,source)
 list,err:=tasks.Load(s.Workspace)
 if err!=nil||len(list)!=1{t.Fatalf("invalid task fixture: %v",err)}
 page,err:=remoteTaskMenuAction(t,s,session,client,agent,
  "11111111111111111111111111111111",remotetransport.RemoteActionTasksPreview)
 if err!=nil||len(page.IDs)!=1||page.IDs[0]!=list[0].ID{
  t.Fatalf("host never published task ID: %+v %v",page,err)
 }
 return list[0].ID
}

func attemptRemoteRunTask(t *testing.T,s *Server,session *remoteTaskDeckSession,
 client,agent *remotetransport.Channel,taskID int)(remotetransport.RemoteTaskRun,error){
 t.Helper()
 id:="22222222222222222222222222222222"
 req,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:id,Operation:remotetransport.RemoteActionTasksRun,TaskID:taskID,
 })
 if err!=nil{t.Fatal(err)}
 result,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req)
 if err!=nil{return remotetransport.RemoteTaskRun{},err}
 return remotetransport.OpenRemoteActionTaskRunResult(client,result,id,taskID)
}
func TestRemoteTaskRunOperatorSignedE2EEAndAudit(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_RUN","1")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 taskID:=previewOneRemoteRunTask(t,s,session,client,agent,
  `{"tasks":[{"label":"Short","type":"process","command":"/bin/sh","args":["-c","printf safe-output; exit 7"]}]}`)
 result,err:=attemptRemoteRunTask(t,s,session,client,agent,taskID)
 if err!=nil||result.TaskID!=taskID||result.ExitCode!=7||result.Output!="safe-output"||result.Truncated{
  t.Fatalf("short host task did not return bounded E2EE exit/output: %+v %v",result,err)
 }
 project,err:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
 if err!=nil{t.Fatal(err)}
 events,err:=s.Identity.ListAudit(context.Background(),identity.AuditQuery{
  ProjectID:project.ID,Action:"remote.action.tasks_run",Limit:5,
 })
 if err!=nil||len(events)!=1||events[0].Result!="attempted"||
  events[0].UserID==nil||*events[0].UserID!=session.userID||
  strings.Contains(events[0].Details,"safe-output"){
  t.Fatalf("host-local audit missing or exposed output: %+v %v",events,err)
 }
}
func TestRemoteTaskRunViewerOrOptOutCannotExecute(t *testing.T){
 for _,tc:=range []struct{name string;operator,enabled bool}{
  {"viewer",false,true},{"operator opt-out",true,false},
 }{
  t.Run(tc.name,func(t *testing.T){
   t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
   if tc.enabled{t.Setenv("TASKDECK_REMOTE_TASKS_RUN","1")}else{
    t.Setenv("TASKDECK_REMOTE_TASKS_RUN","0")
   }
   var s *Server
   var session *remoteTaskDeckSession
   var client,agent *remotetransport.Channel
   if tc.operator{
    s,session,client,agent=remoteTaskOperatorFixture(t)
   }else{s,session,client,agent=remoteDispatchFixture(t)}
   sentinel:=filepath.Join(s.Workspace,"should-not-exist")
   taskID:=previewOneRemoteRunTask(t,s,session,client,agent,
    `{"tasks":[{"label":"Denied","type":"process","command":"/bin/sh","args":["-c","touch should-not-exist"]}]}`)
   if _,err:=attemptRemoteRunTask(t,s,session,client,agent,taskID);err==nil{
    t.Fatal("denied task execution bypassed Portal role or host opt-in")
   }
   if _,err:=os.Stat(sentinel);!os.IsNotExist(err){
    t.Fatalf("a denied task made a filesystem change: %v",err)
   }
  })
 }
}
func TestRemoteTaskRunRefusesStaleSnapshotBeforeSideEffects(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_RUN","1")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 taskID:=previewOneRemoteRunTask(t,s,session,client,agent,
  `{"tasks":[{"label":"Original","type":"process","command":"/bin/sh","args":["-c","touch modified-file"]}]}`)
 source:=filepath.Join(s.Workspace,".vscode","tasks.json")
 data,err:=os.ReadFile(source)
 if err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(source,append(data,' '),0600);err!=nil{t.Fatal(err)}
 if _,err:=attemptRemoteRunTask(t,s,session,client,agent,taskID);err==nil{
  t.Fatal("stale published task was executed after source changed")
 }
 if _,err:=os.Stat(filepath.Join(s.Workspace,"modified-file"));!os.IsNotExist(err){
  t.Fatal("stale task created a file")
 }
}
func TestRemoteTaskCaptureSanitizesAndBoundsOutput(t *testing.T){
 cap:=&remoteTaskCapture{}
 large:=append([]byte("hello \x1b[31m world\n"),[]byte(strings.Repeat("v",4096))...)
 n,err:=cap.Write(large)
 if err!=nil||n!=len(large){t.Fatal("bounded capture must always drain")}
 output,truncated:=cap.snapshot()
 if !truncated||strings.ContainsRune(output,'\x1b')||len(output)>remotetransport.MaxRemoteTaskOutputBytes{
  t.Fatalf("remote output was not bounded/sanitized: %q",output)
 }
}

const remotePickTaskFixture = `{
 "inputs":[{"id":"target","type":"pickString","description":"Target","options":["gateway","main"]}],
 "tasks":[{"label":"Pick","type":"process","command":"/bin/echo","args":["${input:target}"]}]
}`

func TestRemoteTaskOnePickInputAuthenticatedAndAudited(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_RUN","1")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 taskID:=previewOneRemoteRunTask(t,s,session,client,agent,remotePickTaskFixture)
 inspectID:="33333333333333333333333333333333"
 frame,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:inspectID,Operation:remotetransport.RemoteActionTasksInspect,TaskID:taskID,
 })
 if err!=nil{t.Fatal(err)}
 result,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,frame)
 if err!=nil{t.Fatal(err)}
 info,err:=remotetransport.OpenRemoteActionTasksInspectResult(client,result,inspectID,taskID)
 if err!=nil||info.InputID!="target"||len(info.Choices)!=2||
  info.Choices[1]!="main"{
  t.Fatalf("inspect leaked or omitted fixed host choices: %+v %v",info,err)
 }
 runID:="44444444444444444444444444444444"
 run,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:runID,Operation:remotetransport.RemoteActionTasksRun,
  TaskID:taskID,Choice:2,
 })
 if err!=nil{t.Fatal(err)}
 sealed,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,run)
 if err!=nil{t.Fatal(err)}
 got,err:=remotetransport.OpenRemoteActionTaskRunResult(client,sealed,runID,taskID)
 if err!=nil||got.ExitCode!=0||got.Output!="main\n"{
  t.Fatalf("host did not substitute approved fixed pick: %+v %v",got,err)
 }
}
func TestRemoteTaskChoiceRejectedWhenMissingOrOutsideOptions(t *testing.T){
 for _,choice:=range []int{0,3}{
  t.Run(fmt.Sprintf("choice-%d",choice),func(t *testing.T){
   t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
   t.Setenv("TASKDECK_REMOTE_TASKS_RUN","1")
   s,session,client,agent:=remoteTaskOperatorFixture(t)
   taskID:=previewOneRemoteRunTask(t,s,session,client,agent,remotePickTaskFixture)
   request,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
    RequestID:"44444444444444444444444444444444",
    Operation:remotetransport.RemoteActionTasksRun,TaskID:taskID,Choice:choice,
   })
   if err!=nil{t.Fatal(err)}
   if frame,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,request);
    err==nil||len(frame)!=0{
    t.Fatal("host accepted missing/out-of-range fixed-choice selection")
   }
  })
 }
}

func TestRemoteTaskChoiceRejectsChangedGlobalInputsWithoutTaskIDChange(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_RUN","1")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 source:="{\"inputs\":[{\"id\":\"target\",\"type\":\"pickString\",\"options\":[\"safe-a\",\"safe-b\"]}],\"tasks\":[{\"label\":\"Deploy\",\"type\":\"process\",\"command\":\"/bin/sh\",\"args\":[\"-c\",\"touch ${input:target}\"]}]}"
 taskID:=previewOneRemoteRunTask(t,s,session,client,agent,source)
 inspectionID:="33333333333333333333333333333333"
 request,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:inspectionID,Operation:remotetransport.RemoteActionTasksInspect,
  TaskID:taskID,
 })
 if err!=nil{t.Fatal(err)}
 frame,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,request)
 if err!=nil{t.Fatal(err)}
 info,err:=remotetransport.OpenRemoteActionTasksInspectResult(
  client,frame,inspectionID,taskID)
 if err!=nil||len(info.Choices)!=2{t.Fatalf("choice inspection failed: %+v %v",info,err)}
 updated:=strings.Replace(source,"safe-a","changed-value",1)
 if updated==source{t.Fatal("fixture never changed input value")}
 writeRemoteTasksPreviewFixture(t,s.Workspace,updated)
 // Global input definitions do not change stable Task.ID, so the host
 // must reject the byte-level snapshot change independently.
 list,err:=tasks.Load(s.Workspace)
 if err!=nil||len(list)!=1||list[0].ID!=taskID{
  t.Fatalf("test no longer proves stable ID is insufficient: %v",err)
 }
 runID:="44444444444444444444444444444444"
 run,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:runID,Operation:remotetransport.RemoteActionTasksRun,
  TaskID:taskID,Choice:1,
 })
 if err!=nil{t.Fatal(err)}
 response,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,run)
 if err==nil||len(response)!=0{
  t.Fatal("changed global pickString definitions reused an authenticated choice")
 }
 for _,name:=range []string{"safe-a","safe-b","changed-value"} {
  if _,err:=os.Stat(filepath.Join(s.Workspace,name));!os.IsNotExist(err){
   t.Fatalf("stale fixed-choice execution modified workspace (%s): %v",name,err)
  }
 }
}
