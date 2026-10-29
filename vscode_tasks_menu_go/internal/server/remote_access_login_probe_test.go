package server

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
 "encoding/base64"
	"encoding/hex"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/config"
	"bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remoteclientkey"
	"bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func encryptedLoginPair(t *testing.T) (*remotetransport.Channel,*remotetransport.Channel) {
	t.Helper()
	pub,key,err:=ed25519.GenerateKey(rand.Reader)
	if err!=nil{t.Fatal(err)}
	digest:=sha256.Sum256(pub)
	device:=hex.EncodeToString(digest[:])
	browser,err:=remotetransport.NewEphemeral()
	if err!=nil{t.Fatal(err)}
	hello,agent,err:=remotetransport.ClientHello(key,device,
		strings.Repeat("1",32),strings.Repeat("2",32),browser.Public)
	if err!=nil{t.Fatal(err)}
	client,err:=remotetransport.AcceptHello(browser,device,
		strings.Repeat("1",32),strings.Repeat("2",32),hello)
	if err!=nil{t.Fatal(err)}
	return client,agent
}

func approvedRemoteLoginFrame(t *testing.T,s *Server,channel *remotetransport.Channel,username,password string)([]byte,error){
 t.Helper()
 pub,priv,err:=ed25519.GenerateKey(rand.Reader);if err!=nil{t.Fatal(err)}
 public:=base64.StdEncoding.EncodeToString(pub)
 path,err:=remoteclientkey.DefaultPath(s.Workspace);if err!=nil{t.Fatal(err)}
 if _,err:=remoteclientkey.Add(path,username,public,"test-client");err!=nil{t.Fatal(err)}
 signature:=base64.StdEncoding.EncodeToString(ed25519.Sign(priv,
  remoteclientkey.ProofMessage(username,channel.RemoteClientProofBinding())))
 return remotetransport.RemoteLoginRequestWithClientKey(channel,username,password,public,signature)
}

func TestEncryptedTaskDeckLoginWithRealAccountHasNoSessionToken(t *testing.T) {
	s:=sharedLoginTestServer(t)
	s.Config.RemoteAccessEnabled=true
	s.Config.Protocol=config.ProtocolHTTPS
	browser,agent:=encryptedLoginPair(t)
	grant:=testRemoteSignedGrant(t,agent,"viewer")
	request,err:=approvedRemoteLoginFrame(t,s,browser,"alice","private-admin-password")
	if err!=nil{t.Fatal(err)}
	if bytes.Contains(request,[]byte("alice")) ||
		bytes.Contains(request,[]byte("private-admin-password")) {
		t.Fatal("Relay can read TaskDeck credentials")
	}
	response,err:=s.VerifyEncryptedRemoteLogin(context.Background(),agent,grant,request)
	if err!=nil{t.Fatalf("valid TaskDeck shared account rejected: %v",err)}
	if bytes.Contains(response,[]byte("authenticated")) {
		t.Fatal("unencrypted authentication state leaked")
	}
	authenticated,err:=remotetransport.OpenRemoteLoginResult(browser,response)
	if err!=nil || !authenticated {t.Fatalf("TaskDeck account login did not verify: %v",err)}
	if _,err:=s.VerifyEncryptedRemoteLogin(context.Background(),agent,grant,request);err==nil{
		t.Fatal("same encrypted login record replayed")
	}
	if agent.Established(){t.Fatal("replayed login did not invalidate Agent channel")}
}

func TestEncryptedTaskDeckLoginFailedPasswordAndOneTimeGrant(t *testing.T) {
	s:=sharedLoginTestServer(t)
	s.Config.RemoteAccessEnabled=true
	s.Config.Protocol=config.ProtocolHTTPS
	browser,agent:=encryptedLoginPair(t)
	grant:=testRemoteSignedGrant(t,agent,"operator")
	wrong,err:=approvedRemoteLoginFrame(t,s,browser,"alice","wrong-password")
	if err!=nil{t.Fatal(err)}
	response,err:=s.VerifyEncryptedRemoteLogin(context.Background(),agent,grant,wrong)
	if err!=nil{t.Fatal(err)}
	ok,err:=remotetransport.OpenRemoteLoginResult(browser,response)
	if err!=nil || ok{t.Fatal("failed password was accepted by TaskDeck")}
	second,err:=approvedRemoteLoginFrame(t,s,browser,"alice","private-admin-password")
	if err!=nil{t.Fatal(err)}
	reply,err:=s.VerifyEncryptedRemoteLogin(context.Background(),agent,grant,second)
	if err!=nil{t.Fatal(err)}
	ok,err=remotetransport.OpenRemoteLoginResult(browser,reply)
	if err!=nil || ok {t.Fatal("one Portal grant allowed a second password guess")}
}

func TestEncryptedTaskDeckLoginRejectsCrossChannelAndUntrustedPayloads(t *testing.T) {
	s:=sharedLoginTestServer(t)
	s.Config.RemoteAccessEnabled=true
	s.Config.Protocol=config.ProtocolHTTPS
	browser,agent:=encryptedLoginPair(t)
	_,foreign:=encryptedLoginPair(t)
	grant:=testRemoteSignedGrant(t,agent,"admin")
	request,err:=approvedRemoteLoginFrame(t,s,browser,"alice","private-admin-password")
	if err!=nil{t.Fatal(err)}
	if _,err:=s.VerifyEncryptedRemoteLogin(context.Background(),foreign,grant,request);err==nil {
		t.Fatal("grant was transferred to a different E2EE channel")
	}
	if _,err:=s.VerifyEncryptedRemoteLogin(context.Background(),agent,nil,request);err==nil {
		t.Fatal("anonymous Portal grant accepted")
	}
	s.Config.RemoteAccessEnabled=false
	if _,err:=s.VerifyEncryptedRemoteLogin(context.Background(),agent,grant,request);err==nil {
		t.Fatal("disabled Remote flag allowed encrypted password processing")
	}
	s.Config.RemoteAccessEnabled=true
	badFrame,err:=browser.Seal([]byte(`{"type":"terminal.run","username":"alice","password":"private-admin-password"}`))
	if err!=nil{t.Fatal(err)}
	if _,err:=s.VerifyEncryptedRemoteLogin(context.Background(),agent,grant,badFrame);err==nil{
		t.Fatal("arbitrary TaskDeck command was parsed as authentication")
	}
	if agent.Established(){t.Fatal("invalid encrypted operation failed to close channel")}
}

// Model total PHP Portal signing-key compromise: the attacker can sign any
// syntactically valid device/user/admin ticket and control every "allowed"
// answer returned by Portal. This cannot replace the TaskDeck-local scrypt
// password check, generate a TaskDeck HTTP cookie or open a remote command.
// It does NOT model a user entering their password into malicious PHP-served
// JavaScript; that user-facing credential path is forbidden separately.
func TestRemoteCompromisedPortalSignerCannotBypassTaskDeckPassword(t *testing.T) {
 s:=sharedLoginTestServer(t)
 s.Config.RemoteAccessEnabled=true
 s.Config.Protocol=config.ProtocolHTTPS
 for _,tc:=range []struct {
  name,username,password string
 }{
  {"forged admin with wrong password","alice","attacker-controlled-password"},
  {"forged admin with unknown account","not-a-real-user","attacker-controlled-password"},
  {"forged admin without password","alice",""},
 }{
  t.Run(tc.name,func(t *testing.T) {
   browser,agent:=encryptedLoginPair(t)
   // testRemoteSignedGrant mints a REAL, cryptographically valid Portal
   // signature with attacker-chosen 'admin' role and browser key binding.
   compromisedPortalTicket:=testRemoteSignedGrant(t,agent,"admin")
   request,err:=approvedRemoteLoginFrame(t,s,browser,tc.username,tc.password)
   if err!=nil {t.Fatal(err)}
   reply,verifyErr:=s.VerifyEncryptedRemoteLogin(
    context.Background(),agent,compromisedPortalTicket,request)
   if verifyErr==nil {
    authenticated,openErr:=remotetransport.OpenRemoteLoginResult(browser,reply)
    if openErr!=nil {t.Fatal(openErr)}
    if authenticated {t.Fatal("attacker-signed admin ticket authenticated without TaskDeck password")}
    if bytes.Contains(reply,[]byte(tc.username)) || bytes.Contains(reply,[]byte(tc.password)) {
     t.Fatal("Remote response disclosed user-supplied login values")
    }
   } else if len(reply)!=0 {
    t.Fatal("failed credential verification returned application data")
   }
  })
 }
}

// A compromised Portal is also free to mint new browser tickets repeatedly.
// The TaskDeck local credential verifier, not a Portal role, must be the
// only source of the successful encrypted login boolean. Successful login
// is still one-shot with no application session or remote-control token.
func TestRemoteCompromisedPortalValidPasswordOnlyGetsOneShotBoolean(t *testing.T) {
 s:=sharedLoginTestServer(t)
 s.Config.RemoteAccessEnabled=true
 s.Config.Protocol=config.ProtocolHTTPS
 browser,agent:=encryptedLoginPair(t)
 attackerSignedGrant:=testRemoteSignedGrant(t,agent,"admin")
 request,err:=approvedRemoteLoginFrame(t,s,browser,
  "alice","private-admin-password")
 if err!=nil {t.Fatal(err)}
 reply,err:=s.VerifyEncryptedRemoteLogin(context.Background(),agent,attackerSignedGrant,request)
 if err!=nil {t.Fatal(err)}
 if ok,err:=remotetransport.OpenRemoteLoginResult(browser,reply);err!=nil || !ok {
  t.Fatalf("correct *local* TaskDeck password not accepted: %v",err)
 }
 if bytes.Contains(reply,[]byte("private-admin-password")) || bytes.Contains(reply,[]byte("alice")) {
  t.Fatal("TaskDeck account data leaked outside E2EE")
 }
 if _,err:=s.VerifyEncryptedRemoteLogin(context.Background(),agent,attackerSignedGrant,request);err==nil{
  t.Fatal("Portal one-time grant authorized a second E2EE login request")
 }
}

type rotatingRemoteLocalStore struct {
	identity.Store
	changed atomic.Bool
	replacementHash string
	disable bool
}

func (s *rotatingRemoteLocalStore) UserByID(ctx context.Context,id identity.ID) (identity.User,error) {
	// Called just AFTER the real scrypt password verification has finished.
	// This deterministic store races the first post-scrypt identity read,
	// without introducing sleeps or changing the production code path.
	if s.changed.CompareAndSwap(false,true) {
		if s.disable {
			if err:=s.Store.SetUserEnabled(ctx,id,false,time.Now().UTC());err!=nil {
				return identity.User{},err
			}
		} else {
			old,err:=s.Store.UserByID(ctx,id)
			if err!=nil{return identity.User{},err}
			if err:=s.Store.ChangeUserPasswordHash(ctx,id,old.PasswordHash,
				s.replacementHash,time.Now().UTC());err!=nil {
				return identity.User{},err
			}
		}
	}
	return s.Store.UserByID(ctx,id)
}

// A fully compromised PHP Portal can issue any signed admin ticket, but
// cannot turn a local password disabled or rotated during scrypt into a
// successful TaskDeck authentication result. This calls the real encrypted
// login entrypoint, not just the snapshot helper.
func TestCompromisedPortalCannotAuthenticateDuringLocalPasswordRevocation(t *testing.T) {
	for _,tc:=range []struct{name string;disable bool}{
		{"password rotation during scrypt verification",false},
		{"local account disabled during scrypt verification",true},
	}{
		t.Run(tc.name,func(t *testing.T){
			s:=sharedLoginTestServer(t)
			s.Config.RemoteAccessEnabled=true
			s.Config.Protocol=config.ProtocolHTTPS
			replacement,err:=identity.HashPassword(context.Background(),
				"new-local-password-not-sent-to-php")
			if err!=nil{t.Fatal(err)}
			trap:=&rotatingRemoteLocalStore{Store:s.Identity,
				replacementHash:replacement,disable:tc.disable}
			s.Identity=trap
			browser,agent:=encryptedLoginPair(t)
			attackerMintedAdminGrant:=testRemoteSignedGrant(t,agent,"admin")
			frame,err:=approvedRemoteLoginFrame(t,s,browser,
				"alice","private-admin-password")
			if err!=nil{t.Fatal(err)}
			response,err:=s.VerifyEncryptedRemoteLogin(context.Background(),
				agent,attackerMintedAdminGrant,frame)
			if err!=nil{t.Fatalf("fail-closed login should return an encrypted denial: %v",err)}
			authenticated,err:=remotetransport.OpenRemoteLoginResult(browser,response)
			if err!=nil{t.Fatal(err)}
			if authenticated{
				t.Fatal("compromised PHP Portal approved stale local TaskDeck credential")
			}
			if !trap.changed.Load(){t.Fatal("concurrent local identity mutation was not exercised")}
			if _,err:=s.VerifyEncryptedRemoteLogin(context.Background(),agent,
				attackerMintedAdminGrant,frame);err==nil {
				t.Fatal("Portal grant was reusable after a revoked local login")
			}
		})
	}
}


// Simulate an attacker-controlled PHP control plane allowing a previously
// valid login, while an actual TaskDeck owner revokes local identity during
// a slow TaskDeck audit write. A one-shot E2EE boolean must not report
// authenticated=true for an identity that no longer has local access.
type pausedRemoteSuccessAuditStore struct {
 identity.Store
 entered chan struct{}
 resume chan struct{}
}
func (store *pausedRemoteSuccessAuditStore) AppendAudit(
 ctx context.Context, event identity.AuditEvent,
) error {
 if event.Action=="auth.remote.login" && event.Result=="success" {
  close(store.entered)
  select {
  case <-store.resume:
  case <-ctx.Done():return ctx.Err()
  }
 }
 return store.Store.AppendAudit(ctx,event)
}

func TestCompromisedPortalCannotReportStaleOneShotLoginAfterLocalRevocationDuringAudit(t *testing.T) {
 for _,name:=range []string{"password rotated","account disabled","project membership revoked"} {
  t.Run(name,func(t *testing.T){
   s:=sharedLoginTestServer(t)
   s.Config.RemoteAccessEnabled=true
   s.Config.Protocol=config.ProtocolHTTPS
   paused:=&pausedRemoteSuccessAuditStore{
    Store:s.Identity,entered:make(chan struct{}),resume:make(chan struct{}),
   }
   s.Identity=paused
   defer func(){select{case <-paused.resume:default:close(paused.resume)}}()
   ctx:=context.Background()
   // Compute scrypt before starting the timed barrier. The test changes
   // only the TaskDeck-local identity database, not PHP/Portal state.
   replacement,err:=identity.HashPassword(ctx,"new-TaskDeck-only-credential-after-revocation")
   if err!=nil{t.Fatal(err)}
   browser,agent:=encryptedLoginPair(t)
   defer browser.Close()
   defer agent.Close()
   portalAdminGrant:=testRemoteSignedGrant(t,agent,"admin")
   frame,err:=approvedRemoteLoginFrame(t,s,browser,"alice","private-admin-password")
   if err!=nil{t.Fatal(err)}
   type outcome struct{ response []byte; err error }
   completed:=make(chan outcome,1)
   go func(){
    response,err:=s.VerifyEncryptedRemoteLogin(ctx,agent,portalAdminGrant,frame)
    completed<-outcome{response,err}
   }()
   select{
   case <-paused.entered:
   case <-time.After(8*time.Second):
    t.Fatal("TaskDeck login did not reach post-scrypt success audit barrier")
   }
   user,err:=paused.Store.UserByUsername(ctx,"alice")
   if err!=nil{t.Fatal(err)}
   switch name {
   case "password rotated":
    err=paused.Store.ChangeUserPasswordHash(ctx,user.ID,user.PasswordHash,
     replacement,time.Now().UTC().Add(-time.Hour))
   case "account disabled":
    err=paused.Store.SetUserEnabled(ctx,user.ID,false,time.Now().UTC())
   case "project membership revoked":
    var project identity.Project
    project,err=paused.Store.ProjectByKey(ctx,s.Config.SharedProjectID)
    if err==nil{
     var member identity.ProjectMember
     member,err=paused.Store.ProjectMember(ctx,project.ID,user.ID)
     if err==nil {member.Enabled=false;err=paused.Store.UpsertProjectMember(ctx,member)}
    }
   }
   if err!=nil{t.Fatal(err)}
   close(paused.resume)
   select{
   case result:=<-completed:
    if result.err!=nil{t.Fatalf("local revocation should produce a sealed denial: %v",result.err)}
    accepted,err:=remotetransport.OpenRemoteLoginResult(browser,result.response)
    if err!=nil{t.Fatalf("invalid encrypted denial: %v",err)}
    if accepted{t.Fatal("P0: compromised PHP still received TaskDeck authentication success after local revocation")}
   case <-time.After(8*time.Second):
    t.Fatal("one-shot login hung after TaskDeck owner revoked identity")
   }
  })
 }
}


// Password theft by PHP alone is insufficient. The V1 legacy password-only
// codec still exists for diagnostic compatibility, but the real TaskDeck
// login entrypoint refuses it even when the local password is correct.
func TestCompromisedPortalWithStolenPasswordCannotLoginWithoutApprovedClientKey(t *testing.T){
 s:=sharedLoginTestServer(t)
 s.Config.RemoteAccessEnabled=true
 s.Config.Protocol=config.ProtocolHTTPS
 browser,agent:=encryptedLoginPair(t)
 grant:=testRemoteSignedGrant(t,agent,"admin")
 frame,err:=remotetransport.RemoteLoginRequestFrame(browser,"alice","private-admin-password")
 if err!=nil{t.Fatal(err)}
 sealed,err:=s.VerifyEncryptedRemoteLogin(context.Background(),agent,grant,frame)
 if err!=nil{t.Fatal(err)}
 authenticated,err:=remotetransport.OpenRemoteLoginResult(browser,sealed)
 if err!=nil{t.Fatal(err)}
 if authenticated{t.Fatal("P0: PHP-stolen TaskDeck password bypassed host-approved client key")}
}

func TestCompromisedPortalCannotForgeClientPrivateKeyOrReuseE2EEProof(t *testing.T){
 for _,kind:=range []string{"unauthorized client","approved key but invalid signature","approved signature replay from foreign channel","approved key revoked"}{
  t.Run(kind,func(t *testing.T){
   s:=sharedLoginTestServer(t)
   s.Config.RemoteAccessEnabled=true
   s.Config.Protocol=config.ProtocolHTTPS
   browser,agent:=encryptedLoginPair(t)
   grant:=testRemoteSignedGrant(t,agent,"admin")
   pub,priv,err:=ed25519.GenerateKey(rand.Reader)
   if err!=nil{t.Fatal(err)}
   public:=base64.StdEncoding.EncodeToString(pub)
   path,err:=remoteclientkey.DefaultPath(s.Workspace)
   if err!=nil{t.Fatal(err)}
   if kind!="unauthorized client"{
    entry,err:=remoteclientkey.Add(path,"alice",public,"approved")
    if err!=nil{t.Fatal(err)}
    if kind=="approved key revoked"{
     if err:=remoteclientkey.Remove(path,entry.Fingerprint);err!=nil{t.Fatal(err)}
    }
   }
   binding:=browser.RemoteClientProofBinding()
   if kind=="approved signature replay from foreign channel"{
    other,_:=encryptedLoginPair(t)
    binding=other.RemoteClientProofBinding()
   }
   signature:=base64.StdEncoding.EncodeToString(ed25519.Sign(priv,
    remoteclientkey.ProofMessage("alice",binding)))
   if kind=="approved key but invalid signature"{
    signature=base64.StdEncoding.EncodeToString(make([]byte,ed25519.SignatureSize))
   }
   frame,err:=remotetransport.RemoteLoginRequestWithClientKey(browser,
    "alice","private-admin-password",public,signature)
   if err!=nil{t.Fatal(err)}
   sealed,err:=s.VerifyEncryptedRemoteLogin(context.Background(),agent,grant,frame)
   if err!=nil{t.Fatal(err)}
   accepted,err:=remotetransport.OpenRemoteLoginResult(browser,sealed)
   if err!=nil{t.Fatal(err)}
   if accepted{t.Fatal("P0: malicious PHP obtained TaskDeck positive login without approved channel-bound key")}
  })
 }
}
