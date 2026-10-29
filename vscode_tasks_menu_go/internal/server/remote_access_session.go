package server

import (
	"context"
	"crypto/subtle"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"bletonfc/vscode_tasks_menu/internal/identity"
	"bletonfc/vscode_tasks_menu/internal/remoteportal"
 "bletonfc/vscode_tasks_menu/internal/remoteclientkey"
	"bletonfc/vscode_tasks_menu/internal/remotetransport"
)

const remoteTaskDeckSessionLifetime = 5 * time.Minute

// remoteTaskDeckSession is NOT an HTTP cookie or transferable bearer token.
// It is only held inside the TaskDeck daemon, bound to one authenticated E2EE
// channel, and cannot be serialized or reused with a different channel.
type remoteTaskDeckSession struct {
	channel *remotetransport.Channel
	userID identity.ID
 // Set only by the owner-approved client-key login entrypoint.
 clientPublicKey string
 clientUsername string
	projectKey string
	role remotetransport.Role
	// Snapshot of the local TaskDeck credential verified with scrypt.
	// It is an internal SHA-256 commitment to TaskDeck's stored password
	// hash (never plaintext), and MUST match on each future action even
	// if a password reset uses a backdated/absent PasswordChangedAt.
	localPasswordCommitment [32]byte
	localUserUpdatedAt time.Time
	createdAt time.Time
	expiresAt time.Time
	closed atomic.Bool
	// Lock protects admission of one bounded action against concurrent Close.
	// activeActionCancel is NEVER a serializable bearer/Portal credential.
	executionMu sync.Mutex
	activeActionCancel context.CancelFunc
	// Local-only lifetime cancellation signal. No cookie/token is issued.
	lifetimeDone chan struct{}
	portalGrant *remoteportal.AgentGrant
	portalCheckMu sync.RWMutex
	// No live Portal authorizer means NO TaskDeck application permission.
	// Never substitute a cached Portal role or the original signed ticket.
	livePortalCheck func(context.Context,*remoteportal.AgentGrant) error
}

// closeWithoutChannel retires ONLY the ephemeral local principal. A one-shot
// login probe must seal its final boolean after destroying that principal,
// but the Agent still owns the Portal grant and E2EE transport until the
// second live Portal introspection completes. Closing the grant here would
// make legitimate cleanup indistinguishable from a concurrent revocation.
// Never use this narrower cleanup for a long-lived application session.
func (session *remoteTaskDeckSession) closeWithoutChannel() {
 if session==nil{return}
 session.executionMu.Lock()
 if !session.closed.Swap(true) && session.lifetimeDone!=nil {
  close(session.lifetimeDone)
 }
 if session.activeActionCancel!=nil {session.activeActionCancel()}
 session.executionMu.Unlock()
}

func (session *remoteTaskDeckSession) Close() {
	if session==nil {return}
	// Atomically block new action admission and cancel the currently active
	// operation. Network checks must never hold this mutex: Close must work
	// even when the Portal is hung or malicious.
	session.executionMu.Lock()
	if !session.closed.Swap(true) && session.lifetimeDone!=nil {
		close(session.lifetimeDone)
	}
	if session.activeActionCancel!=nil {session.activeActionCancel()}
	session.executionMu.Unlock()
	if session.portalGrant!=nil {session.portalGrant.Close()}
	// Long-lived application sessions own their E2EE lifetime. Revocation
	// closes the AEAD channel; the login-only probe is the explicit exception.
	if session.channel!=nil {session.channel.Close()}
}

// startRemoteSessionLifetime is used only after an encrypted login has
// completed its local TaskDeck verification and live Agent binding. The
// session is never persisted or serialized: physical channel loss, grant
// revocation, local logout or the five-minute deadline destroys it.
func (session *remoteTaskDeckSession) startRemoteSessionLifetime() {
 if session==nil || session.channel==nil || session.portalGrant==nil ||
  session.lifetimeDone==nil || session.closed.Load(){return}
 go func(){
  remaining:=time.Until(session.expiresAt)
  if remaining<=0 {session.Close();return}
  timer:=time.NewTimer(remaining)
  defer timer.Stop()
  select{
  case <-session.lifetimeDone:return
  case <-session.channel.Done():
  case <-session.portalGrant.Done():
  case <-timer.C:
  }
  session.Close()
 }()
}

// bindPortalAgent is only for an authenticated local Agent that already
// consumed the signed Portal ticket for this E2EE channel. Every call to
// authorizeRemoteAuthenticatedSession then performs fresh signed HTTPS
// introspection, including revocation/disable/MFA-generation checks.
// No Agent binding or failed Portal request => no application permission.
func (session *remoteTaskDeckSession) bindPortalAgent(agent *remoteportal.Agent) error {
	if session==nil || session.closed.Load() || agent==nil ||
		!agent.OwnsPortalGrant(session.portalGrant) {
		return remotetransport.ErrRemoteActionDenied
	}
	session.portalCheckMu.Lock()
	defer session.portalCheckMu.Unlock()
	// Never silently swap a previously pinned Agent for a different Portal
	// or local Device ID after the authenticated session starts.
	if session.closed.Load() || session.livePortalCheck!=nil {
		return remotetransport.ErrRemoteActionDenied
	}
	session.livePortalCheck=agent.VerifyCurrentPortalGrant
	return nil
}

// auditRemoteTaskDeckLogin records only the outcome and authenticated
// project/user identifier. No username, password, encrypted payload, Portal
// ticket, E2EE transcript or guessed remote IP is written to the audit trail.
func (s *Server) auditRemoteTaskDeckLogin(principal *identity.Principal, outcome string) error {
	ctx,cancel:=context.WithTimeout(context.Background(),3*time.Second)
	defer cancel()
	project,err:=s.Identity.ProjectByKey(ctx,s.Config.SharedProjectID)
	if err!=nil{return err}
	eventID,err:=identity.NewID()
	if err!=nil{return err}
	projectID:=project.ID
	var userID *identity.ID
	if principal!=nil {
		id:=principal.UserID
		userID=&id
	}
	return s.Identity.AppendAudit(ctx,identity.AuditEvent{
		ID:eventID,Timestamp:time.Now().UTC(),
		UserID:userID,ProjectID:&projectID,
		Action:"auth.remote.login",ResourceType:"session",
		Result:outcome,Details:`{"transport":"e2ee","taskdeck_auth":true}`,
	})
}

// revalidateRemoteLoginSnapshot prevents a correct *old* password from
// authenticating after an administrator disables, resets or replaces the
// TaskDeck-local account while scrypt is verifying it. A signed Portal grant
// never overrides the local password store. Include timestamps so a
// disable/re-enable or same-hash rotation cannot revive an old snapshot.
//
// This check is deliberately fail-closed, and must run after scrypt and again
// immediately before logging a successful one-shot result. The full remote
// dispatcher remains disabled: future actions still need per-action RBAC
// and live Portal introspection.
func (s *Server) revalidateRemoteLoginSnapshot(ctx context.Context, verified identity.User) error {
	if s==nil || s.Identity==nil || !verified.Enabled || verified.ID=="" {
		return remotetransport.ErrRemoteActionDenied
	}
	current,err:=s.Identity.UserByID(ctx,verified.ID)
	if err!=nil || !current.Enabled ||
		subtle.ConstantTimeCompare([]byte(current.PasswordHash),[]byte(verified.PasswordHash))!=1 ||
		!current.UpdatedAt.Equal(verified.UpdatedAt) {
		return remotetransport.ErrRemoteActionDenied
	}
	if (current.PasswordChangedAt==nil)!=(verified.PasswordChangedAt==nil) {
		return remotetransport.ErrRemoteActionDenied
	}
	if current.PasswordChangedAt!=nil &&
		!current.PasswordChangedAt.Equal(*verified.PasswordChangedAt) {
		return remotetransport.ErrRemoteActionDenied
	}
	return nil
}

// authenticateRemoteTaskDeckCredentials is a deliberately unmounted backend
// for a future E2EE-only login dispatcher. Callers MUST supply the Portal role
// from a VALIDATED signed ticket, not from browser-submitted untrusted JSON.
//
// The role parameter is no longer caller-supplied: only a signed Portal grant
// that Agent consumed and bound to this E2EE channel can enter this function.
// It reuses the same scrypt verifier, project membership and login concurrency
// bound as normal TaskDeck shared-server login. Unlike normal browser login,
// it creates NO transferable TaskDeck cookie and persists NO token to Portal.
func (s *Server) authenticateRemoteTaskDeckCredentials(
	ctx context.Context, channel *remotetransport.Channel, grant *remoteportal.AgentGrant,
	username, password string,
) (*remoteTaskDeckSession,error) {
	if s==nil || !s.Config.RemoteAccessEnabled || !s.Config.SharedServerEnabled ||
		!s.Config.TLS() || s.Identity==nil || s.Config.SharedProjectID=="" ||
		channel==nil || !channel.Established() {
		return nil,remotetransport.ErrRemoteActionDenied
	}
	role,grantErr:=grant.TakeForLogin(channel,time.Now())
	if grantErr!=nil || (role!=remotetransport.RoleViewer &&
		role!=remotetransport.RoleOperator && role!=remotetransport.RoleAdmin) {
		return nil,remotetransport.ErrRemoteActionDenied
	}
	username=strings.TrimSpace(username)
	if len(username)==0 || len(username)>128 || len(password)==0 || len(password)>4096 {
		return nil,identity.ErrUnauthenticated
	}
	// A rejected attempt is audited without recording credential material.
	// A failed audit fails closed, including for an otherwise valid password.
	deny:=func() (*remoteTaskDeckSession,error) {
		if err:=s.auditRemoteTaskDeckLogin(nil,"denied");err!=nil {
			return nil,remotetransport.ErrRemoteActionDenied
		}
		return nil,identity.ErrUnauthenticated
	}
	// Rate limits are per account, not per ephemeral E2EE key, so creating
	// new channels cannot reset the failed-login counter. No plaintext
	// usernames or passwords are used as rate-limit keys.
	digest:=sha256.Sum256([]byte(remoteclientkey.FoldASCIIUsername(username)))
	rateKey:="remote-taskdeck-login:"+hex.EncodeToString(digest[:])
	if blocked,_:=s.authBlockedKey(rateKey,time.Now());blocked {
		return deny()
	}
	if !s.beginSharedLogin() {
		return deny()
	}
	defer s.endSharedLogin()
	// Reserve a failed attempt *before* the expensive scrypt operation,
	// matching sharedLogin's defense against parallel brute-force requests.
	s.authRecordFailureKey(rateKey,time.Now())
	ctx,cancel:=context.WithTimeout(ctx,12*time.Second)
	defer cancel()
	user,lookupErr:=s.Identity.UserByUsername(ctx,username)
	if lookupErr!=nil && !errors.Is(lookupErr,identity.ErrNotFound) {
		return deny()
	}
	hash:=sharedDummyPasswordHash
	if lookupErr==nil { hash=user.PasswordHash }
	verified,verifyErr:=identity.VerifyPassword(ctx,password,hash)
	password=""
	if verifyErr!=nil || lookupErr!=nil || !verified || !user.Enabled {
		return deny()
	}
	// Local password verification can take seconds. Recheck the exact
	// TaskDeck account state rather than trusting the pre-scrypt snapshot.
	if err:=s.revalidateRemoteLoginSnapshot(ctx,user);err!=nil {
		return deny()
	}
	principal,err:=identity.ResolveUserPrincipal(ctx,s.Identity,s.Config.SharedProjectID,user)
	if err!=nil || principal.UserID=="" || principal.ProjectKey!=s.Config.SharedProjectID {
		return deny()
	}
	// Membership resolution and audit I/O can block; a second local check
	// closes the most common identity race before reporting a success.
	if err:=s.revalidateRemoteLoginSnapshot(ctx,user);err!=nil {
		return deny()
	}
	if err:=s.auditRemoteTaskDeckLogin(&principal,"success");err!=nil {
		return nil,remotetransport.ErrRemoteActionDenied
	}
	s.authRecordSuccessKey(rateKey)
	now:=time.Now().UTC()
	return &remoteTaskDeckSession{
		channel:channel,userID:principal.UserID,
		projectKey:principal.ProjectKey,role:role,
		createdAt:now,expiresAt:now.Add(remoteTaskDeckSessionLifetime),
		localPasswordCommitment:sha256.Sum256([]byte(user.PasswordHash)),
		localUserUpdatedAt:user.UpdatedAt,
		portalGrant:grant,lifetimeDone:make(chan struct{}),
	},nil
}

// Keep the comparison fixed-length. The scrypt/argon password verifier
// material remains in the TaskDeck identity database, never in the PHP
// Portal, Relay, or a remote browser response.
func sha256DigestOfLocalPasswordHash(value string) []byte {
	sum:=sha256.Sum256([]byte(value))
	return sum[:]
}


// verifyRemoteSessionLocalPrincipal independently checks the account and
// project membership snapshot that was established using a REAL local
// TaskDeck password. A compromised PHP Portal can report allowed=true
// forever, so these checks run again after any asynchronous HTTPS wait,
// including the SECOND login-stage grant check. Never cache their result
// across a network round trip or issue a session after password rotation.
func (s *Server) verifyRemoteSessionLocalPrincipal(ctx context.Context,
 session *remoteTaskDeckSession) (identity.Principal,error) {
 var zero identity.Principal
 if s==nil || s.Identity==nil || session==nil || session.closed.Load() ||
  session.channel==nil || !session.channel.Established() ||
  session.portalGrant==nil || !session.portalGrant.ActiveForSession(session.channel) ||
  session.projectKey=="" || session.projectKey!=s.Config.SharedProjectID ||
  !time.Now().Before(session.expiresAt) || ctx==nil || ctx.Err()!=nil {
  return zero,remotetransport.ErrRemoteActionDenied
 }
 user,err:=s.Identity.UserByID(ctx,session.userID)
 if err!=nil || !user.Enabled ||
  subtle.ConstantTimeCompare(
   sha256DigestOfLocalPasswordHash(user.PasswordHash),
   session.localPasswordCommitment[:])!=1 ||
  !user.UpdatedAt.Equal(session.localUserUpdatedAt) ||
  (user.PasswordChangedAt!=nil && !user.PasswordChangedAt.Before(session.createdAt)) {
  return zero,remotetransport.ErrRemoteActionDenied
 }
 // The host's local CLI may revoke a software client key at any time.
 // A previously password-authenticated application session cannot keep
 // operating after that key is removed. Legacy internal test fixtures
 // without an externally admitted key never establish public Relay routes.
 if session.clientPublicKey!="" {
  path,e:=remoteclientkey.DefaultPath(s.Workspace)
  if e!=nil || !remoteclientkey.SameTaskDeckUsername(session.clientUsername,user.Username) ||
   remoteclientkey.IsApproved(path,session.clientUsername,session.clientPublicKey)!=nil {
   return zero,remotetransport.ErrRemoteActionDenied
  }
 }
 principal,err:=identity.ResolveUserPrincipal(ctx,s.Identity,session.projectKey,user)
 if err!=nil || principal.UserID!=session.userID || principal.ProjectKey!=session.projectKey ||
  ctx.Err()!=nil || session.closed.Load() ||
  !session.portalGrant.ActiveForSession(session.channel) {
  return zero,remotetransport.ErrRemoteActionDenied
 }
 return principal,nil
}

// authorizeRemoteAuthenticatedSession MUST be invoked for EVERY future remote
// application action, not only once at login. Role/membership/permissions
// are resolved again from the authoritative TaskDeck identity store, so
// changes to password, account status and project grants take effect.
func (s *Server) authorizeRemoteAuthenticatedSession(
	ctx context.Context, session *remoteTaskDeckSession,
	channel *remotetransport.Channel, permission string,
) error {
	if s==nil || session==nil || session.closed.Load() ||
		!s.Config.RemoteAccessEnabled || !s.Config.SharedServerEnabled ||
		!s.Config.TLS() || s.Identity==nil || s.Config.SharedProjectID=="" ||
		channel==nil || !channel.Established() ||
		channel!=session.channel || session.projectKey!=s.Config.SharedProjectID ||
		!time.Now().Before(session.expiresAt) || session.portalGrant==nil {
		return remotetransport.ErrRemoteActionDenied
	}
	session.portalCheckMu.RLock()
	liveCheck:=session.livePortalCheck
	session.portalCheckMu.RUnlock()
	if liveCheck==nil {return remotetransport.ErrRemoteActionDenied}
	checkCtx,cancel:=context.WithTimeout(ctx,8*time.Second)
	defer cancel()
	// The live Portal grant must be rechecked BEFORE every local TaskDeck
	// permission evaluation. This is deliberately uncached: a revocation
	// between two actions takes effect at the next request. Network failure
	// is a denial; the previously issued signed ticket is not a substitute.
	if err:=liveCheck(checkCtx,session.portalGrant);err!=nil {
		return remotetransport.ErrRemoteActionDenied
	}
	principal,err:=s.verifyRemoteSessionLocalPrincipal(checkCtx,session)
	if err!=nil {return remotetransport.ErrRemoteActionDenied}
	if err:=remotetransport.AuthorizeAction(true,session.role,
		&principal,s.Config.SharedProjectID,permission);err!=nil {
		return err
	}
	// Portal introspection and TaskDeck identity reads can block. Close,
	// channel disconnection or expiry during those calls must veto the
	// decision; the previous initial check alone is insufficient.
	if session.closed.Load() || !session.portalGrant.ActiveForSession(channel) ||
		!time.Now().Before(session.expiresAt) {
		return remotetransport.ErrRemoteActionDenied
	}
	return nil
}
