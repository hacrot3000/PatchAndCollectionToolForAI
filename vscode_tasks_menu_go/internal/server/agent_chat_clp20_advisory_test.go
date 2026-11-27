package server

import(
 "context"
 "fmt"
 "encoding/json"
 "strings"
 "testing"
 "bletonfc/vscode_tasks_menu/internal/agentchat"
 cp "bletonfc/vscode_tasks_menu/internal/agentchat/collectionprogress"
 "bletonfc/vscode_tasks_menu/internal/patchtool"
)

func TestAgentCLP20StructuredCollectionAdviceBoundedAndPermissionNeutral(t *testing.T){
 for _,tc:=range []struct{intent cp.Intent;goal string;allow bool;reason cp.Reason;want bool}{
  {cp.Implementation,"Fix PRIVATE_OWNER_GOAL",true,cp.AssessReadiness,true},
  {cp.Implementation,"Fix PRIVATE_OWNER_GOAL",false,cp.BoundNextScope,false},
  {cp.ReadOnly,"Audit PRIVATE_OWNER_GOAL read-only",true,cp.RepeatedEvidence,true},
  {cp.UnknownIntent,"Continue",false,cp.RepeatedEvidence,true},
  {cp.ReadOnly,"Audit PRIVATE_OWNER_GOAL read-only",true,cp.AssessReadiness,false},
 }{
  o,e:=cp.BindObjective(cp.ObjectiveScope{Owner:"local",ProjectID:"p",OwnerWorkspace:"/private",ExecutionWorkspace:"/private",ConversationID:"c",Goal:tc.goal,Generation:1})
  if e!=nil||o.Intent!=tc.intent{t.Fatalf("fixture %+v %v",o,e)}
  c,e:=cp.NewCounterState(o);if e!=nil{t.Fatal(e)}
  c.NativeReads=15;c.DuplicateEvidence=8;c.NewEvidence=7;c.Progress.ReadsSinceProgress=15
  advice:=collectionProgressAdvisory(cp.Decision{Kind:cp.Advise,Reason:tc.reason},c,tc.allow)
  if (advice!="")!=tc.want{t.Fatalf("unexpected advisory: %q",advice)}
  if !tc.want{continue}
  const prefix="Collection advisory metadata (not new evidence or authorization): "
  if strings.Count(advice,prefix)!=1||strings.Contains(advice,"PRIVATE_OWNER_GOAL")||strings.Contains(advice,o.Key)||strings.Contains(advice,"/private")||len(advice)>1200{t.Fatal("unsafe advisory")}
  var got map[string]any
  raw:=strings.TrimSpace(strings.SplitN(advice,prefix,2)[1])
  if e=json.Unmarshal([]byte(raw),&got);e!=nil||got["native_reads"]!=float64(15)||got["verified_duplicate"]!=float64(8)||got["reason"]!=string(tc.reason){t.Fatalf("bad bounded metadata %s",raw)}
  if tc.intent==cp.ReadOnly&&got["next_strategy"]!="continue_read_only_with_distinct_missing_scope"{t.Fatal("read-only advice changed intent")}
 }
}

func TestAgentCLP20TextFallbackAdvisoryPreservesExactObservation(t *testing.T) {
 for _, tc := range []struct {
  name, goal string
  intent cp.Intent
  readiness bool
 }{
  {"implementation", "Fix the defect", cp.Implementation, true},
  {"mixed", "Audit source and fix the defect", cp.Mixed, true},
  {"read_only", "Audit the source read-only", cp.ReadOnly, false},
  {"unknown", "Consider the source", cp.UnknownIntent, false},
 } {
  t.Run(tc.name, func(t *testing.T) {
   s, id := newAgentConversationFixture(t, "previous_response")
   identity, err := patchtool.ReadProjectIdentity(s.Workspace)
   if err != nil { t.Fatal(err) }
   receipts := map[int]string{}
   requests := map[int]string{}
   reads, calls, advisories := 0, 0, 0
   s.AgentCollectionCall = func(_ string, project patchtool.ProjectIdentity, _ []byte) string {
    reads++
    path := "src/a.go"
    if reads == 5 { path = "src/b.go" }
    raw, err := json.Marshal(map[string]any{
     "source": "native_patch_tool_collect", "project": project,
     "native_session_id": "synthetic-native", "run_id": fmt.Sprintf("text-native-%d", reads),
     "status": "PASS", "native_status": "PASS", "rc": 0,
     "evidence_verified": true, "text_excerpt_truncated": false,
     "native_full_file_reads": []map[string]any{{"path": path, "action_index": 1, "total_lines": 10, "report_sha256": strings.Repeat("a", 64)}},
    })
    if err != nil { t.Fatal(err) }
    receipts[reads] = agentchat.NativeCollectionTextObservationPrefix + string(raw)
    return string(raw)
   }
   s.AgentChatCall = func(_ context.Context, _ agentchat.ProviderConfig, _ string, payload []byte) (agentchat.ResponsesResult, error) {
    calls++
    var wire struct {
     Input []map[string]any `json:"input"`
     Instructions string `json:"instructions"`
     PreviousResponseID string `json:"previous_response_id"`
    }
    if err := json.Unmarshal(payload, &wire); err != nil { t.Fatal(err) }
    if calls > 1 {
     if wire.PreviousResponseID != "" { t.Fatal("text fallback reused unstored provider reference") }
     n := 0
     for i, item := range wire.Input {
      if item["type"] == "function_call_output" || item["type"] == "function_call" { t.Fatal("fabricated typed tool pair for text fallback") }
      if item["content"] == receipts[calls-1] {
       n++
       if item["role"] != "user" || i == 0 || wire.Input[i-1]["content"] != requests[calls-1] || wire.Input[i-1]["role"] != "assistant" {
        t.Fatal("text observation detached from original request")
       }
      }
     }
     if n != 1 { t.Fatalf("exact current observation count %d at call %d", n, calls) }
    }
    const prefix = "Collection advisory metadata (not new evidence or authorization): "
    want := cp.NoAdvisory
    if calls == 5 { want = cp.RepeatedEvidence }
    if calls == 6 && tc.readiness { want = cp.AssessReadiness }
    count := strings.Count(wire.Instructions, prefix)
    if want == cp.NoAdvisory {
     if count != 0 { t.Fatalf("unexpected advisory at call %d", calls) }
    } else {
     advisories++
     if count != 1 { t.Fatalf("advisory count %d", count) }
     line := strings.SplitN(strings.SplitN(wire.Instructions, prefix, 2)[1], "\n", 2)[0]
     var v struct {
      Reason cp.Reason `json:"reason"`
      Intent cp.Intent `json:"intent"`
      Reads uint64 `json:"native_reads"`
      Strategy string `json:"next_strategy"`
     }
     if json.Unmarshal([]byte(line), &v) != nil || v.Reason != want || v.Intent != tc.intent || v.Reads != uint64(calls-1) || len(line) > 768 {
      t.Fatalf("invalid advisory: %s", line)
     }
     if tc.intent == cp.ReadOnly && v.Strategy != "continue_read_only_with_distinct_missing_scope" { t.Fatal("read-only advisory changed intent") }
    }
    if calls == 6 { return agentchat.ResponsesResult{ResponseID: "done", Text: "Inspected the requested scopes."}, nil }
    if calls > 6 { t.Fatalf("unexpected provider call %d", calls) }
    path := "src/a.go"
    if calls == 5 { path = "src/b.go" }
    raw, err := json.Marshal(map[string]any{
     "project": identity, "id": fmt.Sprintf("text-%d", calls), "title": fmt.Sprintf("Read %d", calls),
     "actions": []map[string]string{{"type":"file", "path":path}},
    })
    if err != nil { t.Fatal(err) }
    requests[calls] = string(raw)
    return agentchat.ResponsesResult{ResponseID: fmt.Sprintf("text-reply-%d", calls), Text: string(raw)}, nil
   }
   w := invokeAI(t, s.agentChatSessionItemAPI, "POST", "/api/agent/sessions/"+id+"/messages", map[string]any{
    "message": tc.goal, "confirm_external": true, "expected_revision": 1,
   })
   expectedAdvisories := 1
   if tc.readiness { expectedAdvisories++ }
   if w.Code != 200 || reads != 5 || calls != 6 || advisories != expectedAdvisories {
    t.Fatalf("HTTP=%d reads=%d calls=%d advisories=%d body=%s", w.Code, reads, calls, advisories, w.Body)
   }
   store, err := s.agentChatStore()
   if err != nil { t.Fatal(err) }
   saved, err := store.Get(id, "local")
   if err != nil { t.Fatal(err) }
   g, ok := saved.ReadCollectionGuard()
   if !ok || g.Counters.NativeReads != 5 || g.Counters.DuplicateEvidence != 3 || g.Counters.NewEvidence != 2 ||
    saved.PreviousResponseID != "" || len(saved.ToolCalls) != 0 { t.Fatal("unexpected durable fallback state") }
   n := 0
   for _, item := range saved.History {
    if strings.HasPrefix(item.Content, agentchat.NativeCollectionTextObservationPrefix) {
     n++
     if item.Type != "user" || item.Content != receipts[n] { t.Fatal("canonical text observation changed") }
    }
   }
   if n != 5 { t.Fatalf("canonical observation count %d", n) }
  })
 }
}
