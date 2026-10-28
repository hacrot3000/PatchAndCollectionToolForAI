package remoteportal

import (
 "crypto/ed25519"
 "crypto/rand"
 "crypto/sha256"
 "encoding/base64"
 "encoding/hex"
 "encoding/json"
 "path/filepath"
 "strings"
 "sync"
 "sync/atomic"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/remoterelay"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func testBoundChannel(t *testing.T) *remotetransport.Channel {
 t.Helper()
 _,private,err:=ed25519.GenerateKey(rand.Reader)
 if err!=nil{t.Fatal(err)}
 return testBoundChannelWithPrivate(t,private)
}

func testBoundChannelWithPrivate(t *testing.T,private ed25519.PrivateKey) *remotetransport.Channel {
 t.Helper()
 pub:=private.Public().(ed25519.PublicKey)
 sum:=sha256.Sum256(pub)
 browser,err:=remotetransport.NewEphemeral()
 if err!=nil{t.Fatal(err)}
 hello,agent,err:=remotetransport.ClientHello(private,hex.EncodeToString(sum[:]),
  strings.Repeat("a",32),strings.Repeat("b",32),browser.Public)
 if err!=nil{t.Fatal(err)}
 browserChannel,err:=remotetransport.AcceptHello(browser,hex.EncodeToString(sum[:]),
  strings.Repeat("a",32),strings.Repeat("b",32),hello)
 if err!=nil{t.Fatal(err)}
 clear,err:=browserChannel.Seal([]byte("authenticated-browser-possession"))
 if err!=nil{t.Fatal(err)}
 if _,err:=agent.Open(clear);err!=nil{t.Fatal(err)}
 return agent
}

func signPortalTicket(t *testing.T,key ed25519.PrivateKey,c remoterelay.Claims) string {
 t.Helper()
 body,err:=json.Marshal(c)
 if err!=nil{t.Fatal(err)}
 encoded:=base64.RawURLEncoding.EncodeToString(body)
 signature:=ed25519.Sign(key,[]byte("taskdeck-relay-ticket-v1."+encoded))
 return encoded+"."+base64.RawURLEncoding.EncodeToString(signature)
}

func TestAgentIndependentPortalGrantVerificationAndChannelBinding(t *testing.T) {
 public,private,err:=ed25519.GenerateKey(rand.Reader)
 if err!=nil{t.Fatal(err)}
 first:=testBoundChannel(t)
 second:=testBoundChannel(t)
 deviceID:=first.BoundDeviceID()
 v,err:=NewAgentGrantVerifier(base64.StdEncoding.EncodeToString(public),
  "https://portal.example.com",deviceID)
 if err!=nil{t.Fatal(err)}
 now:=time.Now()
 claims:=remoterelay.Claims{AuthEpoch:1,V:1,Issuer:"https://portal.example.com",
  Audience:"taskdeck-relay",Subject:"21",DeviceID:deviceID,Role:"viewer",
  IssuedAt:now.Unix(),Expires:now.Unix()+30,JTI:strings.Repeat("b",32)}
 first:=testBoundChannel(t)
 second:=testBoundChannel(t)
 claims.V=2
 claims.BrowserBinding=first.BrowserTicketBinding()
 token:=signPortalTicket(t,private,claims)
 // A Relay can forward the signed ticket but cannot swap in a different
 // browser ECDH public key and retain a usable Agent grant.
 wrongVerifier,err:=NewAgentGrantVerifier(base64.StdEncoding.EncodeToString(public),
  "https://portal.example.com",deviceID)
 if err!=nil{t.Fatal(err)}
 if _,err:=wrongVerifier.Consume(token,second,now);err==nil{
  t.Fatal("Agent accepted signed Portal ticket on substituted E2EE channel")
 }
 if _,err:=v.Consume(token,nil,now);err==nil {t.Fatal("grant accepted without E2EE")}
 if _,err:=v.Consume(token,&remotetransport.Channel{},now);err==nil {
  t.Fatal("grant accepted zero-value channel")
 }
 grant,err:=v.Consume(token,first,now)
 if err!=nil{t.Fatal(err)}
 role,err:=grant.RoleFor(first,now)
 if err!=nil||role!=remotetransport.RoleViewer {
  t.Fatalf("verified Portal role unavailable: %s %v",role,err)
 }
 if _,err:=grant.RoleFor(second,now);err==nil {
  t.Fatal("Portal grant transferred to another encrypted channel")
 }
 if _,err:=grant.RoleFor(first,now.Add(31*time.Second));err==nil {
  t.Fatal("expired Portal role still authorized")
 }
 if _,err:=v.Consume(token,second,now);err==nil {
  t.Fatal("one-use Portal grant was replayed to a second encrypted channel")
 }
 first.Close()
 if _,err:=grant.RoleFor(first,now);err==nil {
  t.Fatal("closed encrypted channel retained a Portal grant")
 }
 grant.Close()
 if _,err:=grant.RoleFor(first,now);err==nil {t.Fatal("closed Portal grant remained usable")}
}

func TestAgentGrantSignatureDeviceAndRoleRejections(t *testing.T) {
 public,private,err:=ed25519.GenerateKey(rand.Reader)
 if err!=nil{t.Fatal(err)}
 key:=base64.StdEncoding.EncodeToString(public)
 deviceID:=strings.Repeat("a",64)
 now:=time.Now()
 claims:=remoterelay.Claims{AuthEpoch:1,V:2,BrowserBinding:strings.Repeat("f",64),Issuer:"https://portal.example.com",
  Audience:"taskdeck-relay",Subject:"21",DeviceID:deviceID,Role:"operator",
  IssuedAt:now.Unix(),Expires:now.Unix()+30,JTI:strings.Repeat("c",32)}
 for name,mutate:=range map[string]func(*remoterelay.Claims){
  "wrong device":func(c *remoterelay.Claims){c.DeviceID=strings.Repeat("d",64)},
  "expired":func(c *remoterelay.Claims){c.Expires=now.Unix()-1},
  "bad role":func(c *remoterelay.Claims){c.Role="root"},
  "wrong audience":func(c *remoterelay.Claims){c.Audience="taskdeck-local"},
  "wrong issuer":func(c *remoterelay.Claims){c.Issuer="https://evil.example"},
 } {
  t.Run(name,func(t *testing.T){
   v,err:=NewAgentGrantVerifier(key,"https://portal.example.com",deviceID)
   if err!=nil{t.Fatal(err)}
   testClaims:=claims
   mutate(&testClaims)
   if _,err:=v.Consume(signPortalTicket(t,private,testClaims),testBoundChannel(t),now);err==nil {
    t.Fatal("invalid signed Portal ticket passed Agent")
   }
  })
 }
 v,err:=NewAgentGrantVerifier(key,"https://portal.example.com",deviceID)
 if err!=nil{t.Fatal(err)}
 forged:=signPortalTicket(t,private,claims)+"x"
 if _,err:=v.Consume(forged,testBoundChannel(t),now);err==nil {t.Fatal("modified signature accepted")}
 if _,err:=NewAgentGrantVerifier(key,"http://portal.example.com",deviceID);err==nil {
  t.Fatal("insecure Portal issuer accepted")
 }
 if _,err:=NewAgentGrantVerifier(key,"https://portal.example.com","bad-id");err==nil {
  t.Fatal("invalid target device accepted")
 }
}

func TestAgentGrantReplayConcurrentRequests(t *testing.T) {
 public,private,err:=ed25519.GenerateKey(rand.Reader)
 if err!=nil{t.Fatal(err)}
 ch:=testBoundChannel(t)
 deviceID:=ch.BoundDeviceID()
 v,err:=NewAgentGrantVerifier(base64.StdEncoding.EncodeToString(public),
  "https://portal.example.com",deviceID)
 if err!=nil{t.Fatal(err)}
 now:=time.Now()
 token:=signPortalTicket(t,private,remoterelay.Claims{AuthEpoch:1,V:2,BrowserBinding:ch.BrowserTicketBinding(),
  Issuer:"https://portal.example.com",Audience:"taskdeck-relay",
  Subject:"21",DeviceID:deviceID,Role:"admin",
  IssuedAt:now.Unix(),Expires:now.Unix()+30,JTI:strings.Repeat("f",32)})
 var accepted atomic.Int32
 var wg sync.WaitGroup
 for i:=0;i<16;i++{
  wg.Add(1)
  go func(){defer wg.Done();if _,err:=v.Consume(token,ch,now);err==nil{accepted.Add(1)}}()
 }
 wg.Wait()
 if accepted.Load()!=1{t.Fatalf("duplicate Agent ticket accepted %d times",accepted.Load())}
}

func TestAgentOnlyAcceptsOwnerPinnedPortalSigner(t *testing.T) {
	agent,err:=NewAgent("https://portal.example.com",
		filepath.Join(t.TempDir(),"private","identity.seed"),nil)
	if err!=nil{t.Fatal(err)}
	pub,private,err:=ed25519.GenerateKey(rand.Reader)
	if err!=nil{t.Fatal(err)}
	now:=time.Now()
	channel:=testBoundChannelWithPrivate(t,agent.identity.private)
	ticket:=signPortalTicket(t,private,remoterelay.Claims{AuthEpoch:1,
		V:2,BrowserBinding:channel.BrowserTicketBinding(),
		Issuer:"https://portal.example.com",Audience:"taskdeck-relay",
		Subject:"21",DeviceID:agent.DeviceID(),Role:"viewer",
		IssuedAt:now.Unix(),Expires:now.Unix()+30,JTI:strings.Repeat("e",32),
	})
	if _,err:=agent.VerifyPortalGrant(ticket,channel,now);err==nil {
		t.Fatal("Agent trusted Portal ticket without locally pinned signing key")
	}
	if err:=agent.ConfigurePortalGrantKey("not-base64");err==nil {
		t.Fatal("invalid signing key accepted")
	}
	if err:=agent.ConfigurePortalGrantKey(base64.StdEncoding.EncodeToString(pub));err!=nil {
		t.Fatal(err)
	}
	other,_,err:=ed25519.GenerateKey(rand.Reader)
	if err!=nil{t.Fatal(err)}
	if err:=agent.ConfigurePortalGrantKey(base64.StdEncoding.EncodeToString(other));err==nil {
		t.Fatal("untrusted key silently replaced locally pinned Portal signer")
	}
	grant,err:=agent.VerifyPortalGrant(ticket,channel,now)
	if err!=nil{t.Fatal(err)}
	if _,err:=grant.TakeForLogin(channel,now);err!=nil{t.Fatal(err)}
	if _,err:=grant.TakeForLogin(channel,now);err==nil {
		t.Fatal("one Portal grant permitted repeated TaskDeck login guesses")
	}
	if _,err:=agent.VerifyPortalGrant(ticket,testBoundChannel(t),now);err==nil {
		t.Fatal("Agent allowed replay after signed Portal grant already consumed")
	}
}
