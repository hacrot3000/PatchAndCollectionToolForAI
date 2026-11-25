package collectionprogress

import (
 "encoding/json"
)

const maxRetainedCoverageIntervals=1024

type nativeLineProof struct{
 Path string `json:"path"`
 ActionIndex int `json:"action_index"`
 Type string `json:"type"`
 TotalLines uint64 `json:"total_lines"`
 StartLine uint64 `json:"start_line"`
 EndLine uint64 `json:"end_line"`
 SourceRevisionSHA256 string `json:"source_revision_sha256"`
 ReportSHA256 string `json:"report_sha256"`
}

// ValidateCoverage rejects corrupt metadata without discarding canonical
// history. The aggregate bound fits the existing guard's byte budget.
func ValidateCoverage(index CoverageIndex,b ObjectiveBinding)bool{
 if !b.Valid()||index.ObjectiveKey!=b.Key||len(index.Entries)>MaxCoverageScopes{return false}
 seen:=map[string]bool{}
 total:=0
 for _,e:=range index.Entries{
  if !e.Key.Valid()||e.Key.ObjectiveKey!=b.Key||seen[e.Key.PathDigest]||len(e.Ranges)==0||len(e.Ranges)>MaxCoverageIntervals{return false}
  seen[e.Key.PathDigest]=true
  total+=len(e.Ranges)
  if total>maxRetainedCoverageIntervals{return false}
  for i,r:=range e.Ranges{if !r.Valid()||(i>0&&r.First<=e.Ranges[i-1].Last+1){return false}}
 }
 return true
}

func requestedLineRange(a map[string]any,total uint64)(LineRange,bool){
 if total==0||total>1<<30{return LineRange{},false}
 number:=func(key string,def uint64)(uint64,bool){
  v,exists:=a[key];if !exists{return def,true}
  n,ok:=v.(json.Number);if !ok{return 0,false}
  i,e:=n.Int64();if e!=nil||i<1{return 0,false}
  return uint64(i),true
 }
 var first,last uint64
 switch a["type"]{
 case "file","range":
  var ok bool
  first,ok=number("start_line",1);if !ok{return LineRange{},false}
  last,ok=number("end_line",total);if !ok{return LineRange{},false}
  if first>total{first=total};if last<first{last=first};if last>total{last=total}
 case "head","tail":
  count,ok:=number("lines",100);if !ok{return LineRange{},false}
  if count>total{count=total}
  if a["type"]=="head"{first,last=1,count}else{first,last=total-count+1,total}
 default:return LineRange{},false
 }
 return LineRange{first,last},true
}

// ObserveNativeRead compares only the exact current authenticated native
// receipt. All actions must have complete, request-bound proof. Older
// full-file receipts keep their exact-content comparison; range, symbol and
// search without proof remain UNKNOWN. It never authorizes a skipped read.
func ObserveNativeRead(exact EvidenceIndex,coverage *CoverageIndex,b ObjectiveBinding,scope ScopeDescriptor,projectKey,projectID string,request []byte,receipt string)(EvidenceIndex,*CoverageIndex,Novelty){
 fallback:=func()(EvidenceIndex,*CoverageIndex,Novelty){
  n,v:=ObserveEvidence(exact,scope,InspectNativeEvidence(scope,projectKey,projectID,request,receipt))
  return n,coverage,v
 }
 unknown:=func()(EvidenceIndex,*CoverageIndex,Novelty){return exact,coverage,UnknownNovelty}
 if len(receipt)==0||len(receipt)>maxEvidenceReceiptBytes||scope.ObjectiveKey!=b.Key{return unknown()}
 var o struct{
  Source string `json:"source"`
  Project struct{Key string `json:"key"`;ID string `json:"id"`} `json:"project"`
  SessionID string `json:"native_session_id"`
  RunID string `json:"run_id"`
  Status string `json:"status"`
  NativeStatus string `json:"native_status"`
  RC *int `json:"rc"`
  Verified bool `json:"evidence_verified"`
  Partial bool `json:"partial_evidence_available"`
  Truncated *bool `json:"text_excerpt_truncated"`
  Lines json.RawMessage `json:"native_line_reads"`
 }
 if json.Unmarshal([]byte(receipt),&o)!=nil{return unknown()}
 if len(o.Lines)==0{return fallback()}
 if o.Source!="native_patch_tool_collect"||o.Project.Key!=projectKey||o.Project.ID!=projectID||o.SessionID==""||o.RunID==""||
  o.Status!="PASS"||o.NativeStatus!="PASS"||o.RC==nil||*o.RC!=0||!o.Verified||o.Partial||o.Truncated==nil||*o.Truncated{return unknown()}
 expected,err:=BindCollectionScope(b,projectKey,projectID,request,scope.SourceRevisionDigest)
 if err!=nil||expected!=scope{return unknown()}
 obj,err:=canonicalRequest(request);if err!=nil{return unknown()}
 actions,ok:=obj["actions"].([]any);if !ok||len(actions)==0||len(actions)>maxEvidenceProofs{return unknown()}
 var proofs []nativeLineProof
 if json.Unmarshal(o.Lines,&proofs)!=nil||len(proofs)!=len(actions){return unknown()}
 keys:=make([]CoverageKey,len(actions));ranges:=make([]LineRange,len(actions));seen:=map[int]bool{};revisions:=map[string]string{}
 for _,p:=range proofs{
  i:=p.ActionIndex-1
  if i<0||i>=len(actions)||seen[i]||!validDigest(p.ReportSHA256){return unknown()}
  a,ok:=actions[i].(map[string]any);if !ok||a["path"]!=p.Path||a["type"]!=p.Type{return unknown()}
  key,ok:=NewCoverageKey(b,p.Path,p.SourceRevisionSHA256);if !ok{return unknown()}
  rr,ok:=requestedLineRange(a,p.TotalLines);if !ok||rr!=(LineRange{p.StartLine,p.EndLine}){return unknown()}
  if prev,ok:=revisions[key.PathDigest];ok&&prev!=key.SourceRevisionDigest{return unknown()}
  revisions[key.PathDigest]=key.SourceRevisionDigest
  seen[i]=true;keys[i]=key;ranges[i]=rr
 }
 next:=NewCoverageIndex(b)
 if coverage!=nil{if !ValidateCoverage(*coverage,b){return unknown()};next=*coverage}else if exact.Evicted||len(exact.Entries)>0{next.Evicted=true}
 // Drop only the old revision of the same path. A/B/A changes are never
 // compared with stale A coverage after the native observation of B.
 fresh:=make([]CoverageEntry,0,len(next.Entries))
 for _,entry:=range next.Entries{
  if rev,ok:=revisions[entry.Key.PathDigest];ok&&rev!=entry.Key.SourceRevisionDigest{continue}
  fresh=append(fresh,entry)
 }
 next.Entries=fresh
 novelty:=VerifiedDuplicate
 for i,key:=range keys{
  var c CoverageComparison
  next,c=ObserveCoverage(next,key,ranges[i])
  if c.Novelty==UnknownNovelty{novelty=UnknownNovelty}else if c.Novelty==VerifiedNew&&novelty!=UnknownNovelty{novelty=VerifiedNew}
 }
 // Resource exhaustion is not a read quota. Evict oldest metadata only.
 for !ValidateCoverage(next,b)&&len(next.Entries)>0{
  next.Entries=next.Entries[1:];next.Evicted=true;novelty=UnknownNovelty
 }
 if !ValidateCoverage(next,b){return unknown()}
 return exact,&next,novelty
}
