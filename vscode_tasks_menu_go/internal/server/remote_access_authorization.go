package server

import (
	"context"

	"bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// authorizeRemoteTransportAction is the sole staged adapter for a future
// encrypted application dispatcher. A regular TaskDeck HTTP request, its
// cookie-based PrincipalFromContext, or a Portal-supplied role is NOT a
// remote authorization credential. PHP Portal is an untrusted rendezvous
// coordinator even when it controls its ticket-signing key.
//
// Each future action must supply the exact in-process session returned by
// TaskDeck's local password verifier plus the original authenticated E2EE
// channel. authorizeRemoteAuthenticatedSession checks live Portal grant,
// project membership, TaskDeck RBAC and local account revocation again on
// EVERY call. No HTTP routing/command dispatch is installed here.
func (s *Server) authorizeRemoteTransportAction(
	ctx context.Context, session *remoteTaskDeckSession,
	channel *remotetransport.Channel, permission string,
) error {
	if s==nil || ctx==nil || session==nil || channel==nil {
		return remotetransport.ErrRemoteActionDenied
	}
	return s.authorizeRemoteAuthenticatedSession(ctx,session,channel,permission)
}
