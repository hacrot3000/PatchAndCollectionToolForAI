package agentchat

import (
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "io"
 "path"
 "regexp"
 "strings"
 "time"
 "unicode/utf8"
)

const (
 TurnInProgress = "IN_PROGRESS"
 TurnNeedsContinuation = "NEEDS_CONTINUATION"
 TurnWaitingTool = "WAITING_TOOL"
 TurnBlocked = "BLOCKED"
 TurnClaimedComplete = "CLAIMED_COMPLETE"
 TurnUnknown = "UNKNOWN"
 // This is a verified-work brake, not a number of permitted provider turns.
 MaxContinuationNoWorkTurns = 5
)

// TurnContinuation is an untrusted provider-turn advisory and a trusted,
// server-owned scheduling ledger. Neither its metadata nor a model assertion
// grants a native tool permission or a task-plan acceptance.
type TurnContinuation struct {
 Disposition string `json:"disposition"`
 NextAction string `json:"next_action,omitempty"`
 CheckpointPath string `json:"checkpoint_path,omitempty"`
 EvidenceRefs []string `json:"evidence_refs,omitempty"`
 Reason string `json:"reason,omitempty"`
 ResponseDigest string `json:"response_digest,omitempty"`
 ProgressSignature string `json:"progress_signature,omitempty"`
 RepeatedTurns int `json:"repeated_turns,omitempty"`
 NoProgressTurns int `json:"no_progress_turns,omitempty"`
 LastWorkSignature string `json:"last_work_signature,omitempty"`
 LastContinuationKey string `json:"last_continuation_key,omitempty"`
 RetryStreak int `json:"retry_streak,omitempty"`
 NextRetryAt *time.Time `json:"next_retry_at,omitempty"`
}
type declaredTurnProgress struct {
 Status string `json:"status"`
 NextAction string `json:"next_action,omitempty"`
 CheckpointPath string `json:"checkpoint_path,omitempty"`
 EvidenceRefs []string `json:"evidence_refs,omitempty"`
}
var progressFence=regexp.MustCompile(`(?is)\x60{3}taskdeck-progress[ \t]*\r?\n([\s\S]+?)\r?\n\x60{3}`)
var progressWhitespace=regexp.MustCompile(`[[:space:]]+`)
var repeatedPunctuation=regexp.MustCompile(`[[:punct:]]+`)
var progressIncomplete=regexp.MustCompile(`(?i)(chưa hoàn tất|chưa hoàn thành|chưa xong|vẫn chưa|còn phải|cần kiểm tiếp|not (yet )?(finished|complete|done)|unfinished|incomplete|remaining (work|items|cases)|still (pending|working)|work remains)`)
var progressNext=regexp.MustCompile(`(?i)(bước tiếp theo|bước kế tiếp|tiếp theo|lần tiếp|next steps?|next action|continue with|sẽ tiếp tục)`)
var progressTaskLead=regexp.MustCompile(`(?i)^(?:tiếp tục|đang tiếp tục|continue|continuing(?: with)?)\s+[a-z][a-z0-9]*(?:[.-][a-z0-9]+)+\s*[:：—-]`)
var progressBlocked=regexp.MustCompile(`(?i)(bị chặn|không thể tiếp tục do|đang chờ phê duyệt|blocked by|cannot proceed without|awaiting approval)`)
var progressComplete=regexp.MustCompile(`(?i)(đã hoàn (tất|thành) toàn bộ|đã hoàn thành tất cả|all (tasks|steps|work) (are )?(done|complete)|task (is )?complete|fully completed)`)

func validTurnDisposition(s string) bool {
 switch s {case TurnInProgress,TurnNeedsContinuation,TurnWaitingTool,TurnBlocked,TurnClaimedComplete,TurnUnknown:return true}
 return false
}

func ValidTurnContinuation(c *TurnContinuation)bool{
 if c==nil{return true}
 if !validTurnDisposition(c.Disposition)||len(c.NextAction)>500||len(c.CheckpointPath)>240||
  len(c.EvidenceRefs)>8||len(c.Reason)>120||len(c.ResponseDigest)>64||len(c.ProgressSignature)>64||
  c.RepeatedTurns<0||c.RepeatedTurns>100||c.NoProgressTurns<0||c.NoProgressTurns>100||
  len(c.LastWorkSignature)>64||len(c.LastContinuationKey)>128||
  c.RetryStreak<0||c.RetryStreak>100{return false}
 for _,s:=range []string{c.ResponseDigest,c.ProgressSignature,c.LastWorkSignature}{
  if s!=""&&!validSHA256(s){return false}
 }
 for _,ref:=range c.EvidenceRefs{if ref==""||len(ref)>128||strings.ContainsAny(ref,"\r\n\x00"){return false}}
 if c.CheckpointPath!=""&&!safeCheckpointPath(c.CheckpointPath){return false}
 if c.NextRetryAt!=nil&&c.NextRetryAt.IsZero(){return false}
 return true
}
func validSHA256(s string)bool{
 if len(s)!=64{return false}
 _,err:=hex.DecodeString(s);return err==nil&&strings.ToLower(s)==s
}
func safeCheckpointPath(s string)bool{
 if strings.TrimSpace(s)!=s||strings.ContainsAny(s,"\\:\x00\r\n")||strings.HasPrefix(s,"/"){return false}
 clean:=path.Clean(s)
 return clean==s&&clean!="."&&clean!=".."&&!strings.HasPrefix(clean,"../")
}
func turnHash(s string)string{
 sum:=sha256.Sum256([]byte(s));return hex.EncodeToString(sum[:])
}
func normalizedTurnAction(s string)string{
 s=strings.ToLower(strings.TrimSpace(s))
 s=progressWhitespace.ReplaceAllString(s," ")
 s=repeatedPunctuation.ReplaceAllString(s," ")
 if len(s)>260{s=s[:260]}
 return s
}
func parseDeclaredTurnProgress(text string)(declaredTurnProgress,bool){
 matches:=progressFence.FindAllStringSubmatch(text,-1)
 if len(matches)!=1||len(matches[0][1])>2500{return declaredTurnProgress{},false}
 var declared declaredTurnProgress
 dec:=json.NewDecoder(strings.NewReader(matches[0][1]))
 dec.DisallowUnknownFields()
 if dec.Decode(&declared)!=nil{return declaredTurnProgress{},false}
 var extra any
 if dec.Decode(&extra)!=io.EOF{return declaredTurnProgress{},false}
 if !validTurnDisposition(declared.Status)||len(declared.NextAction)>500||
  len(declared.CheckpointPath)>240||len(declared.EvidenceRefs)>8{return declaredTurnProgress{},false}
 declared.NextAction=strings.TrimSpace(declared.NextAction)
 if declared.CheckpointPath!=""&&!safeCheckpointPath(declared.CheckpointPath){return declaredTurnProgress{},false}
 for _,ref:=range declared.EvidenceRefs{if ref==""||len(ref)>128||strings.ContainsAny(ref,"\r\n\x00"){return declaredTurnProgress{},false}}
 return declared,true
}
func boundedNextAction(text string)string{
 lines:=strings.Split(text,"\n")
 for i:=len(lines)-1;i>=0;i--{
  line:=strings.TrimSpace(lines[i])
  if !progressNext.MatchString(line){continue}
  if len(line)>500{line=line[:500]}
  if idx:=strings.Index(line,":");idx>=0&&idx+1<len(line){return strings.TrimSpace(line[idx+1:])}
  if idx:=strings.Index(line,"：");idx>=0{return strings.TrimSpace(line[idx+len("："):])}
  return line
 }
 return ""
}

// A short action announcement beginning with a concrete milestone is not
// a completion report. Providers sometimes return a clipped sentence with
// no progress fence or tool call, yet mark the transport response completed.
// This is advisory only; the existing scheduler still requires owner opt-in
// and an unambiguous terminal provider state with no unresolved native work.
func leadingTaskAction(text string)string{
 line:=strings.TrimSpace(strings.SplitN(strings.TrimSpace(text),"\n",2)[0])
 if !progressTaskLead.MatchString(line){return ""}
 if len(line)>500{
  line=line[:500]
  for !utf8.ValidString(line)&&len(line)>0{line=line[:len(line)-1]}
 }
 return strings.TrimSpace(line)
}

// AnalyzeTurnText parses ONLY provider-produced assistant text. Input file,
// Collection and tool output content MUST NOT be passed to this function.
// The result is advisory for scheduling, never acceptance or execution.
func AnalyzeTurnText(text string)*TurnContinuation{
 c:=&TurnContinuation{Disposition:TurnUnknown,ResponseDigest:turnHash(text)}
 if metadata,ok:=parseDeclaredTurnProgress(text);ok{
  c.Disposition=metadata.Status
  c.NextAction=metadata.NextAction
  c.CheckpointPath=metadata.CheckpointPath
  c.EvidenceRefs=append([]string(nil),metadata.EvidenceRefs...)
  c.Reason="model_progress_metadata_unverified"
 }else{
  // Conservative bilingual fallback: explicit incomplete work takes
  // precedence over a closing claim inside a progress report.
  tail:=text
  if len(tail)>3500{tail=tail[len(tail)-3500:]}
  switch {
  case progressBlocked.MatchString(tail):
   c.Disposition=TurnBlocked
   c.Reason="model_blocker_unverified"
  case progressIncomplete.MatchString(tail):
   c.Disposition=TurnNeedsContinuation
   c.Reason="incomplete_progress_report"
  case progressNext.MatchString(tail):
   c.Disposition=TurnInProgress
   c.Reason="next_action_in_report"
  case progressComplete.MatchString(tail):
   c.Disposition=TurnClaimedComplete
   c.Reason="model_completion_claim_unverified"
  case leadingTaskAction(text)!="":
   c.Disposition=TurnNeedsContinuation
   c.Reason="unfinished_action_announcement"
  default:
   c.Reason="unknown_model_turn"
  }
  c.NextAction=boundedNextAction(tail)
  if c.NextAction==""&&c.Reason=="unfinished_action_announcement"{
   c.NextAction=leadingTaskAction(text)
  }
 }
 c.ProgressSignature=turnHash(c.Disposition+"\n"+normalizedTurnAction(c.NextAction))
 return c
}

// RecordTurnDisposition preserves the independent server counters. The
// model controls only bounded advisory fields; it cannot reset no-work
// counters by rewording a response or naming a fabricated receipt.
func RecordTurnDisposition(x *Session,text string){
 if x==nil||strings.TrimSpace(text)==""{return}
 next:=AnalyzeTurnText(text)
 old:=x.Continuation
 if old!=nil{
  next.NoProgressTurns=old.NoProgressTurns
  next.LastWorkSignature=old.LastWorkSignature
  next.LastContinuationKey=old.LastContinuationKey
  next.RetryStreak=old.RetryStreak
  next.NextRetryAt=old.NextRetryAt
  if old.ProgressSignature==next.ProgressSignature{
   next.RepeatedTurns=old.RepeatedTurns+1
   if next.RepeatedTurns>100{next.RepeatedTurns=100}
  }
 }
 x.Continuation=next
}

func StandaloneAutoContinueCandidate(x Session)bool{
 c:=x.Continuation
 if !x.AutoContinueText||x.TaskPlan!=nil||x.Owner!="local"||x.Archived||
  x.Status!="completed"||c==nil||c.NoProgressTurns>=MaxContinuationNoWorkTurns||
  (c.Disposition!=TurnNeedsContinuation&&c.Disposition!=TurnInProgress)||
  HasUnresolvedWork(x){return false}
 if c.NextRetryAt!=nil&&time.Now().UTC().Before(*c.NextRetryAt){return false}
 if x.ProviderRun!=nil{
  switch x.ProviderRun.Phase{case "prepared","sending","unknown":return false}
 }
 for _,call:=range x.ToolCalls{
  if (call.Status=="done"||call.Status=="error"||call.Status=="rejected")&&
   (!call.FeedbackSent||call.FeedbackStatus!="acknowledged")&&!HistoricalUnconfirmedFeedback(x,call){return false}
 }
 return true
}
