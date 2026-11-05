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
 "unicode"
)

// RemoteActionProtocol is NOT a general-purpose HTTP/terminal/file RPC.
// Only explicitly enumerated E2EE actions are permitted. Read-only
// session/git/project/task preview and inspect coexist with an independently
// host-opted-in bounded tasks.run action. No path, command, environment,
// arbitrary shell arguments or transferable permissions reach the Agent.
// Each operation MUST pass TaskDeck per-project RBAC and revocation checks.
const RemoteActionProtocol = "taskdeck-e2ee-action-v1"
const RemoteActionResultProtocol = "taskdeck-e2ee-action-result-v1"
const RemoteActionSessionPing = "session.ping"
const RemoteActionSessionFeatures = "session.features"
const RemoteActionGitStatus = "git.status"
const RemoteActionProjectInfo = "project.info"
const RemoteActionTasksPreview = "tasks.preview"
const RemoteActionTasksPreviewNext = "tasks.preview.next"
// Selected, read-only preflight; no process or command can start.
const RemoteActionTasksInspect = "tasks.inspect"
// Host-opted-in bounded task execution; no client-selected shell or arguments.
const RemoteActionTasksRun = "tasks.run"

const maxRemoteActionPlaintext = 512
var ErrInvalidRemoteAction = errors.New("invalid authenticated TaskDeck action envelope")

type RemoteAction struct {
 RequestID string
 Operation string
 TaskID int
 Choice int // 1-based index into exactly one host-owned pickString; 0 means none
 Text string // bounded single process-only promptString value
 Message string // bounded subject for git.commit, never a command
 TerminalInput string // bounded PTY input, never URL, path or environment
 FileIndex int // one-based index of a verified Git Status snapshot
 Snapshot string // expected E2EE status; host requires a fresh exact match
 Cols int // terminal.resize only
 Rows int // terminal.resize only
}
type remoteActionWire struct {
 Type string `json:"type"`
 RequestID string `json:"request_id"`
 Operation string `json:"operation"`
 TaskID int `json:"task_id,omitempty"`
 Choice int `json:"choice,omitempty"`
 Text string `json:"text,omitempty"`
 Message string `json:"message,omitempty"`
 TerminalInput string `json:"terminal_input,omitempty"`
 FileIndex int `json:"file_index,omitempty"`
 Snapshot string `json:"snapshot,omitempty"`
 Cols int `json:"cols,omitempty"`
 Rows int `json:"rows,omitempty"`
}
type remoteActionResultWire struct {
 Type string `json:"type"`
 RequestID string `json:"request_id"`
 OK bool `json:"ok"`
 RemoteControlAvailable bool `json:"remote_control_available"`
}

// validActionID rejects ambiguous UTF-8, mixed case and values which could
// be interpreted differently by another JSON implementation.
// ValidRemoteCommitMessage denies control, bidi, shell, URL and Git option
// syntax. Message goes only into trusted git commit-tree stdin (no shell).
// ValidRemoteTaskDeckBranchSlug confines every remote-created ref to
// refs/heads/taskdeck/<slug>. No slashes, dot-segments, options, control
// characters, Git ref syntax, or shell expansion are accepted.
func ValidRemoteTaskDeckBranchSlug(slug string)bool{
 if len(slug)<2||len(slug)>32||slug[0]<'a'||slug[0]>'z'||
  slug[len(slug)-1]=='-'{return false}
 for _,c:=range slug{
  if (c>='a'&&c<='z')||(c>='0'&&c<='9')||c=='-'{continue}
  return false
 }
 return true
}

func ValidRemoteCommitMessage(msg string) bool {
 if len(msg)<3||len(msg)>96||strings.TrimSpace(msg)!=msg||
  strings.HasPrefix(msg,"-")||strings.Contains(msg,"--"){return false}
 for _,c:=range msg {
  if (c>='a'&&c<='z')||(c>='A'&&c<='Z')||
   (c>='0'&&c<='9')||c==' '||c=='.'||c=='-'||
   c=='_'||c==','||c=='('||c==')' {continue}
  return false
 }
 return true
}

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

// Parse a bounded features object in a single pass even when many independent
// optional flags are present. Exact keys, no duplicates, no unknown fields,
// and ALL required keys are mandatory. Avoid exponential subset enumeration.
func decodeActionObjectWithOptionals(clear []byte,required,optional []string)(map[string]json.RawMessage,error){
 if len(clear)==0||len(clear)>maxRemoteActionPlaintext||!utf8.Valid(clear){
  return nil,ErrInvalidRemoteAction
 }
 expected:=make(map[string]bool,len(required)+len(optional))
 mandatory:=make(map[string]bool,len(required))
 for _,key:=range required{expected[key]=true;mandatory[key]=true}
 for _,key:=range optional{expected[key]=true}
 d:=json.NewDecoder(bytes.NewReader(clear))
 first,err:=d.Token()
 if err!=nil||first!=json.Delim('{'){return nil,ErrInvalidRemoteAction}
 found:=make(map[string]json.RawMessage,len(expected))
 for d.More(){
  token,err:=d.Token()
  key,ok:=token.(string)
  if err!=nil||!ok||!expected[key]{return nil,ErrInvalidRemoteAction}
  if _,dup:=found[key];dup{return nil,ErrInvalidRemoteAction}
  var value json.RawMessage
  if d.Decode(&value)!=nil{return nil,ErrInvalidRemoteAction}
  found[key]=value
 }
 last,err:=d.Token()
 if err!=nil||last!=json.Delim('}') {return nil,ErrInvalidRemoteAction}
 if _,err=d.Token();!errors.Is(err,io.EOF){return nil,ErrInvalidRemoteAction}
 for key:=range mandatory{
  if _,ok:=found[key];!ok{return nil,ErrInvalidRemoteAction}
 }
 return found,nil
}

func validSelectedTaskID(id int)bool{return id>0 && id<=0x7fffffff}

func selectedRemoteTaskOperation(op string)bool{
 return op==RemoteActionTasksInspect||op==RemoteActionTasksRun||
  op==RemoteActionTasksStart||op==RemoteActionTasksPoll||
  op==RemoteActionTasksCancel||op==RemoteActionTasksClear
}
func remoteTaskInputOperation(op string)bool{
 return op==RemoteActionTasksRun||op==RemoteActionTasksStart
}
func validRemoteOperation(op string)bool{
 return terminalOperation(op)||op==RemoteActionSessionPing || op==RemoteActionSessionFeatures ||
  op==RemoteActionGitStatus || op==RemoteActionGitLog || op==RemoteActionGitBranches || op==RemoteActionGitBranchCreate || op==RemoteActionGitDiff || op==RemoteActionGitPatchStart || op==RemoteActionGitPatchNext || op==RemoteActionGitFetch || op==RemoteActionGitStageTracked || op==RemoteActionGitUnstageAll || op==RemoteActionGitStageFile || op==RemoteActionGitUnstageFile || op==RemoteActionGitPull || op==RemoteActionGitCommit || op==RemoteActionGitPush || op==RemoteActionProjectInfo ||
  op==RemoteActionTasksPreview || op==RemoteActionTasksPreviewNext ||
  op==RemoteActionTasksInspect || op==RemoteActionTasksRun ||
  op==RemoteActionTasksStart || op==RemoteActionTasksPoll ||
  op==RemoteActionTasksCancel || op==RemoteActionTasksClear
}

// Only a small, single, non-secret identifier token can become an argv
// value for a host-authored PROCESS task. Never allow shell metacharacters,
// path separators, whitespace, Unicode controls or leading dashes.
// Two fixed pickString input lists are encoded in one bounded index.
const MaxRemoteTaskCombinedChoices=MaxRemoteTaskPickChoices*MaxRemoteTaskPickChoices
const MaxRemoteTaskPromptBytes=64
func ValidRemoteTaskPromptValue(value string)bool{
 if len(value)<1||len(value)>MaxRemoteTaskPromptBytes{return false}
 for i:=0;i<len(value);i++{
  b:=value[i]
  letter:=(b>='A'&&b<='Z')||(b>='a'&&b<='z')
  digit:=b>='0'&&b<='9'
  if i==0&&!letter&&!digit{return false}
  if !letter&&!digit&&b!='_'&&b!='.'&&b!='-'{return false}
 }
 return true
}

func validateActionRequest(clear []byte) (RemoteAction,error){
 var denied RemoteAction
 // The only accepted file selection contains an index and an expected
 // status snapshot. No caller path, repo URL or Git argument is possible.
 if fields,err:=decodeActionObject(clear,"type","request_id","operation","file_index","snapshot");err==nil{
  var typ,id,op,snapshot string
  var index int
  if json.Unmarshal(fields["type"],&typ)!=nil||
   json.Unmarshal(fields["request_id"],&id)!=nil||
   json.Unmarshal(fields["operation"],&op)!=nil||
   json.Unmarshal(fields["file_index"],&index)!=nil||
   json.Unmarshal(fields["snapshot"],&snapshot)!=nil||
   typ!=RemoteActionProtocol||!validActionID(id)||
   !RemoteGitFileOperation(op)||!ValidRemoteGitFileSelection(index,snapshot){
   return denied,ErrInvalidRemoteAction
  }
  return RemoteAction{RequestID:id,Operation:op,FileIndex:index,Snapshot:snapshot},nil
 }
 raw,err:=decodeActionObject(clear,"type","request_id","operation")
 selected:=false
 hasChoice:=false
 hasText:=false
 hasMessage:=false
 hasTerminalInput:=false
 hasTerminalSize:=false
 if err!=nil{
  raw,err=decodeActionObject(clear,"type","request_id","operation","message")
  if err==nil{hasMessage=true}else{
   raw,err=decodeActionObject(clear,"type","request_id","operation","terminal_input")
   if err==nil{hasTerminalInput=true}else{
    raw,err=decodeActionObject(clear,"type","request_id","operation","cols","rows")
    if err==nil{hasTerminalSize=true}else{
     raw,err=decodeActionObject(clear,"type","request_id","operation","task_id")
     if err==nil{selected=true}else{
      raw,err=decodeActionObject(clear,"type","request_id","operation","task_id","choice")
      if err==nil{selected=true;hasChoice=true}else{
       raw,err=decodeActionObject(clear,"type","request_id","operation","task_id","text")
       if err!=nil{return denied,ErrInvalidRemoteAction}
       selected=true;hasText=true
      }
     }
    }
   }
  }
 }
 var typ,id,op string
 if json.Unmarshal(raw["type"],&typ)!=nil ||
  json.Unmarshal(raw["request_id"],&id)!=nil ||
  json.Unmarshal(raw["operation"],&op)!=nil ||
  typ!=RemoteActionProtocol || !validActionID(id) || !validRemoteOperation(op) ||
  selectedRemoteTaskOperation(op)!=selected ||
  (op==RemoteActionGitCommit)!=hasMessage ||
  (op==RemoteActionTerminalWrite)!=hasTerminalInput ||
  (op==RemoteActionTerminalResize)!=hasTerminalSize {
  return denied,ErrInvalidRemoteAction
 }
 action:=RemoteAction{RequestID:id,Operation:op}
 if selected{
  if json.Unmarshal(raw["task_id"],&action.TaskID)!=nil ||
   !validSelectedTaskID(action.TaskID){return denied,ErrInvalidRemoteAction}
 }
 if hasChoice {
  if !remoteTaskInputOperation(op)||
   json.Unmarshal(raw["choice"],&action.Choice)!=nil ||
   action.Choice<1||action.Choice>MaxRemoteTaskCombinedChoices{
   return denied,ErrInvalidRemoteAction
  }
 }
 if hasMessage{
  if json.Unmarshal(raw["message"],&action.Message)!=nil||
   !ValidRemoteGitCommitMessage(action.Message){
   return denied,ErrInvalidRemoteAction
  }
 }
 if hasTerminalInput{
  if json.Unmarshal(raw["terminal_input"],&action.TerminalInput)!=nil||
   !ValidRemoteTerminalInput(action.TerminalInput){
   return denied,ErrInvalidRemoteAction
  }
 }
 if hasTerminalSize{
  if json.Unmarshal(raw["cols"],&action.Cols)!=nil||
   json.Unmarshal(raw["rows"],&action.Rows)!=nil||
   !ValidRemoteTerminalSize(action.Cols,action.Rows){
   return denied,ErrInvalidRemoteAction
  }
 }
 if hasText {
  if !remoteTaskInputOperation(op)||
   json.Unmarshal(raw["text"],&action.Text)!=nil ||
   !ValidRemoteTaskPromptValue(action.Text){
   return denied,ErrInvalidRemoteAction
  }
 }
 return action,nil
}

// SealRemoteAction is only for independently trusted, local native clients.
// A PHP-served script cannot be trusted to collect the TaskDeck password.
func SealRemoteAction(ch *Channel,action RemoteAction)([]byte,error){
 if ch==nil || !ch.Established() || !validActionID(action.RequestID) ||
  !validRemoteOperation(action.Operation) ||
  (selectedRemoteTaskOperation(action.Operation) &&
   !validSelectedTaskID(action.TaskID)) ||
  (!selectedRemoteTaskOperation(action.Operation) && action.TaskID!=0) ||
  (!remoteTaskInputOperation(action.Operation) && action.Choice!=0) ||
  action.Choice<0 || action.Choice>MaxRemoteTaskCombinedChoices ||
  (action.Operation==RemoteActionGitCommit)!=(action.Message!="") ||
  (RemoteGitFileOperation(action.Operation)!=((action.FileIndex!=0)||(action.Snapshot!=""))) ||
  (RemoteGitFileOperation(action.Operation)&&!ValidRemoteGitFileSelection(action.FileIndex,action.Snapshot)) ||
  (!RemoteGitFileOperation(action.Operation)&&(action.FileIndex!=0||action.Snapshot!="")) ||
  (action.Operation==RemoteActionTerminalWrite)!=(action.TerminalInput!="") ||
  (action.TerminalInput!=""&&!ValidRemoteTerminalInput(action.TerminalInput)) ||
  (action.Operation==RemoteActionTerminalResize)!=((action.Cols!=0)||(action.Rows!=0)) ||
  ((action.Cols!=0||action.Rows!=0)&&!ValidRemoteTerminalSize(action.Cols,action.Rows)) ||
  (action.Message!=""&&!ValidRemoteGitCommitMessage(action.Message)) ||
  (action.Text!="" && (!remoteTaskInputOperation(action.Operation) ||
   action.Choice!=0 || !ValidRemoteTaskPromptValue(action.Text))) {
  return nil,ErrInvalidRemoteAction
 }
 var buffer bytes.Buffer
 encoder:=json.NewEncoder(&buffer)
 encoder.SetEscapeHTML(false)
 err:=encoder.Encode(remoteActionWire{
  Type:RemoteActionProtocol,RequestID:action.RequestID,
  Operation:action.Operation,TaskID:action.TaskID,Choice:action.Choice,
  Text:action.Text,Message:action.Message,
  TerminalInput:action.TerminalInput,Cols:action.Cols,Rows:action.Rows,
  FileIndex:action.FileIndex,Snapshot:action.Snapshot,
 })
 if err!=nil{return nil,ErrInvalidRemoteAction}
 raw:=bytes.TrimSuffix(buffer.Bytes(),[]byte{10})
 if len(raw)>maxRemoteActionPlaintext{return nil,ErrInvalidRemoteAction}
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
