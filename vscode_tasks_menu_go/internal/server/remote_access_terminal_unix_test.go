//go:build linux || darwin

package server

import (
 "context"
 "errors"
 "crypto/ed25519"
 "crypto/rand"
 "crypto/sha256"
 "encoding/hex"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/config"
 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func remoteTerminalAdminFixture(t *testing.T)(
 *Server,*remoteTaskDeckSession,*remotetransport.Channel,*remotetransport.Channel){
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
 handshake,err:=client.Seal([]byte("authenticated-client-possession"))
 if err!=nil{t.Fatal(err)}
 if _,err:=agent.Open(handshake);err!=nil{t.Fatal(err)}
 server:=sharedLoginTestServer(t)
 server.Config.RemoteAccessEnabled=true
 server.Config.Protocol=config.ProtocolHTTPS
 grant:=testRemoteSignedGrant(t,agent,remotetransport.RoleAdmin)
 session,err:=server.authenticateRemoteTaskDeckCredentials(
  context.Background(),agent,grant,"alice","private-admin-password")
 if err!=nil{t.Fatal(err)}
 bindTestRemoteOwnerClient(t,server,session)
 t.Cleanup(session.Close)
 t.Cleanup(client.Close)
 allowTestLivePortal(session)
 return server,session,client,agent
}

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
