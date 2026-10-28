// Package remotetransport provides an end-to-end authenticated encrypted
// envelope. The Relay sees ciphertext only. No TaskDeck HTTP/WS or command
// handler is connected to this package until separate authorization is ready.
package remotetransport

import (
	"crypto/aes"
	"crypto/cipher"
		"crypto/ed25519"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sync"
)

const (
	Protocol = "taskdeck-e2ee-p256-v1"
	MaxPlaintext = 32768
	frameVersion byte = 1
)
const frameHeader = 9

// Ephemeral is a fresh P-256 key per browser session. Never reuse it across
// sessions or put the private scalar in a Portal, Relay, log or config file.
type Ephemeral struct {
	private []byte
	Public []byte
}

func NewEphemeral() (*Ephemeral, error) {
	private, x, y, err := elliptic.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil { return nil, err }
	return &Ephemeral{private: private, Public: elliptic.Marshal(elliptic.P256(), x, y)}, nil
}

// Hello carries the Agent's ephemeral key and a signature by the persistent
// per-instance Ed25519 device key. Public identity is not a user credential.
type Hello struct {
	Identity []byte
	Ephemeral []byte
	Signature []byte
}

func validHex(s string, bytes int) bool {
	if len(s) != bytes*2 { return false }
	raw, err := hex.DecodeString(s)
	return err == nil && len(raw) == bytes && hex.EncodeToString(raw) == s
}

func transcript(deviceID, probeID, browserNonce string, browserKey, agentKey []byte) ([]byte,error) {
	if !validHex(deviceID,32) || !validHex(probeID,16) || !validHex(browserNonce,16) {
		return nil, errors.New("invalid device/session/challenge identifier")
	}
	if _,_,err := parseP256(browserKey); err != nil { return nil, err }
	if _,_,err := parseP256(agentKey); err != nil { return nil, err }
	// Fixed-width canonical transcript; swapping either key changes the
	// signature and the session's derived AES-GCM keys.
	buf := make([]byte,0,len(Protocol)+1+64+32+32+65+65)
	buf = append(buf, []byte(Protocol)...)
	buf = append(buf, 0)
	buf = append(buf, []byte(deviceID)...)
	buf = append(buf, []byte(probeID)...)
	buf = append(buf, []byte(browserNonce)...)
	buf = append(buf, browserKey...)
	buf = append(buf, agentKey...)
	return buf,nil
}

func parseP256(raw []byte) (x,y *big.Int,err error) {
	if len(raw) != 65 || raw[0] != 4 { return nil,nil,errors.New("invalid P-256 public key encoding") }
	x,y = elliptic.Unmarshal(elliptic.P256(),raw)
	if x == nil || y == nil { return nil,nil,errors.New("P-256 point not on curve") }
	return x,y,nil
}

func sharedSecret(local *Ephemeral, peer []byte) ([]byte,error) {
	if local == nil || len(local.private)!=32 { return nil,errors.New("missing fresh session scalar") }
	x,y,err:=parseP256(peer)
	if err!=nil{return nil,err}
	sx,_:=elliptic.P256().ScalarMult(x,y,local.private)
	if sx==nil {return nil,errors.New("invalid elliptic curve shared secret")}
	secret:=make([]byte,32)
	padded:=sx.Bytes()
	copy(secret[32-len(padded):],padded)
	return secret,nil
}

func ClientHello(agentIdentity ed25519.PrivateKey, deviceID, probeID, browserNonce string, browserPublic []byte) (Hello,*Channel,error) {
	if len(agentIdentity)!=ed25519.PrivateKeySize {return Hello{},nil,errors.New("invalid Agent identity key")}
	agent,err:=NewEphemeral()
	if err!=nil{return Hello{},nil,err}
	wire,err:=transcript(deviceID,probeID,browserNonce,browserPublic,agent.Public)
	if err!=nil{return Hello{},nil,err}
	identity:=agentIdentity.Public().(ed25519.PublicKey)
	sum:=sha256.Sum256(identity)
	if hex.EncodeToString(sum[:])!=deviceID {return Hello{},nil,errors.New("Agent identity does not match Device ID")}
	secret,err:=sharedSecret(agent,browserPublic)
	if err!=nil{return Hello{},nil,err}
	channel,err:=deriveChannel(secret,wire,false)
	if err!=nil{return Hello{},nil,err}
	return Hello{Identity:append([]byte(nil),identity...),Ephemeral:agent.Public,
		Signature:ed25519.Sign(agentIdentity,wire)},channel,nil
}

// AcceptHello independently validates the Agent identity, both ephemeral
// keys and the signed transcript before granting a browser encryption key.
func AcceptHello(browser *Ephemeral, deviceID, probeID, browserNonce string, hello Hello) (*Channel,error) {
	if len(hello.Identity)!=ed25519.PublicKeySize || len(hello.Signature)!=ed25519.SignatureSize {
		return nil,errors.New("invalid Agent signature or key size")
	}
	sum:=sha256.Sum256(hello.Identity)
	if hex.EncodeToString(sum[:])!=deviceID {return nil,errors.New("unexpected Agent Device ID")}
	if browser==nil {return nil,errors.New("missing browser ephemeral key")}
	wire,err:=transcript(deviceID,probeID,browserNonce,browser.Public,hello.Ephemeral)
	if err!=nil{return nil,err}
	if !ed25519.Verify(hello.Identity,wire,hello.Signature) {
		return nil,errors.New("Agent did not sign this browser session")
	}
	secret,err:=sharedSecret(browser,hello.Ephemeral)
	if err!=nil{return nil,err}
	return deriveChannel(secret,wire,true)
}

type Channel struct {
	mu sync.Mutex
	send cipher.AEAD
	recv cipher.AEAD
	sendPrefix [4]byte
	recvPrefix [4]byte
	sent uint64
	received uint64
	binding [32]byte
	sendDirection byte
	recvDirection byte
}

// hkdf implements HKDF-SHA256 extract/expand (RFC 5869) using Go's
// standard library, to remain compatible with the project's Go 1.19 module.
func expand(secret, salt, info []byte, n int) []byte {
	h:=hmac.New(sha256.New,salt)
	h.Write(secret)
	prk:=h.Sum(nil)
	var prev, out []byte
	for counter:=byte(1);len(out)<n;counter++ {
		mac:=hmac.New(sha256.New,prk)
		mac.Write(prev)
		mac.Write(info)
		mac.Write([]byte{counter})
		prev=mac.Sum(nil)
		out=append(out,prev...)
	}
	return out[:n]
}

func deriveChannel(secret, wire []byte, browser bool) (*Channel,error) {
	binding:=sha256.Sum256(wire)
	material:=expand(secret,binding[:],[]byte("taskdeck-e2ee-p256-v1/aes-gcm"),72)
	one,err:=aes.NewCipher(material[0:32]);if err!=nil{return nil,err}
	two,err:=aes.NewCipher(material[36:68]);if err!=nil{return nil,err}
	a,err:=cipher.NewGCM(one);if err!=nil{return nil,err}
	b,err:=cipher.NewGCM(two);if err!=nil{return nil,err}
	ch:=&Channel{binding:binding}
	if browser {
		ch.send=a;ch.recv=b
		copy(ch.sendPrefix[:],material[32:36])
		copy(ch.recvPrefix[:],material[68:72])
		ch.sendDirection=1;ch.recvDirection=2
	} else {
		ch.send=b;ch.recv=a
		copy(ch.sendPrefix[:],material[68:72])
		copy(ch.recvPrefix[:],material[32:36])
		ch.sendDirection=2;ch.recvDirection=1
	}
	return ch,nil
}

func nonce(prefix [4]byte,seq uint64) []byte {
	iv:=make([]byte,12)
	copy(iv,prefix[:])
	binary.BigEndian.PutUint64(iv[4:],seq)
	return iv
}
func (ch *Channel) additional(direction byte,header []byte) []byte {
	data:=make([]byte,0,len(ch.binding)+1+len(header))
	data=append(data,ch.binding[:]...)
	data=append(data,direction)
	return append(data,header...)
}

// Seal sends a bounded, strictly ordered AEAD record; the Relay cannot read,
// modify, reorder or replay the payload without authentication failure.
func (ch *Channel) Seal(clear []byte) ([]byte,error) {
	if ch==nil {return nil,errors.New("no secure channel")}
	if len(clear)>MaxPlaintext {return nil,errors.New("plaintext frame too large")}
	ch.mu.Lock()
	defer ch.mu.Unlock()
	if ch.sent==math.MaxUint64 {return nil,errors.New("send sequence exhausted")}
	seq:=ch.sent+1
	frame:=make([]byte,frameHeader)
	frame[0]=frameVersion
	binary.BigEndian.PutUint64(frame[1:9],seq)
	data:=ch.send.Seal(nil,nonce(ch.sendPrefix,seq),clear,ch.additional(ch.sendDirection,frame))
	frame=append(frame,data...)
	ch.sent=seq
	return frame,nil
}

func (ch *Channel) Open(frame []byte) ([]byte,error) {
	if ch==nil {return nil,errors.New("no secure channel")}
	if len(frame)<frameHeader+16 || len(frame)>frameHeader+MaxPlaintext+16 {
		return nil,errors.New("invalid encrypted frame length")
	}
	ch.mu.Lock()
	defer ch.mu.Unlock()
	if ch.received==math.MaxUint64 || frame[0]!=frameVersion {return nil,errors.New("invalid frame version or exhausted sequence")}
	seq:=binary.BigEndian.Uint64(frame[1:9])
	if seq!=ch.received+1{return nil,fmt.Errorf("reordered or replayed encrypted record")}
	plaintext,err:=ch.recv.Open(nil,nonce(ch.recvPrefix,seq),frame[frameHeader:],ch.additional(ch.recvDirection,frame[:frameHeader]))
	if err!=nil{return nil,errors.New("ciphertext authentication failed")}
	ch.received=seq
	return plaintext,nil
}
