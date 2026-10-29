package server

import (
 "context"
 "time"

)

// Periodic grant introspection closes a staged E2EE session while idle,
// rather than waiting for the next action to notice a revoked Portal grant.
// It MUST NOT share the host-local revocation goroutine: an unresponsive or
// compromised PHP endpoint cannot stall local key/password revocation.
const remotePortalRevokePoll = 10 * time.Second
const remotePortalRevokeCheckDeadline = 6 * time.Second

func (s *Server) watchRemoteSessionPortalRevoke(session *remoteTaskDeckSession) {
 s.watchRemoteSessionPortalRevokeAt(session,remotePortalRevokePoll)
}

// watchRemoteSessionPortalRevokeAt allows fast deterministic negative tests.
// It never creates an HTTP endpoint or a remotely usable TaskDeck principal.
func (s *Server) watchRemoteSessionPortalRevokeAt(session *remoteTaskDeckSession,interval time.Duration) {
 s.watchRemoteSessionPortalRevokeWithDeadline(session,interval,remotePortalRevokeCheckDeadline)
}

func (s *Server) watchRemoteSessionPortalRevokeWithDeadline(
 session *remoteTaskDeckSession, interval, checkDeadline time.Duration,
) {
 if session==nil{return}
 if interval<=0 || checkDeadline<=0 {session.Close();return}
 if session.portalGrant==nil || session.channel==nil || session.lifetimeDone==nil {
  session.Close()
  return
 }
 go func() {
  ticker:=time.NewTicker(interval)
  defer ticker.Stop()
  for {
   select {
   case <-session.lifetimeDone:
    return
   case <-session.channel.Done():
    session.Close()
    return
   case <-session.portalGrant.Done():
    session.Close()
    return
   case <-ticker.C:
    // Fresh uncached grant verification every interval, independent from
    // each action's two mandatory Portal checks and TaskDeck-local RBAC.
    session.portalCheckMu.RLock()
    check:=session.livePortalCheck
    session.portalCheckMu.RUnlock()
    if check==nil {session.Close();return}
    ctx,cancel:=context.WithTimeout(context.Background(),checkDeadline)
    watcherDone:=make(chan struct{})
    // Session.Close() cannot wait for an untrusted network request. The
    // network call normally honors ctx, but a misbehaving callback must
    // NEVER keep a revoked local E2EE session alive indefinitely.
    go func(){
     select{
     case <-session.lifetimeDone: cancel()
     case <-session.channel.Done(): cancel()
     case <-session.portalGrant.Done(): cancel()
     case <-watcherDone:
     }
    }()
    checkResult:=make(chan error,1)
    // At most one introspection worker is started per session check.
    // On timeout the watcher closes the session and exits permanently,
    // so an uncooperative network client cannot generate unlimited work.
    go func(){checkResult<-check(ctx,session.portalGrant)}()
    var err error
    select{
    case err=<-checkResult:
    case <-ctx.Done():
     err=ctx.Err()
    }
    expired:=ctx.Err()!=nil
    close(watcherDone)
    cancel()
    if err!=nil || expired || session.closed.Load() ||
      !session.portalGrant.ActiveForSession(session.channel) {
     session.Close()
     return
    }
   }
  }
 }()
}
