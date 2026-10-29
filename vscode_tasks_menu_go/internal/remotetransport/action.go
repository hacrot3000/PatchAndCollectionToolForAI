package remotetransport

import (
 "bytes"
 "encoding/hex"
 "encoding/json"
 "errors"
 "io"
 "unicode/utf8"
)

// RemoteActionProtocol is NOT a general-purpose HTTP/terminal/file RPC.
// The only admitted operation in this phase is a bounded, read-only,
// token-free authenticated ping. Additional operations require review
// against their concrete TaskDeck-local permissions and cancellation paths.
const RemoteActionProtocol = "taskdeck-e2ee-action-v1"
const RemoteActionResultProtocol = "taskdeck-e2ee-action-result-v1"
const RemoteActionSessionPing = "session.ping"

const maxRemoteActionPlaintext = 512
var ErrInvalidRemoteAction = errors.New("invalid authenticated TaskDeck action envelope")

type RemoteAction struct {
 RequestID string
 Operation string
}
type remoteActionWire struct {
 Type string `json:"type"`
 RequestID string `json:"request_id"`
 Operation string `json:"operation"`
}
type remoteActionResultWire struct {
 Type string `json:"type"`
 RequestID string `json:"request_id"`
 OK bool `json:"ok"`
 RemoteControlAvailable bool `json:"remote_control_available"`
}

// validActionID rejects ambiguous UTF-8, mixed case and values which could
// be interpreted differently by another JSON implementation.
func validActionID(s string) bool {
 if len(s)!=32 {return false}
 _,err:=hex.DecodeString(s)
 if err!=nil{return false}
 for _,ch:=range s {
  if !((ch>='0'&&ch<='9')||(ch>='a'&&ch<='f')) {return false}
 }
 return true
}

// decodeActionObject implements a narrow exact-key JSON token parser.
// encoding/json's case-insensitive field handling and duplicate-key
// behavior must not be used to authorize privileged operations.
func decodeActionObject(clear []byte, keys ...string) (map[string]json.RawMessage,error) {
 if len(clear)==0 || len(clear)>maxRemoteActionPlaintext || !utf8.Valid(clear) {
  return nil,ErrInvalidRemoteAction
 }
 d:=json.NewDecoder(bytes.NewReader(clear))
 first,err:=d.Token()
 if err!=nil || first!=json.Delim('{') {return nil,ErrInvalidRemoteAction}
 expected:=make(map[string]bool,len(keys))
 for _,key:=range keys{expected[key]=true}
 found:=make(map[string]json.RawMessage,len(keys))
 for d.More(){
  k,err:=d.Token()
  key,ok:=k.(string)
  if err!=nil || !ok || !expected[key] {return nil,ErrInvalidRemoteAction}
  if _,duplicate:=found[key];duplicate{return nil,ErrInvalidRemoteAction}
  var value json.RawMessage
  if err:=d.Decode(&value);err!=nil{return nil,ErrInvalidRemoteAction}
  found[key]=value
 }
 last,err:=d.Token()
 if err!=nil || last!=json.Delim('}') || len(found)!=len(expected) {return nil,ErrInvalidRemoteAction}
 _,err=d.Token()
 if !errors.Is(err,io.EOF){return nil,ErrInvalidRemoteAction}
 return found,nil
}

func validateActionRequest(clear []byte) (RemoteAction,error){
 var denied RemoteAction
 raw,err:=decodeActionObject(clear,"type","request_id","operation")
 if err!=nil{return denied,err}
 var typ,id,op string
 if json.Unmarshal(raw["type"],&typ)!=nil ||
  json.Unmarshal(raw["request_id"],&id)!=nil ||
  json.Unmarshal(raw["operation"],&op)!=nil ||
  typ!=RemoteActionProtocol || !validActionID(id) ||
  op!=RemoteActionSessionPing {
  return denied,ErrInvalidRemoteAction
 }
 return RemoteAction{RequestID:id,Operation:op},nil
}

// SealRemoteAction is only for independently trusted, local native clients.
// A PHP-served script cannot be trusted to collect the TaskDeck password.
func SealRemoteAction(ch *Channel,action RemoteAction)([]byte,error){
 if ch==nil || !ch.Established() ||
  !validActionID(action.RequestID) || action.Operation!=RemoteActionSessionPing {
  return nil,ErrInvalidRemoteAction
 }
 raw,err:=json.Marshal(remoteActionWire{
  Type:RemoteActionProtocol,RequestID:action.RequestID,Operation:action.Operation,
 })
 if err!=nil{return nil,err}
 return ch.Seal(raw)
}

// OpenRemoteAction MUST run only after the caller verifies that the exact
// in-process TaskDeck-local password session is bound to this channel.
// Invalid decrypted schemas permanently poison the E2EE channel.
func OpenRemoteAction(ch *Channel,frame []byte)(RemoteAction,error){
 if ch==nil || !ch.Established(){return RemoteAction{},ErrInvalidRemoteAction}
 clear,err:=ch.Open(frame)
 if err!=nil{return RemoteAction{},ErrInvalidRemoteAction}
 defer wipeLoginBytes(clear)
 action,err:=validateActionRequest(clear)
 if err!=nil {
  ch.Close()
  return RemoteAction{},ErrInvalidRemoteAction
 }
 return action,nil
}

// A successful ping proves only that TaskDeck's local per-action permission
// gate was reached. It never grants a command capability or serializes tokens.
func SealRemoteActionPingResult(ch *Channel,id string)([]byte,error){
 if ch==nil || !ch.Established() || !validActionID(id) {
  return nil,ErrInvalidRemoteAction
 }
 clear,err:=json.Marshal(remoteActionResultWire{
  Type:RemoteActionResultProtocol,RequestID:id,OK:true,
  RemoteControlAvailable:false,
 })
 if err!=nil{return nil,err}
 return ch.Seal(clear)
}

func OpenRemoteActionPingResult(ch *Channel,frame []byte,wantID string)error{
 if ch==nil || !ch.Established() || !validActionID(wantID) {
  return ErrInvalidRemoteAction
 }
 clear,err:=ch.Open(frame)
 if err!=nil{return ErrInvalidRemoteAction}
 defer wipeLoginBytes(clear)
 raw,err:=decodeActionObject(clear,"type","request_id","ok","remote_control_available")
 if err==nil {
  var typ,id string
  var ok,available bool
  if json.Unmarshal(raw["type"],&typ)!=nil ||
   json.Unmarshal(raw["request_id"],&id)!=nil ||
   json.Unmarshal(raw["ok"],&ok)!=nil ||
   json.Unmarshal(raw["remote_control_available"],&available)!=nil ||
   typ!=RemoteActionResultProtocol || id!=wantID || !ok || available {
   err=ErrInvalidRemoteAction
  }
 }
 if err!=nil {ch.Close();return ErrInvalidRemoteAction}
 return nil
}
