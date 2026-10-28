package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"sync/atomic"
	"time"

	"bletonfc/vscode_tasks_menu/internal/identity"
	"bletonfc/vscode_tasks_menu/internal/remotetransport"
)

const remoteTaskDeckSessionLifetime = 5 * time.Minute

// remoteTaskDeckSession is NOT an HTTP cookie or transferable bearer token.
// It is only held inside the TaskDeck daemon, bound to one authenticated E2EE
// channel, and cannot be serialized or reused with a different channel.
type remoteTaskDeckSession struct {
	channel *remotetransport.Channel
	userID identity.ID
	projectKey string
	role remotetransport.Role
	createdAt time.Time
	expiresAt time.Time
	closed atomic.Bool
}

func (session *remoteTaskDeckSession) Close() {
	if session!=nil { session.closed.Store(true) }
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

// authenticateRemoteTaskDeckCredentials is a deliberately unmounted backend
// for a future E2EE-only login dispatcher. Callers MUST supply the Portal role
// from a VALIDATED signed ticket, not from browser-submitted untrusted JSON.
//
// It reuses the same scrypt verifier, project membership and login concurrency
// bound as normal TaskDeck shared-server login. Unlike normal browser login,
// it creates NO transferable TaskDeck cookie and persists NO token to Portal.
func (s *Server) authenticateRemoteTaskDeckCredentials(
	ctx context.Context, channel *remotetransport.Channel, role remotetransport.Role,
	username, password string,
) (*remoteTaskDeckSession,error) {
	if s==nil || !s.Config.RemoteAccessEnabled || !s.Config.SharedServerEnabled ||
		!s.Config.TLS() || s.Identity==nil || s.Config.SharedProjectID=="" ||
		channel==nil || !channel.Established() {
		return nil,remotetransport.ErrRemoteActionDenied
	}
	if role!=remotetransport.RoleViewer && role!=remotetransport.RoleOperator &&
		role!=remotetransport.RoleAdmin {
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
	digest:=sha256.Sum256([]byte(strings.ToLower(username)))
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
	principal,err:=identity.ResolveUserPrincipal(ctx,s.Identity,s.Config.SharedProjectID,user)
	if err!=nil || principal.UserID=="" || principal.ProjectKey!=s.Config.SharedProjectID {
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
	},nil
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
		!time.Now().Before(session.expiresAt) {
		return remotetransport.ErrRemoteActionDenied
	}
	checkCtx,cancel:=context.WithTimeout(ctx,8*time.Second)
	defer cancel()
	user,err:=s.Identity.UserByID(checkCtx,session.userID)
	if err!=nil || !user.Enabled ||
		(user.PasswordChangedAt!=nil && !user.PasswordChangedAt.Before(session.createdAt)) {
		return remotetransport.ErrRemoteActionDenied
	}
	principal,err:=identity.ResolveUserPrincipal(checkCtx,s.Identity,session.projectKey,user)
	if err!=nil || principal.UserID!=session.userID {
		return remotetransport.ErrRemoteActionDenied
	}
	return remotetransport.AuthorizeAction(true,session.role,
		&principal,s.Config.SharedProjectID,permission)
}
