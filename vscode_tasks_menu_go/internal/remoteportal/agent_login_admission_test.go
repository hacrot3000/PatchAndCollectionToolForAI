package remoteportal

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	if _,err:=noProof.TakeForLogin(emptyChannel,time.Now());err==nil {
		t.Fatal("ECDH setup without authenticated browser AES-GCM proof allowed TaskDeck login")
	}
	if _,err:=agent.verifyEncryptedLogin(context.Background(),emptyChannel,noProof,[]byte("encrypted"));err==nil {
		t.Fatal("callback that skipped decryption granted a login without browser private-key possession")
	}
	if liveChecks.Load()!=1 || callbacks.Load()!=1 || !noProof.loginStarted.Load() ||
		!noProof.closed.Load() || noProof.loginUsed.Load() {
		t.Fatal("unverified callback did not permanently consume and reject one login attempt")
	}

	// Two agents intentionally share the same local device signing seed
	// and pinned Portal public key, but maintain independent verifier
	// instances. A grant verified by the other Agent is still not owned
	// by the configured Agent, even when origin and Device ID are identical.
	otherSeed:=filepath.Join(t.TempDir(),"private","identity.seed")
	if err:=os.MkdirAll(filepath.Dir(otherSeed),0700);err!=nil{t.Fatal(err)}
	if err:=os.WriteFile(otherSeed,agent.identity.private.Seed(),0600);err!=nil{t.Fatal(err)}
	other,err:=NewAgent(agent.PortalOrigin(),otherSeed,nil)
	if err!=nil{t.Fatal(err)}
	if err:=other.ConfigurePortalGrantKey(base64.StdEncoding.EncodeToString(
		signer.Public().(ed25519.PublicKey)));err!=nil {t.Fatal(err)}
	channel:=testBoundChannelWithPrivate(t,agent.identity.private)
	foreign:=signedLoginGrant(t,other,signer,channel,strings.Repeat("b",32))
	if _,err:=agent.verifyEncryptedLogin(context.Background(),channel,foreign,[]byte("payload"));err==nil {
		t.Fatal("Agent trusted another instance's Portal signature verifier")
	}
	if foreign.loginStarted.Load() || liveChecks.Load()!=1 || callbacks.Load()!=1 {
		t.Fatal("untrusted foreign verifier reached live Portal check or login callback")
	}
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
		return remotetransport.SealRemoteLoginResult(ch,true)
	});err!=nil {t.Fatal(err)}
	got,err:=agent.verifyEncryptedLogin(context.Background(),channel,grant,[]byte("payload"))
	if err!=nil || remotetransport.VerifyOutgoingRemoteLoginResult(channel,got)!=nil {
		t.Fatalf("legitimate encrypted boolean callback rejected: %v",err)
	}
	if _,err:=agent.verifyEncryptedLogin(context.Background(),channel,grant,[]byte("again"));err==nil {
		t.Fatal("second password attempt admitted with consumed ticket")
	}
	if calls.Load()!=1 {t.Fatalf("Agent callback executed %d times",calls.Load())}
	preUsed:=signedLoginGrant(t,agent,signer,channel,strings.Repeat("e",32))
	if _,err:=preUsed.TakeForLogin(channel,time.Now());err!=nil {t.Fatal(err)}
	if _,err:=agent.verifyEncryptedLogin(context.Background(),channel,preUsed,[]byte("again"));err==nil {
		t.Fatal("pre-consumed grant laundered through Agent login callback")
	}
	if calls.Load()!=1 || preUsed.loginStarted.Load() {
		t.Fatal("pre-consumed grant reached callback or Agent one-use admission")
	}
}

func TestAgentOneSignedTicketCannotTriggerConcurrentPasswordVerifiers(t *testing.T) {
	agent,signer:=newAgentLoginFixture(t)
	ch:=testBoundChannelWithPrivate(t,agent.identity.private)
	grant:=signedLoginGrant(t,agent,signer,ch,strings.Repeat("f",32))
	var checks,called,accepted atomic.Int32
	agent.liveGrantCheck=func(context.Context,*AgentGrant)error {
		checks.Add(1)
		return nil
	}
	if err:=agent.ConfigureEncryptedLoginVerifier(func(_ context.Context,
		channel *remotetransport.Channel,g *AgentGrant,_ []byte)([]byte,error){
		called.Add(1)
		if _,err:=g.TakeForLogin(channel,time.Now());err!=nil{return nil,err}
		return remotetransport.SealRemoteLoginResult(channel,true)
	});err!=nil{t.Fatal(err)}
	const concurrent=40
	var wg sync.WaitGroup
	start:=make(chan struct{})
	for i:=0;i<concurrent;i++{
		wg.Add(1)
		go func(){
			defer wg.Done()
			<-start
			if _,err:=agent.verifyEncryptedLogin(context.Background(),ch,grant,[]byte("opaque"));err==nil {
				accepted.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()
	if accepted.Load()!=1 || checks.Load()!=2 || called.Load()!=1 {
		t.Fatalf("one Portal ticket reached multiple login callbacks or skipped post-login revocation: accepted=%d checks=%d called=%d",
			accepted.Load(),checks.Load(),called.Load())
	}
}

func TestAgentRevokedValidTicketNeverReachesPasswordCallback(t *testing.T) {
	agent,signer:=newAgentLoginFixture(t)
	ch:=testBoundChannelWithPrivate(t,agent.identity.private)
	grant:=signedLoginGrant(t,agent,signer,ch,strings.Repeat("1",32))
	var checks,called atomic.Int32
	agent.liveGrantCheck=func(context.Context,*AgentGrant)error{
		checks.Add(1)
		return errors.New("Portal revoked grant")
	}
	if err:=agent.ConfigureEncryptedLoginVerifier(func(_ context.Context,
		_ *remotetransport.Channel,_ *AgentGrant,_ []byte)([]byte,error){
		called.Add(1)
		return []byte("unexpected"),nil
	});err!=nil {t.Fatal(err)}
	for i:=0;i<3;i++ {
		if _,err:=agent.verifyEncryptedLogin(context.Background(),ch,grant,[]byte("ciphertext"));err==nil{
			t.Fatal("revoked signed ticket reached TaskDeck login")
		}
	}
	if checks.Load()!=1 || called.Load()!=0 || !grant.closed.Load() {
		t.Fatalf("revoked ticket caused repeated live-check or password work: checks=%d callback=%d",
			checks.Load(),called.Load())
	}
}

func TestAgentNeverForwardsPlaintextOrUnauthorizedSealedLoginCallbackData(t *testing.T) {
	for _,tc:=range []struct{
		name string
		result func(*remotetransport.Channel)([]byte,error)
	}{
		{"plaintext",func(*remotetransport.Channel)([]byte,error){
			return []byte(`{"username":"alice","authenticated":true,"password":"LEAK"}`),nil
		}},
		{"encrypted bearer",func(ch *remotetransport.Channel)([]byte,error){
			return ch.Seal([]byte(`{"type":"taskdeck-e2ee-login-result-v1","authenticated":true,"remote_control_available":false,"token":"LEAK"}`))
		}},
	} {
		t.Run(tc.name,func(t *testing.T){
			agent,signer:=newAgentLoginFixture(t)
			ch:=testBoundChannelWithPrivate(t,agent.identity.private)
			grant:=signedLoginGrant(t,agent,signer,ch,strings.Repeat("2",32))
			agent.liveGrantCheck=func(context.Context,*AgentGrant)error{return nil}
			if err:=agent.ConfigureEncryptedLoginVerifier(func(_ context.Context,
				channel *remotetransport.Channel,g *AgentGrant,_ []byte)([]byte,error){
				if _,err:=g.TakeForLogin(channel,time.Now());err!=nil{return nil,err}
				return tc.result(channel)
			});err!=nil{t.Fatal(err)}
			raw,err:=agent.verifyEncryptedLogin(context.Background(),ch,grant,[]byte("test"))
			if err==nil || raw!=nil || !grant.closed.Load() || ch.Established() {
				t.Fatal("Agent leaked callback reply or kept misconfigured E2EE login open")
			}
		})
	}
}

// A legitimate one-shot login must not release its sealed TaskDeck boolean
// after the Portal revokes access while the costly local credential check is
// running. A second signed HTTPS introspection is required on completion.
func TestAgentLoginRejectsPortalRevocationDuringPasswordVerification(t *testing.T) {
 agent,signer:=newAgentLoginFixture(t)
 ch:=testBoundChannelWithPrivate(t,agent.identity.private)
 grant:=signedLoginGrant(t,agent,signer,ch,strings.Repeat("3",32))
 var checks,callbackCalls atomic.Int32
 var revoked atomic.Bool
 agent.liveGrantCheck=func(context.Context,*AgentGrant)error{
  checks.Add(1)
  if revoked.Load(){return errors.New("Portal grant revoked during scrypt")}
  return nil
 }
 started:=make(chan struct{})
 release:=make(chan struct{})
 if err:=agent.ConfigureEncryptedLoginVerifier(func(_ context.Context,
  channel *remotetransport.Channel,g *AgentGrant,_ []byte)([]byte,error){
  callbackCalls.Add(1)
  if _,err:=g.TakeForLogin(channel,time.Now());err!=nil{return nil,err}
  close(started)
  <-release
  return remotetransport.SealRemoteLoginResult(channel,true)
 });err!=nil{t.Fatal(err)}
 done:=make(chan error,1)
 go func(){
  ciphertext,err:=agent.verifyEncryptedLogin(context.Background(),ch,grant,[]byte("encrypted"))
  if len(ciphertext)!=0 {done<-errors.New("stale authenticated result was exposed");return}
  done<-err
 }()
 select{
 case <-started:
 case <-time.After(5*time.Second):t.Fatal("TaskDeck credential verifier did not start")
 }
 revoked.Store(true)
 close(release)
 select{
 case err:=<-done:
  if !errors.Is(err,remotetransport.ErrRemoteActionDenied){
   t.Fatalf("late Portal revocation allowed login: %v",err)
  }
 case <-time.After(5*time.Second):t.Fatal("revoked login did not finish")
 }
 if ch.Established() || !grant.closed.Load() ||
  callbackCalls.Load()!=1 || checks.Load()!=2 {
  t.Fatalf("revoked verifier left E2EE alive or skipped second grant check: checks=%d callbacks=%d",
   checks.Load(),callbackCalls.Load())
 }
}
