package server

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/config"
	"bletonfc/vscode_tasks_menu/internal/identity"
	"bletonfc/vscode_tasks_menu/internal/remoteportal"
	"bletonfc/vscode_tasks_menu/internal/remoterelay"
	"bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func remoteSessionTestChannel(t *testing.T) *remotetransport.Channel {
	t.Helper()
	_,private,err:=ed25519.GenerateKey(rand.Reader)
	if err!=nil{t.Fatal(err)}
	return remoteSessionTestChannelWithPrivate(t,private)
}
func remoteSessionTestChannelWithPrivate(t *testing.T,private ed25519.PrivateKey) *remotetransport.Channel {
	t.Helper()
	pub:=private.Public().(ed25519.PublicKey)
	digest:=sha256.Sum256(pub)
	device:=hex.EncodeToString(digest[:])
	browser,err:=remotetransport.NewEphemeral()
	if err!=nil{t.Fatal(err)}
	hello,agent,err:=remotetransport.ClientHello(private,device,
		strings.Repeat("a",32),strings.Repeat("b",32),browser.Public)
	if err!=nil{t.Fatal(err)}
	client,err:=remotetransport.AcceptHello(browser,device,
		strings.Repeat("a",32),strings.Repeat("b",32),hello)
	if err!=nil{t.Fatal(err)}
	if !agent.Established(){t.Fatal("channel was not established")}
	encrypted,err:=client.Seal([]byte("authenticated-browser-possession"))
	if err!=nil{t.Fatal(err)}
	if _,err:=agent.Open(encrypted);err!=nil{t.Fatal(err)}
	return agent
}

// A real Portal-signed ticket is required even in TaskDeck login tests;
// never construct an E2EE session by injecting an arbitrary role string.
func testRemoteSignedGrant(t *testing.T, channel *remotetransport.Channel,
 role remotetransport.Role, override ...string) *remoteportal.AgentGrant {
 t.Helper()
 pub,sk,err:=ed25519.GenerateKey(rand.Reader)
 if err!=nil{t.Fatal(err)}
 device:=channel.BoundDeviceID()
 issuer:="https://portal.example.com"
 if len(override)==2 {device=override[0];issuer=override[1]}
 verifier,err:=remoteportal.NewAgentGrantVerifier(
  base64.StdEncoding.EncodeToString(pub),issuer,device)
 if err!=nil{t.Fatal(err)}
 now:=time.Now()
 claims:=remoterelay.Claims{
  V:2,BrowserBinding:channel.BrowserTicketBinding(),Issuer:issuer,Audience:"taskdeck-relay",
  DeviceID:device,Role:string(role),Subject:"21",AuthEpoch:1,
  IssuedAt:now.Unix(),Expires:now.Unix()+30,JTI:strings.Repeat("b",32),
 }
 payload,err:=json.Marshal(claims)
 if err!=nil{t.Fatal(err)}
 body:=base64.RawURLEncoding.EncodeToString(payload)
 signature:=ed25519.Sign(sk,[]byte("taskdeck-relay-ticket-v1."+body))
 ticket:=body+"."+base64.RawURLEncoding.EncodeToString(signature)
 grant,err:=verifier.Consume(ticket,channel,now)
 if err!=nil{t.Fatal(err)}
 return grant
}

// Tests of local TaskDeck permissions explicitly provide a successful
// live-Portal check. Production sessions have no such fallback: they must
// bind the actual HTTPS Agent with bindPortalAgent before any action can pass.
func allowTestLivePortal(session *remoteTaskDeckSession) {
	session.livePortalCheck=func(_ context.Context, grant *remoteportal.AgentGrant) error {
		if grant==nil{return remotetransport.ErrRemoteActionDenied}
		return nil
	}
}

func TestRemoteTaskDeckPasswordLoginBoundToEncryptedChannel(t *testing.T) {
	ctx:=context.Background()
	s:=sharedLoginTestServer(t)
	s.Config.RemoteAccessEnabled=true
	s.Config.Protocol=config.ProtocolHTTPS
	channel:=remoteSessionTestChannel(t)
	other:=remoteSessionTestChannel(t)
	// The caller must not accept a Portal role as TaskDeck authentication.
	if _,err:=s.authenticateRemoteTaskDeckCredentials(ctx,nil,testRemoteSignedGrant(t,channel,remotetransport.RoleAdmin),
		"alice","private-admin-password");err==nil {
		t.Fatal("authenticated TaskDeck credentials accepted without E2EE channel")
	}
	if _,err:=s.authenticateRemoteTaskDeckCredentials(ctx,&remotetransport.Channel{},
		testRemoteSignedGrant(t,channel,remotetransport.RoleAdmin),"alice","private-admin-password");err==nil {
		t.Fatal("zero-valued unencrypted channel passed login")
	}
	if _,err:=s.authenticateRemoteTaskDeckCredentials(ctx,channel,nil,
		"alice","private-admin-password");err==nil {
		t.Fatal("unknown Portal role accepted")
	}
	if _,err:=s.authenticateRemoteTaskDeckCredentials(ctx,channel,testRemoteSignedGrant(t,channel,remotetransport.RoleAdmin),
		"alice","wrong-password");err==nil {
		t.Fatal("incorrect TaskDeck credentials accepted")
	}
	session,err:=s.authenticateRemoteTaskDeckCredentials(ctx,channel,testRemoteSignedGrant(t,channel,remotetransport.RoleViewer),
		"alice","private-admin-password")
	if err!=nil{t.Fatalf("existing TaskDeck account rejected: %v",err)}
	defer session.Close()
	allowTestLivePortal(session)
	// Even a successful TaskDeck password login is NOT an application
	// session until a fresh Portal introspection mechanism is provided.
	session.livePortalCheck=nil
	if err:=s.authorizeRemoteAuthenticatedSession(ctx,session,channel,identity.PermissionFilesRead);err==nil {
		t.Fatal("cached Portal grant authorized TaskDeck action without live revocation check")
	}
	allowTestLivePortal(session)
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
	session,err:=s.authenticateRemoteTaskDeckCredentials(ctx,ch,testRemoteSignedGrant(t,ch,remotetransport.RoleAdmin),
		"alice","private-admin-password")
	if err!=nil{t.Fatal(err)}
	defer session.Close()
	allowTestLivePortal(session)
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

// A compromised Portal may mint arbitrary "admin" tickets, but it cannot
// use a TaskDeck password snapshot captured before local revocation/rotation
// to force a positive login after scrypt finishes. This deterministic test
// moves TaskDeck identity state between the snapshot and the post-hash check.
func TestRemoteLoginRejectsStaleTaskDeckPasswordAndAccountSnapshots(t *testing.T) {
 ctx:=context.Background()
 s:=sharedLoginTestServer(t)
 s.Config.RemoteAccessEnabled=true
 s.Config.Protocol=config.ProtocolHTTPS
 user,err:=s.Identity.UserByUsername(ctx,"alice")
 if err!=nil{t.Fatal(err)}
 if err:=s.revalidateRemoteLoginSnapshot(ctx,user);err!=nil{
  t.Fatalf("unchanged TaskDeck account rejected: %v",err)
 }
 newHash,err:=identity.HashPassword(ctx,"new-local-credential-not-known-to-portal")
 if err!=nil{t.Fatal(err)}
 if err:=s.Identity.ChangeUserPasswordHash(ctx,user.ID,user.PasswordHash,newHash,time.Now().UTC());err!=nil{
  t.Fatal(err)
 }
 if err:=s.revalidateRemoteLoginSnapshot(ctx,user);err==nil{
  t.Fatal("old password snapshot passed after local TaskDeck password rotation")
 }
 rotated,err:=s.Identity.UserByID(ctx,user.ID)
 if err!=nil{t.Fatal(err)}
 if err:=s.revalidateRemoteLoginSnapshot(ctx,rotated);err!=nil{
  t.Fatalf("fresh rotated account snapshot unexpectedly denied: %v",err)
 }
 if err:=s.Identity.SetUserEnabled(ctx,user.ID,false,time.Now().UTC().Add(time.Second));err!=nil{
  t.Fatal(err)
 }
 if err:=s.revalidateRemoteLoginSnapshot(ctx,rotated);err==nil{
  t.Fatal("local TaskDeck account disable allowed stale verified password")
 }
 if err:=s.Identity.SetUserEnabled(ctx,user.ID,true,time.Now().UTC().Add(2*time.Second));err!=nil{
  t.Fatal(err)
 }
 // Even if enabled again before the old scrypt calculation finishes,
 // a changed account generation cannot adopt the prior password proof.
 if err:=s.revalidateRemoteLoginSnapshot(ctx,rotated);err==nil{
  t.Fatal("local disable/re-enable adopted an earlier credential snapshot")
 }
 reenabled,err:=s.Identity.UserByID(ctx,user.ID)
 if err!=nil{t.Fatal(err)}
 if err:=s.revalidateRemoteLoginSnapshot(ctx,reenabled);err!=nil{
  t.Fatalf("fresh TaskDeck account should be eligible after re-enablement: %v",err)
 }
}

func TestRemoteTaskDeckSessionExpirationAndPasswordChange(t *testing.T) {
	ctx:=context.Background()
	s:=sharedLoginTestServer(t)
	s.Config.RemoteAccessEnabled=true
	s.Config.Protocol=config.ProtocolHTTPS
	ch:=remoteSessionTestChannel(t)
	session,err:=s.authenticateRemoteTaskDeckCredentials(ctx,ch,testRemoteSignedGrant(t,ch,remotetransport.RoleAdmin),
		"alice","private-admin-password")
	if err!=nil{t.Fatal(err)}
	defer session.Close()
	allowTestLivePortal(session)
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

func TestRemoteTaskDeckActionChecksPortalRevocationOnEveryCall(t *testing.T) {
	ctx:=context.Background()
	s:=sharedLoginTestServer(t)
	s.Config.RemoteAccessEnabled=true
	s.Config.Protocol=config.ProtocolHTTPS
	channel:=remoteSessionTestChannel(t)
	grant:=testRemoteSignedGrant(t,channel,remotetransport.RoleViewer)
	session,err:=s.authenticateRemoteTaskDeckCredentials(ctx,channel,grant,
		"alice","private-admin-password")
	if err!=nil{t.Fatal(err)}
	defer session.Close()
	if err:=session.bindPortalAgent(nil);err==nil {
		t.Fatal("missing live Portal Agent was accepted")
	}
	var calls int
	revoked:=false
	session.livePortalCheck=func(_ context.Context,g *remoteportal.AgentGrant) error {
		calls++
		if g!=grant || revoked {return remotetransport.ErrRemoteActionDenied}
		return nil
	}
	if err:=s.authorizeRemoteAuthenticatedSession(ctx,session,channel,identity.PermissionFilesRead);err!=nil {
		t.Fatalf("authorized read denied: %v",err)
	}
	if err:=s.authorizeRemoteAuthenticatedSession(ctx,session,channel,identity.PermissionTasksView);err!=nil {
		t.Fatalf("second authorized read denied: %v",err)
	}
	if calls!=2 {t.Fatalf("Portal grant was cached across separate actions: checks=%d",calls)}
	revoked=true
	if err:=s.authorizeRemoteAuthenticatedSession(ctx,session,channel,identity.PermissionFilesRead);err==nil {
		t.Fatal("revoked Portal user kept a previously authenticated TaskDeck session")
	}
	if calls!=3 {t.Fatal("revocation was not checked for the next action")}
}

func TestTaskDeckSessionBindsExactPinnedAgentAndChecksLiveRevocation(t *testing.T) {
	ctx:=context.Background()
	s:=sharedLoginTestServer(t)
	s.Config.RemoteAccessEnabled=true
	s.Config.Protocol=config.ProtocolHTTPS
	pub,priv,err:=ed25519.GenerateKey(rand.Reader)
	if err!=nil{t.Fatal(err)}
	var allowed atomic.Bool
	allowed.Store(true)
	var offline atomic.Bool
	var calls atomic.Int32
	server:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request) {
		if r.URL.Path!="/api/agent/grant-check" || r.Method!=http.MethodPost {
			http.Error(w,"wrong endpoint",http.StatusNotFound);return
		}
		var body struct {
			DeviceID string `json:"device_id"`
			PortalUserID string `json:"portal_user_id"`
			AccessRole string `json:"access_role"`
			PortalAuthEpoch int64 `json:"portal_auth_epoch"`
		}
		if json.NewDecoder(r.Body).Decode(&body)!=nil ||
			body.PortalUserID!="21" || body.AccessRole!="viewer" ||
			body.PortalAuthEpoch!=1 {
			http.Error(w,"invalid grant",http.StatusForbidden);return
		}
		calls.Add(1)
		if offline.Load(){http.Error(w,"portal offline",http.StatusServiceUnavailable);return}
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok":true,"allowed":allowed.Load()})
	}))
	defer server.Close()
	keyPath:=filepath.Join(t.TempDir(),"private","identity.seed")
	if err:=os.MkdirAll(filepath.Dir(keyPath),0700);err!=nil{t.Fatal(err)}
	seed:=make([]byte,ed25519.SeedSize)
	if _,err:=rand.Read(seed);err!=nil{t.Fatal(err)}
	if err:=os.WriteFile(keyPath,seed,0600);err!=nil{t.Fatal(err)}
	agent,err:=remoteportal.NewAgent(server.URL,keyPath,server.Client())
	if err!=nil{t.Fatal(err)}
	if err:=agent.ConfigurePortalGrantKey(base64.StdEncoding.EncodeToString(pub));err!=nil{t.Fatal(err)}
	ch:=remoteSessionTestChannelWithPrivate(t,ed25519.NewKeyFromSeed(seed))
	now:=time.Now()
	claims:=remoterelay.Claims{V:2,AuthEpoch:1,BrowserBinding:ch.BrowserTicketBinding(),
		Issuer:server.URL,Audience:"taskdeck-relay",Subject:"21",
		DeviceID:agent.DeviceID(),Role:"viewer",
		IssuedAt:now.Unix(),Expires:now.Unix()+30,JTI:strings.Repeat("d",32)}
	raw,err:=json.Marshal(claims)
	if err!=nil{t.Fatal(err)}
	body:=base64.RawURLEncoding.EncodeToString(raw)
	signature:=ed25519.Sign(priv,[]byte("taskdeck-relay-ticket-v1."+body))
	ticket:=body+"."+base64.RawURLEncoding.EncodeToString(signature)
	grant,err:=agent.VerifyPortalGrant(ticket,ch,now)
	if err!=nil{t.Fatal(err)}
	session,err:=s.authenticateRemoteTaskDeckCredentials(ctx,ch,grant,"alice","private-admin-password")
	if err!=nil{t.Fatal(err)}
	defer session.Close()
	otherAgent,err:=remoteportal.NewAgent(server.URL,
		filepath.Join(t.TempDir(),"other","identity.seed"),server.Client())
	if err!=nil{t.Fatal(err)}
	if err:=session.bindPortalAgent(otherAgent);err==nil {
		t.Fatal("unrelated Device ID was allowed to check another Agent's grant")
	}
	wrongPortal,err:=remoteportal.NewAgent("https://other-portal.example.com",keyPath,nil)
	if err!=nil{t.Fatal(err)}
	if err:=wrongPortal.ConfigurePortalGrantKey(base64.StdEncoding.EncodeToString(pub));err!=nil{t.Fatal(err)}
	if err:=session.bindPortalAgent(wrongPortal);err==nil {
		t.Fatal("different Portal HTTPS issuer accepted for signed grant")
	}
	wrongPin,err:=remoteportal.NewAgent(server.URL,keyPath,server.Client())
	if err!=nil{t.Fatal(err)}
	if err:=wrongPin.ConfigurePortalGrantKey(base64.StdEncoding.EncodeToString(pub));err!=nil{t.Fatal(err)}
	if err:=session.bindPortalAgent(wrongPin);err==nil {
		t.Fatal("different signing-verifier instance substituted for verified Agent")
	}
	if err:=session.bindPortalAgent(agent);err!=nil {
		t.Fatalf("legitimate Portal Agent could not bind live introspection: %v",err)
	}
	if err:=session.bindPortalAgent(agent);err==nil {
		t.Fatal("authenticated session allowed a second verifier binding")
	}
	for i:=0;i<2;i++ {
		if err:=s.authorizeRemoteAuthenticatedSession(ctx,session,ch,identity.PermissionFilesRead);err!=nil {
			t.Fatalf("granted Portal viewer denied on action %d: %v",i,err)
		}
	}
	if calls.Load()!=2{t.Fatalf("authorized actions reused stale Portal decision, checks=%d",calls.Load())}
	allowed.Store(false)
	if err:=s.authorizeRemoteAuthenticatedSession(ctx,session,ch,identity.PermissionFilesRead);err==nil {
		t.Fatal("revoked Portal grant retained remote application permissions")
	}
	allowed.Store(true)
	offline.Store(true)
	if err:=s.authorizeRemoteAuthenticatedSession(ctx,session,ch,identity.PermissionFilesRead);err==nil {
		t.Fatal("Portal outage permitted an old signed ticket to authorize action")
	}
	session.Close()
	if err:=s.authorizeRemoteAuthenticatedSession(ctx,session,ch,identity.PermissionFilesRead);err==nil {
		t.Fatal("closed session retained live Portal action authorization")
	}
}

func TestRemoteTaskDeckRequiresSharedAuthenticationAndTLS(t *testing.T) {
	ctx:=context.Background()
	s:=sharedLoginTestServer(t)
	s.Config.RemoteAccessEnabled=true
	s.Config.Protocol=config.ProtocolHTTPS
	ch:=remoteSessionTestChannel(t)
	s.Config.Protocol=config.ProtocolHTTP
	if _,err:=s.authenticateRemoteTaskDeckCredentials(ctx,ch,testRemoteSignedGrant(t,ch,remotetransport.RoleViewer),
		"alice","private-admin-password");err==nil{
		t.Fatal("plaintext TaskDeck protocol allowed remote login")
	}
	s.Config.Protocol=config.ProtocolHTTPS
	s.Config.SharedServerEnabled=false
	if _,err:=s.authenticateRemoteTaskDeckCredentials(ctx,ch,testRemoteSignedGrant(t,ch,remotetransport.RoleViewer),
		"alice","private-admin-password");err==nil {
		t.Fatal("legacy single-user login was treated as shared identity login")
	}
}

func TestRemoteTaskDeckChannelClosureImmediatelyRevokesLocalSession(t *testing.T) {
	ctx:=context.Background()
	s:=sharedLoginTestServer(t)
	s.Config.RemoteAccessEnabled=true
	s.Config.Protocol=config.ProtocolHTTPS
	channel:=remoteSessionTestChannel(t)
	session,err:=s.authenticateRemoteTaskDeckCredentials(ctx,channel,
		testRemoteSignedGrant(t,channel,remotetransport.RoleViewer),"alice","private-admin-password")
	if err!=nil{t.Fatal(err)}
	defer session.Close()
	allowTestLivePortal(session)
	if err:=s.authorizeRemoteAuthenticatedSession(ctx,session,channel,
		identity.PermissionFilesRead);err!=nil {t.Fatal(err)}
	channel.Close()
	if channel.Established(){t.Fatal("closed E2EE channel was still established")}
	if err:=s.authorizeRemoteAuthenticatedSession(ctx,session,channel,
		identity.PermissionFilesRead);err==nil {
		t.Fatal("disconnected encrypted transport retained TaskDeck session authority")
	}
}

func TestRemoteTaskDeckLoginAuditOmitsCredentialsAndTokens(t *testing.T) {
	ctx:=context.Background()
	s:=sharedLoginTestServer(t)
	s.Config.RemoteAccessEnabled=true
	s.Config.Protocol=config.ProtocolHTTPS
	ch:=remoteSessionTestChannel(t)
	if _,err:=s.authenticateRemoteTaskDeckCredentials(ctx,ch,testRemoteSignedGrant(t,ch,remotetransport.RoleViewer),
		"alice","invalid-sensitive-password");err==nil {
		t.Fatal("incorrect password authenticated")
	}
	session,err:=s.authenticateRemoteTaskDeckCredentials(ctx,ch,testRemoteSignedGrant(t,ch,remotetransport.RoleViewer),
		"alice","private-admin-password")
	if err!=nil{t.Fatal(err)}
	defer session.Close()
	allowTestLivePortal(session)
	project,err:=s.Identity.ProjectByKey(ctx,s.Config.SharedProjectID)
	if err!=nil{t.Fatal(err)}
	events,err:=s.Identity.ListAudit(ctx,identity.AuditQuery{
		ProjectID:project.ID,Action:"auth.remote.login",Limit:10,
	})
	if err!=nil{t.Fatal(err)}
	var good,bad bool
	for _,event:=range events{
		if event.Result=="success"{
			good=true
			if event.UserID==nil || *event.UserID!=session.userID{
				t.Fatal("successful remote auth not bound to actual TaskDeck user")
			}
		}
		if event.Result=="denied"{bad=true}
		if strings.Contains(event.Details,"private-admin-password") ||
			strings.Contains(event.Details,"invalid-sensitive-password") ||
			strings.Contains(event.Details,"alice") ||
			event.ClientIP!=""{
			t.Fatal("remote login audit exposed credential, username or guessed network address")
		}
	}
	if !good || !bad{t.Fatalf("missing remote login success/denied audit: %+v",events)}
}

func TestRemoteTaskDeckLoginUsesPortalGrantOnlyOnce(t *testing.T) {
 ctx:=context.Background()
 s:=sharedLoginTestServer(t)
 s.Config.RemoteAccessEnabled=true
 s.Config.Protocol=config.ProtocolHTTPS
 channel:=remoteSessionTestChannel(t)
 grant:=testRemoteSignedGrant(t,channel,remotetransport.RoleAdmin)
 if _,err:=s.authenticateRemoteTaskDeckCredentials(ctx,channel,grant,
  "alice","wrong-password");err==nil {
  t.Fatal("wrong TaskDeck password accepted")
 }
 if _,err:=s.authenticateRemoteTaskDeckCredentials(ctx,channel,grant,
  "alice","private-admin-password");err==nil {
  t.Fatal("same Portal grant reused for second TaskDeck password attempt")
 }
 renewed:=testRemoteSignedGrant(t,channel,remotetransport.RoleViewer)
 session,err:=s.authenticateRemoteTaskDeckCredentials(ctx,channel,renewed,
  "alice","private-admin-password")
 if err!=nil{t.Fatalf("separate valid Portal grant rejected: %v",err)}
 defer session.Close()
	allowTestLivePortal(session)
}

// A grant check can be pending on a slow Portal while local revocation,
// session shutdown or TCP disconnect occurs. None may be undone by a stale
// allowed result. This is a deterministic concurrency regression: no sleeps.
func TestRemoteTaskDeckSessionRevocationDuringPortalCheckFailsClosed(t *testing.T) {
 for _,tc:=range []struct{
  name string
  revoke func(*remoteTaskDeckSession,*remotetransport.Channel,*remoteportal.AgentGrant)
 }{
  {"local session close",func(s *remoteTaskDeckSession,_ *remotetransport.Channel,_ *remoteportal.AgentGrant){s.Close()}},
  {"Portal grant close",func(_ *remoteTaskDeckSession,_ *remotetransport.Channel,g *remoteportal.AgentGrant){g.Close()}},
  {"E2EE disconnect",func(_ *remoteTaskDeckSession,c *remotetransport.Channel,_ *remoteportal.AgentGrant){c.Close()}},
 }{
  t.Run(tc.name,func(t *testing.T){
   ctx:=context.Background()
   server:=sharedLoginTestServer(t)
   server.Config.RemoteAccessEnabled=true
   server.Config.Protocol=config.ProtocolHTTPS
   channel:=remoteSessionTestChannel(t)
   grant:=testRemoteSignedGrant(t,channel,remotetransport.RoleViewer)
   session,err:=server.authenticateRemoteTaskDeckCredentials(ctx,channel,grant,
    "alice","private-admin-password")
   if err!=nil{t.Fatal(err)}
   defer session.Close()
   entered:=make(chan struct{})
   release:=make(chan struct{})
   session.livePortalCheck=func(context.Context,*remoteportal.AgentGrant)error{
    close(entered)
    <-release
    return nil // result predates the revocation
   }
   results:=make(chan error,1)
   go func(){
    results<-server.authorizeRemoteAuthenticatedSession(ctx,session,channel,
     identity.PermissionFilesRead)
   }()
   select {
   case <-entered:
   case <-time.After(3*time.Second):t.Fatal("action never started live Portal introspection")
   }
   tc.revoke(session,channel,grant)
   close(release)
   select {
   case err:=<-results:
    if !errors.Is(err,remotetransport.ErrRemoteActionDenied){
     t.Fatalf("late positive Portal response authorized a dead session: %v",err)
    }
   case <-time.After(3*time.Second):t.Fatal("authorization did not complete")
   }
   if tc.name=="local session close" && channel.Established() {
    t.Fatal("local session shutdown left E2EE channel usable")
   }
  })
 }
}
