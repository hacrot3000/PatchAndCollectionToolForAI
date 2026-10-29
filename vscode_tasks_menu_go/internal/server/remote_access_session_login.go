package server

import (
 "context"

 "bletonfc/vscode_tasks_menu/internal/remoteportal"
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
  !s.Config.RemoteAccessEnabled || !s.Config.SharedServerEnabled ||
  !s.Config.TLS() || s.Identity==nil {
  return denied()
 }
 // Durable, one-use local admission first; compromised PHP tickets can't
 // reset the device-wide scrypt budget by changing an admin claim/JTI.
 if err:=agent.AdmitEncryptedLogin(ctx,channel,grant);err!=nil {return denied()}
 credentials,err:=remotetransport.OpenRemoteLogin(channel,ciphertext)
 if err!=nil{return denied()}
 session,err:=s.authenticateRemoteTaskDeckCredentials(ctx,channel,grant,
  credentials.Username,credentials.Password)
 credentials.Password=""
 if err!=nil || session==nil {return denied()}
 // Session cleanup is mandatory on ANY later authorization or sealing
 // failure; never leak a password-authenticated principal on a stale grant.
 success:=false
 defer func(){if !success {session.Close()}}()
 if err:=session.bindPortalAgent(agent);err!=nil {return denied()}
 if err:=agent.VerifyCurrentPortalGrant(ctx,grant);err!=nil {return denied()}
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
 session.startRemoteSessionLifetime()
 success=true
 return session,response,nil
}
