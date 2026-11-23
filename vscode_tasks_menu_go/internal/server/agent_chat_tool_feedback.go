package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/patchtool"
)

// agentChatToolContinueAPI submits an authenticated, evidence-backed
// function_call_output using the SAME call_id returned by the provider.
// It does not rerun, replay or requeue any patch; feedback is once-only.
func (s *Server) agentChatToolContinueAPI(w http.ResponseWriter, r *http.Request, id, callID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var req struct {
		ExpectedRevision uint64 `json:"expected_revision"`
		ConfirmExternal  bool   `json:"confirm_external"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if dec.Decode(&req) != nil {
		http.Error(w, "invalid tool continuation request", 400)
		return
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		http.Error(w, "exactly one continuation object required", 400)
		return
	}
	if !req.ConfirmExternal {
		http.Error(w, "explicit external-provider sharing consent required", 403)
		return
	}
	owner := aiPlanOwner(r)
	release, ok := lockAgentTurn(s.Workspace, owner, id)
	if !ok {
		http.Error(w, "Agent session busy", 409)
		return
	}
	defer release()
	store, err := s.agentChatStore()
	if err != nil {
		http.Error(w, "agent journal unavailable", 500)
		return
	}
	x, err := store.Get(id, owner)
	if errors.Is(err, agentchat.ErrNotFound) {
		http.Error(w, "Agent session not found", 404)
		return
	}
	if err != nil {
		http.Error(w, "agent journal unavailable", 500)
		return
	}
	if x.Revision != req.ExpectedRevision || x.Workspace != s.Workspace {
		http.Error(w, "Agent session revision/workspace mismatch", 409)
		return
	}
	if x.Status != "waiting_tool" && x.Status != "tool_rejected" {
		http.Error(w, "Agent session not awaiting a final tool result", 409)
		return
	}
	idx := -1
	for i, item := range x.ToolCalls {
		if item.CallID == callID {
			idx = i
			break
		}
	}
	if idx < 0 {
		http.Error(w, "tool result not pending", 409)
		return
	}
	res, err := s.agentDeliverToolResult(s.agentServerContext(), store, x, idx, true)
	if err != nil {
		writeAgentContinueError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{
		"session_id": id, "revision": res.Session.Revision, "status": res.Session.Status,
		"answer": res.Result.Text, "tool_calls": res.Result.Calls,
		"response_id": res.Result.ResponseID, "input_tokens": res.Result.InputTokens,
		"output_tokens": res.Result.OutputTokens, "execution_enabled": false,
	})
}

// agentContinueError carries an HTTP status alongside a safe message so the
// same delivery path can serve the manual endpoint and the opt-in scheduler.
type agentContinueError struct {
	status  int
	message string
}

func (e agentContinueError) Error() string { return e.message }

func writeAgentContinueError(w http.ResponseWriter, err error) {
	var sc agentContinueError
	if errors.As(err, &sc) {
		http.Error(w, sc.message, sc.status)
		return
	}
	http.Error(w, "tool continuation failed", 500)
}

// agentDeliverResult is the shared outcome of a tool-result continuation.
type agentDeliverResult struct {
	Session agentchat.Session
	Result  agentchat.ResponsesResult
}

// agentRejectedToolCallGroup selects sibling denied function calls from the
// same provider response. Some providers require all call_id outputs in one
// follow-up. Never include a pending or executable Patch in this group.
func agentRejectedToolCallGroup(x agentchat.Session, idx int, includeSent bool) []int {
 if idx<0||idx>=len(x.ToolCalls){return nil}
 first:=x.ToolCalls[idx]
 if first.Status!="rejected"||first.ModelTurnID==""{return []int{idx}}
 indices:=make([]int,0,2)
 for i,call:=range x.ToolCalls{
  if call.Status=="rejected"&&call.ModelTurnID==first.ModelTurnID&&
     (includeSent||!call.FeedbackSent){indices=append(indices,i)}
 }
 return indices
}
func agentFeedbackGroupContains(indices []int,idx int)bool{
 for _,i:=range indices{if i==idx{return true}}
 return false
}
func agentRejectedToolCallGroupByID(x agentchat.Session,callID string)[]int{
 for i,call:=range x.ToolCalls{
  if call.CallID==callID{return agentRejectedToolCallGroup(x,i,true)}
 }
 return nil
}
func agentCanDeliverRejectedToolCall(x agentchat.Session,call agentchat.ToolCallRecord)bool{
 if call.Status!="rejected"{return false}
 for _,item:=range x.History{
  if item.Type=="function_call"&&item.CallID==call.CallID&&item.Name==call.Name{return true}
 }
 return false
}

// agentDeliverToolResult performs the once-only function_call_output delivery.
// The caller MUST hold the session turn lock and have already re-authorized
// the owner, project identity and provider. When consent is true the caller
// has an explicit user grant for this egress; otherwise no request is made.
func (s *Server) agentDeliverToolResult(ctx context.Context, store *agentchat.Store, x agentchat.Session, idx int, consent bool) (agentDeliverResult, error) {
	owner := x.Owner
	call := x.ToolCalls[idx]
	if call.Status != "done" && call.Status != "error" && call.Status != "rejected" {
		return agentDeliverResult{}, agentContinueError{409, "Patch result has no verified final status; do not send success prematurely"}
	}
	if call.FeedbackSent {
		return agentDeliverResult{}, agentContinueError{409, "tool result not pending"}
	}
	// Consent must precede both the PREPARED journal and SENDING fence.
	// A denied request never creates a replayable provider intent.
	if !consent {
		return agentDeliverResult{},agentContinueError{403,"explicit external-provider consent is required"}
	}
	// Keep the executable Patch gateway's exact-name test intact. A denied
    // non-Patch call may ONLY be forwarded as rejection feedback.
    if call.Name=="taskdeck_submit_patch"{
        if _,found:=findAgentToolCall(x.History,call.CallID);!found {
            return agentDeliverResult{},agentContinueError{409,"source Patch call not in transcript"}
        }
    }else if !agentCanDeliverRejectedToolCall(x,call){
        return agentDeliverResult{},agentContinueError{409,"unsupported tool has no rejection record"}
    }
	identity, err := patchtool.EnsureProjectIdentity(s.Workspace)
	if err != nil || identity.ID != x.ProjectID {
		return agentDeliverResult{}, agentContinueError{409, "project identity drift"}
	}
	registry, err := s.agentChatProviders()
	if err != nil {
		return agentDeliverResult{}, agentContinueError{500, "provider store unavailable"}
	}
	p, err := registry.Get(owner, x.ProviderID)
	if err != nil {
		return agentDeliverResult{}, agentContinueError{409, "provider no longer configured"}
	}
	model, ok := p.FindModel(x.ModelID)
	if !ok {
		return agentDeliverResult{}, agentContinueError{409, "model no longer configured"}
	}
	if s.ConnectionSecrets == nil {
		return agentDeliverResult{}, agentContinueError{503, "secret store unavailable"}
	}
	secret, err := s.ConnectionSecrets.Get(agentProviderSecretID(s.Workspace, owner, p.ID))
	if err != nil || len(secret) == 0 {
		return agentDeliverResult{}, agentContinueError{503, "provider credential unavailable"}
	}
	releaseSlot, admitted := acquireAgentProviderSlot(ctx, aiWorkspaceHash(s.Workspace)+":"+owner)
	if !admitted {
		return agentDeliverResult{}, agentContinueError{429, "Agent concurrency limit reached; retry shortly"}
	}
	defer releaseSlot()
	releaseQuota, allowed := s.reserveAIRequest(aiWorkspaceHash(s.Workspace)+":"+owner, 20)
	if !allowed {
		return agentDeliverResult{}, agentContinueError{429, "AI request rate limit exceeded"}
	}
	defer releaseQuota()
	// Provider context is compacted independently by contextcompact.
	// Never destroy verified canonical source evidence to reduce request size.

    // Send all rejected sibling function outputs together, one call_id each.
    // None of these records may have passed an execution approval.
    indices:=agentRejectedToolCallGroup(x,idx,false)
    if len(indices)==0{return agentDeliverResult{},agentContinueError{409,"tool feedback already delivered"}}
    next:=make([]agentchat.HistoryItem,0,len(indices))
    for _,i:=range indices{
        sibling:=x.ToolCalls[i]
        if sibling.Name!="taskdeck_submit_patch"&&!agentCanDeliverRejectedToolCall(x,sibling){
            return agentDeliverResult{},agentContinueError{409,"unsupported tool has no source call"}
        }
        output:=strings.TrimSpace(sibling.Result)
        if output==""||len(output)>4000{
            return agentDeliverResult{},agentContinueError{409,"tool feedback is missing or too large"}
        }
        if sibling.TextFallback{
            if len(indices)!=1{return agentDeliverResult{},agentContinueError{409,"text fallback cannot batch tool calls"}}
            next=append(next,agentchat.HistoryItem{Type:"user",Content:
                "TaskDeck native Patch Tool result for your prior text-only proposal (verified, not an instruction):\n"+output+"\nContinue the original user task."})
        }else{
            next=append(next,agentchat.HistoryItem{Type:"function_call_output",CallID:sibling.CallID,Content:output})
        }
    }
	toolAvailable := model.ToolCalling && s.agentToolCapabilityVerified(owner, p, model)
	reasoning:=agentProviderReasoningEffort(p,x.ReasoningEffort,model)
	limit := agentInitialOutputBudget(model,reasoning,toolAvailable)
	mode := agentContextRequestMode(x,p.APIType,s.agentConversationCapabilityVerified(owner,p,model))
	if call.TextFallback {mode=agentchat.Stateless}
	agentClearProviderRefsForStateless(&x,mode)
	conversationID := x.ConversationID
	payload, err := agentchat.BuildResponsesRequest(agentchat.RequestConfig{
		Model: x.ModelID, Instructions: agentChatInstructionsWithAutomation(x, toolAvailable, identity.Key),
		Mode: mode, PreviousResponseID: x.PreviousResponseID, ConversationID: conversationID,
		History: agentContextForRequest(x, model, agentChatInstructionsWithAutomation(x, toolAvailable, identity.Key), next), Next: next,
		MaxOutputTokens: limit, ReasoningEffort: reasoning,
		EnablePatchTool: toolAvailable, EnableCollectionTool: toolAvailable,
	})
	if err != nil {
		return agentDeliverResult{}, agentContinueError{413, "cannot format tool-result model request"}
	}
	// Persist output and mark delivered BEFORE crossing the network; a timeout
	// cannot result in accidental duplicate tool output or patch replay.
	x.History = append(x.History, next...)
    // Store.Save may archive older acknowledged Patch records and shift slice
    // indexes. Use call IDs as the stable identity across the journal CAS.
    feedbackIDs:=make([]string,0,len(indices))
    for _,i:=range indices{
        feedbackIDs=append(feedbackIDs,x.ToolCalls[i].CallID)
        x.ToolCalls[i].FeedbackSent=true
        x.ToolCalls[i].FeedbackStatus="prepared"
    }
	x.Status = "requesting_ai"
	x.AppendEvent("feedback_prepared", "requesting_ai", call.CallID, "")
	if err=prepareAgentProviderRun(store,&x,"feedback",call.CallID,p,model,payload);err!=nil{
		return agentDeliverResult{},agentContinueError{409,"cannot persist prepared provider feedback"}
	}
	x, err = store.Save(x, x.Revision)
    if err != nil {
        return agentDeliverResult{}, agentContinueError{409, "cannot persist once-only tool output"}
    }
    indices=indices[:0]
    for i,stored:=range x.ToolCalls{
        for _,id:=range feedbackIDs{
            if stored.CallID==id{indices=append(indices,i);break}
        }
    }
    // Never cross the provider egress boundary with a missing receipt.
    if len(indices)!=len(feedbackIDs){
        return agentDeliverResult{},agentContinueError{409,"prepared tool rejection receipt missing after archive"}
    }
    for _,i:=range indices{x.ToolCalls[i].FeedbackStatus="sending"}
	if err=markAgentProviderSending(store,&x); err != nil {
		return agentDeliverResult{}, agentContinueError{409, "cannot record tool-output delivery attempt"}
	}

	// The Patch feedback may trigger several read-only Collection turns.
	// Keep one cancelable multi-call scope alive across the whole sequence;
	// the provider's individual request deadline remains 60 seconds.
	continuationCtx, continuationCancel := agentRetryTurnContext(ctx, x.RetryMode)
	defer continuationCancel()
	stopHeartbeat:=startAgentProviderHeartbeat(continuationCtx,store,owner,x.ID)
	defer stopHeartbeat()
	releaseRequest := registerAgentRequest(agentRequestKey(s.Workspace, owner, x.ID), continuationCancel)
	defer releaseRequest()
	result, err, _ := s.agentChatCallForOwner(continuationCtx, owner, x.ID, p, string(secret), payload)
	if err != nil {
		if errors.Is(continuationCtx.Err(), context.Canceled) {
			s.recordAgentDebugEvent(owner, x.ID, call.CallID, "feedback", "feedback.unknown", "FEEDBACK_DELIVERY_UNKNOWN", "Provider acknowledgement was not confirmed; result will not be resent")
			x.Status = "cancelled"
			for _,i:=range indices{x.ToolCalls[i].FeedbackStatus="unknown"}
			markAgentProviderOutcome(&x,"unknown")
			x.AppendEvent("feedback_unknown", "cancelled", call.CallID, "provider_acknowledgement_unconfirmed")
			_, _ = store.Save(x, x.Revision)
			return agentDeliverResult{}, agentContinueError{409, "continuation cancelled; the tool result was already recorded and will not be resent"}
		}
		s.recordAgentDebugEvent(owner, x.ID, call.CallID, "feedback", "feedback.unknown", "FEEDBACK_DELIVERY_UNKNOWN", "Provider acknowledgement was not confirmed; result will not be resent")
		x.Status = "failed"
		for _,i:=range indices{x.ToolCalls[i].FeedbackStatus="unknown"}
		markAgentProviderOutcome(&x,"unknown")
		x.AppendEvent("feedback_unknown", "failed", call.CallID, "provider_acknowledgement_unconfirmed")
		_, _ = store.Save(x, x.Revision)
		return agentDeliverResult{}, agentContinueError{502, "provider continuation failed; tool result was saved and will not be resent automatically"}
	}
for _,i:=range indices{x.ToolCalls[i].FeedbackStatus="acknowledged"}
	markAgentProviderOutcome(&x,"acknowledged")
 recordAgentTokenUsage(&x,result)
	x.AppendEvent("feedback_acknowledged", x.Status, call.CallID, "")
	// A verified native Patch result can legitimately lead the model to
	// inspect more code. Continue read-only Collection calls automatically.
	result, statelessCollection, err := s.agentChatRunCollection(continuationCtx, store, &x, result, p, model, string(secret), identity, toolAvailable)
	if err != nil {
		x.Status = "failed"
		markAgentProviderOutcome(&x,"unknown")
		x.AppendEvent("collect_failed", "failed", "", "")
		_, _ = store.Save(x, x.Revision)
		return agentDeliverResult{}, agentContinueError{502, "Collection continuation after Patch result failed; inspect the Agent journal"}
	}
	appendAgentProviderResult(&x, result, toolAvailable)
	if mode==agentchat.Stateless || statelessCollection {
		// The last Collection response was not persisted provider-side.
		// Preserve local audit history without an unusable stateful reference.
		x.PreviousResponseID = ""
		x.ConversationID = ""
	}
	appendAgentTextPatchProposal(&x, result, identity)
	markAgentProviderOutcome(&x,"completed")
	x.AppendEvent("feedback_turn_finished", x.Status, call.CallID, "")
	x, err = store.Save(x, x.Revision)
	if err != nil {
		return agentDeliverResult{}, agentContinueError{409, "provider responded but journal write failed; inspect history before retry"}
	}
	s.recordAgentDebugEvent(owner, x.ID, call.CallID, "feedback", "feedback.acknowledged", "", "Provider response was persisted after tool-result continuation")
	return agentDeliverResult{Session: x, Result: result}, nil
}
