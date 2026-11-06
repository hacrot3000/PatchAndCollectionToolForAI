package server

import (
 "context"
 "os"
 "path/filepath"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
 "bletonfc/vscode_tasks_menu/internal/tasks"
)

const remoteWorkflowFixture=`{"tasks":[
 {"label":"Compile","type":"process","command":"/bin/echo","args":["compile"]},
 {"label":"Test","type":"process","command":"/bin/echo","args":["test"],"dependsOn":"Compile"},
 {"label":"Release","type":"process","command":"/bin/echo","args":["release"],"dependsOn":["Test"]}
]}`

func TestRemoteTaskDependenciesHostSnapshotAndE2EE(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_DEPS","1")
 s,session,client,agent:=remoteDispatchFixture(t)
 writeRemoteTasksPreviewFixture(t,s.Workspace,remoteWorkflowFixture)
 list,err:=tasks.Load(s.Workspace)
 if err!=nil||len(list)!=3{t.Fatalf("invalid task fixture: %v",err)}
 var root int
 ids:=map[string]int{}
 for _,item:=range list{
  ids[item.Label]=item.ID
  if item.Label=="Release"{root=item.ID}
 }
 if root==0{t.Fatal("no stable root Task.ID")}
 if _,err:=s.readRemoteTaskDependencies(context.Background(),session,root);err==nil{
  t.Fatal("root Task.ID guessed before authenticated preview")
 }
 preview,err:=remoteTaskMenuAction(t,s,session,client,agent,
  remoteDispatchTestID,remotetransport.RemoteActionTasksPreview)
 if err!=nil{t.Fatal(err)}
 issued:=false
 for _,id:=range preview.IDs{if id==root{issued=true}}
 if !issued{t.Fatalf("missing E2EE-issued root id: %+v",preview)}
 result,err:=s.readRemoteTaskDependencies(context.Background(),session,root)
 if err!=nil||!result.Available||len(result.Order)!=3||len(result.Edges)!=2||
  result.Order[0]!=ids["Compile"]||result.Order[1]!=ids["Test"]||
  result.Order[2]!=ids["Release"]{
  t.Fatalf("invalid host topological plan: %+v %v",result,err)
 }
 req,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionTasksDependencies,
  TaskID:root,
 })
 if err!=nil{t.Fatal(err)}
 reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req)
 if err!=nil{t.Fatal(err)}
 decoded,err:=remotetransport.OpenRemoteTaskDependencies(client,reply,remoteDispatchTestID,root)
 if err!=nil||len(decoded.Order)!=3||len(decoded.Edges)!=2{
  t.Fatalf("E2EE graph incorrectly decoded: %+v %v",decoded,err)
 }
 project,err:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
 if err!=nil{t.Fatal(err)}
 audits,err:=s.Identity.ListAudit(context.Background(),identity.AuditQuery{
  ProjectID:project.ID,UserID:session.userID,
  Action:"remote.action.tasks_dependencies",Limit:3,
 })
 if err!=nil||len(audits)!=1||audits[0].Result!="authorized"||
  audits[0].ResourceType!="tasks"{
  t.Fatalf("principal-authenticated read audit missing: %+v %v",audits,err)
 }
 // Still no remote workflow execution: the old one-task preflight and
 // tasks.run path continue to reject ANY dependsOn definition.
 if _,_,err:=prepareRemoteTaskExecutionInput(context.Background(),
  s.Workspace,session.taskPageHash,root,0,false,"");err==nil{
  t.Fatal("dependency inspection escalated into workflow task execution")
 }
 // Reuse of a published Task.ID after source mutation is forbidden.
 if err:=os.WriteFile(filepath.Join(s.Workspace,".vscode","tasks.json"),
  []byte(remoteWorkflowFixture+" "),0600);err!=nil{t.Fatal(err)}
 if _,err:=s.readRemoteTaskDependencies(context.Background(),session,root);
  err==nil{t.Fatal("stale task source admitted dependency graph")}
}

func TestRemoteTaskDependenciesOptOutDoesNotPoisonReadSession(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_DEPS","0")
 s,session,client,agent:=remoteDispatchFixture(t)
 writeRemoteTasksPreviewFixture(t,s.Workspace,remoteWorkflowFixture)
 page,err:=remoteTaskMenuAction(t,s,session,client,agent,
  remoteDispatchTestID,remotetransport.RemoteActionTasksPreview)
 if err!=nil{t.Fatal(err)}
 if len(page.IDs)==0{t.Fatal("empty preview")}
 id:=page.IDs[0]
 frame,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionTasksDependencies,
  TaskID:id,
 })
 if err!=nil{t.Fatal(err)}
 reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,frame)
 if err!=nil{t.Fatalf("disabled feature should reply without leaking graph: %v",err)}
 result,err:=remotetransport.OpenRemoteTaskDependencies(client,reply,remoteDispatchTestID,id)
 if err!=nil||result.Available||len(result.Order)!=0||len(result.Edges)!=0{
  t.Fatalf("host disabled dependencies but sent data: %+v %v",result,err)
 }
 if !client.Established()||!agent.Established(){t.Fatal("read-only optout poisoned session")}
}

func TestRemoteTaskDependenciesCyclesAndUnissuedIDsFailClosed(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_DEPS","1")
 t.Run("unissued",func(t *testing.T){
  s,session,_,_:=remoteDispatchFixture(t)
  writeRemoteTasksPreviewFixture(t,s.Workspace,remoteWorkflowFixture)
  list,err:=tasks.Load(s.Workspace)
  if err!=nil{t.Fatal(err)}
  for _,task:=range list{
   if _,err:=s.readRemoteTaskDependencies(context.Background(),session,task.ID);
    err==nil{t.Fatal("accepted unissued task dependency selection")}
  }
 })
 for name,fixture:=range map[string]string{
  "cycle":`{"tasks":[{"label":"A","dependsOn":"B"},{"label":"B","dependsOn":"A"}]}`,
  "missing":`{"tasks":[{"label":"A","dependsOn":"Absent"}]}`,
  "self":`{"tasks":[{"label":"A","dependsOn":"A"}]}`,
 }{
  t.Run(name,func(t *testing.T){
   s,session,client,agent:=remoteDispatchFixture(t)
   writeRemoteTasksPreviewFixture(t,s.Workspace,fixture)
   preview,err:=remoteTaskMenuAction(t,s,session,client,agent,
    remoteDispatchTestID,remotetransport.RemoteActionTasksPreview)
   if err!=nil||len(preview.IDs)==0{t.Fatalf("failed fixture preview: %+v %v",preview,err)}
   if _,err:=s.readRemoteTaskDependencies(context.Background(),session,preview.IDs[0]);
    err==nil{t.Fatal("invalid workflow graph accepted")}
  })
 }
}
