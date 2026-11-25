// Package contextcompact builds provider-visible history windows. It never
// changes the canonical Agent Chat journal or executes any tool. All policies
// and token heuristics live here so they can be replaced independently.
package contextcompact

import (
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "fmt"
 "strings"
 "unicode/utf8"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

const (
 DefaultInputTokens=32768
 SafeInputPercent=70
 DefaultCollectionPercent=30
 MaxProviderHistoryItems=200
 MaxOldEvidenceCatalog=36
 // Reserve bounded catalog space for safety-relevant historical receipts.
 MaxPriorityCatalogEntries=12
)

type Policy struct {
 MaxInputTokens int
 CollectionPercent int
 Instructions string
 Next []agentchat.HistoryItem
 // The archive is not replayed; its count is reported so the model knows
 // older evidence exists, but cannot assume anything about its contents.
 ArchivedItems uint64
 ArchivedParts uint64
 ArchivedPatchCalls uint64
 ArchivedPatchSummaries []string
 SyntheticPatchIDs map[string]bool
}
type Stats struct {
 EstimatedTotalTokens int
 CollectionBudgetTokens int
 RecentFullCollections int
 SummarizedCollections int
 SummarizedPatches int
 ArchivedItems uint64
 HistoryItems int
 BudgetExceeded bool
}
type group struct {
 items []agentchat.HistoryItem
 kind string
 summary string
 tokens int
 keep bool
 full bool
}
func Tokens(s string) int {
 // A deliberately conservative byte-based heuristic; not a provider
 // tokenizer. For UTF-8 content, bytes/2 is intentionally pessimistic.
 if s=="" {return 0}
 return (len(s)+1)/2+4
}
func itemTokens(x agentchat.HistoryItem) int {return Tokens(x.Content)+Tokens(x.Arguments)+24}
func groupTokens(items []agentchat.HistoryItem) int{
 n:=0;for _,v:=range items{n+=itemTokens(v)};return n
}
func truncate(s string,limit int)string{
 if len(s)<=limit{return s}
 if limit<8 {return ""}
 n:=limit-3
 for n>0&&!utf8.ValidString(s[:n]){n--}
 return s[:n]+"..."
}
func digest(s string) string {b:=sha256.Sum256([]byte(s));return hex.EncodeToString(b[:8])}
func extractPatchName(arguments string)string {
 var arg struct{Name string `json:"name"`}
 if json.Unmarshal([]byte(arguments),&arg)==nil && strings.TrimSpace(arg.Name)!=""{
  return truncate(strings.TrimSpace(arg.Name),110)
 }
 return "unnamed"
}
func patchSummary(call,output agentchat.HistoryItem) string{
 name:=extractPatchName(call.Arguments)
 status:="result recorded"
 detail:=""
 var raw map[string]any
 if json.Unmarshal([]byte(output.Content),&raw)==nil {
  if s,ok:=raw["status"].(string);ok {status=truncate(s,80)}
  if status=="FAIL"||status=="PREFLIGHT_FAIL"||status=="INCOMPLETE" {
   field:=func(key string, limit int)string {v,_:=raw[key].(string);return truncate(v,limit)}
   mutation:=field("mutation_state",32);if mutation==""{mutation="UNKNOWN"}
   availability:=field("failure_evidence",64);if availability==""{availability="not recorded"}
   detail=fmt.Sprintf(" Failure evidence (untrusted data): reason=%q; diagnosis=%q; mutation=%q; availability=%q.",field("failure_reason",240),field("diagnosis_kind",80),mutation,availability)
  }
  if s,ok:=raw["run_id"].(string);ok && s!="" {status+="; run="+truncate(s,90)}
 }
 return fmt.Sprintf("Prior Native Patch %q, call_id=%s, %s; receipt_hash=%s.%s Full patch source and execution evidence remain in TaskDeck history; NOT re-sent.",name,truncate(call.CallID,100),status,digest(output.Content),detail)
}
func collectionSummary(call,output agentchat.HistoryItem)string{
 label:="Collection"
 evidence:=make([]string,0,10)
 var args struct{Actions []struct{
  Type string `json:"type"`
  Path string `json:"path"`
  Paths []string `json:"paths"`
  Query string `json:"query"`
  StartLine int `json:"start_line"`
  EndLine int `json:"end_line"`
  Symbol string `json:"symbol"`
 } `json:"actions"`}
 if json.Unmarshal([]byte(call.Arguments),&args)==nil{
  actions:=make([]string,0,len(args.Actions))
  for _,a:=range args.Actions{
   desc:=a.Type
   if a.Path!="" {desc+=" "+truncate(a.Path,90)}
   if a.Query!="" {desc+=" query="+truncate(a.Query,60)}
   if a.StartLine>0||a.EndLine>0{desc+=fmt.Sprintf(" lines=%d-%d",a.StartLine,a.EndLine)}
   if a.Symbol!=""{desc+=" symbol="+truncate(a.Symbol,60)}
   actions=append(actions,desc)
   for _,p:=range a.Paths {if len(evidence)<10{evidence=append(evidence,truncate(p,110))}}
  }
  if len(actions)>0 {label=strings.Join(actions,", ")}
 }
 var data struct{
 Status string `json:"status"`
 RunID string `json:"run_id"`
 ResultText string `json:"result_text"`
 Error *struct{Code string `json:"code"`;Message string `json:"message"`} `json:"error"`
 Results []struct {
  Files []struct{Path string `json:"path"`; Content string `json:"content"`; Error string `json:"error"`} `json:"files"`
  Entries []string `json:"entries"`
  Matches []string `json:"matches"`
 } `json:"results"`}
 if json.Unmarshal([]byte(output.Content),&data)==nil {
  for _,r:=range data.Results{
   for _,f:=range r.Files {
    if len(evidence)>=10 {break}
    p:=truncate(f.Path,110)
    if f.Error!="" {p+=" (read error)"} else if f.Content!=""{
     first:=strings.TrimSpace(strings.SplitN(f.Content,"\n",2)[0])
     if first!=""{p+=" (first line: "+truncate(first,64)+")"}
    }
    evidence=append(evidence,p)
   }
   for _,s:=range r.Entries{if len(evidence)<10 {evidence=append(evidence,truncate(s,110))}}
   for _,s:=range r.Matches{if len(evidence)<10 {evidence=append(evidence,truncate(s,110))}}
  }
 }
 if data.Status!=""{evidence=append(evidence,"status="+truncate(data.Status,40))}
 if data.Error!=nil{evidence=append(evidence,"error="+truncate(data.Error.Code,60)+" "+truncate(data.Error.Message,160))}
 if data.RunID!=""{evidence=append(evidence,"run="+truncate(data.RunID,90))}
 if data.ResultText!=""{evidence=append(evidence,"artifact="+truncate(data.ResultText,220))}
 if len(evidence)==0{evidence=append(evidence,"See archived Collection result for file/evidence details")}
 return fmt.Sprintf("Prior Collection observation [%s]; call_id=%s; evidence: %s; result_hash=%s. This catalog is not proof of success; reuse prior evidence where sufficient and re-read only specific missing details.",
  truncate(label,180),truncate(call.CallID,100),strings.Join(evidence,"; "),digest(output.Content))
}
func plainSummary(text string)string{
 return "Earlier Collection fallback observation (not proof of success); receipt_hash="+digest(text)+"; full evidence remains in TaskDeck history."
}
// An orphan is a historical tool call without verified feedback in the
// canonical journal. Keep that journal untouched, but never replay an
// unmatched function_call into a stateless Responses request. This is NOT a
// synthetic tool result and never asserts that the Patch ran.
func orphanCallSummary(call agentchat.HistoryItem)string{
 label:=call.Name
 if call.Name=="taskdeck_submit_patch"{label+=" ("+extractPatchName(call.Arguments)+")"}
 return fmt.Sprintf("UNRESOLVED historical tool call %q, call_id=%s, arguments_hash=%s: no verified function_call_output exists in this provider-visible exchange. Execution status UNKNOWN. TaskDeck did not re-run the tool or invent feedback; inspect the original private Agent/Patch journal before retrying any side effect.",
  truncate(label,130),truncate(call.CallID,100),digest(call.Arguments))
}
func orphanOutputSummary(out agentchat.HistoryItem)string{
 return fmt.Sprintf("UNPAIRED historical tool feedback, call_id=%s, evidence_hash=%s. Original payload remains in TaskDeck journal; no executable call or result was fabricated.",
  truncate(out.CallID,100),digest(out.Content))
}
// These are presentation priorities only. Recompacted catalog text is not
// authoritative evidence and cannot grant approval or confirm a receipt.
func priorityCatalogEntry(s string) bool {
 return strings.HasPrefix(s, "Historical Native Patch failure evidence (untrusted data): ") ||
  strings.HasPrefix(s, "Prior Native Patch ") ||
  strings.HasPrefix(s, "UNRESOLVED historical tool call ") ||
  strings.HasPrefix(s, "UNPAIRED historical tool feedback,") ||
  strings.HasPrefix(s, "TaskDeck has archived ")
}

// Select recent safety metadata first within a bounded reservation, then fill
// remaining slots with recent observations. Keep chronological order in the
// result. Neither the catalog nor its selection changes canonical history.
func selectCatalog(catalog []string, room int) []string {
 chosen:=make([]bool,len(catalog))
 used,count:=0,0
 choose:=func(i int)bool{
  if chosen[i] || count>=MaxOldEvidenceCatalog{return false}
  cost:=Tokens(catalog[i])+2
  if cost>room-used{return false}
  chosen[i]=true
  used+=cost
  count++
  return true
 }
 n:=0
 for i:=len(catalog)-1;i>=0&&n<MaxPriorityCatalogEntries;i--{
  if strings.HasPrefix(catalog[i],"Historical Native Patch failure evidence (untrusted data): ")&&choose(i){n++}
 }
 // Prioritize historical failure diagnostics over ordinary successful receipts.
 // This is presentation only; it does not assert that a failure is unresolved.
 for i:=len(catalog)-1;i>=0&&n<MaxPriorityCatalogEntries;i--{
  if strings.HasPrefix(catalog[i],"Prior Native Patch ") && strings.Contains(catalog[i]," Failure evidence (untrusted data):") && choose(i){n++}
 }
 for i:=len(catalog)-1;i>=0&&n<MaxPriorityCatalogEntries;i--{
  if priorityCatalogEntry(catalog[i])&&choose(i){n++}
 }
 for i:=len(catalog)-1;i>=0&&count<MaxOldEvidenceCatalog;i--{choose(i)}
 selected:=make([]string,0,count)
 for i,entry:=range catalog{if chosen[i]{selected=append(selected,entry)}}
 return selected
}
func groups(history []agentchat.HistoryItem,synthetic map[string]bool,next []agentchat.HistoryItem) []group{
 // Fresh once-only Patch/Collection feedback is delivered in Next, not yet
 // persisted in History. Preserve exactly that pending call for this request.
 awaiting:=map[string]bool{}
 for _,n:=range next {if n.Type=="function_call_output"{awaiting[n.CallID]=true}}
 out:=make([]group,0,len(history))
 for i:=0;i<len(history);i++{
  a:=history[i]
  if a.Type=="function_call"&&synthetic[a.CallID]{
   if i+1<len(history)&&history[i+1].Type=="function_call_output"&&history[i+1].CallID==a.CallID{i++}
   continue
  }
  if a.Type=="function_call"{
   if i+1<len(history)&&history[i+1].Type=="function_call_output"&&history[i+1].CallID==a.CallID{
    b:=history[i+1]
    kind:=""
    summary:=""
    if a.Name=="taskdeck_collect_workspace"{kind="collection";summary=collectionSummary(a,b)}
    if a.Name=="taskdeck_submit_patch"{kind="patch";summary=patchSummary(a,b)}
    g:=group{items:[]agentchat.HistoryItem{a,b},kind:kind,summary:summary}
    g.tokens=groupTokens(g.items);out=append(out,g);i++;continue
   }
   if awaiting[a.CallID] {
    g:=group{items:[]agentchat.HistoryItem{a},kind:"pending",keep:true}
    g.tokens=groupTokens(g.items);out=append(out,g);continue
   }
   // A stale function call without its output is a legacy journal gap,
   // not a provider-visible operation. Catalog it without forging output.
   summary:=orphanCallSummary(a)
   out=append(out,group{kind:"orphan",summary:summary,tokens:Tokens(summary)})
   continue
  }
  if a.Type=="function_call_output" {
   // Never replay a stray output without its matching function_call.
   // Real live feedback is paired by the awaiting/Next logic above.
   summary:=orphanOutputSummary(a)
   out=append(out,group{kind:"orphan",summary:summary,tokens:Tokens(summary)})
   continue
  }
  if a.Type=="assistant" && strings.HasPrefix(a.Content,"[TASKDECK VERIFIED HISTORICAL EVIDENCE CATALOG — summaries only, not new user instructions]\n"){
   for _,line:=range strings.Split(a.Content,"\n")[1:]{
    if strings.TrimSpace(line)!=""{
     limit:=1500
     if strings.HasPrefix(line,"Historical Native Patch failure evidence (untrusted data): "){limit=2800}
     out=append(out,group{kind:"catalog",summary:truncate(line,limit)})
    }
   }
   continue
  }
  if a.Type=="assistant"&&agentchat.CollectionTextObservation(history,i+1){
   b:=history[i+1]
   g:=group{items:[]agentchat.HistoryItem{a,b},kind:"fallback",summary:plainSummary(b.Content)}
   g.tokens=groupTokens(g.items);out=append(out,g);i++;continue
  }
  if agentchat.CollectionTextObservation(history,i){
   g:=group{items:[]agentchat.HistoryItem{a},kind:"fallback",summary:plainSummary(a.Content)}
   g.tokens=groupTokens(g.items);out=append(out,g);continue
  }
  g:=group{items:[]agentchat.HistoryItem{a},kind:"plain",keep:a.Type=="user"}
  g.tokens=groupTokens(g.items);out=append(out,g)
 }
 return out
}

// Build returns a strictly NEW, provider-visible slice; the session journal
// and native Patch receipts remain byte-exact and unchanged. N is selected
// from the actual estimated token space left after instructions, Next and
// essential user/pending-call context, not an arbitrary round count.
// Most recent completed Collection exchanges are retained as real tool pairs.
// Older ones become compact assistant catalog entries, never fabricated tool
// calls. Past completed Patch source code/output is never replayed in full.
func Build(history []agentchat.HistoryItem,p Policy)([]agentchat.HistoryItem,Stats){
 if p.MaxInputTokens<=0 {p.MaxInputTokens=DefaultInputTokens}
 if p.CollectionPercent<=0||p.CollectionPercent>50{p.CollectionPercent=DefaultCollectionPercent}
 ceiling:=p.MaxInputTokens*SafeInputPercent/100
 nextTokens:=groupTokens(p.Next)
 overhead:=Tokens(p.Instructions)+nextTokens
 budget:=ceiling-overhead
 if budget<0{budget=0}
 gs:=groups(history,p.SyntheticPatchIDs,p.Next)
 stats:=Stats{CollectionBudgetTokens:ceiling*p.CollectionPercent/100,ArchivedItems:p.ArchivedItems}
 // Keep the original user goal and all unresolved model tool calls, plus
 // the latest human instructions. Older plain narration is supplementary.
 firstUser:=false
 for i:=range gs{
  if gs[i].kind=="plain"&&len(gs[i].items)==1&&gs[i].items[0].Type=="user"{
   if !firstUser{gs[i].keep=true;firstUser=true}else{gs[i].keep=false}
  }
 }
 recentUsers:=0
 for i:=len(gs)-1;i>=0;i--{
  if gs[i].kind=="plain"&&len(gs[i].items)==1&&gs[i].items[0].Type=="user"{
   if recentUsers<8{gs[i].keep=true;recentUsers++}
  }
 }
 essential:=0
 for i:=range gs {if gs[i].keep{essential+=gs[i].tokens}}
 avail:=budget-essential
 if avail<0{avail=0}
 collectionBudget:=stats.CollectionBudgetTokens
 if collectionBudget>avail{collectionBudget=avail}
 // The immediately previous completed Collection is full only when the
 // budget allows; the current Collection output in Next remains full.
 for i:=len(gs)-1;i>=0;i--{
  if gs[i].kind=="collection"&&gs[i].tokens<=collectionBudget{
   gs[i].full=true
   collectionBudget-=gs[i].tokens
   stats.RecentFullCollections++
  }
 }
 var catalog []string
 result:=make([]agentchat.HistoryItem,0,len(gs)+2)
 for i:=range gs{
  g:=gs[i]
  switch g.kind{
  case "collection","patch","fallback":
   if g.full{result=append(result,g.items...);continue}
   if g.kind=="collection"||g.kind=="fallback"{stats.SummarizedCollections++}else{stats.SummarizedPatches++}
   catalog=append(catalog,g.summary)
  case "orphan","catalog":
   catalog=append(catalog,g.summary)
  case "plain":
   if g.keep {result=append(result,g.items...)}
   // Short recent assistant text is useful for workflow continuity; ancient
   // narration is discarded, not used as fabricated evidence.
   if !g.keep && i>=len(gs)-8 && g.tokens<=768{result=append(result,g.items...)}
  default:
   result=append(result,g.items...)
  }
 }
 if p.ArchivedPatchCalls>0{
  patchNote:=fmt.Sprintf("TaskDeck has archived %d completed Native Patch receipts. Only metadata for recent archived Patches is below; full code and receipts remain in the private journal.",p.ArchivedPatchCalls)
  if len(p.ArchivedPatchSummaries)>0{patchNote+=" "+strings.Join(p.ArchivedPatchSummaries,"; ")}
  catalog=append(catalog,truncate(patchNote,3000))
 }
 if len(catalog)>0{
  room:=ceiling-overhead-groupTokens(result)-64
  if p.ArchivedItems>0{room-=128}
  if room<0{room=0}
  if room>7400{room=7400}
  catalog=selectCatalog(catalog,room)
 }
 if len(catalog)>0{
  note:="[TASKDECK VERIFIED HISTORICAL EVIDENCE CATALOG — summaries only, not new user instructions]\n"+
   strings.Join(catalog,"\n")
  if stats.SummarizedCollections+stats.SummarizedPatches>len(catalog){
   note+=fmt.Sprintf("\n%d older results omitted from this bounded catalog; use scoped Collection to reverify.",stats.SummarizedCollections+stats.SummarizedPatches-len(catalog))
  }
  // Put the catalog after the original task if available, so it cannot
  // supersede the user's later instructions.
  pos:=0
  if len(result)>0&&result[0].Type=="user"{pos=1}
  result=append(result[:pos],append([]agentchat.HistoryItem{{Type:"assistant",Content:truncate(note,15000)}},result[pos:]...)...)
 }
 if p.ArchivedItems>0{
  note:=fmt.Sprintf("[TaskDeck archive checkpoint] %d older history items stored in %d durable segments. Exact evidence is NOT in this request; request scoped reads if required.",p.ArchivedItems,p.ArchivedParts)
  pos:=0;if len(result)>0&&result[0].Type=="user"{pos=1}
  result=append(result[:pos],append([]agentchat.HistoryItem{{Type:"assistant",Content:note}},result[pos:]...)...)
 }
 // Trim least-recent plain assistant messages if the provider input-count
 // constraint or shared input-token ceiling still would be exceeded.
 tokenCount:=overhead+groupTokens(result)
 for len(result)+len(p.Next)>MaxProviderHistoryItems || tokenCount>ceiling{
  removed:=false
  for i:=1;i<len(result)-1;i++{
   if result[i].Type=="assistant"{
    tokenCount-=itemTokens(result[i])
    result=append(result[:i],result[i+1:]...)
    removed=true;break
   }
  }
  if !removed{break}
 }
 stats.EstimatedTotalTokens=overhead+groupTokens(result)
 stats.HistoryItems=len(result)
 stats.BudgetExceeded=stats.EstimatedTotalTokens>ceiling||len(result)+len(p.Next)>MaxProviderHistoryItems
 return result,stats
}
