package server

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/config"
	"bletonfc/vscode_tasks_menu/internal/identity"
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

func TestEncryptedTaskDeckLoginWithRealAccountHasNoSessionToken(t *testing.T) {
	s:=sharedLoginTestServer(t)
	s.Config.RemoteAccessEnabled=true
	s.Config.Protocol=config.ProtocolHTTPS
	browser,agent:=encryptedLoginPair(t)
	grant:=testRemoteSignedGrant(t,agent,"viewer")
	request,err:=remotetransport.RemoteLoginRequestFrame(browser,"alice","private-admin-password")
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
	wrong,err:=remotetransport.RemoteLoginRequestFrame(browser,"alice","wrong-password")
	if err!=nil{t.Fatal(err)}
	response,err:=s.VerifyEncryptedRemoteLogin(context.Background(),agent,grant,wrong)
	if err!=nil{t.Fatal(err)}
	ok,err:=remotetransport.OpenRemoteLoginResult(browser,response)
	if err!=nil || ok{t.Fatal("failed password was accepted by TaskDeck")}
	second,err:=remotetransport.RemoteLoginRequestFrame(browser,"alice","private-admin-password")
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
	request,err:=remotetransport.RemoteLoginRequestFrame(browser,"alice","private-admin-password")
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
   request,err:=remotetransport.RemoteLoginRequestFrame(browser,tc.username,tc.password)
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
 request,err:=remotetransport.RemoteLoginRequestFrame(browser,
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
			frame,err:=remotetransport.RemoteLoginRequestFrame(browser,
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
