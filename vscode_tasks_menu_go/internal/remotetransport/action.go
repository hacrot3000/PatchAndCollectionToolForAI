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

// RemoteActionProtocol is NOT a general-purpose HTTP/PTY/RPC proxy.
// Exactly typed, bounded operations are admitted only after TaskDeck-local
// password + owner-approved client-key proof: ping, text preview, Git status,
// small optimistic file replacement, and a host-allowlisted task.run.
// Task execution and file mutation require INDEPENDENT default-off host flags.
// Arbitrary command strings, URLs, bearer/session tokens remain forbidden.
const RemoteActionProtocol = "taskdeck-e2ee-action-v1"
const RemoteActionResultProtocol = "taskdeck-e2ee-action-result-v1"
const RemoteActionSessionPing = "session.ping"
// Explicit, bounded workspace TEXT read. No HTTP proxy or arbitrary command.
const RemoteActionProjectFileRead = "project.file.read"
// Full, read-only visible UTF-8 file <=4096B; not a file transfer/download.
const RemoteActionProjectFilePreview = "project.file.preview"
// Explicitly bounded binary file window; file hash pins the entire
// 2 MiB-or-smaller version even across independent E2EE login sessions.
const RemoteActionProjectFileWindow = "project.file.window"
// Distinct 4KiB authenticated file read, no mutation or bearer capabilities.
const RemoteActionProjectFileChunk = "project.file.chunk"
// Explicitly opt-in 32KiB E2EE file block. No extra authorizations or commands.
const RemoteActionProjectFileBlock = "project.file.block"
// Bounded, read-only visible-directory listing. Never enumerates hidden files.
const RemoteActionProjectDirectoryList = "project.dir.list"
const RemoteActionGitStatus = "git.status"
// Long, fixed read-only status for native interactive console; old 192B status unchanged.
const RemoteActionGitStatusFull = "git.status_full"
// Fixed, read-only repository metadata; no arbitrary revision or Git arguments.
const RemoteActionGitLog = "git.log"
const RemoteActionGitBranches = "git.branches"
// Fixed read-only diff of one existing visible workspace file, no Git arguments.
const RemoteActionGitDiff = "git.diff"
// Fixed read-only staged diff for one pinned visible file.
const RemoteActionGitStagedDiff = "git.diff_staged"
// Small text replacement is a distinct, reviewed file-write capability.
// It MUST NOT accept arbitrary commands or HTTP/cookie relaying.
const RemoteActionProjectFileReplace = "project.file.replace"
// Separate host-opt-in complete 4KiB UTF-8 optimistic edit (never file upload).
const RemoteActionProjectFileReplaceText = "project.file.replace_text"
// Only executes a TaskDeck HOST-allowlisted existing task; never accepts a
// command string, arbitrary args, environment variables or deploy approval.
const RemoteActionTaskRun = "task.run"
// Same host-approved process with complete <=4096B output instead of a 192B tail.
const RemoteActionTaskRunText = "task.run_text"
// Read-only list of host-owned, digest-approved executable TaskDeck tasks.
const RemoteActionTaskList = "task.list"

const maxRemoteActionPlaintext = 512
var ErrInvalidRemoteAction = errors.New("invalid authenticated TaskDeck action envelope")

type RemoteAction struct {
 RequestID string
 Operation string
 Path string
 Content string
 ExpectedSHA256 string
 TaskID int
 Offset int64
}
type remoteActionWire struct {
 Type string `json:"type"`
 RequestID string `json:"request_id"`
 Operation string `json:"operation"`
 Path string `json:"path,omitempty"`
 Content *string `json:"content,omitempty"`
 ExpectedSHA256 string `json:"expected_sha256,omitempty"`
 TaskID *int `json:"task_id,omitempty"`
 Offset *int64 `json:"offset,omitempty"`
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
 return decodeActionObjectBounded(clear,maxRemoteActionPlaintext,keys...)
}
func decodeActionObjectBounded(clear []byte,maxBytes int,keys ...string) (map[string]json.RawMessage,error) {
 if maxBytes<1 || maxBytes>RemoteBlockMaxPlaintext ||
  len(clear)==0 || len(clear)>maxBytes || !utf8.Valid(clear) {
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

func validRemoteDirectoryPath(path string) bool {
 return path=="." || validRemoteReadPath(path)
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
   raw,err=decodeActionObject(clear,"type","request_id","operation","task_id")
   if err!=nil{
    raw,err=decodeActionObject(clear,"type","request_id","operation","path",
     "content","expected_sha256")
    if err!=nil{
     raw,err=decodeActionObject(clear,"type","request_id","operation","path","offset")
     if err!=nil{
      // Larger actions must match ONLY this exact six-key typed schema.
      // Other application requests retain their 512B plaintext limit.
      raw,err=decodeActionObjectBounded(clear,RemoteTextEditMaxPlaintext,
       "type","request_id","operation","path","content","expected_sha256")
      if err!=nil{return denied,ErrInvalidRemoteAction}
     }
    }
   }
  }
 }
 var typ,id,op,path,content,expectedSHA string
 var taskID int
 var offset int64
 if json.Unmarshal(raw["type"],&typ)!=nil ||
  json.Unmarshal(raw["request_id"],&id)!=nil ||
  json.Unmarshal(raw["operation"],&op)!=nil ||
  typ!=RemoteActionProtocol || !validActionID(id) {
  return denied,ErrInvalidRemoteAction
 }
 switch op {
 case RemoteActionSessionPing,RemoteActionGitStatus,RemoteActionGitStatusFull,RemoteActionGitLog,RemoteActionGitBranches,RemoteActionTaskList:
  if len(raw)!=3{return denied,ErrInvalidRemoteAction}
 case RemoteActionTaskRun,RemoteActionTaskRunText:
  if len(raw)!=4||json.Unmarshal(raw["task_id"],&taskID)!=nil||
   taskID<1||taskID>0x7fffffff{return denied,ErrInvalidRemoteAction}
 case RemoteActionProjectFileWindow,RemoteActionProjectFileChunk,RemoteActionProjectFileBlock:
  if len(raw)!=5||json.Unmarshal(raw["path"],&path)!=nil||
   !validRemoteReadPath(path)||json.Unmarshal(raw["offset"],&offset)!=nil||
   offset<0||offset>2<<20 {return denied,ErrInvalidRemoteAction}
 case RemoteActionProjectFileRead,RemoteActionProjectFilePreview,RemoteActionProjectDirectoryList,RemoteActionGitDiff,RemoteActionGitStagedDiff:
  if len(raw)!=4 || json.Unmarshal(raw["path"],&path)!=nil {return denied,ErrInvalidRemoteAction}
  if op==RemoteActionProjectDirectoryList {
   if !validRemoteDirectoryPath(path){return denied,ErrInvalidRemoteAction}
  } else if !validRemoteReadPath(path){return denied,ErrInvalidRemoteAction}
 case RemoteActionProjectFileReplace,RemoteActionProjectFileReplaceText:
  if len(raw)!=6 ||
   json.Unmarshal(raw["path"],&path)!=nil||
   json.Unmarshal(raw["content"],&content)!=nil||
   json.Unmarshal(raw["expected_sha256"],&expectedSHA)!=nil||
   !validRemoteReadPath(path)||len(path)>120 ||
   (op==RemoteActionProjectFileReplace&&!validRemoteReplacement(content,expectedSHA))||
   (op==RemoteActionProjectFileReplaceText&&!validRemoteTextEdit(content,expectedSHA)){
    return denied,ErrInvalidRemoteAction
   }
 default:
  return denied,ErrInvalidRemoteAction
 }
 return RemoteAction{RequestID:id,Operation:op,Path:path,
 Content:content,ExpectedSHA256:expectedSHA,TaskID:taskID,Offset:offset},nil
}

// SealRemoteAction is only for independently trusted, local native clients.
// A PHP-served script cannot be trusted to collect the TaskDeck password.
func SealRemoteAction(ch *Channel,action RemoteAction)([]byte,error){
 if ch==nil || !ch.Established() || !validActionID(action.RequestID) {
  return nil,ErrInvalidRemoteAction
 }
 switch action.Operation {
 case RemoteActionSessionPing,RemoteActionGitStatus,RemoteActionGitStatusFull,RemoteActionGitLog,RemoteActionGitBranches,RemoteActionTaskList:
  if action.Path!="" {return nil,ErrInvalidRemoteAction}
 case RemoteActionTaskRun,RemoteActionTaskRunText:
  if action.TaskID<=0||action.TaskID>0x7fffffff||
   action.Path!=""||action.Content!=""||action.ExpectedSHA256!=""{
   return nil,ErrInvalidRemoteAction
  }
 case RemoteActionProjectFileWindow,RemoteActionProjectFileChunk,RemoteActionProjectFileBlock:
  if !validRemoteReadPath(action.Path)||action.Offset<0||action.Offset>2<<20||
   action.Content!=""||action.ExpectedSHA256!=""||action.TaskID!=0{
   return nil,ErrInvalidRemoteAction
  }
 case RemoteActionProjectFileRead,RemoteActionProjectFilePreview,RemoteActionProjectDirectoryList,RemoteActionGitDiff,RemoteActionGitStagedDiff:
  valid:=validRemoteReadPath(action.Path)
  if action.Operation==RemoteActionProjectDirectoryList{valid=validRemoteDirectoryPath(action.Path)}
  if !valid||action.Content!=""||action.ExpectedSHA256!=""||
   action.TaskID!=0{return nil,ErrInvalidRemoteAction}
 case RemoteActionProjectFileReplace,RemoteActionProjectFileReplaceText:
  if !validRemoteReadPath(action.Path)||len(action.Path)>120||
   (action.Operation==RemoteActionProjectFileReplace&&!validRemoteReplacement(action.Content,action.ExpectedSHA256))||
   (action.Operation==RemoteActionProjectFileReplaceText&&!validRemoteTextEdit(action.Content,action.ExpectedSHA256)){
   return nil,ErrInvalidRemoteAction
  }
 default: return nil,ErrInvalidRemoteAction
 }
 wire:=remoteActionWire{
  Type:RemoteActionProtocol,RequestID:action.RequestID,
  Operation:action.Operation,Path:action.Path,
 }
 if action.Operation==RemoteActionProjectFileReplace||action.Operation==RemoteActionProjectFileReplaceText{
  wire.Content=&action.Content
  wire.ExpectedSHA256=action.ExpectedSHA256
 } else if action.Operation==RemoteActionTaskRun||action.Operation==RemoteActionTaskRunText {
  wire.TaskID=&action.TaskID
 } else if (action.Operation==RemoteActionProjectFileWindow||action.Operation==RemoteActionProjectFileChunk||action.Operation==RemoteActionProjectFileBlock) {
  wire.Offset=&action.Offset
 }
 raw,err:=json.Marshal(wire)
 maxBytes:=maxRemoteActionPlaintext
 if action.Operation==RemoteActionProjectFileReplaceText{maxBytes=RemoteTextEditMaxPlaintext}
 if err!=nil||len(raw)>maxBytes{return nil,ErrInvalidRemoteAction}
 return ch.Seal(raw)
}

// OpenRemoteAction MUST run only after the caller verifies that the exact
// in-process TaskDeck-local password session is bound to this channel.
// Invalid decrypted schemas permanently poison the E2EE channel.
func OpenRemoteAction(ch *Channel,frame []byte)(RemoteAction,error){
 if ch==nil || !ch.Established(){return RemoteAction{},ErrInvalidRemoteAction}
 if len(frame)>MaxEncryptedTextEditFrame{ch.Close();return RemoteAction{},ErrInvalidRemoteAction}
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
func SealRemoteDirectoryListResult(ch *Channel,id string,content []byte)([]byte,error){
 return sealRemoteTextResult(ch,id,content,"taskdeck-e2ee-directory-list-result-v1")
}
func sealRemoteTextResult(ch *Channel,id string,content []byte,typ string)([]byte,error){
 if typ!="taskdeck-e2ee-file-read-result-v1"&&
  typ!="taskdeck-e2ee-git-status-result-v1"&&
  typ!="taskdeck-e2ee-directory-list-result-v1"{return nil,ErrInvalidRemoteAction}
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
func OpenRemoteDirectoryListResult(ch *Channel,frame []byte,wantID string)(string,error){
 return openRemoteTextResult(ch,frame,wantID,"taskdeck-e2ee-directory-list-result-v1")
}
func openRemoteTextResult(ch *Channel,frame []byte,wantID,expectedType string)(string,error){
 if expectedType!="taskdeck-e2ee-file-read-result-v1"&&
  expectedType!="taskdeck-e2ee-git-status-result-v1"&&
  expectedType!="taskdeck-e2ee-directory-list-result-v1"{return "",ErrInvalidRemoteAction}
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


// One bounded, synchronous host-allowlisted task invocation. A result must
// NOT grant a shell/terminal/session handle, TaskDeck cookie or arbitrary
// follow-up capability; only a short UTF-8 preview of TaskDeck-local output.
type remoteTaskRunResultWire struct{
 Type string `json:"type"`
 RequestID string `json:"request_id"`
 OK bool `json:"ok"`
 Output string `json:"output"`
 RemoteControlAvailable bool `json:"remote_control_available"`
}
func SealRemoteTaskRunResult(ch *Channel,id,output string)([]byte,error){
 if ch==nil||!ch.Established()||!validActionID(id)||
  len(output)>192||!utf8.ValidString(output){return nil,ErrInvalidRemoteAction}
 raw,err:=json.Marshal(remoteTaskRunResultWire{
  Type:"taskdeck-e2ee-task-run-result-v1",RequestID:id,OK:true,
  Output:output,RemoteControlAvailable:false,
 })
 if err!=nil||len(raw)>maxRemoteActionPlaintext{return nil,ErrInvalidRemoteAction}
 return ch.Seal(raw)
}
func OpenRemoteTaskRunResult(ch *Channel,frame []byte,id string)(string,error){
 if ch==nil||!ch.Established()||!validActionID(id){return "",ErrInvalidRemoteAction}
 rawFrame,err:=ch.Open(frame)
 if err!=nil{return "",ErrInvalidRemoteAction}
 defer wipeLoginBytes(rawFrame)
 raw,err:=decodeActionObject(rawFrame,"type","request_id","ok","output","remote_control_available")
 if err!=nil{ch.Close();return "",ErrInvalidRemoteAction}
 var typ,gotID,output string
 var ok,available bool
 if json.Unmarshal(raw["type"],&typ)!=nil||
  json.Unmarshal(raw["request_id"],&gotID)!=nil||
  json.Unmarshal(raw["output"],&output)!=nil||
  json.Unmarshal(raw["ok"],&ok)!=nil||
  json.Unmarshal(raw["remote_control_available"],&available)!=nil||
  typ!="taskdeck-e2ee-task-run-result-v1"||gotID!=id||
  !ok||available||len(output)>192||!utf8.ValidString(output) {
   ch.Close();return "",ErrInvalidRemoteAction
 }
 return output,nil
}
