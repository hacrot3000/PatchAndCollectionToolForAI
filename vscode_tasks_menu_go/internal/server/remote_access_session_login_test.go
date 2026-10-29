package server

import (
 "context"
 "crypto/ed25519"
 "crypto/rand"
 "crypto/sha256"
 "encoding/base64"
 "encoding/hex"
 "encoding/json"
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

type stagedLoginFixture struct {
 server *Server
 agent *remoteportal.Agent
 client *remotetransport.Channel
 channel *remotetransport.Channel
 grant *remoteportal.AgentGrant
 portalAllowed *atomic.Bool
 portalChecks *atomic.Int32
}

func newStagedLoginFixture(t *testing.T,role remotetransport.Role)*stagedLoginFixture{
 t.Helper()
 s:=sharedLoginTestServer(t)
 s.Config.RemoteAccessEnabled=true
 s.Config.Protocol=config.ProtocolHTTPS
 portalKey,portalSigner,err:=ed25519.GenerateKey(rand.Reader)
 if err!=nil{t.Fatal(err)}
 var allowed atomic.Bool
 allowed.Store(true)
 var checks atomic.Int32
 portal:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if r.Method!=http.MethodPost || r.URL.Path!="/api/agent/grant-check" {
   http.Error(w,"invalid endpoint",http.StatusNotFound);return
  }
  checks.Add(1)
  _=json.NewEncoder(w).Encode(map[string]bool{"ok":true,"allowed":allowed.Load()})
 }))
 t.Cleanup(portal.Close)
 keyPath:=filepath.Join(t.TempDir(),"secure","agent.seed")
 if err:=os.MkdirAll(filepath.Dir(keyPath),0700);err!=nil{t.Fatal(err)}
 seed:=make([]byte,ed25519.SeedSize)
 if _,err:=rand.Read(seed);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(keyPath,seed,0600);err!=nil{t.Fatal(err)}
 agent,err:=remoteportal.NewAgent(portal.URL,keyPath,portal.Client())
 if err!=nil{t.Fatal(err)}
 if err:=agent.ConfigurePortalGrantKey(base64.StdEncoding.EncodeToString(portalKey));err!=nil{t.Fatal(err)}
 private:=ed25519.NewKeyFromSeed(seed)
 pub:=private.Public().(ed25519.PublicKey)
 hash:=sha256.Sum256(pub)
 device:=hex.EncodeToString(hash[:])
 browser,err:=remotetransport.NewEphemeral()
 if err!=nil{t.Fatal(err)}
 hello,channel,err:=remotetransport.ClientHello(private,device,
  strings.Repeat("a",32),strings.Repeat("b",32),browser.Public)
 if err!=nil{t.Fatal(err)}
 client,err:=remotetransport.AcceptHello(browser,device,
  strings.Repeat("a",32),strings.Repeat("b",32),hello)
 if err!=nil{t.Fatal(err)}
 t.Cleanup(client.Close)
 t.Cleanup(channel.Close)
 now:=time.Now()
 claims:=remoterelay.Claims{
  V:2,AuthEpoch:1,BrowserBinding:channel.BrowserTicketBinding(),
  Issuer:portal.URL,Audience:"taskdeck-relay",Subject:"21",
  DeviceID:agent.DeviceID(),Role:string(role),
  IssuedAt:now.Unix(),Expires:now.Unix()+30,JTI:strings.Repeat("e",32),
 }
 raw,err:=json.Marshal(claims)
 if err!=nil{t.Fatal(err)}
 payload:=base64.RawURLEncoding.EncodeToString(raw)
 ticket:=payload+"."+base64.RawURLEncoding.EncodeToString(ed25519.Sign(portalSigner,
  []byte("taskdeck-relay-ticket-v1."+payload)))
 grant,err:=agent.VerifyPortalGrant(ticket,channel,now)
 if err!=nil{t.Fatal(err)}
 t.Cleanup(grant.Close)
 return &stagedLoginFixture{s,agent,client,channel,grant,&allowed,&checks}
}

func (f *stagedLoginFixture) request(t *testing.T,username,password string)[]byte{
 t.Helper()
 frame,err:=remotetransport.RemoteLoginRequestFrame(f.client,username,password)
 if err!=nil{t.Fatal(err)}
 return frame
}

func TestRemoteStagedPasswordSessionRequiresRealTaskDeckCredentials(t *testing.T){
 f:=newStagedLoginFixture(t,remotetransport.RoleAdmin)
 // Attacker has full Portal signing control (the test signer) and returns
 // allowed=true for every grant, including arbitrary admin tickets.
 session,response,err:=f.server.StartEncryptedRemoteSession(
  context.Background(),f.agent,f.channel,f.grant,
  f.request(t,"alice","PHP-attacker-does-not-know-local-password"))
 if err==nil || session!=nil || response!=nil{
  t.Fatal("P0: stolen PHP signing key minted TaskDeck local session")
 }
 if f.channel.Established() || !f.grantIsClosed() {
  t.Fatal("denied password retained a usable Agent E2EE channel/grant")
 }
}

func (f *stagedLoginFixture) grantIsClosed() bool {
 return !f.grant.ActiveForSession(f.channel)
}

func TestRemoteStagedSessionSuccessIsLocalPasswordBoundAndTokenFree(t *testing.T){
 f:=newStagedLoginFixture(t,remotetransport.RoleViewer)
 session,response,err:=f.server.StartEncryptedRemoteSession(context.Background(),
  f.agent,f.channel,f.grant,f.request(t,"alice","private-admin-password"))
 if err!=nil{t.Fatal(err)}
 defer session.Close()
 if session.closed.Load() || !f.channel.Established(){
  t.Fatal("legitimate locally authenticated E2EE session was not live")
 }
 valid,err:=remotetransport.OpenRemoteLoginResult(f.client,response)
 if err!=nil || !valid{t.Fatalf("verified TaskDeck login reply invalid: %v %v",valid,err)}
 if f.portalChecks.Load()!=2{t.Fatalf("missing pre/post Portal checks: %d",f.portalChecks.Load())}
 var checkCount atomic.Int32
 // The Agent itself binds trusted live grant-check. No test-injected
 // caller-supplied Portal verifier is installed in this session.
 if session.livePortalCheck==nil {t.Fatal("valid local principal lacks pinned Agent binding")}
 // Real Portal checks must occur before and after read-only operations.
 actionFrame,err:=remotetransport.SealRemoteAction(f.client,remotetransport.RemoteAction{
  RequestID:"0123456789abcdef0123456789abcdef",
  Operation:remotetransport.RemoteActionSessionPing,
 })
 if err!=nil{t.Fatal(err)}
 reply,err:=f.server.executeRemoteE2EEPing(context.Background(),session,f.channel,actionFrame)
 if err!=nil{t.Fatal(err)}
 if err:=remotetransport.OpenRemoteActionPingResult(f.client,reply,
  "0123456789abcdef0123456789abcdef");err!=nil{t.Fatal(err)}
 checkCount.Store(f.portalChecks.Load())
 if checkCount.Load()!=4{t.Fatalf("missing two action Portal checks: %d",checkCount.Load())}
 if err:=f.server.authorizeRemoteAuthenticatedSession(context.Background(),session,f.channel,
  identity.PermissionTerminalCreate);err==nil{
  t.Fatal("compromised admin Portal ticket bypassed viewer TaskDeck role ceiling")
 }
}

func TestRemoteStagedAuthenticatedSessionRejectsTaskDeckPasswordRotation(t *testing.T){
 f:=newStagedLoginFixture(t,remotetransport.RoleAdmin)
 session,_,err:=f.server.StartEncryptedRemoteSession(context.Background(),
  f.agent,f.channel,f.grant,f.request(t,"alice","private-admin-password"))
 if err!=nil{t.Fatal(err)}
 defer session.Close()
 ctx:=context.Background()
 user,err:=f.server.Identity.UserByID(ctx,session.userID)
 if err!=nil{t.Fatal(err)}
 replacement,err:=identity.HashPassword(ctx,"new-only-local-TaskDeck-password")
 if err!=nil{t.Fatal(err)}
 if err:=f.server.Identity.ChangeUserPasswordHash(ctx,user.ID,user.PasswordHash,
  replacement,time.Now().UTC().Add(-time.Hour));err!=nil{t.Fatal(err)}
 req,err:=remotetransport.SealRemoteAction(f.client,remotetransport.RemoteAction{
  RequestID:"0123456789abcdef0123456789abcdef",
  Operation:remotetransport.RemoteActionSessionPing,
 })
 if err!=nil{t.Fatal(err)}
 reply,err:=f.server.executeRemoteE2EEPing(ctx,session,f.channel,req)
 if err==nil || len(reply)!=0{
  t.Fatal("P0: compromised PHP kept an E2EE session after TaskDeck-local password rotation")
 }
}

func TestRemoteStagedSessionAutoClosesOnGrantRevocationAndTransportLoss(t *testing.T){
 for _,mode:=range []string{"Portal revoked","E2EE disconnect","local logout"} {
  t.Run(mode,func(t *testing.T){
   f:=newStagedLoginFixture(t,remotetransport.RoleViewer)
   session,_,err:=f.server.StartEncryptedRemoteSession(context.Background(),
    f.agent,f.channel,f.grant,f.request(t,"alice","private-admin-password"))
   if err!=nil{t.Fatal(err)}
   defer session.Close()
   switch mode{
   case "Portal revoked":f.grant.Close()
   case "E2EE disconnect":f.channel.Close()
   case "local logout":session.Close()
   }
   deadline:=time.After(2*time.Second)
   for !session.closed.Load() {
    select{
    case <-deadline:t.Fatal("stale authenticated session survived revocation")
    case <-time.After(2*time.Millisecond):
    }
   }
   if f.channel.Established() || f.grant.ActiveForSession(f.channel) {
    t.Fatal("closed session retained transport or Portal authority")
   }
  })
 }
}


// Timer expiry is a local TaskDeck decision. It must close both Portal grant
// and the AEAD channel even if compromised PHP keeps returning allowed=true.
func TestRemoteSessionLifetimeExpiresWithoutPeerDisconnect(t *testing.T){
 s,session,ch:=testRemoteExecutionSession(t)
 _=s
 session.expiresAt=time.Now().Add(25*time.Millisecond)
 session.startRemoteSessionLifetime()
 deadline:=time.After(time.Second)
 for !session.closed.Load() {
  select {
  case <-deadline:t.Fatal("locally expired TaskDeck session survived")
  case <-time.After(2*time.Millisecond):
  }
 }
 if ch.Established() || session.portalGrant.ActiveForSession(ch){
  t.Fatal("expired local TaskDeck principal retained an E2EE/Portal grant")
 }
}

func TestRemoteStagedSessionDeniedWithoutPortalCurrentAuthorization(t *testing.T){
 f:=newStagedLoginFixture(t,remotetransport.RoleAdmin)
 f.portalAllowed.Store(false)
 session,encrypted,err:=f.server.StartEncryptedRemoteSession(context.Background(),
  f.agent,f.channel,f.grant,f.request(t,"alice","private-admin-password"))
 if err==nil || session!=nil || encrypted!=nil{
  t.Fatal("compromised PHP revocation or outage still issued valid local session")
 }
 if f.portalChecks.Load()!=1 {
  t.Fatalf("pre-credential Portal gate was skipped: %d",f.portalChecks.Load())
 }
 if f.channel.Established(){t.Fatal("denied grant left E2EE reusable")}
}
