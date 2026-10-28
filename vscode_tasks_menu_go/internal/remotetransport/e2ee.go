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

// BrowserTicketBinding ties the Portal-issued ticket to the EXACT browser
// P-256 public key and nonce used for this encrypted session. The Portal does
// not need the browser's private scalar; the Agent derives the same binding
// from the authenticated ECDH transcript, never from Relay assertions.
func BrowserTicketBinding(browserNonce string,browserPublic []byte) (string,error) {
 if !validHex(browserNonce,16) {return "",errors.New("invalid browser nonce")}
 if _,_,err:=parseP256(browserPublic);err!=nil{return "",err}
 wire:=make([]byte,0,30+len(browserNonce)+len(browserPublic))
 wire=append(wire,[]byte("taskdeck-browser-ticket-v2")...)
 wire=append(wire,0)
 wire=append(wire,[]byte(browserNonce)...)
 wire=append(wire,0)
 wire=append(wire,browserPublic...)
 sum:=sha256.Sum256(wire)
 return hex.EncodeToString(sum[:]),nil
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
	ticketBinding,err:=BrowserTicketBinding(browserNonce,browserPublic)
	if err!=nil{return Hello{},nil,err}
	channel.browserTicketBinding=ticketBinding
	return Hello{Identity:append([]byte(nil),identity...),Ephemeral:agent.Public,
		Signature:ed25519.Sign(agentIdentity,wire)},channel,nil
}

// VerifyPublicKey validates the exact 65-byte uncompressed P-256 encoding.
func VerifyPublicKey(key []byte) error {
	_,_,err:=parseP256(key)
	return err
}

// VerifyHello independently checks the signed Agent ephemeral key against
// a specific browser public key without learning the browser's private key.
// The Relay can call it, but cannot derive the shared session encryption key.
func VerifyHello(deviceID, probeID, browserNonce string, browserPublic []byte, hello Hello) error {
	if len(hello.Identity)!=ed25519.PublicKeySize || len(hello.Signature)!=ed25519.SignatureSize {
		return errors.New("invalid Agent identity/signature size")
	}
	sum:=sha256.Sum256(hello.Identity)
	if hex.EncodeToString(sum[:])!=deviceID {
		return errors.New("Agent identity fingerprint mismatch")
	}
	wire,err:=transcript(deviceID,probeID,browserNonce,browserPublic,hello.Ephemeral)
	if err!=nil{return err}
	if !ed25519.Verify(hello.Identity,wire,hello.Signature) {
		return errors.New("Agent ephemeral key is not signed for this browser session")
	}
	return nil
}

// AcceptHello independently validates the Agent identity, both ephemeral
// keys and the signed transcript before granting a browser encryption key.
func AcceptHello(browser *Ephemeral, deviceID, probeID, browserNonce string, hello Hello) (*Channel,error) {
	if browser==nil {return nil,errors.New("missing browser ephemeral key")}
	if err:=VerifyHello(deviceID,probeID,browserNonce,browser.Public,hello);err!=nil{
		return nil,err
	}
	wire,err:=transcript(deviceID,probeID,browserNonce,browser.Public,hello.Ephemeral)
	if err!=nil{return nil,err}
	secret,err:=sharedSecret(browser,hello.Ephemeral)
	if err!=nil{return nil,err}
	channel,err:=deriveChannel(secret,wire,true)
	if err!=nil{return nil,err}
	ticketBinding,err:=BrowserTicketBinding(browserNonce,browser.Public)
	if err!=nil{return nil,err}
	channel.browserTicketBinding=ticketBinding
	return channel,nil
}

type Channel struct {
	mu sync.Mutex
	poisoned bool
	send cipher.AEAD
	recv cipher.AEAD
	sendPrefix [4]byte
	recvPrefix [4]byte
	sent uint64
	received uint64
	binding [32]byte
	browserTicketBinding string
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

// Established reports whether this channel resulted from a completed
// authenticated ECDH setup, rather than a zero-valued placeholder. Remote
// identity binding is only valid for an established encrypted channel.
// BrowserTicketBinding identifies the browser key/nonce pinned by Portal
// for this channel. A caller cannot inject an unrelated key into this value.
func (ch *Channel) BrowserTicketBinding() string {
 if ch==nil{return ""}
 ch.mu.Lock()
 defer ch.mu.Unlock()
 if ch.poisoned || ch.send==nil || ch.recv==nil {return ""}
 return ch.browserTicketBinding
}

func (ch *Channel) Established() bool {
	if ch==nil {return false}
	ch.mu.Lock()
	defer ch.mu.Unlock()
	return !ch.poisoned && ch.send!=nil && ch.recv!=nil && ch.binding!=[32]byte{}
}

// Close invalidates a channel and its session binding. AES-GCM keys are
// discarded with the cipher instances; callers must not reuse the channel.
func (ch *Channel) Close() {
	if ch==nil {return}
	ch.mu.Lock()
	defer ch.mu.Unlock()
	ch.poisoned=true
	ch.send=nil
	ch.recv=nil
	ch.binding=[32]byte{}
	ch.browserTicketBinding=""
}

// Seal sends a bounded, strictly ordered AEAD record; the Relay cannot read,
// modify, reorder or replay the payload without authentication failure.
func (ch *Channel) Seal(clear []byte) ([]byte,error) {
	if ch==nil {return nil,errors.New("no secure channel")}
	if len(clear)>MaxPlaintext {return nil,errors.New("plaintext frame too large")}
	ch.mu.Lock()
	defer ch.mu.Unlock()
	if ch.poisoned || ch.send==nil {return nil,errors.New("encrypted channel closed")}
	if ch.sent==^uint64(0) {return nil,errors.New("send sequence exhausted")}
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
	ch.mu.Lock()
	defer ch.mu.Unlock()
	if ch.poisoned || ch.recv==nil {
		return nil,errors.New("encrypted channel closed")
	}
	// Any malformed, tampered, replayed or reordered inbound record
	// permanently invalidates the connection. A later legitimate record
	// cannot revive a session after an authentication failure.
	if len(frame)<frameHeader+16 || len(frame)>frameHeader+MaxPlaintext+16 {
		ch.poisoned=true
		return nil,errors.New("invalid encrypted frame length")
	}
	if ch.received==^uint64(0) || frame[0]!=frameVersion {
		ch.poisoned=true
		return nil,errors.New("invalid frame version or exhausted sequence")
	}
	seq:=binary.BigEndian.Uint64(frame[1:9])
	if seq!=ch.received+1 {
		ch.poisoned=true
		return nil,fmt.Errorf("reordered or replayed encrypted record")
	}
	plaintext,err:=ch.recv.Open(nil,nonce(ch.recvPrefix,seq),frame[frameHeader:],ch.additional(ch.recvDirection,frame[:frameHeader]))
	if err!=nil {
		ch.poisoned=true
		return nil,errors.New("ciphertext authentication failed")
	}
	ch.received=seq
	return plaintext,nil
}
