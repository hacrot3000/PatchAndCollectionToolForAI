package remoteportal

import (
 "errors"
 "sync/atomic"
 "time"

 "bletonfc/vscode_tasks_menu/internal/remoterelay"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// AgentGrantVerifier independently checks Portal's signed ticket at the Agent.
// It must use the locally pinned Portal public key, NOT one supplied by Relay.
type AgentGrantVerifier struct {
 verifier *remoterelay.Verifier
 deviceID string
}

// NewAgentGrantVerifier is explicitly independent of Relay's own verifier.
// The same ticket must pass two separate signature checks before any future
// TaskDeck login is considered. This component exposes NO application APIs.
func NewAgentGrantVerifier(publicKey, issuer, deviceID string) (*AgentGrantVerifier,error) {
 if len(deviceID)!=64 {return nil,errors.New("invalid Agent device identity")}
 for _,ch:=range deviceID {
  if (ch<'0'||ch>'9')&&(ch<'a'||ch>'f') {return nil,errors.New("invalid Agent device identity")}
 }
 v,err:=remoterelay.NewVerifier(publicKey,issuer)
 if err!=nil{return nil,err}
 return &AgentGrantVerifier{verifier:v,deviceID:deviceID},nil
}

// AgentGrant is an in-memory, one-use Portal role ceiling, not a TaskDeck login.
// Its role is usable only with the exact authenticated E2EE Channel on which
// the Agent independently consumed the signed Portal ticket.
type AgentGrant struct {
 deviceID string
 issuer string
 verifier *AgentGrantVerifier
 channel *remotetransport.Channel
 role remotetransport.Role
 subject string
 ticketJTI string
 authEpoch int64
 expiresAt time.Time
 closed atomic.Bool
 // The Agent owns one-shot admission before any Portal request or callback.
 // TaskDeck login consumes loginUsed separately after decrypting credentials.
 loginStarted atomic.Bool
 loginUsed atomic.Bool
}

// BelongsToDevice checks the authenticated signed grant's target without
// exposing ticket/user credentials to the TaskDeck session layer.
func (g *AgentGrant) BelongsToAgent(id, portalOrigin string) bool {
	return g!=nil && g.deviceID!="" && g.deviceID==id &&
		g.issuer!="" && g.issuer==portalOrigin && !g.closed.Load()
}

func (g *AgentGrant) Close() {if g!=nil {g.closed.Store(true)}}

// RoleFor refuses disconnected, substituted or expired channel bindings.
func (g *AgentGrant) RoleFor(channel *remotetransport.Channel,now time.Time) (remotetransport.Role,error) {
 if g==nil || g.closed.Load() || channel==nil || channel!=g.channel ||
  !channel.Established() || !now.Before(g.expiresAt) {
  return "",remotetransport.ErrRemoteActionDenied
 }
 return g.role,nil
}

// TakeForLogin atomically permits at most one TaskDeck password attempt per
// Portal ticket, even when a previously submitted password is incorrect.
// A browser cannot recycle one valid Portal ticket for repeated guesses.
func (g *AgentGrant) TakeForLogin(channel *remotetransport.Channel,now time.Time) (remotetransport.Role,error) {
 role,err:=g.RoleFor(channel,now)
 if err!=nil{return "",err}
 // A signed Portal ticket + P-256 public key is not proof that the browser
 // possesses its private scalar. TaskDeck login is forbidden until the Agent
 // has successfully authenticated actual browser ciphertext with AES-GCM.
 if !channel.AuthenticatedPeerTraffic(){
  return "",remotetransport.ErrRemoteActionDenied
 }
 if !g.loginUsed.CompareAndSwap(false,true) {
  return "",remotetransport.ErrRemoteActionDenied
 }
 return role,nil
}

// Consume binds the signed Portal grant to exactly one authenticated E2EE
// channel. It cannot authorize TaskDeck commands without separate TaskDeck
// credentials, project membership, RBAC and command-path checks.
func (v *AgentGrantVerifier) Consume(ticket string,channel *remotetransport.Channel,now time.Time) (*AgentGrant,error) {
 if v==nil || channel==nil || !channel.Established() ||
  channel.BoundDeviceID()!=v.deviceID {
  return nil,remotetransport.ErrRemoteActionDenied
 }
 claims,err:=v.verifier.ConsumeForAgent(ticket,channel.BrowserTicketBinding(),v.deviceID,now)
 if err!=nil {
  return nil,remotetransport.ErrRemoteActionDenied
 }
 return &AgentGrant{deviceID:v.deviceID,issuer:claims.Issuer,
  verifier:v,channel:channel,
  role:remotetransport.Role(claims.Role),
  subject:claims.Subject,ticketJTI:claims.JTI,authEpoch:claims.AuthEpoch,
  expiresAt:time.Unix(claims.Expires,0).UTC()},nil
}
