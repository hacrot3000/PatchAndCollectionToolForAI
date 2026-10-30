package server

import (
 "context"

 "bletonfc/vscode_tasks_menu/internal/remoteportal"
 "bletonfc/vscode_tasks_menu/internal/remoteclientkey"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// StartEncryptedRemoteSession stages a *distinct*, non-HTTP, password-bound
// TaskDeck application session. The old VerifyEncryptedRemoteLogin remains a
// one-shot BOOLEAN diagnostic and still destroys its temporary principal.
//
// This deliberately has NO Internet/Relay/HTTP route or callback registration.
// The only caller which may eventually expose it must be the owner-pinned
// Agent running in the TaskDeck host, after a native trusted client has
// authenticated the actual Agent Ed25519 identity and E2EE transcript.
// Portal/Relay cannot provide local credentials or mint this session.
//
// The return value is a private in-process session pointer and an encrypted
// yes/no response. NEVER marshal the pointer, identity or any credentials
// into the response or issue a cookie/bearer token.
func (s *Server) StartEncryptedRemoteSession(
 ctx context.Context, agent *remoteportal.Agent,
 channel *remotetransport.Channel, grant *remoteportal.AgentGrant,
 ciphertext []byte,
) (*remoteTaskDeckSession,[]byte,error) {
 denied:=func()(*remoteTaskDeckSession,[]byte,error){
  if grant!=nil {grant.Close()}
  if channel!=nil {channel.Close()}
  return nil,nil,remotetransport.ErrRemoteActionDenied
 }
 if s==nil || ctx==nil || ctx.Err()!=nil || agent==nil || grant==nil ||
  channel==nil || !channel.Established() || !agent.OwnsPortalGrant(grant) ||
  !s.remoteWorkspaceAgentMatches(agent,channel) ||
  !s.Config.RemoteAccessEnabled || !s.Config.SharedServerEnabled ||
  !s.Config.TLS() || s.Identity==nil {
  return denied()
 }
 // Durable, one-use local admission first; compromised PHP tickets can't
 // reset the device-wide scrypt budget by changing an admin claim/JTI.
 if err:=agent.AdmitEncryptedLogin(ctx,channel,grant);err!=nil {return denied()}
 credentials,err:=remotetransport.OpenRemoteLogin(channel,ciphertext)
 if err!=nil{return denied()}
 // A PHP-issued ticket, even one carrying admin role, never approves a
 // client machine. No local allowlist entry or signed E2EE challenge => NO
 // TaskDeck session; the host CLI alone can approve keys.
 keyPath,pathErr:=remoteclientkey.DefaultPath(s.Workspace)
 if pathErr!=nil || remoteclientkey.Verify(keyPath,credentials.Username,
   credentials.ClientPublicKey,credentials.ClientSignature,
   channel.RemoteClientProofBinding())!=nil {
  credentials.Password=""
  return denied()
 }
 session,err:=s.authenticateRemoteTaskDeckCredentials(ctx,channel,grant,
  credentials.Username,credentials.Password)
 credentials.Password=""
 if err!=nil || session==nil {return denied()}
 // Bind the authenticated application principal to this host-approved
 // client identity. Every subsequent action re-reads the local key registry;
 // Portal can neither insert nor restore a revoked client key.
 session.clientPublicKey=credentials.ClientPublicKey
 session.clientUsername=credentials.Username
 // Session cleanup is mandatory on ANY later authorization or sealing
 // failure; never leak a password-authenticated principal on a stale grant.
 success:=false
 defer func(){if !success {session.Close()}}()
 if err:=session.bindPortalAgent(agent);err!=nil {return denied()}
 if err:=agent.VerifyCurrentPortalGrant(ctx,grant);err!=nil {return denied()}
 // The final HTTPS Portal check is attacker-controlled in the worst case.
 // If the real TaskDeck account/password or project membership changed
 // during that wait, an old password MUST NOT create a live session.
 if _,err:=s.verifyRemoteSessionLocalPrincipal(ctx,session);err!=nil {return denied()}
 if ctx.Err()!=nil || session.closed.Load() ||
  !grant.ActiveForSession(channel) {return denied()}
 // The result is exactly the existing strict login boolean schema; neither
 // PHP nor Relay receives an application token, user ID or project scope.
 response,err:=remotetransport.SealRemoteLoginResult(channel,true)
 if err!=nil || remotetransport.VerifyOutgoingRemoteLoginResult(channel,response)!=nil {
  return denied()
 }
 // The grant and local TaskDeck identity may be revoked during the last
 // HTTPS read or AEAD sealing. Recheck before releasing any success bytes.
 if ctx.Err()!=nil || session.closed.Load() ||
  !grant.ActiveForSession(channel) {return denied()}
 // AEAD sealing and local identity storage are independently asynchronous
 // to a password reset. Repeat local checks at the final success boundary.
 if _,err:=s.verifyRemoteSessionLocalPrincipal(ctx,session);err!=nil {return denied()}
 session.startRemoteSessionLifetime()
 // A host-side password/key/account revocation must also wake an action that
 // is waiting inside a cancellation-aware callback. The per-action checks
 // remain mandatory; this watchdog is an additional fail-closed backstop.
 s.watchRemoteSessionLocalRevoke(session)
 success=true
 return session,response,nil
}
