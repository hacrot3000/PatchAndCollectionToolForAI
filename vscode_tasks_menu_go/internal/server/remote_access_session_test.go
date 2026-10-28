package server

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/config"
	"bletonfc/vscode_tasks_menu/internal/identity"
	"bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func remoteSessionTestChannel(t *testing.T) *remotetransport.Channel {
	t.Helper()
	pub,private,err:=ed25519.GenerateKey(rand.Reader)
	if err!=nil{t.Fatal(err)}
	digest:=sha256.Sum256(pub)
	device:=hex.EncodeToString(digest[:])
	browser,err:=remotetransport.NewEphemeral()
	if err!=nil{t.Fatal(err)}
	hello,agent,err:=remotetransport.ClientHello(private,device,
		strings.Repeat("a",32),strings.Repeat("b",32),browser.Public)
	if err!=nil{t.Fatal(err)}
	_,err=remotetransport.AcceptHello(browser,device,
		strings.Repeat("a",32),strings.Repeat("b",32),hello)
	if err!=nil{t.Fatal(err)}
	if !agent.Established(){t.Fatal("channel was not established")}
	return agent
}

func TestRemoteTaskDeckPasswordLoginBoundToEncryptedChannel(t *testing.T) {
	ctx:=context.Background()
	s:=sharedLoginTestServer(t)
	s.Config.RemoteAccessEnabled=true
	s.Config.Protocol=config.ProtocolHTTPS
	channel:=remoteSessionTestChannel(t)
	other:=remoteSessionTestChannel(t)
	// The caller must not accept a Portal role as TaskDeck authentication.
	if _,err:=s.authenticateRemoteTaskDeckCredentials(ctx,nil,remotetransport.RoleAdmin,
		"alice","private-admin-password");err==nil {
		t.Fatal("authenticated TaskDeck credentials accepted without E2EE channel")
	}
	if _,err:=s.authenticateRemoteTaskDeckCredentials(ctx,&remotetransport.Channel{},
		remotetransport.RoleAdmin,"alice","private-admin-password");err==nil {
		t.Fatal("zero-valued unencrypted channel passed login")
	}
	if _,err:=s.authenticateRemoteTaskDeckCredentials(ctx,channel,remotetransport.Role("owner"),
		"alice","private-admin-password");err==nil {
		t.Fatal("unknown Portal role accepted")
	}
	if _,err:=s.authenticateRemoteTaskDeckCredentials(ctx,channel,remotetransport.RoleAdmin,
		"alice","wrong-password");err==nil {
		t.Fatal("incorrect TaskDeck credentials accepted")
	}
	session,err:=s.authenticateRemoteTaskDeckCredentials(ctx,channel,remotetransport.RoleViewer,
		"alice","private-admin-password")
	if err!=nil{t.Fatalf("existing TaskDeck account rejected: %v",err)}
	defer session.Close()
	if session.userID=="" || session.channel!=channel || session.role!=remotetransport.RoleViewer {
		t.Fatal("remote session did not bind to TaskDeck identity and encrypted channel")
	}
	if err:=s.authorizeRemoteAuthenticatedSession(ctx,session,channel,identity.PermissionFilesRead);err!=nil{
		t.Fatalf("TaskDeck viewer read rejected: %v",err)
	}
	for _,tc:=range []struct{
		name string
		ch *remotetransport.Channel
		permission string
	}{
		{"viewer cannot run",channel,identity.PermissionTasksRun},
		{"viewer cannot open terminal",channel,identity.PermissionTerminalCreate},
		{"other E2EE session cannot reuse login",other,identity.PermissionFilesRead},
		{"zero E2EE cannot reuse login",&remotetransport.Channel{},identity.PermissionFilesRead},
		{"unknown scope",channel,"remote.execute_all"},
	}{
		t.Run(tc.name,func(t *testing.T){
			if err:=s.authorizeRemoteAuthenticatedSession(ctx,session,tc.ch,tc.permission);
				!errors.Is(err,remotetransport.ErrRemoteActionDenied) {
				t.Fatalf("unauthorized remote operation succeeded: %v",err)
			}
		})
	}
	s.Config.SharedProjectID="other-project"
	if err:=s.authorizeRemoteAuthenticatedSession(ctx,session,channel,identity.PermissionFilesRead);err==nil{
		t.Fatal("cross-project session reuse accepted")
	}
	s.Config.SharedProjectID="test-project"
	s.Config.RemoteAccessEnabled=false
	if err:=s.authorizeRemoteAuthenticatedSession(ctx,session,channel,identity.PermissionFilesRead);err==nil{
		t.Fatal("disabled Remote permitted existing session")
	}
	s.Config.RemoteAccessEnabled=true
	session.Close()
	if err:=s.authorizeRemoteAuthenticatedSession(ctx,session,channel,identity.PermissionFilesRead);err==nil{
		t.Fatal("closed encrypted session still usable")
	}
}

func TestRemoteTaskDeckPermissionsRecheckedAfterAccountDisable(t *testing.T) {
	ctx:=context.Background()
	s:=sharedLoginTestServer(t)
	s.Config.RemoteAccessEnabled=true
	s.Config.Protocol=config.ProtocolHTTPS
	ch:=remoteSessionTestChannel(t)
	session,err:=s.authenticateRemoteTaskDeckCredentials(ctx,ch,remotetransport.RoleAdmin,
		"alice","private-admin-password")
	if err!=nil{t.Fatal(err)}
	defer session.Close()
	if err:=s.authorizeRemoteAuthenticatedSession(ctx,session,ch,identity.PermissionProjectAdmin);err!=nil{
		t.Fatalf("admin permission unexpectedly denied: %v",err)
	}
	if err:=s.Identity.SetUserEnabled(ctx,session.userID,false,time.Now().UTC());err!=nil{
		t.Fatal(err)
	}
	if err:=s.authorizeRemoteAuthenticatedSession(ctx,session,ch,identity.PermissionProjectAdmin);err==nil{
		t.Fatal("disabled TaskDeck account retained remote rights")
	}
}

func TestRemoteTaskDeckSessionExpirationAndPasswordChange(t *testing.T) {
	ctx:=context.Background()
	s:=sharedLoginTestServer(t)
	s.Config.RemoteAccessEnabled=true
	s.Config.Protocol=config.ProtocolHTTPS
	ch:=remoteSessionTestChannel(t)
	session,err:=s.authenticateRemoteTaskDeckCredentials(ctx,ch,remotetransport.RoleAdmin,
		"alice","private-admin-password")
	if err!=nil{t.Fatal(err)}
	defer session.Close()
	originalExpiry:=session.expiresAt
	session.expiresAt=time.Now().Add(-time.Second)
	if err:=s.authorizeRemoteAuthenticatedSession(ctx,session,ch,identity.PermissionTasksView);err==nil{
		t.Fatal("expired remote TaskDeck session accepted")
	}
	session.expiresAt=originalExpiry
	user,err:=s.Identity.UserByID(ctx,session.userID)
	if err!=nil{t.Fatal(err)}
	newHash,err:=identity.HashPassword(ctx,"replacement-secure-password")
	if err!=nil{t.Fatal(err)}
	if err:=s.Identity.ChangeUserPasswordHash(ctx,user.ID,user.PasswordHash,newHash,
		time.Now().UTC());err!=nil{t.Fatal(err)}
	if err:=s.authorizeRemoteAuthenticatedSession(ctx,session,ch,identity.PermissionTasksView);err==nil{
		t.Fatal("password change did not invalidate encrypted remote session")
	}
}

func TestRemoteTaskDeckRequiresSharedAuthenticationAndTLS(t *testing.T) {
	ctx:=context.Background()
	s:=sharedLoginTestServer(t)
	s.Config.RemoteAccessEnabled=true
	s.Config.Protocol=config.ProtocolHTTPS
	ch:=remoteSessionTestChannel(t)
	s.Config.Protocol=config.ProtocolHTTP
	if _,err:=s.authenticateRemoteTaskDeckCredentials(ctx,ch,remotetransport.RoleViewer,
		"alice","private-admin-password");err==nil{
		t.Fatal("plaintext TaskDeck protocol allowed remote login")
	}
	s.Config.Protocol=config.ProtocolHTTPS
	s.Config.SharedServerEnabled=false
	if _,err:=s.authenticateRemoteTaskDeckCredentials(ctx,ch,remotetransport.RoleViewer,
		"alice","private-admin-password");err==nil {
		t.Fatal("legacy single-user login was treated as shared identity login")
	}
}
