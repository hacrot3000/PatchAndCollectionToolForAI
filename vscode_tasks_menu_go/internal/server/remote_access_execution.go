package server

import (
    "context"
    "time"

    "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// A future Remote Control dispatcher must never call a TaskDeck operation
// directly after an authorization preflight. The authorization and one
// bounded operation are admitted as a single, session-bound lifecycle.
// There is NO transport route or command handler mounted by this file.
//
// Keep this deadline short while the transport has no stream multiplexing:
// long-lived PTYs/SSH and partial file writes are NOT supported.
const remoteActionExecutionDeadline = 15 * time.Second

// executeRemoteAuthenticatedAction is an INTERNAL-ONLY execution gate. It
// rechecks the TaskDeck-local password generation, account, project RBAC and
// uncached Portal grant through authorizeRemoteTransportAction on EVERY call.
// A compromised PHP Portal admin ticket is never an action credential.
//
// Only one action may occupy a local E2EE session. Close() cancels its
// context, and a late positive authorization cannot start the callback after
// session closure. Callbacks MUST honor ctx cancellation BEFORE and DURING
// all operations with externally observable effects; this wrapper cannot
// undo a non-cooperative filesystem write or process launch.
//
// Not a general purpose HTTP/PTY dispatcher: adding action routes requires
// a separate review of capability scope, streaming, cancellation and audit.
func (s *Server) executeRemoteAuthenticatedAction(
    ctx context.Context,
    session *remoteTaskDeckSession,
    channel *remotetransport.Channel,
    permission string,
    action func(context.Context) error,
) error {
    if ctx == nil || session == nil || channel == nil || action == nil {
        return remotetransport.ErrRemoteActionDenied
    }
    if err := s.authorizeRemoteTransportAction(ctx, session, channel, permission); err != nil {
        return err
    }

    actionCtx, cancel := context.WithTimeout(ctx, remoteActionExecutionDeadline)
    session.executionMu.Lock()
    // The asynchronous Portal/TaskDeck authorization may have returned after
    // a concurrent Close(), revocation, or loss of the E2EE channel.
    // Admission is serialized with Close(), not just read from an atomic flag.
    if session.closed.Load() || session.activeActionCancel != nil ||
        session.channel != channel || session.portalGrant == nil ||
        !session.portalGrant.ActiveForSession(channel) || actionCtx.Err() != nil {
        session.executionMu.Unlock()
        cancel()
        return remotetransport.ErrRemoteActionDenied
    }
    session.activeActionCancel = cancel
    session.executionMu.Unlock()
    // An E2EE disconnect, failed AES-GCM record, or local signed Portal
    // grant revocation may occur without session.Close(). Wake the active
    // operation promptly instead of merely rejecting its eventual result.
    go func() {
        select {
        case <-channel.Done():
            cancel()
        case <-session.portalGrant.Done():
            cancel()
        case <-actionCtx.Done():
        }
    }()
    defer func() {
        session.executionMu.Lock()
        session.activeActionCancel = nil
        session.executionMu.Unlock()
        cancel()
    }()

    // A callback can be admitted just before concurrent Close(). A compliant
    // callback must inspect this context before any side effect.
    if actionCtx.Err() != nil {
        return remotetransport.ErrRemoteActionDenied
    }
    err := action(actionCtx)

    // A slow operation may span Portal grant revocation, TaskDeck password
    // rotation, account disabling or local project RBAC changes. Perform a
    // SECOND uncached Portal + TaskDeck-local permission check before a
    // response is accepted. This cannot undo side effects, so destructive
    // commands still require a separate cancellable transactional adapter.
    if actionCtx.Err() != nil || session.closed.Load() ||
        !session.portalGrant.ActiveForSession(channel) {
        return remotetransport.ErrRemoteActionDenied
    }
    if recheckErr:=s.authorizeRemoteTransportAction(actionCtx,session,channel,permission);recheckErr!=nil {
        return remotetransport.ErrRemoteActionDenied
    }
    if actionCtx.Err()!=nil || session.closed.Load() ||
        !session.portalGrant.ActiveForSession(channel) {
        return remotetransport.ErrRemoteActionDenied
    }
    return err
}
