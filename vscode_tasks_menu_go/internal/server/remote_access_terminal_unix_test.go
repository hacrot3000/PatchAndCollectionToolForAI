//go:build linux || darwin

package server

import (
 "context"
 "errors"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func terminalAuthenticatedAction(t *testing.T,s *Server,session *remoteTaskDeckSession,
 client,agent *remotetransport.Channel,requestID,op,input string,
 cols,rows int)(remotetransport.RemoteTerminalProgress,error){
 t.Helper()
 frame,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:requestID,Operation:op,TerminalInput:input,Cols:cols,Rows:rows,
 })
 if err!=nil{t.Fatal(err)}
 reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,frame)
 if err!=nil{return remotetransport.RemoteTerminalProgress{},err}
 return remotetransport.OpenRemoteTerminalProgress(client,reply,requestID)
}


func TestRemoteTerminalE2EEFailedSealPreservesOutputCursor(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TERMINAL","1")
 s,session,client,agent:=remoteTerminalAdminFixture(t)
 capture:=newRemoteLongTaskCapture()
 const output="HOST-PTY-UNDELIVERED"
 if _,err:=capture.Write([]byte(output));err!=nil{t.Fatal(err)}
 job:=&remoteTerminalJob{
  state:"invalid", // Typed AEAD response must reject this state.
  capture:capture,cancel:func(){},
 }
 session.terminalMu.Lock()
 session.terminal=job
 session.terminalMu.Unlock()
 invalid,err:=terminalAuthenticatedAction(t,s,session,client,agent,
  "81818181818181818181818181818181",
  remotetransport.RemoteActionTerminalPoll,"",0,0)
 if err==nil||invalid.Output!=""{
  t.Fatalf("invalid PTY status unexpectedly sealed: %+v %v",invalid,err)
 }
 job.mu.Lock()
 cursor:=job.cursor
 job.state="running"
 job.mu.Unlock()
 if cursor!=0{t.Fatalf("failed E2EE seal lost %d PTY bytes",cursor)}
 restored,err:=terminalAuthenticatedAction(t,s,session,client,agent,
  "82828282828282828282828282828282",
  remotetransport.RemoteActionTerminalPoll,"",0,0)
 if err!=nil||restored.Output!=output||restored.More||
  restored.State!="running"{
  t.Fatalf("next authenticated Poll lost PTY data: %+v %v",restored,err)
 }
 job.mu.Lock()
 cursor=job.cursor
 job.mu.Unlock()
 if cursor!=len(output){t.Fatalf("successful Seal cursor=%d",cursor)}
 again,err:=terminalAuthenticatedAction(t,s,session,client,agent,
  "83838383838383838383838383838383",
  remotetransport.RemoteActionTerminalPoll,"",0,0)
 if err!=nil||again.Output!=""||again.More{
  t.Fatalf("previously sealed PTY output replayed: %+v %v",again,err)
 }
}

func TestRemoteTerminalDeferredCursorDoesNotOverwriteNewerRead(t *testing.T){
 capture:=newRemoteLongTaskCapture()
 content:=strings.Repeat("X",remoteLongTaskMaxChunkBytes)+"TAIL"
 if _,err:=capture.Write([]byte(content));err!=nil{t.Fatal(err)}
 job:=&remoteTerminalJob{capture:capture,state:"exited"}
 session:=&remoteTaskDeckSession{terminal:job}
 first,stale,err:=session.remoteTerminalSnapshotWithMode(true)
 if err!=nil||stale==nil||len(first.Output)!=remoteLongTaskMaxChunkBytes||
  !first.More{t.Fatalf("deferred PTY snapshot invalid: %+v %v",first,err)}
 job.mu.Lock();cursor:=job.cursor;job.mu.Unlock()
 if cursor!=0{t.Fatalf("unsealed PTY output consumed: %d",cursor)}
 direct,err:=session.remoteTerminalSnapshot()
 if err!=nil||direct.Output!=first.Output{
  t.Fatalf("direct host read did not consume first chunk: %+v %v",direct,err)
 }
 stale()
 job.mu.Lock();cursor=job.cursor;job.mu.Unlock()
 if cursor!=remoteLongTaskMaxChunkBytes{
  t.Fatalf("stale E2EE commit overwrote newer cursor: %d",cursor)
 }
 tail,commit,err:=session.remoteTerminalSnapshotWithMode(true)
 if err!=nil||commit==nil||tail.Output!="TAIL"||tail.More{
  t.Fatalf("remaining PTY tail missing: %+v %v",tail,err)
 }
 commit()
 final,err:=session.remoteTerminalSnapshot()
 if err!=nil||final.Output!=""||final.More{
  t.Fatalf("already committed PTY tail replayed: %+v %v",final,err)
 }
}

func TestRemoteTerminalHostPTYOpenWriteReadResizeAndClose(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TERMINAL","1")
 s,session,client,agent:=remoteTerminalAdminFixture(t)
 started,err:=terminalAuthenticatedAction(t,s,session,client,agent,
  "11111111111111111111111111111111",
  remotetransport.RemoteActionTerminalOpen,"",0,0)
 if err!=nil||started.State!="running"{
  t.Fatalf("remote PTY never opened: %+v %v",started,err)
 }
 if _,err:=terminalAuthenticatedAction(t,s,session,client,agent,
  "22222222222222222222222222222222",
  remotetransport.RemoteActionTerminalOpen,"",0,0);err==nil{
  t.Fatal("second PTY started inside a single authenticated session")
 }
 resized,err:=terminalAuthenticatedAction(t,s,session,client,agent,
  "33333333333333333333333333333333",
  remotetransport.RemoteActionTerminalResize,"",120,32)
 if err!=nil||resized.State!="running"{t.Fatalf("PTY resize failed: %+v %v",resized,err)}
 written,err:=terminalAuthenticatedAction(t,s,session,client,agent,
  "44444444444444444444444444444444",
  remotetransport.RemoteActionTerminalWrite,"printf remote-pty-ready\\n\n",0,0)
 if err!=nil||written.State!="running"{t.Fatalf("PTY input not sent: %+v %v",written,err)}
 collected:=""
 for i:=0;i<8&&!strings.Contains(collected,"remote-pty-ready");i++{
  time.Sleep(60*time.Millisecond)
  requestID:=strings.Repeat(string(rune('5'+i)),32)
  chunk,pollErr:=terminalAuthenticatedAction(t,s,session,client,agent,
   requestID,remotetransport.RemoteActionTerminalPoll,"",0,0)
  if pollErr!=nil{t.Fatalf("PTY output never readable: %v",pollErr)}
  collected+=chunk.Output
 }
 if !strings.Contains(collected,"remote-pty-ready"){
  t.Fatalf("interactive shell did not run typed command: %q",collected)
 }
 stopped,err:=terminalAuthenticatedAction(t,s,session,client,agent,
  "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
  remotetransport.RemoteActionTerminalClose,"",0,0)
 if err!=nil||stopped.State!="closed"{t.Fatalf("PTY close failed: %+v %v",stopped,err)}
 if session.terminal!=nil{t.Fatal("closed PTY leaked session-owned job")}
 project,err:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
 if err!=nil{t.Fatal(err)}
 events,err:=s.Identity.ListAudit(context.Background(),identity.AuditQuery{
  ProjectID:project.ID,Action:"remote.action.terminal_write",Limit:10,
 })
 if err!=nil||len(events)!=1||events[0].Result!="attempted"||
  strings.Contains(events[0].Details,"remote-pty-ready"){
  t.Fatalf("audited terminal input leaked or missing: %+v %v",events,err)
 }
}

func TestRemoteTerminalSessionCloseStopsPTYAndChildProcesses(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TERMINAL","1")
 s,session,_,_:=remoteTerminalAdminFixture(t)
 if err:=s.startRemoteTerminal(context.Background(),session);err!=nil{t.Fatal(err)}
 command:="touch started; (sleep 2; touch escaped) >/dev/null 2>&1 & wait\n"
 if _,err:=s.writeRemoteTerminal(context.Background(),session,command);err!=nil{t.Fatal(err)}
 if !remoteTestAwaitFile(filepath.Join(s.Workspace,"started"),3*time.Second){
  t.Fatal("PTY command did not start")
 }
 began:=time.Now()
 session.Close()
 if time.Since(began)>time.Second{t.Fatal("logout blocked on interactive PTY")}
 select{
 case <-session.terminal.done:
 case <-time.After(3*time.Second):t.Fatal("PTY not terminated after logout")
 }
 time.Sleep(2300*time.Millisecond)
 if _,err:=os.Stat(filepath.Join(s.Workspace,"escaped"));!os.IsNotExist(err){
  t.Fatal("background shell child survived authenticated session revocation")
 }
}

func TestRemoteTerminalViewerAndOptOutDenied(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TERMINAL","1")
 s,session,client,agent:=remoteDispatchFixture(t)
 if _,err:=terminalAuthenticatedAction(t,s,session,client,agent,
  "11111111111111111111111111111111",
  remotetransport.RemoteActionTerminalOpen,"",0,0);err==nil{
  t.Fatal("viewer opened a privileged PTY")
 }
 s2,session2,_,_:=remoteTerminalAdminFixture(t)
 t.Setenv("TASKDECK_REMOTE_TERMINAL","0")
 if err:=s2.startRemoteTerminal(context.Background(),session2);err==nil{
  t.Fatal("local admin bypassed independent host terminal opt-in")
 }
}


func TestRemoteTerminalNaturalShellExitKillsDetachedSessionChildren(t *testing.T){
 if os.Geteuid()==0{t.Skip("PTY host must not run as root")}
 t.Setenv("TASKDECK_REMOTE_TERMINAL","1")
 s,session,_,_:=remoteTerminalAdminFixture(t)
 if err:=s.startRemoteTerminal(context.Background(),session);err!=nil{
  t.Fatalf("could not start PTY: %v",err)
 }
 job:=session.terminal
 // The background child ignores SIGHUP and redirects its output, so
 // simply closing the PTY master or reaping the parent shell cannot
 // terminate this process. It must be killed as an authenticated
 // PTY-session descendant before the host releases the workspace.
 command:="nohup /bin/sh -c 'touch child-started; sleep 2; touch escaped' </dev/null >/dev/null 2>&1 &\n"
 if _,err:=s.writeRemoteTerminal(context.Background(),session,command);err!=nil{
  t.Fatalf("could not run detached PTY child: %v",err)
 }
 // Avoid a vacuous PASS if interactive shell parsing, job launch or
 // platform-specific nohup behavior prevented the child from starting.
 if !remoteTestAwaitFile(filepath.Join(s.Workspace,"child-started"),3*time.Second){
  t.Fatal("background PTY child never started")
 }
 if _,err:=s.writeRemoteTerminal(context.Background(),session,"exit\n");err!=nil{
  t.Fatalf("cannot terminate parent shell: %v",err)
 }
 select{
 case <-job.workspaceReleased:
 case <-time.After(4*time.Second):t.Fatal("normal PTY exit left a workspace lease")
 }
 job.mu.Lock()
 state:=job.state
 job.mu.Unlock()
 if state!="exited"{t.Fatalf("PTY did not report natural exit: %q",state)}
 time.Sleep(2300*time.Millisecond)
 if _,err:=os.Stat(filepath.Join(s.Workspace,"escaped"));!os.IsNotExist(err){
  t.Fatal("background PTY process survived the parent shell's natural exit")
 }
}


func TestRemoteTerminalCloseTimeoutRetainsJobForAuthenticatedRetry(t *testing.T){
 t.Setenv("TASKDECK_REMOTE_TERMINAL","1")
 s,session,_,_:=remoteTerminalAdminFixture(t)
 // Model Wait delayed past the action deadline. An in-flight host
 // process can remain alive while E2EE Close has already been sent.
 // The test intentionally does not start a shell or leak a child.
 cancelCalled:=make(chan struct{})
 released:=make(chan struct{})
 job:=&remoteTerminalJob{
  state:"running",done:make(chan struct{}),
  workspaceReleased:released,
  cancel:func(){close(cancelCalled)},
 }
 session.terminalMu.Lock()
 session.terminal=job
 session.terminalMu.Unlock()
 ctx,cancel:=context.WithCancel(context.Background())
 finished:=make(chan error,1)
 go func(){
  _,err:=s.closeRemoteTerminal(ctx,session)
  finished<-err
 }()
 select{
 case <-cancelCalled:
 case <-time.After(3*time.Second):
  cancel()
  t.Fatal("Close never requested host PTY cancellation")
 }
 cancel()
 select{
 case err:=<-finished:
  if !errors.Is(err,remotetransport.ErrRemoteActionDenied){
   t.Fatalf("cancelled Close wrongly confirmed workspace release: %v",err)
  }
 case <-time.After(3*time.Second):
  t.Fatal("cancelled close action failed to stop waiting")
 }
 session.terminalMu.Lock()
 retained:=session.terminal==job
 session.terminalMu.Unlock()
 if !retained{t.Fatal("timed-out Close silently detached an unretired host PTY")}
 // A retry is permitted after the waiter confirms process exit and
 // releases its workspace reservation. We install a one-shot fake cancel
 // to model the already requested shutdown while preserving the job.
 job.cancel=func(){}
 close(released)
 result,err:=s.closeRemoteTerminal(context.Background(),session)
 if err!=nil||result.State!="closed"{
  t.Fatalf("retry failed to retire completed PTY: %+v %v",result,err)
 }
 session.terminalMu.Lock()
 stillAttached:=session.terminal!=nil
 session.terminalMu.Unlock()
 if stillAttached{t.Fatal("successfully retried PTY Close leaked job handle")}
}


// Host-only audit fixture changes a real TaskDeck membership after the
// attempted OPEN has been durably appended, not via browser or Relay data.
type remoteTerminalOpenAuditDowngrade struct{
 identity.Store
 projectKey string
 userID identity.ID
}
func(store remoteTerminalOpenAuditDowngrade)AppendAudit(ctx context.Context,
 event identity.AuditEvent)error{
 if err:=store.Store.AppendAudit(ctx,event);err!=nil{return err}
 if event.Action!="remote.action.terminal_open"||event.Result!="attempted"{
  return nil
 }
 project,err:=store.Store.ProjectByKey(ctx,store.projectKey)
 if err!=nil{return err}
 member,err:=store.Store.ProjectMember(ctx,project.ID,store.userID)
 if err!=nil{return err}
 member.RoleID=identity.RestrictedDeveloperRoleID
 member.UpdatedAt=time.Now().UTC()
 return store.Store.UpsertProjectMember(ctx,member)
}

func TestRemoteTerminalOpenRejectsHostRoleRevokedDuringAudit(t *testing.T){
 if os.Geteuid()==0{t.Skip("PTY must not run as root")}
 t.Setenv("TASKDECK_REMOTE_TERMINAL","1")
 s,session,_,_:=remoteTerminalAdminFixture(t)
 if !s.remoteTerminalHostConsent(context.Background(),session){
  t.Fatal("valid TerminalCreate and TerminalControlOwn permission denied")
 }
 s.Identity=remoteTerminalOpenAuditDowngrade{
  Store:s.Identity,projectKey:session.projectKey,userID:session.userID,
 }
 err:=s.startRemoteTerminal(context.Background(),session)
 if !errors.Is(err,remotetransport.ErrRemoteActionDenied){
  t.Fatalf("PTY opened after Terminal role downgrade during audit: %v",err)
 }
 session.terminalMu.Lock()
 attached:=session.terminal!=nil
 session.terminalMu.Unlock()
 if attached{t.Fatal("audit-time role downgrade launched a host PTY")}
 // Denied OPEN must release its workspace reservation.
 release,err:=reserveRemoteWorkspaceExecution(context.Background(),s.Workspace)
 if err!=nil{t.Fatalf("denied PTY leaked workspace lock: %v",err)}
 release()
 if s.remoteTerminalHostConsent(context.Background(),session){
  t.Fatal("post-audit Terminal privilege unexpectedly remained")
 }
}

func TestRemoteTerminalActivePTYStopsOnHostOptOutOrRoleDowngrade(t *testing.T){
 if os.Geteuid()==0{t.Skip("PTY must not run as root")}
 for _,mode:=range []string{"host flag","TaskDeck role"}{
  t.Run(mode,func(t *testing.T){
   t.Setenv("TASKDECK_REMOTE_TERMINAL","1")
   s,session,_,_:=remoteTerminalAdminFixture(t)
   if err:=s.startRemoteTerminal(context.Background(),session);err!=nil{
    t.Fatalf("could not start approved PTY: %v",err)
   }
   job:=session.terminal
   if _,err:=s.writeRemoteTerminal(context.Background(),session,
    "touch started; sleep 5; touch escaped\n");err!=nil{
    t.Fatalf("could not start test PTY process: %v",err)
   }
   if !remoteTestAwaitFile(filepath.Join(s.Workspace,"started"),2*time.Second){
    t.Fatal("PTY did not execute first instruction before host opt-out")
   }
   if mode=="host flag"{
    t.Setenv("TASKDECK_REMOTE_TERMINAL","0")
   }else{
    project,err:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
    if err!=nil{t.Fatal(err)}
    member,err:=s.Identity.ProjectMember(context.Background(),project.ID,session.userID)
    if err!=nil{t.Fatal(err)}
    member.RoleID=identity.RestrictedDeveloperRoleID
    member.UpdatedAt=time.Now().UTC()
    if err:=s.Identity.UpsertProjectMember(context.Background(),member);err!=nil{
     t.Fatal(err)
    }
   }
   select{
   case <-job.done:
   case <-time.After(4*time.Second):t.Fatal("revoked host PTY kept running")
   }
   select{
   case <-job.workspaceReleased:
   case <-time.After(2*time.Second):t.Fatal("revoked PTY retained workspace lock")
   }
   if _,err:=os.Stat(filepath.Join(s.Workspace,"escaped"));!os.IsNotExist(err){
    t.Fatalf("revoked PTY continued executing after forced shutdown: %v",err)
   }
  })
 }
}
