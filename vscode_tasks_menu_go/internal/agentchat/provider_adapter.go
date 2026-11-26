package agentchat

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
)

var (
	ErrProviderBindingMismatch   = errors.New("resolved provider does not match immutable run binding")
	ErrProviderKeyUnavailable    = errors.New("provider credential is unavailable")
	ErrProviderCredentialChanged = errors.New("provider credential changed since this run was created")
)

// ResolvedProvider is an in-memory view returned by the existing owner-scoped
// configuration and secret stores. Credential must never be persisted or logged.
type ResolvedProvider struct {
	Config                ProviderConfig
	ConfigVersion         string
	Credential            string
	CredentialFingerprint string
	StreamingVerified     bool
}

type ProviderResolver interface {
	Resolve(context.Context, RunSpec) (ResolvedProvider, error)
}

type ProviderPayloadBuilder interface {
	Build(RunSpec, ProviderConfig, ModelConfig, []HistoryItem) ([]byte, error)
}

type ProviderInvoker interface {
	Invoke(context.Context, ProviderConfig, string, []byte) (ResponsesResult, error)
}

type StreamingProviderInvoker interface {
	InvokeStream(context.Context, ProviderConfig, string, []byte, func(StreamEvent) error) (ResponsesResult, error)
}

// ConfiguredProviderAdapter adapts existing API-specific transports to the
// run-scoped contract. OwnerWorkspaceID selects config/credentials; execution
// workspace remains part of the run context and is passed to payload building.
type ConfiguredProviderAdapter struct {
	ProviderID     string
	Resolver       ProviderResolver
	Builder        ProviderPayloadBuilder
	Invoker        ProviderInvoker
	RequestTimeout time.Duration
	RetryDelay     time.Duration
	Wait           func(context.Context, time.Duration) error
}

// toolCallAccumulator collects streamed function-call fragments and publishes a
// canonical tool-call event only when the name and arguments are complete and
// the arguments are valid JSON. Partial or ambiguous fragments never reach the
// run journal.
type toolCallAccumulator struct {
	calls   map[string]*accumulatedToolCall
	order   []string
	emitted map[string]bool
}

type accumulatedToolCall struct {
	callID    string
	name      string
	arguments strings.Builder
}

func newToolCallAccumulator() *toolCallAccumulator {
	return &toolCallAccumulator{calls: make(map[string]*accumulatedToolCall), emitted: make(map[string]bool)}
}

func (a *toolCallAccumulator) add(event StreamEvent) (FunctionCall, bool) {
	if event.CallID == "" {
		return FunctionCall{}, false
	}
	call, exists := a.calls[event.CallID]
	if !exists {
		call = &accumulatedToolCall{callID: event.CallID}
		a.calls[event.CallID] = call
		a.order = append(a.order, event.CallID)
	}
	if event.CallName != "" {
		call.name = event.CallName
	}
	switch event.Type {
	case "response.output_item.done":
		// A completed item carries the full argument text; replace the
		// accumulated fragments instead of appending them twice.
		if event.Arguments != "" {
			call.arguments.Reset()
			call.arguments.WriteString(event.Arguments)
		}
	default:
		if event.Arguments != "" {
			call.arguments.WriteString(event.Arguments)
		}
	}
	if a.emitted[event.CallID] || call.name == "" || call.arguments.Len() == 0 {
		return FunctionCall{}, false
	}
	arguments := call.arguments.String()
	if !json.Valid([]byte(arguments)) {
		return FunctionCall{}, false
	}
	a.emitted[event.CallID] = true
	return FunctionCall{CallID: call.callID, Name: call.name, Arguments: arguments}, true
}

// complete publishes any fully accumulated calls that were not emitted while
// streaming, for example after a non-streaming response.
func (a *toolCallAccumulator) complete() []FunctionCall {
	out := make([]FunctionCall, 0, len(a.order))
	for _, callID := range a.order {
		if a.emitted[callID] {
			continue
		}
		call := a.calls[callID]
		if call.name == "" || call.arguments.Len() == 0 || !json.Valid([]byte(call.arguments.String())) {
			continue
		}
		a.emitted[callID] = true
		out = append(out, FunctionCall{CallID: call.callID, Name: call.name, Arguments: call.arguments.String()})
	}
	return out
}

func (a ConfiguredProviderAdapter) Execute(ctx context.Context, spec RunSpec, history []HistoryItem, sink EventSink) error {
	if a.ProviderID == "" || a.Resolver == nil || a.Builder == nil || a.Invoker == nil {
		return errors.New("provider adapter is not fully configured")
	}
	if spec.ProviderID != a.ProviderID {
		return ErrProviderBindingMismatch
	}
	resolved, err := a.Resolver.Resolve(ctx, spec)
	if err != nil {
		return err
	}
	if resolved.Config.ID != spec.ProviderID || resolved.ConfigVersion != spec.ProviderConfigVersion {
		return ErrProviderBindingMismatch
	}
	if resolved.Credential == "" {
		return ErrProviderKeyUnavailable
	}
	if spec.CredentialFingerprint != "" && resolved.CredentialFingerprint != spec.CredentialFingerprint {
		return ErrProviderCredentialChanged
	}
	model, ok := resolved.Config.FindModel(spec.ModelID)
	if !ok {
		return ErrProviderModelMissing
	}
	payload, err := a.Builder.Build(spec, resolved.Config, model, append([]HistoryItem(nil), history...))
	if err != nil {
		return err
	}
	result, streamedText, publishedCalls, err := a.invokeWithRetry(ctx, spec, resolved, payload, sink)
	if err != nil {
		err = safeRunProviderError(err)
		if sink != nil && ctx.Err() == nil {
			if eventPayload, payloadErr := runProviderErrorPayload(err); payloadErr == nil {
				if publishErr := publishRunEvent(ctx, sink, RunEventError, eventPayload); publishErr != nil {
					return errors.Join(err, publishErr)
				}
			}
		}
		return err
	}
	if sink == nil {
		return nil
	}
	if result.Text != "" && !streamedText {
		if err := publishRunEvent(ctx, sink, RunEventTextDelta, []byte(result.Text)); err != nil {
			return err
		}
	}
	for _, call := range result.Calls {
		if publishedCalls[call.CallID] {
			continue
		}
		encoded, err := json.Marshal(call)
		if err != nil {
			return err
		}
		if err := publishRunEvent(ctx, sink, RunEventToolCall, encoded); err != nil {
			return err
		}
	}
	usage, err := json.Marshal(struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	}{result.InputTokens, result.OutputTokens})
	if err != nil {
		return err
	}
	return publishRunEvent(ctx, sink, RunEventUsage, usage)
}

func (a ConfiguredProviderAdapter) invokeWithRetry(ctx context.Context, spec RunSpec, resolved ResolvedProvider, payload []byte, sink EventSink) (ResponsesResult, bool, map[string]bool, error) {
	attempt := 1
	// Transport retries keep the exact canonical bytes. Only a structured
	// context rejection or explicit output exhaustion may adapt the request.
	contextRecoveries:=0
	incompleteRecoverySent:=false
	stagnant := 0
	lastFailedDigest:=""
	for {
		// Stop or daemon shutdown must win even if an injected wait or
		// provider transport returns a stale success after cancellation.
		if err := ctx.Err(); err != nil {
			return ResponsesResult{}, false, nil, err
		}
		measurement, _ := MeasureProviderPayload(resolved.Config.APIType, payload)
		digest := BodyDigest(payload)
		requestTimeout := a.RequestTimeout
		if requestTimeout <= 0 {
			requestTimeout = 600 * time.Second
		}
		callCtx, cancel := context.WithTimeout(ctx, requestTimeout)
		streamedText := false
		accumulator := newToolCallAccumulator()
		streamedCalls := make(map[string]FunctionCall)
		streamedCallIDs := make([]string, 0, 1)
		publishedCalls := make(map[string]bool)
		var onStreamEvent func(StreamEvent) error
		if sink != nil {
			onStreamEvent = func(event StreamEvent) error {
				switch event.Type {
				case "assistant.delta", "response.output_text.delta":
					if event.TextDelta == "" {
						return nil
					}
					streamedText = true
					return publishRunEvent(callCtx, sink, RunEventTextDelta, []byte(event.TextDelta))
				case "tool.call.delta", "response.function_call_arguments.delta",
					"response.output_item.added", "response.output_item.done":
					call, complete := accumulator.add(event)
					delta, marshalErr := json.Marshal(struct {
						CallID    string `json:"call_id"`
						Name      string `json:"name,omitempty"`
						Arguments string `json:"arguments,omitempty"`
					}{event.CallID, event.CallName, event.Arguments})
					if marshalErr != nil {
						return marshalErr
					}
					if publishErr := publishRunEvent(callCtx, sink, RunEventToolCallDelta, delta); publishErr != nil {
						return publishErr
					}
					if complete {
						if _, exists := streamedCalls[call.CallID]; !exists {
							streamedCallIDs = append(streamedCallIDs, call.CallID)
						}
						streamedCalls[call.CallID] = call
					}
					return nil
				default:
					return nil
				}
			}
		}
		var result ResponsesResult
		var err error
		if streaming, ok := a.Invoker.(StreamingProviderInvoker); ok && resolved.Config.Stream && resolved.StreamingVerified {
			result, err = streaming.InvokeStream(callCtx, resolved.Config, resolved.Credential, payload, onStreamEvent)
		} else {
			result, err = a.Invoker.Invoke(callCtx, resolved.Config, resolved.Credential, payload)
		}
		cancel()
		if stopErr := ctx.Err(); stopErr != nil {
			return ResponsesResult{}, streamedText, publishedCalls, stopErr
		}
		if err == nil && len(streamedCalls) > 0 {
			seen := make(map[string]bool, len(result.Calls))
			for _, call := range result.Calls {
				seen[call.CallID] = true
			}
			for _, callID := range streamedCallIDs {
				if !seen[callID] {
					result.Calls = append(result.Calls, streamedCalls[callID])
				}
			}
		}
		if err == nil || errors.Is(err,context.Canceled) || (spec.RetryMode!=RetryLimited && spec.RetryMode!=RetryUnlimited) || (spec.RetryMode==RetryLimited && attempt>=6) {
			return result, streamedText, publishedCalls, err
		}
		outcome := ClassifyProviderAttemptOutcome(ctx.Err(), err)
		if outcome.StagnantTransport() {
			if lastFailedDigest==digest{stagnant++}else{stagnant=1;lastFailedDigest=digest}
		} else {
			stagnant = 0
			lastFailedDigest=""
		}
		recovery:=""
		nextPayload:=payload
		if ContextLimitRejected(err){
			if contextRecoveries>=MaxContextRecoveries{return result,streamedText,publishedCalls,err}
			reduced,ok:=RetryReducedOutputReservation(payload)
			if !ok{return result,streamedText,publishedCalls,err}
			if _,e:=MeasureProviderPayload(resolved.Config.APIType,reduced);e!=nil{return result,streamedText,publishedCalls,err}
			nextPayload=reduced
			contextRecoveries++
			recovery="context_output_reservation"
		}else{
			var pe *ProviderError
			if errors.As(err,&pe)&&pe.Code=="RESPONSE_INCOMPLETE"&&pe.ProviderErrorCode=="max_output_tokens"{
				if incompleteRecoverySent||!runProviderRetryAllowed(spec.RetryMode,attempt,err){return result,streamedText,publishedCalls,err}
				constrained,_,ok:=RetryConstrainedOutputPayload(payload,resolved.Config)
				if !ok{return result,streamedText,publishedCalls,err}
				m,e:=MeasureProviderPayload(resolved.Config.APIType,constrained)
				model,_:=resolved.Config.FindModel(spec.ModelID)
				if e!=nil||(model.MaxInputTokens>0&&m.WireBytes>model.MaxInputTokens){return result,streamedText,publishedCalls,err}
				nextPayload=constrained
				incompleteRecoverySent=true
				recovery="bounded_output"
			}else if !runProviderRetryAllowed(spec.RetryMode,attempt,err){
				return result,streamedText,publishedCalls,err
			}
		}
		generation:=contextRecoveries
		if incompleteRecoverySent{generation++}
		if sink != nil {
			// Existing retry event only: no extra journal events, no prompt,
			// credential or response contents, and no claim of delivery ACK.
			retryEvent, marshalErr := json.Marshal(struct {
				Attempt      int    `json:"attempt"`
				NextAttempt  int    `json:"next_attempt"`
				ErrorCode    string `json:"error_code"`
				ResetPartial bool   `json:"reset_partial_output"`
				Outcome ProviderAttemptOutcome `json:"outcome"`
				RetryMode RetryMode `json:"retry_mode"`
				PayloadSHA256 string `json:"payload_sha256"`
				CanonicalBytes int `json:"canonical_bytes,omitempty"`
				WireTemplateBytes int `json:"wire_template_bytes,omitempty"`
				EstimatedInputTokens int `json:"estimated_input_tokens,omitempty"`
				OutputBudget int `json:"output_budget,omitempty"`
				SameDigestFailures int `json:"same_digest_failures"`
				Recovery string `json:"recovery,omitempty"`
				RecoveryGeneration int `json:"recovery_generation,omitempty"`
				NextPayloadSHA256 string `json:"next_payload_sha256,omitempty"`
			}{attempt, attempt + 1, runProviderRetryCode(err), true, outcome,
				spec.RetryMode, digest, measurement.CanonicalBytes, measurement.WireBytes,
				measurement.EstimatedInputTokens, measurement.OutputBudget, stagnant,recovery,generation,BodyDigest(nextPayload)})
			if marshalErr != nil {
				return ResponsesResult{}, streamedText, publishedCalls, marshalErr
			}
			if publishErr := publishRunEvent(ctx, sink, RunEventRetry, retryEvent); publishErr != nil {
				return ResponsesResult{}, streamedText, publishedCalls, publishErr
			}
		}
		delay := a.RetryDelay
		if delay <= 0 {
			delay = 10 * time.Second
		}
		if a.Wait != nil {
			err = a.Wait(ctx, delay)
		} else {
			err = waitRunRetry(ctx, delay)
		}
		if err != nil {
			return ResponsesResult{}, streamedText, publishedCalls, err
		}
		payload=nextPayload
		attempt++
	}
}

func runProviderRetryAllowed(mode RetryMode, attempt int, err error) bool {
	if mode == RetryOff || err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	var providerErr *ProviderError
	retryable := false
	if errors.As(err, &providerErr) {
		retryable = providerErr.Retryable || providerErr.HTTPStatus == 408 || providerErr.HTTPStatus == 429 || providerErr.HTTPStatus == 499 || providerErr.HTTPStatus >= 500 || providerErr.Ambiguous && providerErr.HTTPStatus == 0
	} else {
		retryable = errors.Is(err, context.DeadlineExceeded) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
	}
	if !retryable {
		return false
	}
	if mode == RetryUnlimited {
		return true
	}
	return mode == RetryLimited && attempt <= 5
}

func runProviderRetryCode(err error) string {
	var providerErr *ProviderError
	if errors.As(err, &providerErr) && providerErr.Code != "" {
		return SafeProviderText(providerErr.Code)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "PROVIDER_TIMEOUT"
	}
	return "TRANSIENT_PROVIDER_FAILURE"
}

func safeRunProviderError(err error) error {
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	var providerErr *ProviderError
	if errors.As(err, &providerErr) {
		return &ProviderError{
			Code: SafeProviderText(providerErr.Code), Stage: SafeProviderText(providerErr.Stage),
			Message: SafeProviderText(providerErr.Message), ProviderErrorType: SafeProviderText(providerErr.ProviderErrorType),
			ProviderErrorCode: SafeProviderText(providerErr.ProviderErrorCode), ProviderMessage: SafeProviderText(providerErr.ProviderMessage),
			HTTPStatus: providerErr.HTTPStatus, RetryAfter: SafeProviderText(providerErr.RetryAfter),
			Retryable: providerErr.Retryable, Ambiguous: providerErr.Ambiguous, BlockedSharedIP: SafeProviderText(providerErr.BlockedSharedIP),
		}
	}
	return errors.New(SafeProviderText(err.Error()))
}

func runProviderErrorPayload(err error) ([]byte, error) {
	payload := struct {
		Code       string `json:"code"`
		Stage      string `json:"stage,omitempty"`
		Message    string `json:"message"`
		HTTPStatus int    `json:"http_status,omitempty"`
		Retryable  bool   `json:"retryable"`
		Ambiguous  bool   `json:"ambiguous"`
	}{Code: runProviderRetryCode(err), Message: SafeProviderText(err.Error())}
	if errors.Is(err, context.DeadlineExceeded) {
		payload.Stage = "provider_http"
	}
	var providerErr *ProviderError
	if errors.As(err, &providerErr) {
		payload.Code = SafeProviderText(providerErr.Code)
		payload.Stage = SafeProviderText(providerErr.Stage)
		payload.Message = SafeProviderText(providerErr.Message)
		payload.HTTPStatus = providerErr.HTTPStatus
		payload.Retryable = providerErr.Retryable
		payload.Ambiguous = providerErr.Ambiguous
	}
	return json.Marshal(payload)
}

func waitRunRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// HTTPProviderInvoker reuses the hardened Responses and Chat Completions
// transports already used by Agent Chat.
type HTTPProviderInvoker struct{}

func (HTTPProviderInvoker) Invoke(ctx context.Context, provider ProviderConfig, credential string, payload []byte) (ResponsesResult, error) {
	if provider.APIType == "chat_completions" {
		return DoChatCompletions(ctx, provider, credential, payload)
	}
	return DoResponses(ctx, provider, credential, payload)
}

func (HTTPProviderInvoker) InvokeStream(ctx context.Context, provider ProviderConfig, credential string, payload []byte, onEvent func(StreamEvent) error) (ResponsesResult, error) {
	if provider.APIType == "chat_completions" {
		return DoChatCompletionsStreamWithEvents(ctx, provider, credential, payload, onEvent)
	}
	return DoResponsesStreamWithEvents(ctx, provider, credential, payload, onEvent)
}

func (a ConfiguredProviderAdapter) Cancel(context.Context, RunSpec) error {
	// The owning run manager cancels the request context. Current configured
	// transports honor that context; remote cancellation is provider-dependent.
	return nil
}

func publishRunEvent(ctx context.Context, sink EventSink, eventType RunEventType, payload []byte) error {
	return sink.Publish(ctx, RunEvent{Type: eventType, Payload: payload})
}
