package server

import (
 "context"
 "crypto/ed25519"
 "crypto/rand"
 "crypto/sha256"
 "encoding/base64"
 "encoding/hex"
 "errors"
 "strconv"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/config"
 "bletonfc/vscode_tasks_menu/internal/remoterelay"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// This adapter represents the future opaque Relay transport entirely inside
// a controlled Go test. It is NOT an HTTP, WebSocket, PHP, Agent callback or
// public TaskDeck command route. Relay only receives ciphertext.
type internalOpaquePingLink struct {
 relay *remoterelay.Registry
 server *Server
 session *remoteTaskDeckSession
 agent *remotetransport.Channel
 deviceID,connectionNonce,probeID string
}
func (l *internalOpaquePingLink) ExchangePing(ctx context.Context,request []byte)([]byte,error){
 if ctx==nil || ctx.Err()!=nil{return nil,remotetransport.ErrRemoteActionDenied}
 encoded:=base64.StdEncoding.EncodeToString(request)
 _,waiting,err:=l.relay.QueueAppPing(l.deviceID,l.probeID,encoded)
 if err!=nil{return nil,err}
 probe,relayID,delivered,ok:=l.relay.TakeAppPing(l.deviceID,l.connectionNonce)
 if !ok || probe!=l.probeID || delivered!=encoded{
  l.relay.CloseAppPing(l.deviceID,l.connectionNonce,l.probeID)
  return nil,remotetransport.ErrRemoteActionDenied
 }
 sealed,err:=base64.StdEncoding.Strict().DecodeString(delivered)
 if err!=nil{
  l.relay.CloseAppPing(l.deviceID,l.connectionNonce,l.probeID)
  return nil,remotetransport.ErrRemoteActionDenied
 }
 response,err:=l.server.executeRemoteE2EEPing(ctx,l.session,l.agent,sealed)
 if err!=nil || ctx.Err()!=nil{
  l.relay.CloseAppPing(l.deviceID,l.connectionNonce,l.probeID)
  return nil,remotetransport.ErrRemoteActionDenied
 }
 if !l.relay.AckAppPing(l.deviceID,l.connectionNonce,probe,relayID,
  base64.StdEncoding.EncodeToString(response)){
  l.relay.CloseAppPing(l.deviceID,l.connectionNonce,l.probeID)
  return nil,remotetransport.ErrRemoteActionDenied
 }
 select {
 case ciphertext,open:=<-waiting:
  if !open {return nil,remotetransport.ErrRemoteActionDenied}
  return base64.StdEncoding.Strict().DecodeString(ciphertext)
 case <-ctx.Done():
  l.relay.CloseAppPing(l.deviceID,l.connectionNonce,l.probeID)
  return nil,remotetransport.ErrRemoteActionDenied
 }
}

// Full opaque transport fixture: the same independent Ed25519 device identity
// signs both Relay presence and Agent P-256 hello/ready/app-readiness.
func newInternalOpaqueSessionFixture(t *testing.T)(
 *Server,*remoteTaskDeckSession,*remotetransport.PingClient,*remotetransport.Channel,
 *remoterelay.Registry,func()){
 t.Helper()
 pub,priv,err:=ed25519.GenerateKey(rand.Reader)
 if err!=nil{t.Fatal(err)}
 sum:=sha256.Sum256(pub)
 deviceID:=hex.EncodeToString(sum[:])
 now:=time.Now().UTC()
 connNonce:=strings.Repeat("a",32)
 browserNonce:=strings.Repeat("b",32)
 relay,err:=remoterelay.NewRegistry("https://relay.example.com")
 if err!=nil{t.Fatal(err)}
 proof:=remoterelay.DeviceProof{
  DeviceID:deviceID,
  PublicKey:base64.StdEncoding.EncodeToString(pub),
  Audience:"https://relay.example.com",
  Challenge:base64.RawURLEncoding.EncodeToString(make([]byte,32)),
  Timestamp:now.Unix(),
  Nonce:connNonce,
 }
 proofBody:=strings.Join([]string{
  remoterelay.DeviceChallengeProtocol,"connect",deviceID,proof.Audience,
  strconv.FormatInt(proof.Timestamp,10),proof.Nonce,proof.Challenge,
 },"\n")
 proof.Signature=base64.StdEncoding.EncodeToString(ed25519.Sign(priv,[]byte(proofBody)))
 if err:=relay.Register(proof,now);err!=nil{t.Fatal(err)}
 release,err:=relay.Acquire(remoterelay.Claims{DeviceID:deviceID},now)
 if err!=nil{t.Fatal(err)}
 t.Cleanup(release)
 browser,err:=remotetransport.NewEphemeral()
 if err!=nil{t.Fatal(err)}
 _,err=relay.BeginProbe(deviceID,browserNonce,
  base64.StdEncoding.EncodeToString(browser.Public))
 if err!=nil{t.Fatal(err)}
 pendingProbe,ok:=relay.TakeProbe(deviceID,connNonce)
 if !ok{t.Fatal("signed Agent did not receive Relay E2EE probe")}
 probeID:=pendingProbe.ProbeID
 hello,agent,err:=remotetransport.ClientHello(priv,deviceID,probeID,
  browserNonce,browser.Public)
 if err!=nil{t.Fatal(err)}
 native,err:=remotetransport.AcceptHello(browser,deviceID,probeID,
  browserNonce,hello)
 if err!=nil{t.Fatal(err)}
 t.Cleanup(native.Close)
 ready,err:=agent.Seal([]byte("taskdeck-e2ee-ready-v1"))
 if err!=nil{t.Fatal(err)}
 reachability:=strings.Join([]string{
  remoterelay.BrowserAttestationProtocol,deviceID,probeID,browserNonce,
 },"\n")
 attestation:=remoterelay.ProbeAttestation{
  ProbeID:probeID,ClientNonce:browserNonce,
  PublicKey:base64.StdEncoding.EncodeToString(pub),
  Signature:base64.StdEncoding.EncodeToString(ed25519.Sign(priv,[]byte(reachability))),
  Crypto:&remoterelay.ProbeCrypto{
   Identity:base64.StdEncoding.EncodeToString(hello.Identity),
   Ephemeral:base64.StdEncoding.EncodeToString(hello.Ephemeral),
   Signature:base64.StdEncoding.EncodeToString(hello.Signature),
   EncryptedReady:base64.StdEncoding.EncodeToString(ready),
  },
 }
 if !relay.AckProbe(deviceID,connNonce,attestation){
  t.Fatal("valid signed E2EE Agent probe refused by Relay")
 }
 clearReady,err:=native.Open(ready)
 if err!=nil || string(clearReady)!="taskdeck-e2ee-ready-v1"{
  t.Fatalf("native did not authenticate Agent ready record: %v",err)
 }
 possession,err:=native.Seal([]byte("authenticated-client-possession"))
 if err!=nil{t.Fatal(err)}
 if _,err:=agent.Open(possession);err!=nil{t.Fatal(err)}

 s:=sharedLoginTestServer(t)
 s.Config.RemoteAccessEnabled=true
 s.Config.Protocol=config.ProtocolHTTPS
 grant:=testRemoteSignedGrant(t,agent,remotetransport.RoleViewer)
 session,err:=s.authenticateRemoteTaskDeckCredentials(context.Background(),
  agent,grant,"alice","private-admin-password")
 if err!=nil{t.Fatal(err)}
 t.Cleanup(session.Close)
 allowTestLivePortal(session)
 // Simulate the PRE-EXISTING opaque login result mailbox after local login.
 // This fixture does not assert that the application network route exists.
 loginRequest,err:=native.Seal([]byte("opaque-encrypted-login-fixture"))
 if err!=nil{t.Fatal(err)}
 loginSlot,err:=relay.QueueLogin(deviceID,probeID,
  base64.StdEncoding.EncodeToString(loginRequest))
 if err!=nil{t.Fatal(err)}
 deliveredProbe,deliveredLogin,ok:=relay.TakeLogin(deviceID,connNonce)
 if !ok || deliveredProbe!=probeID {t.Fatal("login not forwarded to signed Agent")}
 forwardedLogin,err:=base64.StdEncoding.Strict().DecodeString(deliveredLogin)
 if err!=nil{t.Fatal(err)}
 openedLogin,err:=agent.Open(forwardedLogin)
 if err!=nil || string(openedLogin)!="opaque-encrypted-login-fixture"{
  t.Fatalf("Agent did not receive the next ordered E2EE login record: %v",err)
 }
 loginResult,err:=agent.Seal([]byte("opaque-encrypted-result-fixture"))
 if err!=nil{t.Fatal(err)}
 if !relay.AckLogin(deviceID,connNonce,probeID,
  base64.StdEncoding.EncodeToString(loginResult)){t.Fatal("Agent login reply rejected")}
 forwardedResult,ok:=<-loginSlot
 if !ok{t.Fatal("opaque login result not returned")}
 forwardedBytes,err:=base64.StdEncoding.Strict().DecodeString(forwardedResult)
 if err!=nil{t.Fatal(err)}
 decryptedResult,err:=native.Open(forwardedBytes)
 if err!=nil || string(decryptedResult)!="opaque-encrypted-result-fixture"{
  t.Fatalf("native received wrong ordered login result: %v",err)
 }
 // App readiness is deliberately signed AFTER the host-local TaskDeck
 // password+membership check; a PHP admin ticket cannot produce this proof.
 appReady:=strings.Join([]string{
  "taskdeck-e2ee-app-ready-v1",deviceID,connNonce,probeID,
 },"\n")
 signature:=base64.StdEncoding.EncodeToString(ed25519.Sign(priv,[]byte(appReady)))
 if err:=relay.ActivateAppPing(deviceID,connNonce,probeID,
  base64.StdEncoding.EncodeToString(pub),signature);err!=nil{t.Fatal(err)}
 link:=&internalOpaquePingLink{
  relay:relay,server:s,session:session,agent:agent,
  deviceID:deviceID,connectionNonce:connNonce,probeID:probeID,
 }
 client,err:=remotetransport.NewPingClient(native,link)
 if err!=nil{t.Fatal(err)}
 t.Cleanup(client.Close)
 return s,session,client,native,relay,release
}

func TestRemoteOpaqueRelayMailboxEncryptedPingRequiresLocalRBAC(t *testing.T){
 s,session,client,native,_,_:=newInternalOpaqueSessionFixture(t)
 for n:=0;n<2;n++{
  if err:=client.Ping(context.Background());err!=nil{
   t.Fatalf("round %d: authenticated E2EE request failed: %v",n,err)
  }
 }
 // Local TaskDeck host revocation always overrides a Portal allowed=true
 // and the opaque Relay's previously signed application-ready proof.
 if err:=s.Identity.SetUserEnabled(context.Background(),session.userID,
  false,time.Now().UTC());err!=nil{t.Fatal(err)}
 if err:=client.Ping(context.Background());!errors.Is(err,remotetransport.ErrRemoteActionDenied){
  t.Fatalf("disabled TaskDeck account was authorized via Relay: %v",err)
 }
 if native.Established(){t.Fatal("revoked TaskDeck action left native E2EE alive")}
}

func TestRemoteOpaqueRelayMailboxStopsOnLivePortalRevoke(t *testing.T){
 _,session,client,native,_,_:=newInternalOpaqueSessionFixture(t)
 if err:=client.Ping(context.Background());err!=nil{t.Fatal(err)}
 session.portalGrant.Close()
 if err:=client.Ping(context.Background());!errors.Is(err,remotetransport.ErrRemoteActionDenied){
  t.Fatalf("revoked Portal grant permitted authenticated E2EE ping: %v",err)
 }
 if native.Established(){t.Fatal("revoked Portal session left native channel reusable")}
}

func TestRemoteOpaqueRelayMailboxQuotaCannotBecomeUnlimitedAppAccess(t *testing.T){
 _,_,client,native,_,_:=newInternalOpaqueSessionFixture(t)
 for round:=0;round<remoterelay.MaxAppPingRounds;round++{
  if err:=client.Ping(context.Background());err!=nil{
   t.Fatalf("bounded ping round %d failed: %v",round,err)
  }
 }
 if err:=client.Ping(context.Background());!errors.Is(err,remotetransport.ErrRemoteActionDenied){
  t.Fatalf("fifth application ping bypassed Relay quota: %v",err)
 }
 if native.Established(){t.Fatal("quota error left native E2EE key reusable")}
}
