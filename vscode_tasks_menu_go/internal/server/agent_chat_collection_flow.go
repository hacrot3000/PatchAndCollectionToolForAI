package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/patchtool"
)

// Collection has no arbitrary round limit. Native reads remain individually
// bounded; identical request fingerprints alone never prove stale or repeated
// content. Successful observations are durably journaled before continuation.

// Every Collection request is read-only; local formatting/persistence
// failures can be diagnosed safely without claiming ambiguous Patch execution.
type agentCollectionLocalError struct {
	Stage string
	Round int
	Cause error
	TraceID string
}
func (e *agentCollectionLocalError) Error() string { return fmt.Sprintf("Collection round %d %s: %v", e.Round, e.Stage, e.Cause) }
func (e *agentCollectionLocalError) Unwrap() error { return e.Cause }

type agentCollectionProviderError struct {
	Round int
	TraceID string
	Cause error
}
func (e *agentCollectionProviderError) Error() string { return fmt.Sprintf("Collection round %d provider continuation: %v", e.Round, e.Cause) }
func (e *agentCollectionProviderError) Unwrap() error { return e.Cause }


// Requests that need workspace access may establish the harmless tool
// capability probe on first use. Ordinary conversation should not silently
// consume an extra provider request or promote an unverified tool declaration.
func agentNeedsWorkspaceTools(message string) bool {
	text := strings.ToLower(strings.TrimSpace(message))
	for _, hint := range []string{
		"file", "folder", "directory", "project", "workspace", "repository", "repo",
		"source", "code", "php", "patch", "collection", "root",
		"create", "write", "edit", "modify", "change", "inspect", "search", "find",
		"implement", "update", "fix", "debug", "delete", "remove", "list files",
		"tạo", "sửa", "thay đổi", "bổ sung", "kiểm tra", "tra soát", "tìm", "đọc",
		"mã nguồn", "dự án", "thư mục", "xoá", "xóa", "liệt kê", "chỉnh",
	} {
		if strings.Contains(text, hint) {
			return true
		}
	}
	return false
}

// A configured model can establish native tool compatibility on its first
// consented turn. This harmless schema probe carries NO project data and
// preserves the existing per-endpoint/model capability evidence requirement.
func (s *Server) agentChatEnsureTools(ctx context.Context, owner string, provider agentchat.ProviderConfig, model agentchat.ModelConfig, key string) bool {
	if !model.ToolCalling {
		return false
	}
	if s.agentToolCapabilityVerified(owner, provider, model) {
		return true
	}
	// Prevent a broken provider from delaying the first user message indefinitely.
	probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req := httptest.NewRequest(http.MethodPost, "/api/agent/providers/probe", nil).WithContext(probeCtx)
	status, detail := s.agentProbeCapability(req, owner, provider, model, key, agentchat.ProbeToolCalling)
	if status != agentchat.CapabilityVerified {
		return false
	}
	store, err := s.agentChatCapabilities()
	if err != nil {
		return false
	}
	record, found, err := store.Get(owner, provider.ID, model.ID, agentchat.CapabilityFingerprint(provider, model))
	if err != nil {
		return false
	}
	if !found {
		record = agentchat.CapabilityRecord{
			ProviderID: provider.ID, ModelID: model.ID,
			Fingerprint: agentchat.CapabilityFingerprint(provider, model),
			Version: agentchat.CapabilityProbeVersion,
			Capabilities: map[string]string{}, Detail: map[string]string{},
		}
	}
	if record.Capabilities == nil {
		record.Capabilities = map[string]string{}
	}
	if record.Detail == nil {
		record.Detail = map[string]string{}
	}
	record.Capabilities[agentchat.ProbeToolCalling] = agentchat.CapabilityVerified
	record.Detail[agentchat.ProbeToolCalling] = "auto_verified: " + detail
	record.ProbedAt = time.Now().UTC()
	return store.Put(owner, record) == nil
}

// agentChatRunCollection executes only bounded read-only actions. Every result
// is saved before its next provider request, and the subsequent model response
// is never retried after an ambiguous provider failure. Patch proposals are
// handled by the existing appendAgentProviderResult + approval gateway.
func (s *Server) agentChatRunCollection(
	ctx context.Context, store *agentchat.Store, x *agentchat.Session,
	first agentchat.ResponsesResult, provider agentchat.ProviderConfig,
	model agentchat.ModelConfig, key string, identity patchtool.ProjectIdentity,
	toolAvailable bool,
) (out agentchat.ResponsesResult, clear bool, finalErr error) {
	lastTraceID:=""
	defer func(){
		var local *agentCollectionLocalError
		if errors.As(finalErr,&local)&&lastTraceID!=""{local.TraceID=lastTraceID}
	}()
	result := first
	// A text-JSON fallback is a full-history stateless exchange. Never reuse
	// its response ID as a previous_response_id on a later Collection round:
	// the fallback request was not stored by the provider.
	forceStateless := provider.APIType == "chat_completions" ||
		(x.Mode == agentchat.Conversation && !s.agentConversationCapabilityVerified(x.Owner, provider, model))
	// A stateless fallback caused by missing conversation capability may still
	// yield a usable conversation ID that the existing session records for
	// later verified adoption. Only the text-JSON fallback guarantees the
	// response was not stored and requires discarding provider references.
	clearProviderRefs := provider.APIType == "chat_completions"
	recentReads:=newCollectionRequestAdvisories()
	consecutiveRepairs := 0
	for round := 0; ; round++ {
		if err := ctx.Err(); err != nil {
			return result, clearProviderRefs, &agentCollectionLocalError{Stage:"cancelled_before_collection",Round:round+1,Cause:err}
		}
		var nativeRequest []byte
		typed, fallback := false, false
		var validationErr error
		if len(result.Calls) == 1 && result.Calls[0].Name == "taskdeck_collect_workspace" && toolAvailable {
			typed = true
			nativeRequest, validationErr = agentNativeCollectionRequest(result.Calls[0].Arguments,identity,false)
		} else if len(result.Calls) == 0 {
			// Models sometimes emit native COLLECT as standalone assistant JSON
			// even when the tool schema was advertised. Do not silently treat
			// malformed JSON or an unsupported action as a completed task.
			nativeRequest, fallback, validationErr = agentNativeCollectionTextRequest(result.Text,identity)
		}
		if !typed && !fallback {
			return result, clearProviderRefs, nil
		}
		latest,settingsErr:=store.Get(x.ID,x.Owner)
		if settingsErr!=nil||latest.Revision!=x.Revision{
			if settingsErr==nil{settingsErr=agentchat.ErrStaleRevision}
			return result,clearProviderRefs,&agentCollectionLocalError{Stage:"refresh_automation",Round:round+1,Cause:settingsErr}
		}
		x.RefreshAutomation(latest)
		if x.AutoCollection != nil && !*x.AutoCollection {
			// Explicit opt-out: do not access workspace contents, even for a
			// read-only model request. Finish with an actionable message.
			return agentchat.ResponsesResult{ResponseID:result.ResponseID,
				ConversationID:result.ConversationID,
				Text:"Automatic Collection is disabled for this conversation. Enable Auto Collection in the Automation menu and send the request again. No Collection or Patch was executed."},clearProviderRefs,nil
		}

		// Persist the actual model response before any native operation. On
		// restart a pending Collection can be closed as unverified, not replayed.
		if result.Text!=""{x.History=append(x.History,agentchat.HistoryItem{Type:"assistant",Content:result.Text})}
		callID:=""
		if typed{
			call:=result.Calls[0];callID=call.CallID
			x.History=append(x.History,agentchat.HistoryItem{Type:"function_call",CallID:call.CallID,Name:call.Name,Arguments:call.Arguments})
		}
		legacyCollectionRequested(x, callID, toolAvailable)
		if x.ProviderRun!=nil&&x.ProviderRun.Phase=="acknowledged"{
			x.AppendEvent("collect_response_checkpoint",x.Status,callID,x.ProviderRun.ID)
			saved,err:=store.Save(*x,x.Revision)
			if err!=nil{return result,clearProviderRefs,&agentCollectionLocalError{Stage:"persist_acknowledgement",Round:round+1,Cause:err}}
			*x=saved
		}
		observed := ""
		nativeRead := false
		if validationErr != nil {
			kind:=agentCollectionIdentityKind(validationErr)
			reason:=scrubAIContext(validationErr.Error(),256)
			if kind!=""{
				reason="Collection rejected before any file read or execution: "+reason+". project must be an object, not a string. Typed calls must omit project; standalone text must use the exact active AI Pack identity."
			}
			if !typed&&(kind=="foreign"||kind=="missing"){
				// Preserve the standalone-text authorization boundary.
				x.AppendEvent("collect_request_rejected","failed",callID,reason)
				x.History=append(x.History,agentchat.HistoryItem{Type:"user",Content:"TaskDeck rejected the preceding standalone Collection before execution: "+reason})
				return result,clearProviderRefs,&agentCollectionLocalError{Stage:"invalid_project_identity",Round:round+1,Cause:validationErr}
			}
			code:="INVALID_COLLECTION_REQUEST"
			if kind=="foreign"{code="COLLECTION_IDENTITY_REJECTED"}
			observed=runCollectionDiagnostic(code,reason,true)
			x.AppendEvent("collect_request_rejected","requesting_ai",callID,reason)
		} else if s.AgentCollectionCall != nil {
			// Only tests install this mock. Production has exactly one Collection
			// backend: the existing native Python Patch Tool runner/queue.
			observed = s.AgentCollectionCall(s.Workspace,identity,nativeRequest)
			nativeRead = true
			if code,_,_:=agentCollectionRepairError(observed);code=="COLLECT_SCHEMA_INVALID"{nativeRead=false}
		} else {
			// A native large-file COLLECT may run for minutes. Persist its phase
			// BEFORE the local process starts so browser polling shows a truthful
			// "Running native Collection" status instead of a frozen "Thinking".
			x.AppendEvent("collect_started","requesting_ai",callID,"native_patch_tool_collect")
			var saveErr error
			saved,saveErr:=store.Save(*x,x.Revision)
			if saveErr!=nil {
				return result,clearProviderRefs,&agentCollectionLocalError{
					Stage:"persist_native_collect_start",Round:round+1,Cause:saveErr}
			}
			*x=saved
			var nativeErr error
			observed,nativeErr = s.agentRunNativeCollection(ctx,x.Owner,nativeRequest,identity)
			nativeRead = nativeErr == nil
			if nativeErr!=nil {
				code:="NATIVE_COLLECTION_FAILED"
				message:="A native COLLECT did not provide verified evidence. Do not claim inspection succeeded. Inspect the native session; never automatically replay an ambiguous run."
				retryAllowed:=false
				var schemaErr *agentNativeCollectValidationError
				if errors.As(nativeErr,&schemaErr){
					code="COLLECT_SCHEMA_INVALID"
					message="Schema-only preflight rejected the unexecuted request. Repair the ORIGINAL JSON arguments against the bundled COLLECT_ACTION_SCHEMA.json, preserve project identity and valid actions, then submit a distinct valid request. A new ZIP was NOT queued."
					retryAllowed=true
				}
				x.AppendEvent("collect_native_failed","requesting_ai",callID,
					scrubAIContext(nativeErr.Error(),450))
				data,_:=json.Marshal(map[string]any{"project":identity,
					"error":map[string]any{"code":code,
					"message":scrubAIContext(nativeErr.Error(),700),
					"retry_allowed":retryAllowed,
					"action":message}})
				observed=string(data)
			}
		}
		guardDecision := legacyCollectionObserved(x, callID, identity, nativeRequest, observed, nativeRead, toolAvailable)
		requestAdvice := ""
		if validationErr==nil && nativeRead {
			requestAdvice=recentReads.Observe(nativeRequest)
		}
		repairExhausted:=false
		if code,message,repair:=agentCollectionRepairError(observed);repair {
			consecutiveRepairs++
			observed=agentCollectionRepairFeedback(code,message,consecutiveRepairs)
			repairExhausted=consecutiveRepairs>agentCollectionRepairBudget
			x.AppendEvent("collect_schema_repair","requesting_ai",callID,fmt.Sprintf("attempt=%d code=%s",consecutiveRepairs,code))
		}else {consecutiveRepairs=0}
		var next agentchat.HistoryItem
		if typed {
			next = agentchat.HistoryItem{Type: "function_call_output", CallID: callID, Content: observed}
		} else {
			// Stateless fallback for providers that emitted legacy Collection
			// JSON as text, not as a real function call. Never fabricate a
			// function_call ID that the provider did not actually return.
			next = agentchat.HistoryItem{Type: "user", Content:
				agentchat.NativeCollectionTextObservationPrefix + observed}
		}
		if ctx.Err()!=nil {
			x.History=append(x.History,next)
			x.PreviousResponseID="";x.ConversationID=""
			x.AppendEvent("collect_observed",x.Status,next.CallID,"cancelled_before_egress")
			return result,true,&agentCollectionLocalError{Stage:"cancelled_after_native_collection",Round:round+1,Cause:ctx.Err()}
		}
		if repairExhausted {
			x.History=append(x.History,next)
			x.PreviousResponseID="";x.ConversationID=""
			x.AppendEvent("collect_repair_checkpoint","failed",next.CallID,"repair budget exhausted; no new verified evidence")
			return result,clearProviderRefs,&agentCollectionLocalError{Stage:"repair_budget_exhausted",Round:round+1,Cause:errors.New("Collection repair budget exhausted; resume this conversation with a narrower native action")}
		}
		// A verified stall requires the real receipt to be durably committed
		// before considering recovery. Ordinary reads still use the existing
		// single observation+PREPARED CAS, with no extra journal transaction.
		nextCommitted:=false
		recoveryAdvice:=""
		if legacyCollectionStallCandidate(*x,callID){
			x.History=append(x.History,next)
			x.AppendEvent("collect_observed",x.Status,next.CallID,"")
			saved,saveErr:=store.Save(*x,x.Revision)
			if saveErr!=nil{return result,clearProviderRefs,&agentCollectionLocalError{Stage:"persist_stall_receipt",Round:round+1,Cause:saveErr}}
			*x=saved
			nextCommitted=true
			decision,err:=assessLegacyCollectionStall(ctx,x,next)
			if err!=nil{return result,clearProviderRefs,&agentCollectionLocalError{Stage:"persist_stall_recovery",Round:round+1,Cause:err}}
			if string(decision.Kind)=="checkpoint"{
				x.PreviousResponseID="";x.ConversationID=""
				return result,true,&agentCollectionLocalError{Stage:"collection_progress_checkpoint",Round:round+1,Cause:fmt.Errorf("Collection paused at a verified receipt: %s; resume the same conversation with a distinct missing scope",decision.Reason)}
			}
			if string(decision.Kind)=="replan"{
				forceStateless=true
				clearProviderRefs=true
				recoveryAdvice=collectionReplanAdvice
			}
		}
		if fallback {
			forceStateless = true
			clearProviderRefs = true
		}
		currentContract := agentInstalledCollectionContract(s.Workspace)
		mode := x.Mode
		if agentContextNeedsRebase(*x) || agentHistoricalCollectionPackStale(*x,currentContract) || (mode==agentchat.Conversation && !s.agentConversationCapabilityVerified(x.Owner,provider,model)){
			forceStateless=true
			clearProviderRefs=true
		}
		if forceStateless {
			mode = agentchat.Stateless
			// Retain canonical local history, but don't manufacture provider
			// state references for requests sent with store:false.
			x.PreviousResponseID = ""
			x.ConversationID = ""
		} else if mode == agentchat.PreviousResponse {
			x.PreviousResponseID = result.ResponseID
		} else if mode == agentchat.Conversation {
			x.ConversationID = result.ConversationID
		}
		conversationID := x.ConversationID
		reasoning:=agentProviderReasoningEffort(provider,x.ReasoningEffort,model)
		latest,settingsErr=store.Get(x.ID,x.Owner)
		if settingsErr!=nil||latest.Revision!=x.Revision{
			if settingsErr==nil{settingsErr=agentchat.ErrStaleRevision}
			if !nextCommitted{x.History=append(x.History,next)}
			return result,clearProviderRefs,&agentCollectionLocalError{Stage:"refresh_automation",Round:round+1,Cause:settingsErr}
		}
		x.RefreshAutomation(latest)
		if recoveryAdvice!=""&&!legacyCollectionReplanAllowed(*x){
			// The current receipt is already canonical. The replan has not
			// been admitted as PREPARED; leave its original budget intact.
			x.CollectionGuard=latest.CollectionGuard
			x.AppendEvent("clp_checkpoint",x.Status,next.CallID,"recovery_not_authorized")
			return result,true,&agentCollectionLocalError{Stage:"collection_progress_checkpoint",Round:round+1,Cause:errors.New("Automatic Collection replan no longer authorized; resume with a new owner message")}
		}
		limit := agentInitialOutputBudget(model,reasoning,toolAvailable)
		contextModel := model
		if mode==agentchat.Stateless{contextModel,limit=agentAdaptiveRequestBudget(store,*x,provider,model,key,limit)}
		instructions := agentChatInstructionsWithAutomation(*x, toolAvailable, identity.Key) + collectionProgressAdvice(guardDecision) + requestAdvice + recoveryAdvice
		projection:=*x
		if nextCommitted{
			// Next must retain the exact just-saved output once, paired with
			// its actual pending call in the provider-visible projection.
			if len(x.History)==0||x.History[len(x.History)-1]!=next{return result,clearProviderRefs,&agentCollectionLocalError{Stage:"stall_receipt_projection",Round:round+1,Cause:errors.New("committed Collection output missing from latest history")}}
			projection.History=x.History[:len(x.History)-1]
		}
		payload, err := agentBuildContextRequest(projection,contextModel,provider,agentchat.RequestConfig{
			Model: x.ModelID, Instructions: instructions,
			Mode: mode, PreviousResponseID: x.PreviousResponseID, ConversationID: conversationID,
			Next: []agentchat.HistoryItem{next},
			MaxOutputTokens: limit,
			ReasoningEffort: reasoning,
			EnablePatchTool: toolAvailable, EnableCollectionTool: toolAvailable,
			NativeCollectionContract: currentContract,
		})
		if err != nil {
			if !nextCommitted{
				x.History=append(x.History,next)
				x.AppendEvent("collect_observed",x.Status,next.CallID,"local_context_build_failed")
			}
			// No new provider run has been prepared or sent. Keep the previous
			// acknowledged run unchanged and preserve the actual observation.
			x.PreviousResponseID="";x.ConversationID=""
			x.AppendEvent("collect_context_checkpoint","failed",next.CallID,"local_request_not_prepared")
			return result,true,&agentCollectionLocalError{Stage: "collection_context_checkpoint", Round: round+1, Cause: err}
		}
		x.Status = "requesting_ai"
		if !nextCommitted{
			x.History = append(x.History, next)
			x.AppendEvent("collect_observed", "requesting_ai", next.CallID, "")
		}
		if err=prepareAgentProviderRun(store,x,"collection",next.CallID,provider,model,payload);err!=nil{
			return result,clearProviderRefs,&agentCollectionLocalError{Stage:"persist_prepared",Round:round+1,Cause:err}
		}
		saved, err := store.Save(*x, x.Revision)
		if err != nil {
			return result, clearProviderRefs, &agentCollectionLocalError{Stage: "journal_write", Round: round+1, Cause: err}
		}
		*x=saved
		if ctx.Err() != nil {
			return result, clearProviderRefs, &agentCollectionLocalError{Stage: "cancelled_before_provider_request", Round: round+1, Cause: ctx.Err()}
		}
		if err=markAgentProviderSending(store,x);err!=nil{
			return result,clearProviderRefs,&agentCollectionLocalError{Stage:"persist_sending",Round:round+1,Cause:err}
		}
		// A slow inspection round must not exhaust the next provider request.
		// The parent is the cancelable, multi-round turn context; each call
		// receives an independent timeout (and keeps the same cancel handle).
		var callErr error
		var traceID string
		result, callErr, traceID = s.agentChatCallForOwner(ctx, x.Owner, x.ID, provider, key, payload)
		lastTraceID=traceID
		if callErr != nil {
			markAgentProviderOutcome(x,"unknown")
			s.recordAgentDebugEvent(x.Owner, x.ID, next.CallID, "collection", "collection.continuation_failed",
				"COLLECTION_PROVIDER_FAILED", fmt.Sprintf("Collection round %d provider continuation failed; see trace %s", round+1, traceID))
			return result, clearProviderRefs, &agentCollectionProviderError{Round: round+1, TraceID: traceID, Cause: callErr}
		}
		markAgentProviderOutcome(x,"acknowledged")
  recordAgentTokenUsage(x,result)
	}
}


const agentCollectionContextCheckpointMessage = "Collection observation saved in the conversation. The next provider request could not be built locally (mandatory context or provider format); it was NOT prepared or sent. Resume the same conversation with a narrower missing scope or review the selected model limits. Existing native evidence remains available; do not rerun an old Patch."
