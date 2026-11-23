package server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
)

const (
	agentDebugMaxTraces   = 64
	agentDebugMaxEvents   = 256
	agentDebugMaxBody     = 256 << 10
	agentDebugMaxRawOwner = 2 << 20
	agentDebugTTL         = 30 * time.Minute
)

type agentDebugState struct {
	Enabled  bool      `json:"enabled"`
	RawUntil time.Time `json:"raw_until,omitempty"`
}
type agentDebugEvent struct {
	Seq               int       `json:"seq"`
	TraceID           string    `json:"trace_id"`
	ToolCallID        string    `json:"tool_call_id,omitempty"`
	At                time.Time `json:"at"`
	Level             string    `json:"level"`
	Stage             string    `json:"stage"`
	Event             string    `json:"event"`
	Code              string    `json:"code,omitempty"`
	SafeMessage       string    `json:"safe_message,omitempty"`
	Endpoint          string    `json:"endpoint,omitempty"`
	APIType           string    `json:"api_type,omitempty"`
	ProviderID        string    `json:"provider_id,omitempty"`
	ModelID           string    `json:"model_id,omitempty"`
	HTTPStatus        int       `json:"http_status,omitempty"`
	ContentType       string    `json:"content_type,omitempty"`
	RetryAfter        string    `json:"retry_after,omitempty"`
	ProviderRequestID string    `json:"provider_request_id,omitempty"`
	DurationMS        int64     `json:"duration_ms,omitempty"`
	RequestBytes      int       `json:"request_bytes,omitempty"`
	ResponseBytes     int       `json:"response_bytes,omitempty"`
	RequestSHA256     string    `json:"request_sha256,omitempty"`
	ResponseSHA256    string    `json:"response_sha256,omitempty"`
	Redacted          bool      `json:"redacted"`
	Truncated         bool      `json:"truncated,omitempty"`
	ContextMode       string    `json:"context_mode,omitempty"`
	Reasoning         string    `json:"reasoning,omitempty"`
	HistoryItems      int       `json:"history_items,omitempty"`
	ToolCount         int       `json:"tool_count"`
	Streaming         bool      `json:"streaming"`
	APIKeyPresent     *bool     `json:"api_key_present,omitempty"`
}
type agentDebugPayload struct {
	Request, Response, SSE        []byte
	RequestBytes, ResponseBytes   int
	SSEBytes                      int
	RequestSHA256, ResponseSHA256 string
	Truncated                     bool
}
type agentDebugTrace struct {
	ID            string            `json:"trace_id"`
	EventCount    int               `json:"event_count"`
	EventSeq      int               `json:"event_seq"`
	DroppedEvents int               `json:"dropped_events,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
	Action        string            `json:"action"`
	SessionID     string            `json:"session_id,omitempty"`
	ToolCallID    string            `json:"tool_call_id,omitempty"`
	ProviderID    string            `json:"provider_id,omitempty"`
	ModelID       string            `json:"model_id,omitempty"`
	Events        []agentDebugEvent `json:"events"`
	payload       agentDebugPayload
}
type agentDebugOwner struct {
	State  agentDebugState
	Traces []*agentDebugTrace
}
type agentDebugStore struct {
	mu     sync.Mutex
	owners map[string]*agentDebugOwner
}
type agentDebugRecorder struct {
	store   *agentDebugStore
	owner   string
	traceID string
}

func (s *Server) debugStore() *agentDebugStore {
	s.agentDebugMu.Lock()
	defer s.agentDebugMu.Unlock()
	if s.agentDebug == nil {
		s.agentDebug = &agentDebugStore{owners: make(map[string]*agentDebugOwner)}
	}
	return s.agentDebug
}
func scheduleAgentRawExpiry(store *agentDebugStore, owner string, expiry time.Time) {
	time.AfterFunc(time.Until(expiry), func() {
		store.mu.Lock()
		defer store.mu.Unlock()
		o := store.owner(owner)
		if !o.State.RawUntil.Equal(expiry) {
			return
		}
		o.State.RawUntil = time.Time{}
		for _, trace := range o.Traces {
			trace.payload = agentDebugPayload{}
		}
	})
}
func randomAgentTraceID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(b[:])
}
func (d *agentDebugStore) owner(owner string) *agentDebugOwner {
	o := d.owners[owner]
	if o == nil {
		// Safe metadata capture is always on. Operators must not need to
		// enable it *before* a failure, and it never stores prompt bodies.
		o = &agentDebugOwner{State: agentDebugState{Enabled: true}}
		d.owners[owner] = o
	}
	return o
}
func (s *Server) startAgentDebug(owner, action, sessionID string, p agentchat.ProviderConfig, modelID string) (*agentDebugRecorder, string) {
	store := s.debugStore()
	id := randomAgentTraceID()
	now := time.Now().UTC()
	recorder := &agentDebugRecorder{store: store, owner: owner, traceID: id}
	store.mu.Lock()
	o := store.owner(owner)
	trace := &agentDebugTrace{ID: id, CreatedAt: now, UpdatedAt: now, Action: action, SessionID: sessionID, ProviderID: p.ID, ModelID: modelID, Events: make([]agentDebugEvent, 0, 8)}
	o.Traces = append(o.Traces, trace)
	store.pruneLocked(o, now)
	store.addLocked(o, trace, agentDebugEvent{At: now, Level: "info", Stage: "agent", Event: action + ".started", ProviderID: p.ID, ModelID: modelID, Redacted: true})
	store.mu.Unlock()
	return recorder, id
}
func (s *Server) recordAgentDebugEvent(owner, sessionID, callID, stage, event, code, message string) {
	store := s.debugStore()
	store.mu.Lock()
	defer store.mu.Unlock()
	o := store.owner(owner)
	// Even a failure before provider egress needs a reportable trace.
	// Bind to the most recent session trace when possible, otherwise add a
	// dedicated local trace rather than silently losing the error.
	var trace *agentDebugTrace
	for i := len(o.Traces) - 1; i >= 0; i-- {
		if o.Traces[i].SessionID == sessionID {
			trace = o.Traces[i]
			break
		}
	}
	if trace == nil {
		now := time.Now().UTC()
		trace = &agentDebugTrace{ID: randomAgentTraceID(), CreatedAt: now, UpdatedAt: now, Action: "local_agent_event", SessionID: sessionID}
		o.Traces = append(o.Traces, trace)
		store.pruneLocked(o, now)
	}
	store.addLocked(o, trace, agentDebugEvent{At: time.Now().UTC(), Level: map[bool]string{true: "error", false: "info"}[code != ""], Stage: stage, Event: event, Code: code, SafeMessage: scrubAIContext(agentchat.SafeProviderText(message), 900), ToolCallID: callID, Redacted: true})
}
func (d *agentDebugStore) pruneLocked(o *agentDebugOwner, now time.Time) {
	if !now.Before(o.State.RawUntil) {
		for _, t := range o.Traces {
			t.payload = agentDebugPayload{}
		}
	}
	kept := o.Traces[:0]
	for _, t := range o.Traces {
		if now.Sub(t.UpdatedAt) <= agentDebugTTL {
			kept = append(kept, t)
		}
	}
	o.Traces = kept
	if len(o.Traces) > agentDebugMaxTraces {
		o.Traces = append([]*agentDebugTrace(nil), o.Traces[len(o.Traces)-agentDebugMaxTraces:]...)
	}
}
func (r *agentDebugRecorder) RecordProviderDiagnostic(ev agentchat.DiagnosticEvent) {
	if ev.At.IsZero() {
		ev.At = time.Now().UTC()
	}
	r.store.mu.Lock()
	defer r.store.mu.Unlock()
	o := r.store.owner(r.owner)
	r.store.pruneLocked(o, time.Now())
	var trace *agentDebugTrace
	for _, candidate := range o.Traces {
		if candidate.ID == r.traceID {
			trace = candidate
			break
		}
	}
	if trace == nil {
		return
	}
	event := agentDebugEvent{TraceID: r.traceID, At: ev.At, Level: ev.Level, Stage: ev.Stage, Event: ev.Event, Code: ev.Code, Endpoint: sanitizeAgentEndpoint(ev.Endpoint), APIType: ev.APIType, ProviderID: ev.ProviderID, ModelID: ev.ModelID, HTTPStatus: ev.HTTPStatus, ContentType: safeHeaderValue(ev.ContentType), RetryAfter: safeHeaderValue(ev.RetryAfter), ProviderRequestID: safeHeaderValue(ev.ProviderRequestID), DurationMS: ev.Duration.Milliseconds(), RequestBytes: ev.RequestBytes, ResponseBytes: ev.ResponseBytes, RequestSHA256: ev.RequestSHA256, ResponseSHA256: ev.ResponseSHA256, Redacted: true, Truncated: ev.Truncated, ContextMode: ev.ContextMode, Reasoning: ev.Reasoning, HistoryItems: ev.HistoryItems, ToolCount: ev.ToolCount, Streaming: ev.Streaming}
	if ev.Event == "request.built" {
		present := ev.APIKeyPresent
		event.APIKeyPresent = &present
	}
	if event.RequestSHA256 == "" && len(ev.RequestBody) > 0 {
		event.RequestSHA256 = agentchat.BodyDigest(ev.RequestBody)
	}
	if event.ResponseSHA256 == "" && len(ev.ResponseBody) > 0 {
		event.ResponseSHA256 = agentchat.BodyDigest(ev.ResponseBody)
	}
	if ev.SafeMessage != "" && ev.HTTPStatus == 0 {
		event.SafeMessage = agentchat.SafeProviderText(ev.SafeMessage)
	}
	if ev.HTTPStatus > 0 && ev.Level == "error" {
		event.SafeMessage = "Provider returned an HTTP error; inspect bounded diagnostics"
	}
	if event.RequestBytes == 0 && len(ev.RequestBody) > 0 {
		event.RequestBytes = len(ev.RequestBody)
	}
	if event.ResponseBytes == 0 && len(ev.ResponseBody) > 0 {
		event.ResponseBytes = len(ev.ResponseBody)
	}
	r.store.addLocked(o, trace, event)
	if time.Now().Before(o.State.RawUntil) {
		r.store.captureLocked(o, trace, &trace.payload, ev)
	}
}
func (s *agentDebugStore) addLocked(o *agentDebugOwner, t *agentDebugTrace, e agentDebugEvent) {
	t.EventSeq++
	t.EventCount++
	e.Seq = t.EventSeq
	e.TraceID = t.ID
	e.Redacted = true
	t.Events = append(t.Events, e)
	if len(t.Events) > agentDebugMaxEvents {
		t.Events = append([]agentDebugEvent(nil), t.Events[len(t.Events)-agentDebugMaxEvents:]...)
		t.DroppedEvents++
	}
	t.UpdatedAt = time.Now().UTC()
	_ = o
}
func (s *agentDebugStore) captureLocked(o *agentDebugOwner, t *agentDebugTrace, p *agentDebugPayload, ev agentchat.DiagnosticEvent) {
	used := 0
	for _, tr := range o.Traces {
		used += len(tr.payload.Request) + len(tr.payload.Response) + len(tr.payload.SSE)
	}
	capture := func(src []byte) []byte {
		if len(src) == 0 || used >= agentDebugMaxRawOwner {
			return nil
		}
		n := len(src)
		if n > agentDebugMaxBody {
			n = agentDebugMaxBody
			p.Truncated = true
		}
		if n > agentDebugMaxRawOwner-used {
			n = agentDebugMaxRawOwner - used
			p.Truncated = true
		}
		out := append([]byte(nil), src[:n]...)
		used += n
		if n < len(src) {
			p.Truncated = true
		}
		return out
	}
	if len(ev.RequestBody) > 0 {
		p.Request = capture(ev.RequestBody)
		p.RequestBytes = len(ev.RequestBody)
		p.RequestSHA256 = agentchat.BodyDigest(ev.RequestBody)
	}
	if len(ev.ResponseBody) > 0 {
		p.Response = capture(ev.ResponseBody)
		p.ResponseBytes = len(ev.ResponseBody)
		p.ResponseSHA256 = agentchat.BodyDigest(ev.ResponseBody)
	}
	if len(ev.StreamFrame) > 0 {
		p.SSEBytes += len(ev.StreamFrame)
		p.SSE = append(p.SSE, capture(ev.StreamFrame)...)
	}
	p.Truncated = p.Truncated || ev.Truncated
}
func sanitizeAgentEndpoint(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "[invalid endpoint]"
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}
func safeHeaderValue(v string) string {
	v = strings.TrimSpace(v)
	if len(v) > 128 {
		return v[:128]
	}
	return v
}
func (s *Server) agentChatDebugStateAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	owner := aiPlanOwner(r)
	store := s.debugStore()
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodGet {
		if !s.requireSharedActionPermission(w, r, "settings.read", "agent.debug.read", "agent_debug") {
			return
		}
		store.mu.Lock()
		o := store.owner(owner)
		store.pruneLocked(o, time.Now())
		state := o.State
		rawBytes := 0
		for _, t := range o.Traces {
			rawBytes += len(t.payload.Request) + len(t.payload.Response) + len(t.payload.SSE)
		}
		store.mu.Unlock()
		rawTTL := int(time.Until(state.RawUntil).Seconds())
		if rawTTL < 0 {
			rawTTL = 0
		}
		writeJSON(w, 200, map[string]any{"enabled": state.Enabled, "raw_active": time.Now().Before(state.RawUntil), "raw_until": state.RawUntil, "raw_ttl_seconds": rawTTL, "limits": map[string]int{"events_per_trace": agentDebugMaxEvents, "traces_per_owner": agentDebugMaxTraces, "body_bytes": agentDebugMaxBody, "raw_bytes_per_owner": agentDebugMaxRawOwner}, "raw_bytes": rawBytes})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	if !s.requireSharedActionPermission(w, r, "settings.write", "agent.debug.capture", "agent_debug") {
		return
	}
	var req struct {
		Enabled    bool `json:"enabled"`
		RawConsent bool `json:"raw_consent"`
		DisableRaw bool `json:"disable_raw"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		http.Error(w, "invalid debug state", 400)
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		http.Error(w, "exactly one debug state object required", 400)
		return
	}
	store.mu.Lock()
	o := store.owner(owner)
	// Preserve the legacy POST contract while keeping privacy-safe diagnostic
	// metadata capture permanently enabled. Raw payloads remain opt-in.
	o.State.Enabled = true
	if !req.Enabled || req.DisableRaw {
		o.State.RawUntil = time.Time{}
		for _, t := range o.Traces {
			t.payload = agentDebugPayload{}
		}
	} else if req.RawConsent {
		o.State.RawUntil = time.Now().Add(agentDebugTTL)
	}
	state := o.State
	store.mu.Unlock()
	if req.Enabled && req.RawConsent && !state.RawUntil.IsZero() {
		scheduleAgentRawExpiry(store, owner, state.RawUntil)
	}
	writeJSON(w, 200, map[string]any{"enabled": state.Enabled, "raw_active": time.Now().Before(state.RawUntil), "raw_until": state.RawUntil})
}
func (s *Server) agentChatDebugTracesAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	if !s.requireSharedActionPermission(w, r, "settings.read", "agent.debug.read", "agent_debug") {
		return
	}
	owner := aiPlanOwner(r)
	store := s.debugStore()
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 64 {
		limit = 32
	}
	since := r.URL.Query().Get("since")
	store.mu.Lock()
	o := store.owner(owner)
	store.pruneLocked(o, time.Now())
	traces := make([]*agentDebugTrace, 0, limit)
	start := 0
	if since != "" {
		for i, trace := range o.Traces {
			if trace.ID == since {
				start = i + 1
				break
			}
		}
	}
	for i := len(o.Traces) - 1; i >= start && len(traces) < limit; i-- {
		copyTrace := *o.Traces[i]
		copyTrace.Events = nil
		copyTrace.payload = agentDebugPayload{}
		traces = append(traces, &copyTrace)
	}
	store.mu.Unlock()
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"traces": traces})
}
func (s *Server) agentChatDebugTraceAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	if !s.requireSharedActionPermission(w, r, "settings.read", "agent.debug.read", "agent_debug") {
		return
	}
	owner := aiPlanOwner(r)
	id := strings.TrimPrefix(r.URL.Path, "/api/agent/debug/traces/")
	id = strings.TrimSuffix(id, "/payload")
	if id == "" || strings.Contains(id, "/") {
		http.Error(w, "invalid trace ID", 400)
		return
	}
	store := s.debugStore()
	store.mu.Lock()
	defer store.mu.Unlock()
	o := store.owner(owner)
	store.pruneLocked(o, time.Now())
	for _, t := range o.Traces {
		if t.ID == id {
			if strings.HasSuffix(r.URL.Path, "/payload") {
				s.agentChatDebugPayloadAPI(w, r, o, t)
				return
			}
			copyTrace := *t
			copyTrace.payload = agentDebugPayload{}
			w.Header().Set("Cache-Control", "no-store")
			writeJSON(w, 200, map[string]any{"trace": copyTrace})
			return
		}
	}
	http.Error(w, "trace not found", 404)
}
func (s *Server) agentChatDebugPayloadAPI(w http.ResponseWriter, r *http.Request, o *agentDebugOwner, t *agentDebugTrace) {
	part := r.URL.Query().Get("part")
	view := r.URL.Query().Get("view")
	if view == "" {
		view = "safe"
	}
	payload := []byte(nil)
	total := 0
	digest := ""
	switch part {
	case "request":
		payload, total, digest = t.payload.Request, t.payload.RequestBytes, t.payload.RequestSHA256
	case "response":
		payload, total, digest = t.payload.Response, t.payload.ResponseBytes, t.payload.ResponseSHA256
	case "sse":
		payload, total, digest = t.payload.SSE, t.payload.SSEBytes, agentchat.BodyDigest(t.payload.SSE)
	default:
		http.Error(w, "invalid payload part", 400)
		return
	}
	result := map[string]any{"part": part, "view": view, "total_bytes": total, "captured_bytes": len(payload), "sha256": digest, "sha256_scope": "captured bytes", "truncated": t.payload.Truncated}
	if view == "raw" {
		if !s.requireSharedActionPermission(w, r, "settings.write", "agent.debug.raw.read", "agent_debug_raw") {
			return
		}
		if !time.Now().Before(o.State.RawUntil) {
			http.Error(w, "raw debug grant expired", 403)
			return
		}
		result["raw_base64"] = base64.StdEncoding.EncodeToString(payload)
		result["warning"] = "RAW payload may contain prompts, project files and tool output; credentials/authorization headers are never captured."
	} else if view == "safe" {
		result["redacted"] = true
		result["body"] = "[free-text payload omitted in safe view]"
	} else {
		http.Error(w, "invalid payload view", 400)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, result)
}
func (s *Server) agentChatDebugClearAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	if !s.requireSharedActionPermission(w, r, "settings.write", "agent.debug.clear", "agent_debug") {
		return
	}
	store := s.debugStore()
	owner := aiPlanOwner(r)
	store.mu.Lock()
	o := store.owner(owner)
	o.Traces = nil
	o.State.RawUntil = time.Time{}
	store.mu.Unlock()
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]bool{"cleared": true})
}
func (s *Server) beginAgentDebug(parent context.Context, owner, action, sessionID string, p agentchat.ProviderConfig, model string) (context.Context, *agentDebugRecorder, string) {
	recorder, id := s.startAgentDebug(owner, action, sessionID, p, model)
	if parent == nil {
		parent = context.Background()
	}
	ctx := agentchat.WithDiagnosticSink(parent, recorder)
	recorder.RecordProviderDiagnostic(agentchat.DiagnosticEvent{Level: "info", Stage: "config", Event: "config.validated", ProviderID: p.ID, APIType: p.APIType, SafeMessage: "Provider configuration selected; credential value omitted"})
	return ctx, recorder, id
}
func (r *agentDebugRecorder) finish(code, message string) {
	r.RecordProviderDiagnostic(agentchat.DiagnosticEvent{Level: map[bool]string{true: "error", false: "info"}[code != ""], Stage: "agent", Event: "request.finished", Code: code, SafeMessage: message})
}
func debugErrorJSON(err error, traceID, fallbackCode, fallbackStage, fallbackMessage string) map[string]any {
	var collectionProvider *agentCollectionProviderError
	if errors.As(err, &collectionProvider) && collectionProvider.TraceID != "" {
		// Report the failing continuation call, not the initial provider turn.
		traceID = collectionProvider.TraceID
	}
	var local *agentCollectionLocalError
	if errors.As(err, &local) {
		code := "COLLECTION_CONTEXT_FAILED"
		switch local.Stage {
		case "request_build": code = "COLLECTION_REQUEST_BUILD_FAILED"
		case "journal_write": code = "COLLECTION_JOURNAL_WRITE_FAILED"
		case "persist_native_collect_start": code = "COLLECTION_JOURNAL_WRITE_FAILED"
		case "invalid_project_identity": code = "COLLECTION_IDENTITY_REJECTED"
		case "cancelled_before_provider_request": code = "COLLECTION_CANCELLED"
		}
		return map[string]any{"error": map[string]any{
			"code": code, "stage": local.Stage, "round": local.Round,
			"message": "Read-only Collection continuation failed locally: " + scrubAIContext(local.Cause.Error(), 260),
			"trace_id": traceID, "retryable": false, "ambiguous": false,
		}}
	}
	var pe *agentchat.ProviderError
	if errors.As(err, &pe) {
		if pe.TraceID != "" {
			traceID = pe.TraceID
		}
		return map[string]any{"error": map[string]any{"code": pe.Code, "stage": pe.Stage, "message": pe.Message, "provider_error_type": pe.ProviderErrorType, "provider_error_code": pe.ProviderErrorCode, "provider_message": pe.ProviderMessage, "trace_id": traceID, "http_status": pe.HTTPStatus, "retry_after": pe.RetryAfter, "retryable": pe.Retryable, "ambiguous": pe.Ambiguous, "blocked_shared_ip": pe.BlockedSharedIP}}
	}
	code, stage, message := fallbackCode, fallbackStage, fallbackMessage
	if errors.Is(err, context.Canceled) {
		code = "REQUEST_CANCELLED"
		stage = "provider_http"
		message = "Provider request was cancelled"
	} else if errors.Is(err, context.DeadlineExceeded) {
		code = "REQUEST_TIMEOUT"
		stage = "provider_http"
		message = "Provider request timed out"
	}
	return map[string]any{"error": map[string]any{"code": code, "stage": stage, "message": message, "trace_id": traceID, "retryable": false, "ambiguous": true}}
}
func writeAgentStructuredError(w http.ResponseWriter, status int, err error, traceID, code, stage, message string) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, status, debugErrorJSON(err, traceID, code, stage, message))
}
