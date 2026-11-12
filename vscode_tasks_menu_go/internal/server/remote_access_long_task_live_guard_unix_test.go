//go:build linux || darwin

package server

import(
 "context"
 "errors"
 "os"
 "path/filepath"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func remoteLongTaskConsentFixture(t *testing.T,source string,
 setup ...func(*Server))(*Server,*remoteTaskDeckSession,int){
 t.Helper()
 t.Setenv("TASKDECK_REMOTE_TASKS_PREVIEW","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_RUN","1")
 t.Setenv("TASKDECK_REMOTE_TASKS_LONG","1")
 s,session,client,agent:=remoteTaskOperatorFixture(t)
 for _,configure:=range setup{configure(s)}
 id:=previewOneRemoteRunTask(t,s,session,client,agent,source)
 return s,session,id
}

func TestRemoteLongTaskPostAuditOptOutAndRoleDowngradeNeverLaunch(t *testing.T){
 for _,tc:=range []struct{name,flag string;role bool}{
  {"preview off","TASKDECK_REMOTE_TASKS_PREVIEW",false},
  {"run off","TASKDECK_REMOTE_TASKS_RUN",false},
  {"long off","TASKDECK_REMOTE_TASKS_LONG",false},
  {"TasksRun role downgraded","",true},
 }{
  t.Run(tc.name,func(t *testing.T){
   s,session,id:=remoteLongTaskConsentFixture(t,
    `{"tasks":[{"label":"Post audit long","type":"process",
     "command":"/bin/sh","args":["-c","touch FORBIDDEN-LONG"]}]}`)
   s.Identity=remoteShortTaskAuditRevoke{
    Store:s.Identity,flag:tc.flag,role:tc.role,session:session,
   }
   err:=s.startRemoteLongTask(context.Background(),session,id,0,"")
   if !errors.Is(err,remotetransport.ErrRemoteActionDenied){
    t.Fatalf("long Task started after host consent revoked: %v",err)
   }
   if session.longTask!=nil{t.Fatal("revoked job leaked an executable slot")}
   if _,err:=os.Stat(filepath.Join(s.Workspace,"FORBIDDEN-LONG"));
    !os.IsNotExist(err){t.Fatalf("long Task executed after audit: %v",err)}
  })
 }
}

func TestRemoteLongTaskPostAuditSourceAndCwdDriftNeverLaunch(t *testing.T){
 for _,tc:=range []string{"tasks.json modified","cwd symlink redirected"}{
  t.Run(tc,func(t *testing.T){
   source:=`{"tasks":[{"label":"Stale long","type":"process",
    "command":"/bin/sh","args":["-c","touch FORBIDDEN-LONG"]}]}`
   var setup func(*Server)
   if tc=="cwd symlink redirected"{
    source=`{"tasks":[{"label":"Stale long","type":"process",
     "command":"/bin/sh","args":["-c","touch FORBIDDEN-LONG"],
     "options":{"cwd":"working"}}]}`
    setup=func(s *Server){
     for _,dir:=range []string{"original-dir","new-dir"}{
      if err:=os.Mkdir(filepath.Join(s.Workspace,dir),0700);err!=nil{
       t.Fatal(err)
      }
     }
     if err:=os.Symlink("original-dir",
      filepath.Join(s.Workspace,"working"));err!=nil{t.Fatal(err)}
    }
   }
   var s *Server
   var session *remoteTaskDeckSession
   var id int
   if setup!=nil{
    s,session,id=remoteLongTaskConsentFixture(t,source,setup)
   }else{
    s,session,id=remoteLongTaskConsentFixture(t,source)
   }
   hook:=remoteShortTaskAuditDrift{Store:s.Identity}
   if tc=="tasks.json modified"{
    hook.filename=filepath.Join(s.Workspace,".vscode","tasks.json")
   }else{
    hook.symlink=filepath.Join(s.Workspace,"working")
    hook.target="new-dir"
   }
   s.Identity=hook
   err:=s.startRemoteLongTask(context.Background(),session,id,0,"")
   if !errors.Is(err,remotetransport.ErrRemoteActionDenied){
    t.Fatalf("long Task ignored post-audit drift: %v",err)
   }
   if session.longTask!=nil{t.Fatal("stale job occupied execution slot")}
   for _,dir:=range []string{"","original-dir","new-dir"}{
    if _,err:=os.Stat(filepath.Join(s.Workspace,dir,"FORBIDDEN-LONG"));
     !os.IsNotExist(err){
     t.Fatalf("stale process touched disk in %q: %v",dir,err)
    }
   }
  })
 }
}

func TestRemoteLongTaskActiveProcessStopsAfterHostOptOut(t *testing.T){
 s,session,id:=remoteLongTaskConsentFixture(t,
  `{"tasks":[{"label":"Active long","type":"process","command":"/bin/sh",
   "args":["-c","touch STARTED-LONG; sleep 8; touch FORBIDDEN-LONG"]}]}`)
 if err:=s.startRemoteLongTask(context.Background(),session,id,0,"");err!=nil{
  t.Fatalf("authorized long Task did not start: %v",err)
 }
 started:=filepath.Join(s.Workspace,"STARTED-LONG")
 deadline:=time.After(3*time.Second)
 for{
  if _,err:=os.Stat(started);err==nil{break}
  select{
  case <-session.longTask.done:t.Fatal("long job quit before first command")
  case <-deadline:t.Fatal("long process never began")
  case <-time.After(10*time.Millisecond):
  }
 }
 t.Setenv("TASKDECK_REMOTE_TASKS_LONG","0")
 select{
 case <-session.longTask.done:
 case <-time.After(3*time.Second):
  t.Fatal("revoked long Task was not stopped and reaped")
 }
 job:=session.longTask
 job.mu.Lock()
 status,code:=job.status,job.exitCode
 job.mu.Unlock()
 if status!="cancelled"||code!= -1{
  t.Fatalf("host opt-out falsely completed long Task: %q/%d",status,code)
 }
 if _,err:=os.Stat(filepath.Join(s.Workspace,"FORBIDDEN-LONG"));
  !os.IsNotExist(err){t.Fatalf("revoked long Task continued: %v",err)}
}

func TestRemoteLongTaskActiveProcessStopsAfterRoleDowngrade(t *testing.T){
 s,session,id:=remoteLongTaskConsentFixture(t,
  `{"tasks":[{"label":"Downgrade active","type":"process",
   "command":"/bin/sh",
   "args":["-c","touch STARTED-LONG; sleep 8; touch FORBIDDEN-LONG"]}]}`)
 if err:=s.startRemoteLongTask(context.Background(),session,id,0,"");err!=nil{
  t.Fatalf("authorized long Task did not start: %v",err)
 }
 deadline:=time.After(3*time.Second)
 for{
  if _,err:=os.Stat(filepath.Join(s.Workspace,"STARTED-LONG"));err==nil{break}
  select{
  case <-session.longTask.done:t.Fatal("long Task quit before permission change")
  case <-deadline:t.Fatal("long Task never started")
  case <-time.After(10*time.Millisecond):
  }
 }
 ctx:=context.Background()
 project,err:=s.Identity.ProjectByKey(ctx,session.projectKey)
 if err!=nil{t.Fatal(err)}
 member,err:=s.Identity.ProjectMember(ctx,project.ID,session.userID)
 if err!=nil{t.Fatal(err)}
 member.RoleID=identity.RestrictedDeveloperRoleID
 member.UpdatedAt=time.Now().UTC()
 if err:=s.Identity.UpsertProjectMember(ctx,member);err!=nil{t.Fatal(err)}
 select{
 case <-session.longTask.done:
 case <-time.After(3*time.Second):t.Fatal("local TasksRun downgrade did not cancel long Task")
 }
 job:=session.longTask
 job.mu.Lock()
 status,code:=job.status,job.exitCode
 job.mu.Unlock()
 if status!="cancelled"||code!= -1{
  t.Fatalf("revoked TasksRun still produced successful long Task: %q/%d",status,code)
 }
 if _,err:=os.Stat(filepath.Join(s.Workspace,"FORBIDDEN-LONG"));
  !os.IsNotExist(err){t.Fatalf("revoked long Task continued: %v",err)}
}

func TestRemoteLongTaskLiveGuardPreservesOrdinaryNonzeroExit(t *testing.T){
 s,session,id:=remoteLongTaskConsentFixture(t,
  `{"tasks":[{"label":"Authorized failure","type":"process",
   "command":"/bin/sh","args":["-c","exit 7"]}]}`)
 if err:=s.startRemoteLongTask(context.Background(),session,id,0,"");err!=nil{
  t.Fatalf("valid long Task denied: %v",err)
 }
 select{
 case <-session.longTask.done:
 case <-time.After(3*time.Second):t.Fatal("short-lived long task never reaped")
 }
 job:=session.longTask
 job.mu.Lock()
 status,code:=job.status,job.exitCode
 job.mu.Unlock()
 if status!="failed"||code!=7{
  t.Fatalf("normal process failure became revoked: %q/%d",status,code)
 }
}
