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
 ReceiptSHA256 string `json:"receipt_sha256"`
 AttemptAudit *ProviderAttemptAudit `json:"attempt_audit,omitempty"`
}

func validCollectionResumeHistory(items []CollectionResumeCheckpoint)bool{
 if len(items)>maxCollectionResumeCheckpoints{return false}
 seen:=map[string]bool{}
 for _,it:=range items{
  if !validProviderRun(&it.Previous)||it.Previous.Kind!="collection"||it.Previous.Phase!="unknown"||
   it.NewRunID==""||len(it.NewRunID)>96||it.NewRunID==it.Previous.ID||
   !validGuardDigest(it.ReceiptSHA256)||seen[it.Previous.ID]{return false}
  seen[it.Previous.ID]=true
  if a:=it.AttemptAudit;a!=nil{
   if a.RunID!=it.Previous.ID||a.Total==0||len(a.Attempts)==0||len(a.Attempts)>maxProviderAttemptRecords||
    uint64(len(a.Attempts))>a.Total||a.Truncated!=(uint64(len(a.Attempts))<a.Total){return false}
  }
 }
 return true
}

// CollectionResumeReceipt is an explicit owner-action eligibility check,
// not a permission or ACK. Only a failed local Legacy COLLECT continuation
// whose current receipt is already canonical may be resumed. Historical
// unconfirmed or orphan mutation remains a blocker, even with LastRequestCode=200.
func CollectionResumeReceipt(x Session)(string,bool){
 run:=x.ProviderRun
 if x.Owner!="local"||x.Archived||x.Status!="failed"||x.PendingUserTurn!=nil||
  run==nil||run.Kind!="collection"||run.Phase!="unknown"||
  len(x.CollectionResumeHistory)>=maxCollectionResumeCheckpoints||
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
 if previous.Status!="failed"{return nil}
 receipt,ok:=CollectionResumeReceipt(previous)
 if !ok||x.ProviderRun.Phase!="prepared"||x.ProviderRun.Kind!="message"||x.LastRequestKey==""{return ErrInvalidSession}
 found:=false
 for _,e:=range x.Events{
  if e.Seq>previous.EventSeq&&e.Kind=="collection_resume_new"&&e.Detail==old.ID{found=true;break}
 }
 if !found{return errors.New("explicit collection resume checkpoint required")}
 checkpoint:=CollectionResumeCheckpoint{Previous:*old,NewRunID:x.ProviderRun.ID,ReceiptSHA256:receipt}
 audit,err:=s.readProviderAttemptsLocked(previous.ID)
 if err!=nil&&!os.IsNotExist(err){return err}
 if err==nil&&audit.RunID==old.ID{
  checkpoint.AttemptAudit=&ProviderAttemptAudit{RunID:audit.RunID,Total:audit.Total,
   Truncated:audit.Total>uint64(len(audit.Attempts)),Attempts:append([]ProviderAttempt(nil),audit.Attempts...)}
 }
 x.CollectionResumeHistory=append(append([]CollectionResumeCheckpoint(nil),previous.CollectionResumeHistory...),checkpoint)
 return nil
}
