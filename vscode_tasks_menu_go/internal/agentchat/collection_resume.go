package agentchat

import (
 "errors"
 "os"
 "reflect"
 "strings"
)

const maxCollectionResumeCheckpoints=16

// Retain the previous UNKNOWN marker and bounded correlation audit in the
// same canonical CAS that admits a NEW request. This is not a replay ledger.
// Full old payloads, credentials and provider-side ACK claims are absent.
type CollectionResumeCheckpoint struct {
 Previous ProviderRun `json:"previous"`
 NewRunID string `json:"new_run_id"`
 ReceiptSHA256 string `json:"receipt_sha256,omitempty"`
 ReceiptUnavailable bool `json:"receipt_unavailable,omitempty"`
 AttemptAudit *ProviderAttemptAudit `json:"attempt_audit,omitempty"`
}

func validCollectionResumeHistory(items []CollectionResumeCheckpoint)bool{
 if len(items)>maxCollectionResumeCheckpoints{return false}
 seen:=map[string]bool{}
 for _,it:=range items{
  if !validProviderRun(&it.Previous)||it.Previous.Kind!="collection"||it.Previous.Phase!="unknown"||
   it.NewRunID==""||len(it.NewRunID)>96||it.NewRunID==it.Previous.ID||
   (it.ReceiptUnavailable && it.ReceiptSHA256!="") ||
   (!it.ReceiptUnavailable && !validGuardDigest(it.ReceiptSHA256)) || seen[it.Previous.ID]{return false}
  seen[it.Previous.ID]=true
  if a:=it.AttemptAudit;a!=nil{
   if a.RunID!=it.Previous.ID||a.Total==0||len(a.Attempts)==0||len(a.Attempts)>maxProviderAttemptRecords||
    uint64(len(a.Attempts))>a.Total||a.Truncated!=(uint64(len(a.Attempts))<a.Total){return false}
  }
 }
 return true
}

// HasUnknownCollectionDelivery identifies a terminal read-only delivery marker.
func HasUnknownCollectionDelivery(x Session) bool {
 return (x.Status=="failed"||x.Status=="cancelled") && x.ProviderRun!=nil &&
  x.ProviderRun.Kind=="collection" && x.ProviderRun.Phase=="unknown"
}

// A new user message does not require AutoCollection or acknowledgement of
// an old terminal Patch. Actual in-flight native work must remain fenced.
func CanSendAfterUnknownCollection(x Session) bool {
 if !HasUnknownCollectionDelivery(x)||x.Archived||x.PendingUserTurn!=nil{return false}
 for _,call:=range x.ToolCalls{
  switch call.Status{case "done","error","rejected":default:return false}
  if call.FeedbackStatus=="prepared"||call.FeedbackStatus=="sending"{return false}
 }
 return true
}

// Never fabricate a receipt digest when the observation is absent.
func unknownCollectionReceiptDigest(x Session) string {
 run:=x.ProviderRun
 if run==nil||len(x.History)==0{return ""}
 last:=x.History[len(x.History)-1]
 if strings.TrimSpace(last.Content)==""{return ""}
 if run.CallID=="" {
  if CollectionTextObservation(x.History,len(x.History)-1){return ProviderPayloadDigest([]byte(last.Content))}
  return ""
 }
 if last.Type!="function_call_output"||last.CallID!=run.CallID{return ""}
 for _,item:=range x.History{
  if item.Type=="function_call"&&item.CallID==run.CallID&&item.Name=="taskdeck_collect_workspace"{
   return ProviderPayloadDigest([]byte(last.Content))
  }
 }
 return ""
}

// CollectionResumeReceipt is an explicit owner-action eligibility check,
// not a permission or ACK. Only a failed local Legacy COLLECT continuation
// whose current receipt is already canonical may be resumed. Historical
// unconfirmed or orphan mutation remains a blocker, even with LastRequestCode=200.
func CollectionResumeReceipt(x Session)(string,bool){
 run:=x.ProviderRun
 if x.Owner!="local"||x.Archived||x.Status!="failed"||x.PendingUserTurn!=nil||
  run==nil||run.Kind!="collection"||run.Phase!="unknown"||
  (x.AutoCollection!=nil&&!*x.AutoCollection)||len(x.History)==0{return "",false}
 for _,c:=range x.ToolCalls{
  if c.Status!="done"&&c.Status!="error"&&c.Status!="rejected"{return "",false}
  if !c.FeedbackSent||c.FeedbackStatus!="acknowledged"{return "",false}
 }
 pending:=map[string]string{}
 for _,it:=range x.History{
  switch it.Type{
  case "function_call":pending[it.CallID]=it.Name
  case "function_call_output":delete(pending,it.CallID)
  }
 }
 if len(pending)!=0{return "",false}
 last:=x.History[len(x.History)-1]
 if run.CallID!=""{
  if last.Type!="function_call_output"||last.CallID!=run.CallID{return "",false}
  found:=false
  for _,it:=range x.History{
   if it.Type=="function_call"&&it.CallID==run.CallID&&it.Name=="taskdeck_collect_workspace"{found=true;break}
  }
  if !found{return "",false}
 }else if !CollectionTextObservation(x.History,len(x.History)-1){
  return "",false
 }
 // Exact output need not be a successful read (schema/partial is truthful).
 if strings.TrimSpace(last.Content)==""{return "",false}
 return ProviderPayloadDigest([]byte(last.Content)),true
}

// CollectionAutoResumeAllowed is checked again against the canonical journal
// at admission, including owner opt-out while the payload is being built.
func CollectionAutoResumeAllowed(x Session) bool {
 if _,ok:=CollectionResumeReceipt(x);!ok{return false}
 if x.TaskPlan!=nil{
  return x.TaskPlan.State=="active"&&x.TaskPlan.AutoContinue&&TaskPlanNext(x.TaskPlan)!=nil&&
   x.TaskPlan.NoProgressTurns<MaxContinuationNoWorkTurns
 }
 return StandaloneContinuationGranted(x)
}

// retainCollectionResumeCheckpoint must run under the existing store lock
// after CAS/owner/staged-payload verification. A caller cannot inject or erase
// prior checkpoints. No attempt sidecar is overwritten here.
func(s *Store) retainCollectionResumeCheckpoint(x *Session,previous Session)error{
 if !reflect.DeepEqual(x.CollectionResumeHistory,previous.CollectionResumeHistory){return ErrInvalidSession}
 old:=previous.ProviderRun
 if old==nil||old.Kind!="collection"||old.Phase!="unknown"||x.ProviderRun==nil||x.ProviderRun.ID==old.ID{return nil}
 // Only explicit new collection recovery is implemented here. Ordinary
 // cancelled turns retain existing semantics; failed collection recovery is
 // fenced at message admission and additionally at the CAS.
 if !HasUnknownCollectionDelivery(previous){return nil}
 if x.ProviderRun.Phase!="prepared"||x.ProviderRun.Kind!="message"{return ErrInvalidSession}
 kind:=""
 for _,e:=range x.Events{
  if e.Seq>previous.EventSeq&&e.Detail==old.ID&&e.CallID==old.CallID&&
   (e.Kind=="collection_resume_new"||e.Kind=="collection_auto_resume_new"||e.Kind=="collection_new_message"){kind=e.Kind;break}
 }
 var receipt string
 switch kind {
 case "collection_resume_new","collection_auto_resume_new":
  var ok bool
  receipt,ok=CollectionResumeReceipt(previous)
  if !ok||x.LastRequestKey==""{return ErrInvalidSession}
  if kind=="collection_auto_resume_new"&&!CollectionAutoResumeAllowed(previous){return ErrInvalidSession}
 case "collection_new_message":
  if !CanSendAfterUnknownCollection(previous){return ErrInvalidSession}
  receipt=unknownCollectionReceiptDigest(previous)
 default:
  return errors.New("new collection message checkpoint required")
 }
 checkpoint:=CollectionResumeCheckpoint{Previous:*old,NewRunID:x.ProviderRun.ID,ReceiptSHA256:receipt,ReceiptUnavailable:receipt==""}
 audit,err:=s.readProviderAttemptsLocked(previous.ID)
 if err!=nil&&!os.IsNotExist(err){return err}
 if err==nil&&audit.RunID==old.ID{
  checkpoint.AttemptAudit=&ProviderAttemptAudit{RunID:audit.RunID,Total:audit.Total,
   Truncated:audit.Total>uint64(len(audit.Attempts)),Attempts:append([]ProviderAttempt(nil),audit.Attempts...)}
 }
 x.CollectionResumeHistory=append(append([]CollectionResumeCheckpoint(nil),previous.CollectionResumeHistory...),checkpoint)
 // Keep the latest bounded diagnostic window without eventually blocking Send.
 if len(x.CollectionResumeHistory)>maxCollectionResumeCheckpoints{
  x.CollectionResumeHistory=append([]CollectionResumeCheckpoint(nil),x.CollectionResumeHistory[len(x.CollectionResumeHistory)-maxCollectionResumeCheckpoints:]...)
 }
 return nil
}
