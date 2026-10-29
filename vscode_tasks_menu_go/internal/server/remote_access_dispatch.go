package server

import (
 "context"

 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// executeRemoteE2EEPing is the first deliberately UNMOUNTED typed remote
// application operation. It is a one-packet, read-only authenticated ping,
// NOT an HTTP/WS/terminal/file/Git/Patch dispatcher and grants no session.
//
// A forged or stolen PHP Portal admin ticket cannot manufacture session:
// only authenticateRemoteTaskDeckCredentials can create its in-memory
// TaskDeck-local password principal. The exact E2EE identity and channel,
// local per-action permission, account status and Portal live role are
// rechecked both before and after running this fixed operation.
//
// Arbitrary commands/URLs/paths/user-provided callbacks are not accepted.
// The caller MUST forward a sealed response only while its Agent Grant
// and browser connection are still live and must retire the session on
// logout, disconnect, revocation or the 5-minute local deadline.
func (s *Server) executeRemoteE2EEPing(
 ctx context.Context, session *remoteTaskDeckSession,
 channel *remotetransport.Channel, frame []byte,
) ([]byte,error) {
 if s==nil || ctx==nil || session==nil || channel==nil ||
  !channel.Established() || channel!=session.channel ||
  session.closed.Load() || session.portalGrant==nil ||
  !session.portalGrant.ActiveForSession(channel) {
  return nil,remotetransport.ErrRemoteActionDenied
 }
 // Validate the encrypted schema only AFTER checking that this exact
 // TaskDeck-local password session owns this authenticated channel.
 // A badly typed but authenticated record permanently poisons the E2EE key.
 request,err:=remotetransport.OpenRemoteAction(channel,frame)
 if err!=nil{return nil,remotetransport.ErrRemoteActionDenied}
 if request.Operation!=remotetransport.RemoteActionSessionPing {
  return nil,remotetransport.ErrRemoteActionDenied
 }
 // Ping reveals neither TaskDeck settings nor task data; still require a
 // genuine project permission to exercise the same locally authorized
 // path that future read-only handlers must use.
 err=s.executeRemoteAuthenticatedAction(ctx,session,channel,
  identity.PermissionTasksView,func(actionCtx context.Context)error {
   return actionCtx.Err()
  })
 if err!=nil{return nil,remotetransport.ErrRemoteActionDenied}
 // Serialize final sealing with local session Close, which cancels active
 // work and destroys the Channel. This does not replace the upstream
 // Agent's final revocation check before forwarding a response.
 session.executionMu.Lock()
 defer session.executionMu.Unlock()
 if session.closed.Load() || !session.portalGrant.ActiveForSession(channel) ||
  ctx.Err()!=nil {
  return nil,remotetransport.ErrRemoteActionDenied
 }
 sealed,err:=remotetransport.SealRemoteActionPingResult(channel,request.RequestID)
 if err!=nil{return nil,remotetransport.ErrRemoteActionDenied}
 return sealed,nil
}
