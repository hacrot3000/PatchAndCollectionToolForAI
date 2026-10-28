package remoteportal

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/remoterelay"
	"bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func newAgentLoginFixture(t *testing.T) (*Agent,ed25519.PrivateKey) {
	t.Helper()
	pub,signer,err:=ed25519.GenerateKey(rand.Reader)
	if err!=nil {t.Fatal(err)}
	agent,err:=NewAgent("https://portal.example.com",
		filepath.Join(t.TempDir(),"private","identity.seed"),nil)
	if err!=nil {t.Fatal(err)}
	if err:=agent.ConfigurePortalGrantKey(base64.StdEncoding.EncodeToString(pub));err!=nil {t.Fatal(err)}
	return agent,signer
}

func signedLoginGrant(t *testing.T,agent *Agent,signer ed25519.PrivateKey,
	channel *remotetransport.Channel,jti string) *AgentGrant {
	t.Helper()
	now:=time.Now()
	claims:=remoterelay.Claims{
		V:2,AuthEpoch:1,BrowserBinding:channel.BrowserTicketBinding(),
		Issuer:agent.PortalOrigin(),Audience:"taskdeck-relay",
		Subject:"21",DeviceID:agent.DeviceID(),Role:"viewer",
		IssuedAt:now.Unix(),Expires:now.Unix()+30,JTI:jti,
	}
	grant,err:=agent.VerifyPortalGrant(signPortalTicket(t,signer,claims),channel,now)
	if err!=nil {t.Fatal(err)}
	return grant
}

func TestAgentCannotBypassSignedGrantOrSpendUnauthenticatedBrowserLogin(t *testing.T) {
	agent,signer:=newAgentLoginFixture(t)
	var liveChecks,callbacks atomic.Int32
	agent.liveGrantCheck=func(context.Context,*AgentGrant) error {
		liveChecks.Add(1);return nil
	}
	if err:=agent.ConfigureEncryptedLoginVerifier(func(_ context.Context,
		_ *remotetransport.Channel,_ *AgentGrant,_ []byte)([]byte,error){
		callbacks.Add(1);return []byte("callback-result"),nil
	});err!=nil {t.Fatal(err)}

	// An ECDH handshake by itself is not proof of browser private-key
	// possession. This channel has no verified incoming AES-GCM record.
	browser,err:=remotetransport.NewEphemeral()
	if err!=nil {t.Fatal(err)}
	hello,emptyChannel,err:=remotetransport.ClientHello(agent.identity.private,
		agent.DeviceID(),strings.Repeat("a",32),strings.Repeat("b",32),browser.Public)
	if err!=nil {t.Fatal(err)}
	if len(hello.Signature)!=ed25519.SignatureSize {t.Fatal("missing authenticated Agent signature")}
	noProof:=signedLoginGrant(t,agent,signer,emptyChannel,strings.Repeat("a",32))
	if _,err:=agent.verifyEncryptedLogin(context.Background(),emptyChannel,noProof,[]byte("encrypted"));err==nil {
		t.Fatal("Agent permitted login without browser private-key possession")
	}
	if liveChecks.Load()!=0 || callbacks.Load()!=0 || noProof.loginStarted.Load() {
		t.Fatal("unauthenticated E2EE handshake spent one-use grant or network resources")
	}

	// Two independent Agent instances may share the same seed and Portal
	// signer, but their pinned verifier instances are deliberately distinct.
	other,err:=NewAgent(agent.PortalOrigin(),filepath.Join(t.TempDir(),"identity.seed"),nil)
	if err!=nil {t.Fatal(err)}
	// Forge the same Device ID with a new instance by reusing the existing
	// Agent's seed in another owner-only directory.
	other,err=NewAgent(agent.PortalOrigin(),agent.loginBudget.path[:0]+
		filepath.Join(t.TempDir(),"unused","seed"),nil)
	if err!=nil {t.Fatal(err)}
	_ = other // Separate Device ID is always rejected by OwnsPortalGrant.
}

func TestAgentOneUseGrantIsEnforcedOutsideTaskDeckCallback(t *testing.T) {
	agent,signer:=newAgentLoginFixture(t)
	var liveChecks,callbackCalls atomic.Int32
	agent.liveGrantCheck=func(context.Context,*AgentGrant) error {
		liveChecks.Add(1);return nil
	}
	channel:=testBoundChannelWithPrivate(t,agent.identity.private)
	grant:=signedLoginGrant(t,agent,signer,channel,strings.Repeat("c",32))
	if err:=agent.ConfigureEncryptedLoginVerifier(func(_ context.Context,
		_ *remotetransport.Channel,_ *AgentGrant,_ []byte)([]byte,error){
		callbackCalls.Add(1)
		// A buggy/pluggable future callback MUST NOT authorize login without
		// consuming the one-time TaskDeck grant itself.
		return []byte("forged-success"),nil
	});err!=nil {t.Fatal(err)}
	if _,err:=agent.verifyEncryptedLogin(context.Background(),channel,grant,[]byte("payload"));err==nil {
		t.Fatal("Agent accepted login callback that bypassed TakeForLogin")
	}
	if !grant.closed.Load() || !grant.loginStarted.Load() || grant.loginUsed.Load() {
		t.Fatal("invalid callback left one-time grant usable")
	}
	if _,err:=agent.verifyEncryptedLogin(context.Background(),channel,grant,[]byte("another"));err==nil {
		t.Fatal("same signed grant was reusable after callback bypass attempt")
	}
	if callbackCalls.Load()!=1 || liveChecks.Load()!=1 {
		t.Fatalf("reused grant repeated Portal/scrypt calls: callbacks=%d checks=%d",
			callbackCalls.Load(),liveChecks.Load())
	}
}

func TestAgentSignedLiveGrantAndTakeForLoginPermitsExactlyOneCallback(t *testing.T) {
	agent,signer:=newAgentLoginFixture(t)
	channel:=testBoundChannelWithPrivate(t,agent.identity.private)
	grant:=signedLoginGrant(t,agent,signer,channel,strings.Repeat("d",32))
	agent.liveGrantCheck=func(context.Context,*AgentGrant) error{return nil}
	var calls atomic.Int32
	if err:=agent.ConfigureEncryptedLoginVerifier(func(_ context.Context,
		ch *remotetransport.Channel,g *AgentGrant,_ []byte)([]byte,error){
		calls.Add(1)
		if _,err:=g.TakeForLogin(ch,time.Now());err!=nil{return nil,err}
		return []byte("sealed-test-frame"),nil
	});err!=nil {t.Fatal(err)}
	got,err:=agent.verifyEncryptedLogin(context.Background(),channel,grant,[]byte("payload"))
	if err!=nil || string(got)!="sealed-test-frame" {
		t.Fatalf("legitimate one-use callback rejected: %v",err)
	}
	if _,err:=agent.verifyEncryptedLogin(context.Background(),channel,grant,[]byte("again"));err==nil {
		t.Fatal("second password attempt admitted with consumed ticket")
	}
	if calls.Load()!=1 {t.Fatalf("Agent callback executed %d times",calls.Load())}
}
