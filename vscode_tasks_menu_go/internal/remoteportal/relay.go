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
)

type relayDeviceProof struct {
	DeviceID string `json:"device_id"`
	PublicKey string `json:"public_key"`
	Audience string `json:"aud"`
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

func (a *Agent) signRelayProof(origin string) (relayDeviceProof,error) {
	if err:=validateRelayOrigin(origin);err!=nil{return relayDeviceProof{},err}
	nonceBytes:=make([]byte,16)
	if _,err:=io.ReadFull(rand.Reader,nonceBytes);err!=nil{return relayDeviceProof{},err}
	nonce:=hex.EncodeToString(nonceBytes)
	now:=time.Now().Unix()
	parts:=[]string{"taskdeck-relay-device-v1","connect",a.identity.DeviceID(),origin,
		strconv.FormatInt(now,10),nonce}
	wire:=strings.Join(parts,string([]byte{10}))
	return relayDeviceProof{
		DeviceID:a.identity.DeviceID(),
		PublicKey:base64.StdEncoding.EncodeToString(a.identity.public),
		Audience:origin,Timestamp:now,Nonce:nonce,
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
	hello,err:=a.signRelayProof(origin)
	if err!=nil{return err}
	conn,_,err:=websocket.Dial(ctx,origin+"/v1/device",&websocket.DialOptions{
		HTTPClient:&a.client,
	})
	if err!=nil{return fmt.Errorf("relay dial: %w",err)}
	defer conn.CloseNow()
	conn.SetReadLimit(4096)
	payload,err:=json.Marshal(hello)
	if err!=nil{return err}
	initCtx,cancel:=context.WithTimeout(ctx,10*time.Second)
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
		err=conn.Write(callCtx,websocket.MessageText,[]byte("ping"))
		if err==nil {
			var typ websocket.MessageType
			var reply []byte
			typ,reply,err=conn.Read(callCtx)
			if err==nil && (typ!=websocket.MessageText || string(reply)!="pong") {
				err=errors.New("invalid relay heartbeat response")
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
