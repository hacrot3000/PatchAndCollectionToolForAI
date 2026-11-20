package remoteportal

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/remoterelay"
	"bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func TestSignedAgentGrantIntrospectionOverHTTPS(t *testing.T){
	portalPub,portalPrivate,err:=ed25519.GenerateKey(rand.Reader)
	if err!=nil{t.Fatal(err)}
	var permitted atomic.Bool
	permitted.Store(true)
	var failing atomic.Bool
	var seen atomic.Int32
	used:=make(map[string]struct{})
	var mu sync.Mutex
	server:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		if r.URL.Path!="/api/agent/grant-check" || r.Method!=http.MethodPost {
			http.Error(w,"unexpected path",http.StatusNotFound);return
		}
		if failing.Load(){http.Error(w,"control plane offline",http.StatusServiceUnavailable);return}
		var envelope liveGrantCheckMessage
		if err:=json.NewDecoder(r.Body).Decode(&envelope);err!=nil{
			http.Error(w,"bad envelope",http.StatusForbidden);return
		}
		p:=envelope.signedMessage
		identityKey,err:=base64.StdEncoding.Strict().DecodeString(p.PublicKey)
		signature,sigErr:=base64.StdEncoding.Strict().DecodeString(p.Signature)
		fingerprint:=sha256.Sum256(identityKey)
		expected:=strings.Join([]string{
			protocol,"grant-check",p.DeviceID,envelope.PortalUserID,
			envelope.AccessRole,envelope.TicketJTI,
			fmt.Sprint(envelope.PortalAuthEpoch),
			fmt.Sprint(p.Timestamp),p.Nonce,
		},"\n")
		if err!=nil || sigErr!=nil || len(identityKey)!=ed25519.PublicKeySize ||
			hex.EncodeToString(fingerprint[:])!=p.DeviceID ||
			!ed25519.Verify(ed25519.PublicKey(identityKey),[]byte(expected),signature) ||
			envelope.PortalUserID!="21" || envelope.AccessRole!="viewer" ||
			envelope.TicketJTI!=strings.Repeat("f",32) || envelope.PortalAuthEpoch!=2 ||
			p.Timestamp<time.Now().Unix()-30 || p.Timestamp>time.Now().Unix()+5 {
			http.Error(w,"invalid proof",http.StatusForbidden);return
		}
		mu.Lock()
		if _,ok:=used[p.Nonce];ok {
			mu.Unlock();http.Error(w,"replay",http.StatusForbidden);return
		}
		used[p.Nonce]=struct{}{}
		mu.Unlock()
		seen.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok":true,"allowed":permitted.Load()})
	}))
	defer server.Close()
	agent,err:=NewAgent(server.URL,filepath.Join(t.TempDir(),"private","identity.seed"),server.Client())
	if err!=nil{t.Fatal(err)}
	if err:=agent.ConfigurePortalGrantKey(base64.StdEncoding.EncodeToString(portalPub));err!=nil{t.Fatal(err)}
	ch:=testBoundChannelWithPrivate(t,agent.identity.private)
	now:=time.Now()
	claims:=remoterelay.Claims{V:2,BrowserBinding:ch.BrowserTicketBinding(),
		Issuer:server.URL,Audience:"taskdeck-relay",Subject:"21",
		DeviceID:agent.DeviceID(),Role:"viewer",AuthEpoch:2,IssuedAt:now.Unix(),
		Expires:now.Unix()+30,JTI:strings.Repeat("f",32)}
	grant,err:=agent.VerifyPortalGrant(signPortalTicket(t,portalPrivate,claims),ch,now)
	if err!=nil{t.Fatal(err)}
	if err:=agent.VerifyCurrentPortalGrant(context.Background(),grant);err==nil{
		t.Fatal("Portal ticket without one-use TaskDeck login granted application rights")
	}
	if _,err:=grant.TakeForLogin(ch,now);err!=nil{t.Fatal(err)}
	if err:=agent.VerifyCurrentPortalGrant(context.Background(),grant);err!=nil{
		t.Fatalf("legitimately granted Agent rejected: %v",err)
	}
	permitted.Store(false)
	if err:=agent.VerifyCurrentPortalGrant(context.Background(),grant);err==nil{
		t.Fatal("disabled/revoked Portal grant retained authority")
	}
	permitted.Store(true)
	failing.Store(true)
	if err:=agent.VerifyCurrentPortalGrant(context.Background(),grant);err==nil{
		t.Fatal("unavailable Portal control plane failed open")
	}
	failing.Store(false)
	other,err:=NewAgent(server.URL,filepath.Join(t.TempDir(),"private","other.seed"),server.Client())
	if err!=nil{t.Fatal(err)}
	if err:=other.VerifyCurrentPortalGrant(context.Background(),grant);err==nil {
		t.Fatal("other signed Device ID inherited this Agent's grant")
	}
	grant.Close()
	if err:=agent.VerifyCurrentPortalGrant(context.Background(),grant);err==nil {
		t.Fatal("closed Portal grant retained application rights")
	}
	if seen.Load()!=2{
		t.Fatalf("unexpected signed Portal grant checks: %d",seen.Load())
	}
}


func TestAgentRevokedGrantCannotReachPasswordVerifier(t *testing.T) {
	server:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		http.Error(w,"grant revoked",http.StatusForbidden)
	}))
	defer server.Close()
	agent,err:=NewAgent(server.URL,filepath.Join(t.TempDir(),"private","seed"),server.Client())
	if err!=nil{t.Fatal(err)}
	channel:=testBoundChannel(t)
	grant:=&AgentGrant{subject:"21",ticketJTI:strings.Repeat("a",32),authEpoch:2,
		channel:channel,role:"viewer",expiresAt:time.Now().Add(time.Minute)}
	var calls atomic.Int32
	if err:=agent.ConfigureEncryptedLoginVerifier(func(_ context.Context,
		_ *remotetransport.Channel,_ *AgentGrant,_ []byte)([]byte,error){
		calls.Add(1);return []byte("not-a-real-result"),nil
	});err!=nil{t.Fatal(err)}
	if _,err:=agent.verifyEncryptedLogin(context.Background(),channel,grant,[]byte("opaque"));err==nil {
		t.Fatal("revoked Portal grant reached TaskDeck password verifier")
	}
	if calls.Load()!=0 {
		t.Fatal("revoked Portal grant caused expensive TaskDeck password computation")
	}
}

// An HTTPS introspection response is only a point-in-time answer. If the
// Browser disconnects, local owner closes the grant, or AEAD transport dies
// while that request is in flight, the late "allowed" reply is not authority.
func TestAgentGrantIntrospectionCannotResurrectClosedChannel(t *testing.T) {
 portalPub,portalPriv,err:=ed25519.GenerateKey(rand.Reader)
 if err!=nil {t.Fatal(err)}
 agent,err:=NewAgent("https://portal.example.com",
  filepath.Join(t.TempDir(),"private","identity.seed"),nil)
 if err!=nil {t.Fatal(err)}
 if err:=agent.ConfigurePortalGrantKey(base64.StdEncoding.EncodeToString(portalPub));err!=nil {t.Fatal(err)}
 for i,tc:=range []struct{name string;closeGrant bool}{
  {"grant closed during Portal check",true},
  {"E2EE disconnected during Portal check",false},
 } {
  t.Run(tc.name,func(t *testing.T){
   channel:=testBoundChannelWithPrivate(t,agent.identity.private)
   now:=time.Now()
   claims:=remoterelay.Claims{
    V:2,AuthEpoch:1,BrowserBinding:channel.BrowserTicketBinding(),
    Issuer:"https://portal.example.com",Audience:"taskdeck-relay",
    Subject:"21",DeviceID:agent.DeviceID(),Role:"viewer",
    IssuedAt:now.Unix(),Expires:now.Unix()+30,
    JTI:fmt.Sprintf("%032x",i+1),
   }
   grant,err:=agent.VerifyPortalGrant(signPortalTicket(t,portalPriv,claims),channel,now)
   if err!=nil {t.Fatal(err)}
   defer grant.Close()
   defer channel.Close()
   if _,err:=grant.TakeForLogin(channel,now);err!=nil {t.Fatal(err)}
   entered:=make(chan struct{})
   release:=make(chan struct{})
   agent.liveGrantCheck=func(context.Context,*AgentGrant)error {
    close(entered)
    <-release
    return nil // stale positive Portal answer
   }
   done:=make(chan error,1)
   go func(){done<-agent.VerifyCurrentPortalGrant(context.Background(),grant)}()
   select {
   case <-entered:
   case <-time.After(3*time.Second):t.Fatal("Portal check did not start")
   }
   if tc.closeGrant {grant.Close()} else {channel.Close()}
   close(release)
   select {
   case err:=<-done:
    if !errors.Is(err,remotetransport.ErrRemoteActionDenied) {
     t.Fatalf("late Portal grant response revived revoked session: %v",err)
    }
   case <-time.After(3*time.Second):t.Fatal("stale Portal check failed to return")
   }
  })
 }
}
