package agentchat

import (
 "encoding/json"
 "errors"
 "strings"
)

// A bounded provider-only recovery is not a native tool retry.
const incompleteRecoveryNote = "\nOUTPUT-BOUND RECOVERY (prior provider response hit max_output_tokens): " +
 "This is a fresh response. No partial output or tool call from the previous attempt was accepted or executed. " +
 "Use your remaining output budget sparingly. Complete ONE small, verifiable part of the original task per response. " +
 "If all work cannot fit, provide a concise checkpoint listing completed and remaining work. " +
 "If using a function tool, submit ONE complete, valid function call or a smaller complete Patch; " +
 "NEVER truncate Python, JSON or tool arguments, and never claim unverified execution. " +
 "Do not dump large source files or repeat prior evidence; stay within max_output_tokens."

func RetryConstrainedOutputPayload(payload []byte,p ProviderConfig)([]byte,int,bool){
 var req map[string]json.RawMessage
 if json.Unmarshal(payload,&req)!=nil||req==nil{return nil,0,false}
 var current int
 var modelID string
 if json.Unmarshal(req["max_output_tokens"],&current)!=nil||current<=0||
  json.Unmarshal(req["model"],&modelID)!=nil||modelID==""{return nil,0,false}
 var instructions string
 if raw,exists:=req["instructions"];exists&&json.Unmarshal(raw,&instructions)!=nil{return nil,0,false}
 if strings.Contains(instructions,"OUTPUT-BOUND RECOVERY"){return nil,0,false}
 if len(instructions)+len(incompleteRecoveryNote)>(1<<20){return nil,0,false}
 cap:=32768
 for _,m:=range p.Models{
  if m.ID==modelID{
   if m.MaxOutputTokens>0{cap=m.MaxOutputTokens}
   break
  }
 }
 if cap>200000{cap=200000}
 if current>cap{cap=current}
 next:=current
 if current<cap{
  next=current*2
  if next>cap{next=cap}
 }
 req["max_output_tokens"],_=json.Marshal(next)
 req["instructions"],_=json.Marshal(instructions+incompleteRecoveryNote)
 b,err:=json.Marshal(req)
 if err!=nil||len(b)>4<<20{return nil,0,false}
 return b,next,true
}

// Only a structured context-limit rejection authorizes a changed request.
// A generic 400, lost response or unknown provider state is not proof.
const MaxContextRecoveries = 3

func ContextLimitRejected(err error) bool {
 var pe *ProviderError
 if !errors.As(err,&pe)||pe.HTTPStatus!=400||pe.Ambiguous {return false}
 switch pe.ProviderErrorCode {
 case "context_length_exceeded","context_window_exceeded":return true
 default:return false
 }
}


// Structured context rejection may justify reducing reserved output while
// keeping every input item, schema, instruction and model exactly as sent.
// Unified snapshots treat the entire submitted suffix as mandatory; do not
// apply Legacy's last-receipt compactor to a Unified run.
func RetryReducedOutputReservation(payload []byte) ([]byte,bool) {
 var req map[string]json.RawMessage
 if json.Unmarshal(payload,&req)!=nil||req==nil{return nil,false}
 if _,ok:=req["previous_response_id"];ok{return nil,false}
 if _,ok:=req["conversation"];ok{return nil,false}
 var stored bool
 if raw,ok:=req["store"];ok&&(json.Unmarshal(raw,&stored)!=nil||stored){return nil,false}
 var limit int
 if json.Unmarshal(req["max_output_tokens"],&limit)!=nil||limit<=8192{return nil,false}
 limit/=2
 if limit<8192{limit=8192}
 req["max_output_tokens"],_=json.Marshal(limit)
 b,err:=json.Marshal(req)
 if err!=nil||len(b)>maxRequestBytes{return nil,false}
 return b,true
}
