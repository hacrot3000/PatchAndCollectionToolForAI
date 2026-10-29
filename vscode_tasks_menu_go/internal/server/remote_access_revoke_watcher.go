package server

import (
 "context"
 "time"

)

// This polling watchdog is defense in depth for an authenticated, in-memory
// E2EE application session. The owner may revoke a key, change a local
// password, or disable TaskDeck membership WHILE a long-running callback is
// active. Checking only before and after a callback would reject its response
// but could leave that callback running until the action deadline.
//
// Every check reads the CURRENT host-local identity and approved-key registry.
// No Portal-supplied role or cached password credential is accepted. A slow
// or unavailable local store fails closed. This is not a substitute for the
// synchronous per-action authorization, transactional work cancellation, or
// independent pentesting required before Remote Control goes live.
const remoteLocalRevokePoll = 500 * time.Millisecond
const remoteLocalRevokeCheckDeadline = 2 * time.Second

func (s *Server) watchRemoteSessionLocalRevoke(session *remoteTaskDeckSession) {
 if session == nil { return }
 if s == nil || s.Identity == nil || session.channel == nil ||
   session.portalGrant == nil || session.lifetimeDone == nil {
  session.Close()
  return
 }
 go func() {
  ticker := time.NewTicker(remoteLocalRevokePoll)
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
    ctx,cancel := context.WithTimeout(context.Background(),remoteLocalRevokeCheckDeadline)
    _,err := s.verifyRemoteSessionLocalPrincipal(ctx,session)
    cancel()
    if err != nil {
     // Error covers revoked/changed key, account/credential, membership,
     // wrong project, expired session, local SQLite failures and loss
     // of the authenticated E2EE channel. Never silently fail open.
     session.Close()
     return
    }
   }
  }
 }()
}
