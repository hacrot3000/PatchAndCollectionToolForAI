package server

import (
 "context"
 "errors"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// The staged application entrypoint remains unmounted from the network.
// This test proves a live local-account revoke DOES cancel a callback,
// not merely reject a later encrypted result.
func TestRemoteLocalRevocationWatcherStopsActiveSessionAndCallback(t *testing.T) {
 s,session,ch := testRemoteExecutionSession(t)
 started:=make(chan struct{})
 finished:=make(chan error,1)
 s.watchRemoteSessionLocalRevoke(session)
 go func(){
  finished <- s.executeRemoteAuthenticatedAction(context.Background(),session,ch,
   identity.PermissionFilesRead,func(ctx context.Context)error{
    close(started)
    <-ctx.Done()
    return nil
   })
 }()
 select{
 case <-started:
 case <-time.After(5*time.Second):t.Fatal("authorized callback did not start")
 }
 if err:=s.Identity.SetUserEnabled(context.Background(),session.userID,false,time.Now().UTC());err!=nil {
  t.Fatal(err)
 }
 select{
 case err:=<-finished:
  if !errors.Is(err,remotetransport.ErrRemoteActionDenied) {
   t.Fatalf("revoked local user retained action: %v",err)
  }
 case <-time.After(5*time.Second):
  t.Fatal("local user revocation did not cancel callback")
 }
 if ch.Established() || !session.closed.Load() {
  t.Fatal("watcher failed to close in-process E2EE session")
 }
}

// A read-only, still-authorized session should not be destroyed by the
// polling watcher merely because the old one-shot ticket has expired.
// Live Portal introspection is checked separately on EVERY action.
func TestRemoteLocalRevocationWatcherKeepsValidSession(t *testing.T) {
 s,session,ch := testRemoteExecutionSession(t)
 s.watchRemoteSessionLocalRevoke(session)
 select{
 case <-ch.Done():t.Fatal("valid locally authenticated session was closed")
 case <-time.After(2*remoteLocalRevokePoll+250*time.Millisecond):
 }
 session.Close()
 select{
 case <-session.lifetimeDone:
 case <-time.After(time.Second):t.Fatal("session Close did not stop watchdog")
 }
}
