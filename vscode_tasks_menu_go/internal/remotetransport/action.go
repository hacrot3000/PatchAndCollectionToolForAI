package remotetransport

import (
 "bytes"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "io"
 "strings"
 "unicode/utf8"
)

// RemoteActionProtocol is NOT a general-purpose HTTP/terminal/file RPC.
// The only admitted operation in this phase is a bounded, read-only,
// token-free authenticated ping. Additional operations require review
// against their concrete TaskDeck-local permissions and cancellation paths.
const RemoteActionProtocol = "taskdeck-e2ee-action-v1"
const RemoteActionResultProtocol = "taskdeck-e2ee-action-result-v1"
const RemoteActionSessionPing = "session.ping"
// Explicit, bounded workspace TEXT read. No HTTP proxy or arbitrary command.
const RemoteActionProjectFileRead = "project.file.read"
const RemoteActionGitStatus = "git.status"
// Small text replacement is a distinct, reviewed file-write capability.
// It MUST NOT accept arbitrary commands or HTTP/cookie relaying.
const RemoteActionProjectFileReplace = "project.file.replace"

const maxRemoteActionPlaintext = 512
var ErrInvalidRemoteAction = errors.New("invalid authenticated TaskDeck action envelope")

type RemoteAction struct {
 RequestID string
 Operation string
 Path string
 Content string
 ExpectedSHA256 string
}
type remoteActionWire struct {
 Type string `json:"type"`
 RequestID string `json:"request_id"`
 Operation string `json:"operation"`
 Path string `json:"path,omitempty"`
 Content *string `json:"content,omitempty"`
 ExpectedSHA256 string `json:"expected_sha256,omitempty"`
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

func validRemoteReadPath(path string) bool {
 return len(path)>0 && len(path)<=180 && utf8.ValidString(path) &&
  path!="." && !strings.HasPrefix(path,"/") &&
  !strings.ContainsAny(path,"\\\x00\r\n") &&
  !strings.Contains(path,"..") && !strings.Contains(path,":")
}

func validRemoteReplacement(content,expectedSHA string)bool{
 if len(content)>96||!utf8.ValidString(content)||strings.ContainsRune(content,'\x00')||
  len(expectedSHA)!=sha256.Size*2{return false}
 _,err:=hex.DecodeString(expectedSHA)
 if err!=nil{return false}
 for _,ch:=range expectedSHA{
  if !((ch>='0'&&ch<='9')||(ch>='a'&&ch<='f')){return false}
 }
 return true
}
func validateActionRequest(clear []byte) (RemoteAction,error){
 var denied RemoteAction
 raw,err:=decodeActionObject(clear,"type","request_id","operation")
 if err!=nil {
  raw,err=decodeActionObject(clear,"type","request_id","operation","path")
  if err!=nil{
   raw,err=decodeActionObject(clear,"type","request_id","operation","path",
    "content","expected_sha256")
   if err!=nil{return denied,ErrInvalidRemoteAction}
  }
 }
 var typ,id,op,path,content,expectedSHA string
 if json.Unmarshal(raw["type"],&typ)!=nil ||
  json.Unmarshal(raw["request_id"],&id)!=nil ||
  json.Unmarshal(raw["operation"],&op)!=nil ||
  typ!=RemoteActionProtocol || !validActionID(id) {
  return denied,ErrInvalidRemoteAction
 }
 switch op {
 case RemoteActionSessionPing,RemoteActionGitStatus:
  if len(raw)!=3{return denied,ErrInvalidRemoteAction}
 case RemoteActionProjectFileRead:
  if len(raw)!=4 || json.Unmarshal(raw["path"],&path)!=nil ||
   !validRemoteReadPath(path) {return denied,ErrInvalidRemoteAction}
 case RemoteActionProjectFileReplace:
  if len(raw)!=6 ||
   json.Unmarshal(raw["path"],&path)!=nil||
   json.Unmarshal(raw["content"],&content)!=nil||
   json.Unmarshal(raw["expected_sha256"],&expectedSHA)!=nil||
   !validRemoteReadPath(path)||len(path)>120 ||
   !validRemoteReplacement(content,expectedSHA){
    return denied,ErrInvalidRemoteAction
   }
 default:
  return denied,ErrInvalidRemoteAction
 }
 return RemoteAction{RequestID:id,Operation:op,Path:path,
 Content:content,ExpectedSHA256:expectedSHA},nil
}

// SealRemoteAction is only for independently trusted, local native clients.
// A PHP-served script cannot be trusted to collect the TaskDeck password.
func SealRemoteAction(ch *Channel,action RemoteAction)([]byte,error){
 if ch==nil || !ch.Established() || !validActionID(action.RequestID) {
  return nil,ErrInvalidRemoteAction
 }
 switch action.Operation {
 case RemoteActionSessionPing,RemoteActionGitStatus:
  if action.Path!="" {return nil,ErrInvalidRemoteAction}
 case RemoteActionProjectFileRead:
  if !validRemoteReadPath(action.Path)||action.Content!=""||
   action.ExpectedSHA256!=""{return nil,ErrInvalidRemoteAction}
 case RemoteActionProjectFileReplace:
  if !validRemoteReadPath(action.Path)||len(action.Path)>120||
   !validRemoteReplacement(action.Content,action.ExpectedSHA256){
   return nil,ErrInvalidRemoteAction
  }
 default: return nil,ErrInvalidRemoteAction
 }
 wire:=remoteActionWire{
  Type:RemoteActionProtocol,RequestID:action.RequestID,
  Operation:action.Operation,Path:action.Path,
 }
 if action.Operation==RemoteActionProjectFileReplace{
  wire.Content=&action.Content
  wire.ExpectedSHA256=action.ExpectedSHA256
 }
 raw,err:=json.Marshal(wire)
 if len(raw)>maxRemoteActionPlaintext{return nil,ErrInvalidRemoteAction}
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


// Project-file reads are deliberately bounded to a small UTF-8 text record.
// The caller must authorize FilesRead on the real TaskDeck principal and open
// a pinned workspace-relative file before sealing anything. No HTTP session,
// path resolver, file handle, or credential ever crosses the Relay.
type remoteFileReadResultWire struct {
 Type string `json:"type"`
 RequestID string `json:"request_id"`
 OK bool `json:"ok"`
 RemoteControlAvailable bool `json:"remote_control_available"`
 Content string `json:"content"`
 SHA256 string `json:"sha256"`
}
func SealRemoteFileReadResult(ch *Channel,id string,content []byte)([]byte,error){
 return sealRemoteTextResult(ch,id,content,"taskdeck-e2ee-file-read-result-v1")
}
func SealRemoteGitStatusResult(ch *Channel,id string,content []byte)([]byte,error){
 return sealRemoteTextResult(ch,id,content,"taskdeck-e2ee-git-status-result-v1")
}
func sealRemoteTextResult(ch *Channel,id string,content []byte,typ string)([]byte,error){
 if typ!="taskdeck-e2ee-file-read-result-v1"&&
  typ!="taskdeck-e2ee-git-status-result-v1"{return nil,ErrInvalidRemoteAction}
 if ch==nil||!ch.Established()||!validActionID(id)||
  len(content)>192||!utf8.Valid(content){return nil,ErrInvalidRemoteAction}
 digest:=sha256.Sum256(content)
 raw,err:=json.Marshal(remoteFileReadResultWire{
  Type:typ,RequestID:id,OK:true,
  RemoteControlAvailable:false,Content:string(content),
  SHA256:hex.EncodeToString(digest[:]),
 })
 if err!=nil||len(raw)>maxRemoteActionPlaintext{return nil,ErrInvalidRemoteAction}
 return ch.Seal(raw)
}
func OpenRemoteFileReadResult(ch *Channel,frame []byte,wantID string)(string,error){
 return openRemoteTextResult(ch,frame,wantID,"taskdeck-e2ee-file-read-result-v1")
}
func OpenRemoteGitStatusResult(ch *Channel,frame []byte,wantID string)(string,error){
 return openRemoteTextResult(ch,frame,wantID,"taskdeck-e2ee-git-status-result-v1")
}
func openRemoteTextResult(ch *Channel,frame []byte,wantID,expectedType string)(string,error){
 if expectedType!="taskdeck-e2ee-file-read-result-v1"&&
  expectedType!="taskdeck-e2ee-git-status-result-v1"{return "",ErrInvalidRemoteAction}
 if ch==nil||!ch.Established()||!validActionID(wantID){return "",ErrInvalidRemoteAction}
 clear,err:=ch.Open(frame)
 if err!=nil{return "",ErrInvalidRemoteAction}
 defer wipeLoginBytes(clear)
 raw,err:=decodeActionObject(clear,"type","request_id","ok",
  "remote_control_available","content","sha256")
 if err!=nil {ch.Close();return "",ErrInvalidRemoteAction}
 var typ,id,content,sha string
 var ok,available bool
 if json.Unmarshal(raw["type"],&typ)!=nil ||
  json.Unmarshal(raw["request_id"],&id)!=nil ||
  json.Unmarshal(raw["content"],&content)!=nil ||
  json.Unmarshal(raw["sha256"],&sha)!=nil ||
  json.Unmarshal(raw["ok"],&ok)!=nil ||
  json.Unmarshal(raw["remote_control_available"],&available)!=nil ||
  typ!=expectedType||id!=wantID||!ok||available||
  len(content)>192||!utf8.ValidString(content) {
  ch.Close();return "",ErrInvalidRemoteAction
 }
 digest:=sha256.Sum256([]byte(content))
 if sha!=hex.EncodeToString(digest[:]) {ch.Close();return "",ErrInvalidRemoteAction}
 return content,nil
}


// The host commits a replacement only after local FilesWrite authorization,
// optimistic SHA verification, a durable TaskDeck audit and final grant check.
// No content, file handle or bearer token is ever serialized to PHP/Relay.
type remoteFileReplaceResultWire struct{
 Type string `json:"type"`
 RequestID string `json:"request_id"`
 OK bool `json:"ok"`
 SHA256 string `json:"sha256"`
 RemoteControlAvailable bool `json:"remote_control_available"`
}
func SealRemoteFileReplaceResult(ch *Channel,id,sha string)([]byte,error){
 if ch==nil||!ch.Established()||!validActionID(id)||
  !validRemoteReplacement("",sha){return nil,ErrInvalidRemoteAction}
 clear,err:=json.Marshal(remoteFileReplaceResultWire{
  Type:"taskdeck-e2ee-file-replace-result-v1",RequestID:id,
  OK:true,SHA256:sha,RemoteControlAvailable:false,
 })
 if err!=nil||len(clear)>maxRemoteActionPlaintext{return nil,ErrInvalidRemoteAction}
 return ch.Seal(clear)
}
func OpenRemoteFileReplaceResult(ch *Channel,frame []byte,id string)(string,error){
 if ch==nil||!ch.Established()||!validActionID(id){return "",ErrInvalidRemoteAction}
 clear,err:=ch.Open(frame)
 if err!=nil{return "",ErrInvalidRemoteAction}
 defer wipeLoginBytes(clear)
 raw,err:=decodeActionObject(clear,"type","request_id","ok","sha256","remote_control_available")
 if err!=nil{ch.Close();return "",ErrInvalidRemoteAction}
 var typ,gotID,sha string
 var ok,available bool
 if json.Unmarshal(raw["type"],&typ)!=nil||
  json.Unmarshal(raw["request_id"],&gotID)!=nil||
  json.Unmarshal(raw["sha256"],&sha)!=nil||
  json.Unmarshal(raw["ok"],&ok)!=nil||
  json.Unmarshal(raw["remote_control_available"],&available)!=nil||
  typ!="taskdeck-e2ee-file-replace-result-v1"||gotID!=id||
  !ok||available||!validRemoteReplacement("",sha){
  ch.Close();return "",ErrInvalidRemoteAction
 }
 return sha,nil
}
