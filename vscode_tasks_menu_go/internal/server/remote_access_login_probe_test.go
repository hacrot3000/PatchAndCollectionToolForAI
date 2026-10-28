package server

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/config"
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
