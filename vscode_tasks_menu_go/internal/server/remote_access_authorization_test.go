package server

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/config"
	"bletonfc/vscode_tasks_menu/internal/identity"
	"bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// A compromised PHP Portal can mint "admin" tickets, supply arbitrary
// roles/grants, or entice a caller to reuse a normal TaskDeck HTTP cookie.
// NONE of these are a TaskDeck-local password-authenticated E2EE session.
// The staged dispatcher must never convert a regular HTTP Principal into
// a remote application session.
func TestRemoteTransportNeverAcceptsHTTPPrincipalOrCompromisedPortalTicket(t *testing.T) {
	ctx:=context.Background()
	s:=sharedLoginTestServer(t)
	s.Config.RemoteAccessEnabled=true
	s.Config.Protocol=config.ProtocolHTTPS

	r:=httptest.NewRequest(http.MethodGet,"https://taskdeck.local/api/tasks",nil)
	r.TLS=&tls.ConnectionState{}
	stolenPrincipal:=identity.Principal{
		UserID:"local-user",ProjectKey:s.Config.SharedProjectID,
		Permissions:map[string]bool{
			identity.PermissionTasksView:true,
			identity.PermissionFilesRead:true,
			identity.PermissionTerminalCreate:true,
		},
	}
	// Simulate an ordinary browser TaskDeck cookie, even though its user
	// has broad local permissions. The remote dispatcher has no API to
	// accept this HTTP request or a Portal-supplied role string.
	httpContext:=context.WithValue(r.Context(),sharedPrincipalContextKey{},stolenPrincipal)
	if err:=s.authorizeRemoteTransportAction(httpContext,nil,nil,identity.PermissionTasksView);
		!errors.Is(err,remotetransport.ErrRemoteActionDenied){
		t.Fatalf("ordinary HTTP principal became a remote identity: %v",err)
	}

	ch:=remoteSessionTestChannel(t)
	maliciousPHPAdmin:=testRemoteSignedGrant(t,ch,remotetransport.RoleAdmin)
	if err:=s.authorizeRemoteTransportAction(ctx,nil,ch,identity.PermissionFilesRead);err==nil{
		t.Fatal("Portal admin ticket alone authorized a TaskDeck file read")
	}
	if _,err:=s.authenticateRemoteTaskDeckCredentials(ctx,ch,maliciousPHPAdmin,
		"alice","PHP-attacker-does-not-know-local-password");err==nil{
		t.Fatal("malicious Portal admin role bypassed the local TaskDeck password")
	}
	if err:=s.authorizeRemoteTransportAction(ctx,nil,ch,identity.PermissionFilesRead);err==nil{
		t.Fatal("failed local login still resulted in a remote TaskDeck session")
	}

	// Correct TaskDeck credentials are necessary, but insufficient for any
	// remote action until a real pinned Agent supplies live grant checks.
	safeChannel:=remoteSessionTestChannel(t)
	grant:=testRemoteSignedGrant(t,safeChannel,remotetransport.RoleViewer)
	session,err:=s.authenticateRemoteTaskDeckCredentials(ctx,safeChannel,grant,
		"alice","private-admin-password")
	if err!=nil{t.Fatalf("TaskDeck local password rejected: %v",err)}
	defer session.Close()
	if err:=s.authorizeRemoteTransportAction(httpContext,session,safeChannel,
		identity.PermissionFilesRead);err==nil{
		t.Fatal("password-only session accepted without fresh Portal introspection")
	}
	allowTestLivePortal(session) // Unit fixture; real sessions bind pinned Agent.
	if err:=s.authorizeRemoteTransportAction(ctx,session,safeChannel,
		identity.PermissionFilesRead);err!=nil {
		t.Fatalf("locally authenticated E2EE viewer with live grant denied: %v",err)
	}
	if err:=s.authorizeRemoteTransportAction(ctx,session,safeChannel,
		identity.PermissionTerminalCreate);err==nil {
		t.Fatal("Portal viewer was elevated to TaskDeck terminal permission")
	}
	if err:=s.authorizeRemoteTransportAction(ctx,session,ch,
		identity.PermissionFilesRead);err==nil {
		t.Fatal("authenticated local session was transferred to another E2EE channel")
	}
	session.Close()
	if err:=s.authorizeRemoteTransportAction(ctx,session,safeChannel,
		identity.PermissionFilesRead);err==nil {
		t.Fatal("closed local E2EE session retained remote file access")
	}
}
