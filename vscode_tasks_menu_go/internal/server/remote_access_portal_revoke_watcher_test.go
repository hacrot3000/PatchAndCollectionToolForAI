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
