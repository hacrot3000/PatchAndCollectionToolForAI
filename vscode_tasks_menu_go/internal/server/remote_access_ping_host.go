package server

import (
 "context"

 "bletonfc/vscode_tasks_menu/internal/remoteportal"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// Host-only adapter. No TaskDeck principal, password or session token crosses
// the Relay: a successful login returns only encrypted Boolean data while the
// process retains an in-memory authority and checks RBAC for EVERY ping.
type encryptedRemotePingHost struct {
 server *Server
 session *remoteTaskDeckSession
 channel *remotetransport.Channel
}
func (h *encryptedRemotePingHost) ExecutePing(ctx context.Context,encrypted []byte)([]byte,error){
 if h==nil||h.server==nil||h.session==nil||h.channel==nil{
  return nil,remotetransport.ErrRemoteActionDenied
 }
 return h.server.executeRemoteE2EEPing(ctx,h.session,h.channel,encrypted)
}
func (h *encryptedRemotePingHost) Close(){
 if h!=nil&&h.session!=nil{h.session.Close()}
}

// StartEncryptedRemotePingHost is a private-functionality gateway, not
// generic proxy/terminal access. Admission is exactly the already-reviewed
// StartEncryptedRemoteSession (local TaskDeck password, host-approved client
// Ed25519 proof, immutable user ID, live Portal grant, TaskDeck permission).
func (s *Server) StartEncryptedRemotePingHost(ctx context.Context,
 agent *remoteportal.Agent,channel *remotetransport.Channel,
 grant *remoteportal.AgentGrant,ciphertext []byte,
)(remoteportal.EncryptedApplicationSession,[]byte,error){
 session,reply,err:=s.StartEncryptedRemoteSession(ctx,agent,channel,grant,ciphertext)
 if err!=nil||session==nil{return nil,nil,remotetransport.ErrRemoteActionDenied}
 return &encryptedRemotePingHost{server:s,session:session,channel:channel},reply,nil
}
