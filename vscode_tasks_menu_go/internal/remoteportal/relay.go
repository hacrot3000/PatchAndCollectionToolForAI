package remoteportal

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
	"bletonfc/vscode_tasks_menu/internal/remotetransport"
)

type relayDeviceProof struct {
	DeviceID string `json:"device_id"`
	PublicKey string `json:"public_key"`
	Audience string `json:"aud"`
	Challenge string `json:"challenge"`
	DeviceLease string `json:"device_lease"`
	Timestamp int64 `json:"timestamp"`
	Nonce string `json:"nonce"`
	Signature string `json:"signature"`
}

func validateRelayOrigin(origin string) error {
	u,err:=url.Parse(origin)
	if err!=nil || u.Scheme!="https" || u.Hostname()=="" || u.User!=nil ||
		u.Path!="" || u.RawQuery!="" || u.Fragment!="" ||
		origin!=u.Scheme+"://"+u.Host {
		return errors.New("Relay URL must be an exact HTTPS origin")
	}
	return nil
}

func (a *Agent) signRelayProof(origin, challenge string) (relayDeviceProof,error) {
	challengeRaw, err := base64.RawURLEncoding.Strict().DecodeString(challenge)
	if err != nil || len(challengeRaw) != 32 { return relayDeviceProof{},errors.New("invalid server challenge") }
	if err:=validateRelayOrigin(origin);err!=nil{return relayDeviceProof{},err}
	nonceBytes:=make([]byte,16)
	if _,err:=io.ReadFull(rand.Reader,nonceBytes);err!=nil{return relayDeviceProof{},err}
	nonce:=hex.EncodeToString(nonceBytes)
	now:=time.Now().Unix()
	parts:=[]string{"taskdeck-relay-device-v2","connect",a.identity.DeviceID(),origin,
		strconv.FormatInt(now,10),nonce,challenge}
	wire:=strings.Join(parts,string([]byte{10}))
	return relayDeviceProof{
		DeviceID:a.identity.DeviceID(),
		PublicKey:base64.StdEncoding.EncodeToString(a.identity.public),
		Audience:origin,Challenge:challenge,DeviceLease:a.currentLease(),Timestamp:now,Nonce:nonce,
		Signature:base64.StdEncoding.EncodeToString(ed25519.Sign(a.identity.private,[]byte(wire))),
	},nil
}

// ConnectRelay attaches an outbound-only authenticated presence socket.
// It never forwards HTTP requests, PTY commands or TaskDeck credentials.
func (a *Agent) ConnectRelay(ctx context.Context, origin string) error {
	return a.connectRelay(ctx,origin,20*time.Second)
}

func (a *Agent) connectRelay(ctx context.Context, origin string, interval time.Duration) error {
	if err:=validateRelayOrigin(origin);err!=nil{return err}
	if interval<=0 {return errors.New("invalid Relay heartbeat interval")}
	if a.currentLease()=="" {return errors.New("Portal device lease required before Relay connection")}
	conn,_,err:=websocket.Dial(ctx,origin+"/v1/device",&websocket.DialOptions{
		HTTPClient:&a.client,
	})
	if err!=nil{return fmt.Errorf("relay dial: %w",err)}
	defer conn.CloseNow()
	// Ciphertext from a TaskDeck login is opaque and can be 6144 bytes
	// before base64; keep a strict bounded websocket cap.
	conn.SetReadLimit(12288)
	initCtx,cancel:=context.WithTimeout(ctx,10*time.Second)
	kind,frame,err:=conn.Read(initCtx)
	if err!=nil{cancel();return fmt.Errorf("relay challenge: %w",err)}
	var welcome struct{
		Protocol string `json:"protocol"`
		Challenge string `json:"challenge"`
	}
	if kind!=websocket.MessageText || json.Unmarshal(frame,&welcome)!=nil ||
		welcome.Protocol!="taskdeck-relay-device-v2" {
		cancel()
		return errors.New("unexpected Relay device handshake")
	}
	hello,err:=a.signRelayProof(origin,welcome.Challenge)
	if err!=nil{cancel();return err}
	payload,err:=json.Marshal(hello)
	if err!=nil{cancel();return err}
	err=conn.Write(initCtx,websocket.MessageText,payload)
	if err==nil {
		var msg []byte
		var typ websocket.MessageType
		typ,msg,err=conn.Read(initCtx)
		if err==nil {
			var ack struct{ Registered bool }
			decodeErr:=json.Unmarshal(msg,&ack)
			if typ!=websocket.MessageText || decodeErr!=nil || !ack.Registered {
				err=errors.New("relay did not accept device identity")
			}
		}
	}
	cancel()
	if err!=nil{return fmt.Errorf("relay authentication: %w",err)}
	var echoChannel *remotetransport.Channel
	var sessionGrant *AgentGrant
	defer func(){
		if echoChannel!=nil{echoChannel.Close()}
		if sessionGrant!=nil{sessionGrant.Close()}
	}()
	var echoProbeID string
	var echoDeadline time.Time
	var echoRounds int
	ticker:=time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():return nil
		case <-ticker.C:
		}
		callCtx,cancel:=context.WithTimeout(ctx,35*time.Second)
		pulse,marshalErr:=json.Marshal(struct{
			Type string `json:"type"`
			DeviceLease string `json:"device_lease"`
		}{Type:"ping",DeviceLease:a.currentLease()})
		if marshalErr!=nil {cancel();return marshalErr}
		err=conn.Write(callCtx,websocket.MessageText,pulse)
		if err==nil {
			var typ websocket.MessageType
			var reply []byte
			typ,reply,err=conn.Read(callCtx)
			if err==nil && typ==websocket.MessageText && string(reply)!="pong" {
				var frame struct {
					Type string `json:"type"`
					ProbeID string `json:"probe_id"`
					ClientNonce string `json:"client_nonce"`
					BrowserKey string `json:"browser_key"`
					PortalTicket string `json:"portal_ticket"`
					Ciphertext string `json:"ciphertext"`
				}
				if json.Unmarshal(reply,&frame)!=nil {
					err=errors.New("malformed Relay diagnostic frame")
				} else {
					switch frame.Type {
					case "reachability_probe":
						id,parseErr:=hex.DecodeString(frame.ProbeID)
						nonceBytes,nonceErr:=hex.DecodeString(frame.ClientNonce)
						if parseErr!=nil || len(id)!=16 || nonceErr!=nil ||
							len(nonceBytes)!=16 || hex.EncodeToString(nonceBytes)!=frame.ClientNonce {
							err=errors.New("invalid Relay probe identifier or browser nonce")
							break
						}
						wire:=strings.Join([]string{
							"taskdeck-browser-attestation-v1",a.identity.DeviceID(),
							frame.ProbeID,frame.ClientNonce,
						},string([]byte{10}))
						if frame.BrowserKey=="" {
							err=errors.New("encrypted browser handshake required")
							break
						}
						browserKey,keyErr:=base64.StdEncoding.Strict().DecodeString(frame.BrowserKey)
						if keyErr!=nil {err=keyErr;break}
						hello,channel,helloErr:=remotetransport.ClientHello(
							a.identity.private,a.identity.DeviceID(),frame.ProbeID,
							frame.ClientNonce,browserKey)
						if helloErr!=nil {err=helloErr;break}
						// Even though Relay verified this Portal ticket, Agent
						// must independently verify its signature, Device ID,
						// issuer and signed ECDH-browser binding. Missing local
						// trust pin is an explicit fail-closed condition.
						grant,grantErr:=a.VerifyPortalGrant(frame.PortalTicket,channel,time.Now())
						if grantErr!=nil {channel.Close();err=grantErr;break}
						encryptedReady,sealErr:=channel.Seal([]byte("taskdeck-e2ee-ready-v1"))
						if sealErr!=nil {err=sealErr;break}
						ack,_:=json.Marshal(struct {
							Type string `json:"type"`
							ProbeID string `json:"probe_id"`
							ClientNonce string `json:"client_nonce"`
							PublicKey string `json:"public_key"`
							Signature string `json:"signature"`
							Crypto map[string]string `json:"crypto"`
							DeviceLease string `json:"device_lease"`
						}{
							Type:"probe_ack",ProbeID:frame.ProbeID,ClientNonce:frame.ClientNonce,
							PublicKey:base64.StdEncoding.EncodeToString(a.identity.public),
							Signature:base64.StdEncoding.EncodeToString(ed25519.Sign(a.identity.private,[]byte(wire))),
							Crypto:map[string]string{
								"identity":base64.StdEncoding.EncodeToString(hello.Identity),
								"ephemeral":base64.StdEncoding.EncodeToString(hello.Ephemeral),
								"signature":base64.StdEncoding.EncodeToString(hello.Signature),
								"encrypted_ready":base64.StdEncoding.EncodeToString(encryptedReady),
							},DeviceLease:a.currentLease(),
						})
						if sessionGrant!=nil {sessionGrant.Close()}
						if echoChannel!=nil {echoChannel.Close()}
						sessionGrant=grant
						echoChannel=channel
						echoProbeID=frame.ProbeID
						echoRounds=0
						echoDeadline=time.Now().Add(90*time.Second)
						err=conn.Write(callCtx,websocket.MessageText,ack)
						if err==nil {typ,reply,err=conn.Read(callCtx)}
					case "e2ee_login":
						if echoChannel==nil || sessionGrant==nil ||
							time.Now().After(echoDeadline) || frame.ProbeID!=echoProbeID ||
							echoRounds!=0 || len(frame.Ciphertext)>9000 {
							err=errors.New("no fresh authorized encrypted login session")
							break
						}
						encrypted,decodeErr:=base64.StdEncoding.Strict().DecodeString(frame.Ciphertext)
						if decodeErr!=nil || len(encrypted)>9+6144+16 {
							err=errors.New("invalid encrypted login length")
							break
						}
						// This is the ONLY Agent application callback exposed by
						// the Relay transport. The TaskDeck verifier accepts one
						// signed Portal grant and returns a sealed boolean only.
						sealed,loginErr:=a.verifyEncryptedLogin(callCtx,echoChannel,sessionGrant,encrypted)
						if loginErr!=nil {err=loginErr;break}
						ack,_:=json.Marshal(struct {
							Type string `json:"type"`
							ProbeID string `json:"probe_id"`
							Ciphertext string `json:"ciphertext"`
							DeviceLease string `json:"device_lease"`
						}{Type:"login_ack",ProbeID:frame.ProbeID,
							Ciphertext:base64.StdEncoding.EncodeToString(sealed),
							DeviceLease:a.currentLease()})
						echoChannel.Close()
						sessionGrant.Close()
						echoChannel=nil
						sessionGrant=nil
						echoProbeID=""
						err=conn.Write(callCtx,websocket.MessageText,ack)
						if err==nil {typ,reply,err=conn.Read(callCtx)}
					case "e2ee_echo":
						if echoChannel==nil || time.Now().After(echoDeadline) ||
							frame.ProbeID!=echoProbeID || echoRounds>=3 ||
							len(frame.Ciphertext)>1500 {
							err=errors.New("no active authorized encrypted echo session")
							break
						}
						encrypted,decodeErr:=base64.StdEncoding.Strict().DecodeString(frame.Ciphertext)
						if decodeErr!=nil || len(encrypted)>1024 {
							err=errors.New("invalid encrypted echo")
							break
						}
						clear,openErr:=echoChannel.Open(encrypted)
						const prefix="taskdeck-e2ee-echo-v1:"
						if openErr!=nil || len(clear)!=len(prefix)+32 ||
							!strings.HasPrefix(string(clear),prefix) {
							err=errors.New("unrecognized encrypted diagnostic request")
							break
						}
						randomHex:=string(clear[len(prefix):])
						randomBytes,decodeErr:=hex.DecodeString(randomHex)
						if decodeErr!=nil || len(randomBytes)!=16 ||
							hex.EncodeToString(randomBytes)!=randomHex {
							err=errors.New("malformed encrypted diagnostic nonce")
							break
						}
						sealed,sealErr:=echoChannel.Seal([]byte("taskdeck-e2ee-echo-reply-v1:"+randomHex))
						if sealErr!=nil {err=sealErr;break}
						ack,_:=json.Marshal(struct {
							Type string `json:"type"`
							ProbeID string `json:"probe_id"`
							Ciphertext string `json:"ciphertext"`
							DeviceLease string `json:"device_lease"`
						}{Type:"echo_ack",ProbeID:frame.ProbeID,
							Ciphertext:base64.StdEncoding.EncodeToString(sealed),
							DeviceLease:a.currentLease()})
						echoRounds++
						if echoRounds==3 {
							if echoChannel!=nil{echoChannel.Close()}
							if sessionGrant!=nil{sessionGrant.Close();sessionGrant=nil}
							echoChannel=nil
							echoProbeID=""
						}
						err=conn.Write(callCtx,websocket.MessageText,ack)
						if err==nil {typ,reply,err=conn.Read(callCtx)}
					default:
						err=errors.New("unknown Relay diagnostic operation")
					}
				}
			}
			if err==nil && (typ!=websocket.MessageText || string(reply)!="pong") {
				err=errors.New("unexpected Relay keepalive acknowledgement")
			}
		}
		cancel()
		if err!=nil{return fmt.Errorf("relay heartbeat: %w",err)}
	}
}

func RunRelayLink(ctx context.Context, agent *Agent, origin string, observe func(error)) {
	if agent==nil {return}
	backoff:=time.Second
	for ctx.Err()==nil {
		err:=agent.ConnectRelay(ctx,origin)
		if err!=nil && ctx.Err()==nil && observe!=nil{observe(err)}
		select{
		case <-ctx.Done():return
		case <-time.After(backoff):
		}
		if backoff<30*time.Second {backoff*=2;if backoff>30*time.Second{backoff=30*time.Second}}
	}
}
