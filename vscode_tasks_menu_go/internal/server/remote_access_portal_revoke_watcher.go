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
 if session==nil || interval<=0 {return}
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
    ctx,cancel:=context.WithTimeout(context.Background(),remotePortalRevokeCheckDeadline)
    done:=make(chan struct{})
    // Session.Close() cannot wait for untrusted network activity. Cancel
    // the outgoing HTTPS check promptly on grant or local session closure.
    go func(){
     select{
     case <-session.lifetimeDone: cancel()
     case <-session.channel.Done(): cancel()
     case <-session.portalGrant.Done(): cancel()
     case <-done:
     }
    }()
    err:=check(ctx,session.portalGrant)
    close(done)
    cancel()
    if err!=nil || session.closed.Load() ||
      !session.portalGrant.ActiveForSession(session.channel) {
     session.Close()
     return
    }
   }
  }
 }()
}
