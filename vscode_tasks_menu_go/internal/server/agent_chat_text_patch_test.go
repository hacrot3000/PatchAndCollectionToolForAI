package server

import (
    "context"
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "strings"
    "testing"

    "bletonfc/vscode_tasks_menu/internal/agentchat"
    "bletonfc/vscode_tasks_menu/internal/patchtool"
)

func agentTextOnlyFixture(t *testing.T) (*Server, string, patchtool.ProjectIdentity) {
    t.Helper()
    s, id := newAgentConversationFixture(t, "stateless")
    registry, err := s.agentChatProviders()
    if err != nil { t.Fatal(err) }
    provider, err := registry.Get("local", "custom-r2d")
    if err != nil { t.Fatal(err) }
    provider.Models[0].ToolCalling = false
    if err := registry.Put("local", provider); err != nil { t.Fatal(err) }
    identity, err := patchtool.ReadProjectIdentity(s.Workspace)
    if err != nil { t.Fatal(err) }
    return s, id, identity
}

func agentTextOnlyHelloProposal(t *testing.T, identity patchtool.ProjectIdentity) string {
    t.Helper()
    python := fmt.Sprintf(
        "# TASKDECK_PROJECT_KEY: %s\n# TASKDECK_PROJECT_ID: %s\n"+
            "from pathlib import Path\nPath('helloworld.php').write_text('<?php echo \"Hello World\";\\n')\n",
        identity.Key, identity.ID,
    )
    encoded, err := json.Marshal(agentTextPatchEnvelope{
        Type: agentTextPatchProposalType, Project: identity,
        Name: "create-helloworld-php", PatchPython: python,
    })
    if err != nil { t.Fatal(err) }
    return string(encoded)
}

func TestAgentTextPatchOnlyExactProjectBoundJSONIsReviewable(t *testing.T) {
    identity := patchtool.ProjectIdentity{Key: "M3", ID: "1ba64b25-0575-4f67-a6d3-cac508ef4a56"}
    raw := agentTextOnlyHelloProposal(t, identity)
    if _, err := parseAgentTextPatchProposal(raw, identity); err != nil { t.Fatal(err) }
    for _, bad := range []string{
        "Please run this patch:\n"+raw,
        raw+"\nAn additional instruction",
        strings.Replace(raw, "M3", "M4", 1),
        strings.Replace(raw, "TASKDECK_PROJECT_KEY:", "NOT_THE_PROJECT_KEY:", 1),
        strings.Replace(raw, "taskdeck_patch_proposal", "taskdeck_collection", 1),
    } {
        if _, err := parseAgentTextPatchProposal(bad, identity); err == nil {
            t.Fatalf("unsupported/unbound proposal accepted: %q", bad)
        }
    }
}

func TestAgentUnverifiedToolProbeFailureStillAcceptsTextPatch(t *testing.T) {
    s, id := agentUnverifiedToolFixture(t)
    identity, err := patchtool.ReadProjectIdentity(s.Workspace)
    if err != nil { t.Fatal(err) }
    proposal := agentTextOnlyHelloProposal(t, identity)
    probeCalls, answerCalls := 0, 0
    s.AgentChatCall = func(_ context.Context, _ agentchat.ProviderConfig, _ string, body []byte) (agentchat.ResponsesResult, error) {
        var req map[string]any
        if err := json.Unmarshal(body,&req); err != nil { t.Fatal(err) }
        if tools, offered := req["tools"]; offered {
            if len(tools.([]any)) != 1 { t.Fatal("unexpected live tool definitions") }
            probeCalls++
            return agentchat.ResponsesResult{},fmt.Errorf("model did not accept the function schema")
        }
        answerCalls++
        if !strings.Contains(req["instructions"].(string),agentTextPatchProposalType) {
            t.Fatal("unverified model did not receive the approved text-only fallback protocol")
        }
        return agentchat.ResponsesResult{ResponseID:"text-after-failed-probe",Text:proposal},nil
    }
    w:=invokeAI(t,s.agentChatSessionItemAPI,"POST","/api/agent/sessions/"+id+"/messages",
        map[string]any{"message":"tạo file helloworld.php trên thư mục gốc",
            "confirm_external":true,"expected_revision":1})
    if w.Code!=200 || !strings.Contains(w.Body.String(),"waiting_tool") || probeCalls!=1 || answerCalls!=1 {
        t.Fatalf("unverified tool model failed over to text: probes=%d answers=%d HTTP=%d %s",
            probeCalls,answerCalls,w.Code,w.Body)
    }
    store,err:=s.agentChatStore()
    if err!=nil { t.Fatal(err) }
    saved,err:=store.Get(id,"local")
    if err!=nil { t.Fatal(err) }
    if len(saved.ToolCalls)!=1 || !saved.ToolCalls[0].TextFallback {
        t.Fatal("unverified model did not stage reviewable Patch")
    }
}

func TestAgentTextOnlyCollectionThenPatchNativeApprovalAndFeedback(t *testing.T) {
    s, id, identity := agentTextOnlyFixture(t)
    proposal := agentTextOnlyHelloProposal(t, identity)
    collectionJSON, err := json.Marshal(agentCollectionArguments{
        Project: identity, Actions: []agentCollectionAction{{Type:"ls", Path: ".", MaxEntries: 100}},
    })
    if err != nil { t.Fatal(err) }
    calls := 0
    s.AgentChatCall = func(_ context.Context, _ agentchat.ProviderConfig, _ string, body []byte) (agentchat.ResponsesResult, error) {
        calls++
        var req map[string]any
        if err := json.Unmarshal(body, &req); err != nil { t.Fatal(err) }
        if _, offered := req["tools"]; offered {
            t.Fatal("disabled model unexpectedly received callable function schemas")
        }
        if !strings.Contains(req["instructions"].(string), agentTextPatchProposalType) {
            t.Fatal("text-only model was told to refuse instead of proposing an approved Patch")
        }
        switch calls {
        case 1:
            return agentchat.ResponsesResult{ResponseID: "text-ls", Text: string(collectionJSON)}, nil
        case 2:
            seenCollection := false
            for _, raw := range req["input"].([]any) {
                msg := raw.(map[string]any)
                text, _ := msg["content"].(string)
                if strings.Contains(text, "VERIFIED observation") && strings.Contains(text, "results") {
                    seenCollection = true
                }
            }
            if !seenCollection { t.Fatal("read-only root inspection was not fed back to text model") }
            return agentchat.ResponsesResult{ResponseID: "text-patch", Text: proposal}, nil
        case 3:
            seenFeedback := false
            for _, raw := range req["input"].([]any) {
                msg := raw.(map[string]any)
                if msg["type"] == "function_call" || msg["type"] == "function_call_output" {
                    t.Fatalf("fabricated provider tool call leaked into text-only transcript: %#v", msg)
                }
                text, _ := msg["content"].(string)
                if strings.Contains(text, "TaskDeck native Patch Tool result") && strings.Contains(text, "PASS") {
                    seenFeedback = true
                }
            }
            if !seenFeedback { t.Fatal("verified Patch result never reached text-only model") }
            return agentchat.ResponsesResult{ResponseID:"text-completed", Text:"Patch completed with verified PASS evidence."}, nil
        default:
            t.Fatalf("unexpected provider request number %d", calls)
            return agentchat.ResponsesResult{}, nil
        }
    }
    w := invokeAI(t, s.agentChatSessionItemAPI, "POST", "/api/agent/sessions/"+id+"/messages",
        map[string]any{"message":"tạo file helloworld.php trên thư mục gốc", "confirm_external":true, "expected_revision":1})
    if w.Code != 200 || !strings.Contains(w.Body.String(), "waiting_tool") || calls != 2 {
        t.Fatalf("text-only Collection-to-Patch failed: requests=%d HTTP=%d response=%s",calls,w.Code,w.Body)
    }
    store, err := s.agentChatStore()
    if err != nil { t.Fatal(err) }
    saved, err := store.Get(id,"local")
    if err != nil { t.Fatal(err) }
    if len(saved.ToolCalls) != 1 || !saved.ToolCalls[0].TextFallback || saved.ToolCalls[0].Status != "pending" {
        t.Fatalf("JSON Patch proposal was not staged for explicit approval: %#v",saved.ToolCalls)
    }
    if _, err := os.Stat(filepath.Join(s.Workspace,"helloworld.php")); !os.IsNotExist(err) {
        t.Fatal("Patch changed project before approval")
    }
    callID := saved.ToolCalls[0].CallID
    endpoint := "/api/agent/sessions/"+id+"/tools/"+callID+"/decision"
    noApproval := invokeAI(t,s.agentChatSessionItemAPI,"POST",endpoint,map[string]any{
        "action":"approve","expected_revision":saved.Revision})
    if noApproval.Code != 403 { t.Fatalf("executed without human approval: %d",noApproval.Code) }
    approved := invokeAI(t,s.agentChatSessionItemAPI,"POST",endpoint,map[string]any{
        "action":"approve","confirm_execution":true,"expected_revision":saved.Revision})
    if approved.Code != 200 {
        t.Fatalf("text-only Patch did not reach native queue: %d %s",approved.Code,approved.Body)
    }
    saved, err = store.Get(id,"local")
    if err != nil { t.Fatal(err) }
    if saved.ToolCalls[0].QueuePath == "" || saved.ToolCalls[0].Status != "queued" {
        t.Fatalf("native Patch receipt not recorded: %+v",saved.ToolCalls[0])
    }
    // No live Patch executor in this fixture. Simulate a verified final result
    // so the test can check the provider continuation protocol.
    saved.ToolCalls[0].Status = "done"
    saved.ToolCalls[0].Result = "{\"status\":\"PASS\",\"rc\":0,\"name\":\"approved.zip\"}"
    saved, err = store.Save(saved,saved.Revision)
    if err != nil { t.Fatal(err) }
    follow := invokeAI(t,s.agentChatSessionItemAPI,"POST",
        "/api/agent/sessions/"+id+"/tools/"+callID+"/continue",
        map[string]any{"expected_revision":saved.Revision,"confirm_external":true})
    if follow.Code != 200 || calls != 3 || !strings.Contains(follow.Body.String(),"verified PASS") {
        t.Fatalf("text-only Patch continuation failed: %d %s (calls %d)",follow.Code,follow.Body,calls)
    }
}

func TestAgentVisibleHistoryCompactsHistoricalPatchAndCollectionWithoutMutatingCanonical(t *testing.T) {
 x:=agentchat.Session{}
 x.History=append(x.History,agentchat.HistoryItem{Type:"user",Content:"Please inspect files and complete the original request"})
 patchReceipt:=strings.Repeat("Patch verified PASS rc=0; ",1500)
 patchArgs:=`{"name":"Fix CLI","patch_python":"print('pass')"}`
 x.History=append(x.History,agentchat.HistoryItem{
  Type:"function_call",CallID:"patch-safety",Name:"taskdeck_submit_patch",Arguments:patchArgs,
 },agentchat.HistoryItem{Type:"function_call_output",CallID:"patch-safety",Content:patchReceipt})
 for i:=0;i<36;i++{
  id:=fmt.Sprintf("read-%d",i)
  x.History=append(x.History,
   agentchat.HistoryItem{Type:"function_call",CallID:id,Name:"taskdeck_collect_workspace",Arguments:`{"actions":[{"type":"read","path":"AGENT.md"}]}`},
   agentchat.HistoryItem{Type:"function_call_output",CallID:id,Content:strings.Repeat("UTF8 xin chào, verified text\n",600)})
 }
 raw:=x.History[4].Content
 shown:=agentModelVisibleHistory(x)
 joined:=fmt.Sprint(shown)
 if strings.Contains(joined,patchReceipt)||strings.Contains(joined,patchArgs) {
  t.Fatal("historical Patch source or full receipt was re-sent")
 }
 if !strings.Contains(joined,"Fix CLI")||!strings.Contains(joined,"Prior Native Patch"){
  t.Fatalf("Patch provenance lost after context compaction: %s",joined)
 }
 if !strings.Contains(joined,"AGENT.md")||!strings.Contains(joined,"evidence:"){
  t.Fatal("prior Collection summary omitted source path and evidence")
 }
 if x.History[4].Content!=raw {t.Fatal("canonical audited history mutated")}
 before,after:=0,0
 for _,v:=range x.History{before+=len(v.Content)+len(v.Arguments)}
 for _,v:=range shown{after+=len(v.Content)+len(v.Arguments)}
 if after>=before || after>250<<10 {
  t.Fatalf("large history was not compacted: before=%d after=%d",before,after)
 }
}
