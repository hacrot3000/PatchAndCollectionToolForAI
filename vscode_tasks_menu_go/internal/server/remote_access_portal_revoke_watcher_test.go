package server

import (
 "context"
 "errors"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remoteportal"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func TestRemotePortalRevokeWatchCancelsIdleE2EE(t *testing.T) {
 s,session,ch:=testRemoteExecutionSession(t)
 checks:=make(chan struct{},1)
 session.portalCheckMu.Lock()
 session.livePortalCheck=func(_ context.Context,_ *remoteportal.AgentGrant)error{
  select{case checks<-struct{}{}:default:}
  return remotetransport.ErrRemoteActionDenied
 }
 session.portalCheckMu.Unlock()
 s.watchRemoteSessionPortalRevokeAt(session,10*time.Millisecond)
 select {
 case <-checks:
 case <-time.After(3*time.Second):t.Fatal("no uncached Portal introspection")
 }
 select {
 case <-ch.Done():
 case <-time.After(3*time.Second):t.Fatal("revoked Portal grant left E2EE alive")
 }
 if !session.closed.Load() {t.Fatal("revoked grant left local session open")}
}

func TestRemotePortalRevokeWatchCancelsActiveCallback(t *testing.T) {
 s,session,ch:=testRemoteExecutionSession(t)
 started:=make(chan struct{})
 completed:=make(chan error,1)
 go func() {
  completed<-s.executeRemoteAuthenticatedAction(context.Background(),session,ch,
   identity.PermissionFilesRead,func(ctx context.Context)error {
    close(started)
    <-ctx.Done()
    return nil
   })
 }()
 select {
 case <-started:
 case <-time.After(5*time.Second):t.Fatal("callback not admitted with current grant")
 }
 session.portalCheckMu.Lock()
 session.livePortalCheck=func(_ context.Context,_ *remoteportal.AgentGrant)error{
  return errors.New("Portal revoke or unavailable")
 }
 session.portalCheckMu.Unlock()
 s.watchRemoteSessionPortalRevokeAt(session,10*time.Millisecond)
 select {
 case err:=<-completed:
  if !errors.Is(err,remotetransport.ErrRemoteActionDenied) {
   t.Fatalf("revoked session returned callback success: %v",err)
  }
 case <-time.After(3*time.Second):t.Fatal("Portal revocation did not stop callback")
 }
 if ch.Established() || !session.closed.Load() {
  t.Fatal("revoked Portal grant left channel or session established")
 }
}

func TestRemotePortalRevokeWatchHealthyGrantDoesNotCloseSession(t *testing.T) {
 s,session,ch:=testRemoteExecutionSession(t)
 s.watchRemoteSessionPortalRevokeAt(session,20*time.Millisecond)
 select{
 case <-ch.Done():t.Fatal("healthy Portal session closed")
 case <-time.After(160*time.Millisecond):
 }
 session.Close()
}

func TestRemotePortalRevokeWatchTimeoutCannotBeBypassedByLateAllowedTrue(t *testing.T) {
 s,session,ch:=testRemoteExecutionSession(t)
 entered:=make(chan struct{})
 release:=make(chan struct{})
 returned:=make(chan struct{})
 session.portalCheckMu.Lock()
 session.livePortalCheck=func(_ context.Context,_ *remoteportal.AgentGrant)error {
  close(entered)
  <-release // deliberately ignores context cancellation, simulates bad transport
  close(returned)
  return nil // stale allowed=true MUST NEVER revive E2EE
 }
 session.portalCheckMu.Unlock()
 s.watchRemoteSessionPortalRevokeWithDeadline(session,5*time.Millisecond,30*time.Millisecond)
 select{
 case <-entered:
 case <-time.After(time.Second):t.Fatal("slow Portal callback not started")
 }
 select{
 case <-ch.Done():
 case <-time.After(time.Second):t.Fatal("expired live Portal check retained E2EE session")
 }
 if !session.closed.Load(){t.Fatal("stale grant check did not close TaskDeck session")}
 close(release)
 select{
 case <-returned:
 case <-time.After(time.Second):t.Fatal("test callback did not exit")
 }
 if ch.Established(){t.Fatal("late allowed=true revived a closed session")}
}

func TestRemoteHostRevocationCannotBeStalledByHungPortalGrantCheck(t *testing.T) {
 s,session,ch:=testRemoteExecutionSession(t)
 entered:=make(chan struct{})
 release:=make(chan struct{})
 finished:=make(chan struct{})
 session.portalCheckMu.Lock()
 session.livePortalCheck=func(_ context.Context,_ *remoteportal.AgentGrant)error {
  close(entered)
  <-release // simulates a malicious PHP server that never returns
  close(finished)
  return nil
 }
 session.portalCheckMu.Unlock()
 // The host-local check must NOT share the blocked Portal goroutine.
 s.watchRemoteSessionLocalRevoke(session)
 s.watchRemoteSessionPortalRevokeWithDeadline(session,5*time.Millisecond,2*time.Second)
 select {
 case <-entered:
 case <-time.After(time.Second):t.Fatal("Portal verification did not start")
 }
 if err:=s.Identity.SetUserEnabled(context.Background(),session.userID,false,time.Now().UTC());err!=nil {
  t.Fatal(err)
 }
 select {
 case <-ch.Done():
 case <-time.After(1500*time.Millisecond):t.Fatal("hung Portal prevented local TaskDeck revocation")
 }
 if !session.closed.Load() {t.Fatal("host local revocation did not destroy session")}
 close(release)
 select {
 case <-finished:
 case <-time.After(time.Second):t.Fatal("test Portal callback did not finish")
 }
 if ch.Established(){t.Fatal("stale Portal allowed=true revived revoked TaskDeck channel")}
}
