package remotetransport

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"unicode/utf8"
)

// RemoteLoginWireProtocol is the *only* authentication payload that may be
// interpreted inside an encrypted browser->Agent frame. It is not HTTP, a
// TaskDeck API, a terminal instruction or a transferable login token.
const RemoteLoginWireProtocol = "taskdeck-e2ee-login-v1"
const RemoteLoginResultProtocol = "taskdeck-e2ee-login-result-v1"
const maxLoginEnvelope = 6144

var ErrInvalidLoginEnvelope = errors.New("invalid encrypted TaskDeck login envelope")

// The repository intentionally targets Go 1.19 language semantics.
func wipeLoginBytes(data []byte) {for i:=range data {data[i]=0}}


type LoginCredentials struct {
	Username string
	Password string
}

type loginRequest struct {
	Type string `json:"type"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResult struct {
	Type string `json:"type"`
	Authenticated bool `json:"authenticated"`
	RemoteControlAvailable bool `json:"remote_control_available"`
}

// OpenRemoteLogin authenticates an AES-GCM record *before* inspecting JSON.
// It never handles URLs, HTTP methods, terminal commands, files, or arbitrary
// operations; the decoder rejects unknown fields, duplicates and trailing data.
// The caller must additionally verify the signed Portal grant and then run
// the normal TaskDeck credential verifier. This parser grants no permissions.
func OpenRemoteLogin(ch *Channel, frame []byte) (LoginCredentials,error) {
	var empty LoginCredentials
	if ch==nil || !ch.Established() {return empty,ErrInvalidLoginEnvelope}
	plaintext,err:=ch.Open(frame)
	if err!=nil{return empty,ErrInvalidLoginEnvelope}
	defer wipeLoginBytes(plaintext)
	if len(plaintext)==0 || len(plaintext)>maxLoginEnvelope || !utf8.Valid(plaintext) {
		ch.Close()
		return empty,ErrInvalidLoginEnvelope
	}
	// Reject duplicate keys as well as unknown keys, so a future parser
	// cannot disagree with the authenticated login code about the operation.
	var raw map[string]json.RawMessage
	dec:=json.NewDecoder(bytes.NewReader(plaintext))
	if err:=dec.Decode(&raw);err!=nil || raw==nil || len(raw)!=3 {
		ch.Close();return empty,ErrInvalidLoginEnvelope
	}
	var extra any
	if err:=dec.Decode(&extra);err!=io.EOF {
		ch.Close();return empty,ErrInvalidLoginEnvelope
	}
	// json.Unmarshal accepts duplicate object members. Count them using a
	// token stream; nested objects/arrays are rejected by the typed decoder.
	tokenDecoder:=json.NewDecoder(bytes.NewReader(plaintext))
	first,err:=tokenDecoder.Token()
	if err!=nil || first!=json.Delim('{') {ch.Close();return empty,ErrInvalidLoginEnvelope}
	seen:=make(map[string]bool,3)
	for tokenDecoder.More() {
		keyToken,e:=tokenDecoder.Token()
		key,ok:=keyToken.(string)
		if e!=nil || !ok || seen[key] || (key!="type" && key!="username" && key!="password") {
			ch.Close();return empty,ErrInvalidLoginEnvelope
		}
		seen[key]=true
		var value string
		if e=tokenDecoder.Decode(&value);e!=nil {
			ch.Close();return empty,ErrInvalidLoginEnvelope
		}
	}
	if _,err:=tokenDecoder.Token();err!=nil {ch.Close();return empty,ErrInvalidLoginEnvelope}
	if len(seen)!=3 {ch.Close();return empty,ErrInvalidLoginEnvelope}
	typed:=loginRequest{}
	decoder:=json.NewDecoder(bytes.NewReader(plaintext))
	decoder.DisallowUnknownFields()
	if err:=decoder.Decode(&typed);err!=nil || typed.Type!=RemoteLoginWireProtocol ||
		len(typed.Username)<1 || len(typed.Username)>128 ||
		len(typed.Password)<1 || len(typed.Password)>4096 ||
		!utf8.ValidString(typed.Username) || !utf8.ValidString(typed.Password) ||
		strings.TrimSpace(typed.Username)=="" ||
		strings.TrimSpace(typed.Username)!=typed.Username ||
		strings.IndexByte(typed.Username,0)>=0 {
		ch.Close();return empty,ErrInvalidLoginEnvelope
	}
	return LoginCredentials{Username:typed.Username,Password:typed.Password},nil
}

// SealRemoteLoginResult returns only a boolean. It deliberately never
// contains TaskDeck cookies, user IDs, roles, project paths or access tokens.
func SealRemoteLoginResult(ch *Channel,authenticated bool) ([]byte,error) {
	if ch==nil || !ch.Established(){return nil,ErrInvalidLoginEnvelope}
	plaintext,err:=json.Marshal(loginResult{
		Type:RemoteLoginResultProtocol,Authenticated:authenticated,
		RemoteControlAvailable:false,
	})
	if err!=nil{return nil,err}
	defer wipeLoginBytes(plaintext)
	return ch.Seal(plaintext)
}

// RemoteLoginRequestFrame is a test/client codec; browser implementations
// must use WebCrypto and NEVER send the cleartext JSON through Relay.
func RemoteLoginRequestFrame(ch *Channel,username,password string) ([]byte,error) {
	if ch==nil || !ch.Established(){return nil,ErrInvalidLoginEnvelope}
	payload,err:=json.Marshal(loginRequest{
		Type:RemoteLoginWireProtocol,Username:username,Password:password,
	})
	if err!=nil{return nil,err}
	defer wipeLoginBytes(payload)
	if len(payload)>maxLoginEnvelope {return nil,ErrInvalidLoginEnvelope}
	return ch.Seal(payload)
}

// parseRemoteLoginResult validates the entire, tightly scoped JSON payload
// without needing to modify E2EE counters. Both Browser inbound and Agent
// outbound validation must agree on this exact three-field schema.
func parseRemoteLoginResult(payload []byte) (bool,error) {
	if len(payload)==0 || len(payload)>256 {return false,ErrInvalidLoginEnvelope}
	keys:=map[string]bool{
		"type":false,"authenticated":false,"remote_control_available":false,
	}
	fields:=json.NewDecoder(bytes.NewReader(payload))
	first,err:=fields.Token()
	if err!=nil || first!=json.Delim('{') {return false,ErrInvalidLoginEnvelope}
	for fields.More(){
		keyValue,e:=fields.Token()
		key,ok:=keyValue.(string)
		used,known:=keys[key]
		if e!=nil || !ok || !known || used {return false,ErrInvalidLoginEnvelope}
		keys[key]=true
		var raw json.RawMessage
		if e:=fields.Decode(&raw);e!=nil {return false,ErrInvalidLoginEnvelope}
	}
	if _,err:=fields.Token();err!=nil{return false,ErrInvalidLoginEnvelope}
	for _,seen:=range keys {if !seen{return false,ErrInvalidLoginEnvelope}}
	var trailing any
	if err:=fields.Decode(&trailing);err!=io.EOF {return false,ErrInvalidLoginEnvelope}
	decoder:=json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var result loginResult
	if err:=decoder.Decode(&result);err!=nil || result.Type!=RemoteLoginResultProtocol ||
		result.RemoteControlAvailable {
		return false,ErrInvalidLoginEnvelope
	}
	return result.Authenticated,nil
}

// OpenRemoteLoginResult verifies the authenticated response is a restricted
// boolean result, never an application authorization or TaskDeck bearer token.
func OpenRemoteLoginResult(ch *Channel,frame []byte) (bool,error) {
	if ch==nil || !ch.Established(){return false,ErrInvalidLoginEnvelope}
	payload,err:=ch.Open(frame)
	if err!=nil{return false,ErrInvalidLoginEnvelope}
	defer wipeLoginBytes(payload)
	valid,err:=parseRemoteLoginResult(payload)
	if err!=nil {ch.Close()}
	return valid,err
}

// VerifyOutgoingRemoteLoginResult checks that a *callback-provided* response
// was produced by the current E2EE Agent send key and is precisely the fixed
// encrypted login boolean. It must succeed before Agent gives any bytes to
// Relay, preventing accidental plaintext credential/authorization leakage.
func VerifyOutgoingRemoteLoginResult(ch *Channel,frame []byte) error {
	payload,err:=ch.inspectLastSentFrame(frame)
	if err!=nil{return ErrInvalidLoginEnvelope}
	defer wipeLoginBytes(payload)
	_,err=parseRemoteLoginResult(payload)
	return err
}
