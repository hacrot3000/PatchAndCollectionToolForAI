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
	conn.SetReadLimit(4096)
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
	ticker:=time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():return nil
		case <-ticker.C:
		}
		callCtx,cancel:=context.WithTimeout(ctx,10*time.Second)
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
				var challenge struct{
					Type string `json:"type"`
					ProbeID string `json:"probe_id"`
					ClientNonce string `json:"client_nonce"`
					BrowserKey string `json:"browser_key"`
				}
				var id []byte
				var parseErr error
				if json.Unmarshal(reply,&challenge)!=nil || challenge.Type!="reachability_probe" ||
					len(challenge.ProbeID)!=32 || len(challenge.ClientNonce)!=32 {
					err=errors.New("invalid Relay reachability probe")
				} else {
					id,parseErr=hex.DecodeString(challenge.ProbeID)
					nonceBytes,nonceErr:=hex.DecodeString(challenge.ClientNonce)
					if parseErr!=nil || len(id)!=16 || nonceErr!=nil ||
						len(nonceBytes)!=16 || hex.EncodeToString(nonceBytes)!=challenge.ClientNonce {
						err=errors.New("invalid Relay probe identifier or browser nonce")
					} else {
						wire:=strings.Join([]string{"taskdeck-browser-attestation-v1",
							a.identity.DeviceID(),challenge.ProbeID,challenge.ClientNonce},string([]byte{10}))
						var encryptedProof map[string]string
						if challenge.BrowserKey != "" {
							browserKey,keyErr:=base64.StdEncoding.Strict().DecodeString(challenge.BrowserKey)
							if keyErr!=nil {
								return errors.New("Relay sent an invalid browser ECDH key")
							}
							hello,channel,helloErr:=remotetransport.ClientHello(
								a.identity.private,a.identity.DeviceID(),challenge.ProbeID,
								challenge.ClientNonce,browserKey)
							if helloErr!=nil {
								return helloErr
							}
							frame,sealErr:=channel.Seal([]byte("taskdeck-e2ee-ready-v1"))
							if sealErr!=nil {
								return sealErr
							}
							encryptedProof=map[string]string{
								"identity":base64.StdEncoding.EncodeToString(hello.Identity),
								"ephemeral":base64.StdEncoding.EncodeToString(hello.Ephemeral),
								"signature":base64.StdEncoding.EncodeToString(hello.Signature),
								"encrypted_ready":base64.StdEncoding.EncodeToString(frame),
							}
						}
						ack,_:=json.Marshal(struct{
							Type string `json:"type"`
							ProbeID string `json:"probe_id"`
							ClientNonce string `json:"client_nonce"`
							PublicKey string `json:"public_key"`
							Signature string `json:"signature"`
							Crypto map[string]string `json:"crypto,omitempty"`
							DeviceLease string `json:"device_lease"`
						}{
							Type:"probe_ack",ProbeID:challenge.ProbeID,
							ClientNonce:challenge.ClientNonce,
							PublicKey:base64.StdEncoding.EncodeToString(a.identity.public),
							Signature:base64.StdEncoding.EncodeToString(ed25519.Sign(a.identity.private,[]byte(wire))),
							Crypto:encryptedProof,
							DeviceLease:a.currentLease(),
						})
						err=conn.Write(callCtx,websocket.MessageText,ack)
						if err==nil {
							typ,reply,err=conn.Read(callCtx)
						}
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
