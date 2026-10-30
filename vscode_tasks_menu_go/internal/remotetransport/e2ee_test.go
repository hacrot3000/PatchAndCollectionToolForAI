package remotetransport

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"strings"
)

func participants(t *testing.T) (*Channel,*Channel,string) {
	t.Helper()
	pub,priv,err:=ed25519.GenerateKey(rand.Reader)
	if err!=nil{t.Fatal(err)}
	hash:=sha256.Sum256(pub)
	device:=hex.EncodeToString(hash[:])
	browser,err:=NewEphemeral()
	if err!=nil{t.Fatal(err)}
	probe:=hex.EncodeToString(bytes.Repeat([]byte{0x11},16))
	nonce:=hex.EncodeToString(bytes.Repeat([]byte{0x22},16))
	hello,agent,err:=ClientHello(priv,device,probe,nonce,browser.Public)
	if err!=nil{t.Fatal(err)}
	client,err:=AcceptHello(browser,device,probe,nonce,hello)
	if err!=nil{t.Fatal(err)}
	return client,agent,device
}

func TestBidirectionalAuthenticatedEncryptedFrames(t *testing.T) {
	client,agent,_:=participants(t)
	clear:=[]byte("test-only payload; not a TaskDeck credential")
	pkt,err:=client.Seal(clear)
	if err!=nil{t.Fatal(err)}
	if bytes.Contains(pkt,clear){t.Fatal("Relay can see plaintext")}
	roundtrip,err:=agent.Open(pkt)
	if err!=nil || !bytes.Equal(roundtrip,clear){t.Fatalf("browser->Agent decrypt error: %v",err)}
	reply,err:=agent.Seal([]byte("echo"))
	if err!=nil{t.Fatal(err)}
	result,err:=client.Open(reply)
	if err!=nil || string(result)!="echo"{t.Fatalf("Agent->browser decrypt error: %v",err)}
	pkt2,err:=client.Seal([]byte("next"))
	if err!=nil{t.Fatal(err)}
	pkt3,err:=client.Seal([]byte("last"))
	if err!=nil{t.Fatal(err)}
	if _,err:=agent.Open(pkt2);err!=nil{t.Fatal(err)}
	if _,err:=agent.Open(pkt3);err!=nil{t.Fatal(err)}
	if _,err:=agent.Open(pkt);err==nil{t.Fatal("replayed packet accepted")}
	if agent.Established(){t.Fatal("replay did not poison Agent E2EE channel")}
	if _,err:=agent.Seal([]byte("still connected"));err==nil{t.Fatal("poisoned channel can send")}
	if _,err:=agent.Open(pkt3);err==nil{t.Fatal("poisoned channel can receive")}
	client.Close()
	if client.Established(){t.Fatal("closed browser channel remained authenticated")}
	if _,err:=client.Seal([]byte("after close"));err==nil{t.Fatal("closed channel can send")}
	otherClient,otherAgent,_:=participants(t)
	outOfOrder1,err:=otherClient.Seal([]byte("first"));if err!=nil{t.Fatal(err)}
	outOfOrder2,err:=otherClient.Seal([]byte("second"));if err!=nil{t.Fatal(err)}
	if _,err:=otherAgent.Open(outOfOrder2);err==nil{t.Fatal("out-of-order packet accepted")}
	if _,err:=otherAgent.Open(outOfOrder1);err==nil{t.Fatal("out-of-order failure did not close channel")}
}

func TestEncryptedFrameTamperingAndSizeBounds(t *testing.T) {
	browser,agent,_:=participants(t)
	if _,err:=browser.Seal(make([]byte,MaxPlaintext+1));err==nil{t.Fatal("oversized plaintext accepted")}
	allowed,err:=browser.Seal(make([]byte,MaxPlaintext))
	if err!=nil{t.Fatal(err)}
	if _,err:=agent.Open(allowed);err!=nil{t.Fatal(err)}
	next,err:=browser.Seal([]byte("integrity"))
	if err!=nil{t.Fatal(err)}
	mutated:=append([]byte(nil),next...)
	mutated[len(mutated)-1]^=0x80
	if _,err:=agent.Open(mutated);err==nil{t.Fatal("tampered tag accepted")}
	// A failed AEAD tag must permanently invalidate the E2EE channel.
	if agent.Established(){t.Fatal("tampered tag did not revoke encrypted session")}
	if _,err:=agent.Open(next);err==nil{t.Fatal("channel recovered after tampered ciphertext")}
	_,shortAgent,_:=participants(t)
	if _,err:=shortAgent.Open([]byte{1,2});err==nil{t.Fatal("short frame accepted")}
	if shortAgent.Established(){t.Fatal("malformed frame did not poison channel")}
}

func TestSessionKeyAuthenticationRejectsSubstitution(t *testing.T) {
	pub,priv,err:=ed25519.GenerateKey(rand.Reader)
	if err!=nil{t.Fatal(err)}
	digest:=sha256.Sum256(pub)
	device:=hex.EncodeToString(digest[:])
	probe:=hex.EncodeToString(bytes.Repeat([]byte{0x33},16))
	nonce:=hex.EncodeToString(bytes.Repeat([]byte{0x44},16))
	browser,err:=NewEphemeral()
	if err!=nil{t.Fatal(err)}
	hello,_,err:=ClientHello(priv,device,probe,nonce,browser.Public)
	if err!=nil{t.Fatal(err)}
	if _,err:=AcceptHello(browser,device,probe,nonce,hello);err!=nil{t.Fatal(err)}
	if _,err:=AcceptHello(browser,device,probe,hex.EncodeToString(bytes.Repeat([]byte{0x55},16)),hello);err==nil{
		t.Fatal("cross-session replay accepted")
	}
	second,err:=NewEphemeral()
	if err!=nil{t.Fatal(err)}
	if _,err:=AcceptHello(second,device,probe,nonce,hello);err==nil{t.Fatal("different browser ECDH key accepted")}
	swapped:=hello
	swapped.Ephemeral=append([]byte(nil),hello.Ephemeral...)
	swapped.Ephemeral[10]^=1
	if _,err:=AcceptHello(browser,device,probe,nonce,swapped);err==nil{t.Fatal("modified Agent ephemeral key accepted")}
	swapped=hello
	swapped.Signature=make([]byte,64)
	if _,err:=AcceptHello(browser,device,probe,nonce,swapped);err==nil{t.Fatal("forged identity accepted")}
	if _,_,err:=ClientHello(priv,device,probe,nonce,make([]byte,65));err==nil{t.Fatal("invalid P-256 point accepted")}
	if _,_,err:=ClientHello(priv,device,probe,"not-hex",browser.Public);err==nil{t.Fatal("invalid browser nonce accepted")}
}

func TestRemoteLoginRequiresAuthenticatedPeerCiphertext(t *testing.T) {
	pub,private,err:=ed25519.GenerateKey(rand.Reader)
	if err!=nil{t.Fatal(err)}
	device:=sha256.Sum256(pub)
	browser,err:=NewEphemeral()
	if err!=nil{t.Fatal(err)}
	hello,agent,err:=ClientHello(private,hex.EncodeToString(device[:]),
		strings.Repeat("a",32),strings.Repeat("b",32),browser.Public)
	if err!=nil{t.Fatal(err)}
	client,err:=AcceptHello(browser,hex.EncodeToString(device[:]),
		strings.Repeat("a",32),strings.Repeat("b",32),hello)
	if err!=nil{t.Fatal(err)}
	if agent.AuthenticatedPeerTraffic(){
		t.Fatal("mere ECDH public key was treated as proof of possession")
	}
	frame,err:=client.Seal([]byte("authenticated-browser-challenge"))
	if err!=nil{t.Fatal(err)}
	if _,err:=agent.Open(frame);err!=nil{t.Fatal(err)}
	if !agent.AuthenticatedPeerTraffic(){t.Fatal("valid browser AEAD proof not recognized")}
	agent.Close()
	if agent.AuthenticatedPeerTraffic(){t.Fatal("closed encrypted channel retained proof-of-possession state")}
}

func TestEncryptedChannelSequenceExhaustion(t *testing.T) {
	client,agent,_:=participants(t)
	client.sent=^uint64(0)
	if _,err:=client.Seal([]byte("overflow"));err==nil{t.Fatal("send counter rolled over")}
	agent.received=^uint64(0)
	if _,err:=agent.Open(make([]byte,frameHeader+16));err==nil{t.Fatal("receive counter rolled over")}
}
