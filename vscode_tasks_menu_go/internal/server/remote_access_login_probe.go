package server

import (
	"context"
	"time"

	"bletonfc/vscode_tasks_menu/internal/remoteportal"
	"bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// VerifyEncryptedRemoteLogin is a narrow, INTERNAL-ONLY authentication probe.
// It can be called only after Agent independently verifies a Portal-signed
// ticket bound to the established browser E2EE channel.
//
// The transport frame contains one strictly typed username/password request.
// It is decrypted here (inside the TaskDeck process), NEVER by Relay/Portal.
// A valid login produces ONLY an encrypted boolean: it does not return an
// HTTP session, cookie, bearer, CSRF token, project data or any TaskDeck API.
// The temporary session is destroyed immediately, even on success.
// This helper is NOT currently exposed by HTTP, Agent or browser messaging.
func (s *Server) VerifyEncryptedRemoteLogin(
	ctx context.Context,
	channel *remotetransport.Channel,
	grant *remoteportal.AgentGrant,
	frame []byte,
) ([]byte,error) {
	if s==nil || channel==nil || !channel.Established() ||
		grant==nil || !s.Config.RemoteAccessEnabled ||
		!s.Config.SharedServerEnabled || !s.Config.TLS() || s.Identity==nil {
		return nil,remotetransport.ErrRemoteActionDenied
	}
	// Reject untrusted/mismatched/expired Portal grants before decrypting a
	// credential envelope, but burn the grant only when attempting login.
	if _,err:=grant.RoleFor(channel,time.Now());err!=nil {
		// A replay against a completed one-shot login, a closed grant, or
		// a mismatched channel is a protocol violation. The verifier must
		// not leave an apparently usable AEAD channel after this rejection.
		channel.Close()
		return nil,remotetransport.ErrRemoteActionDenied
	}
	credentials,err:=remotetransport.OpenRemoteLogin(channel,frame)
	if err!=nil {return nil,remotetransport.ErrRemoteActionDenied}
	session,authErr:=s.authenticateRemoteTaskDeckCredentials(
		ctx,channel,grant,credentials.Username,credentials.Password)
	credentials.Password=""
	authenticated:=authErr==nil && session!=nil
	// No TaskDeck principal or application session may outlive this probe.
	// The browser still needs ONE encrypted boolean reply; a full Close()
	// would poison this channel before SealRemoteLoginResult can send it.
	// The Agent remains the owner of the WebSocket/E2EE channel and closes
	// it immediately after forwarding the result.
	if session!=nil {session.closeWithoutChannel()}
	return remotetransport.SealRemoteLoginResult(channel,authenticated)
}
