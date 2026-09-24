package session

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
)

const (
	patchProtocolName     = "taskdeck.patch"
	patchProtocolVersion  = 1
	maxProtocolEventBytes   = 1 << 20
	maxProtocolCommandBytes = 1 << 20
	maxProtocolItems        = 4096
	maxProtocolArtifacts    = 256
)

type protocolEnvelope struct {
	Protocol string `json:"protocol"`
	Version  int    `json:"version"`
	Type     string `json:"type"`
	Seq      int64  `json:"seq"`
}

type ProtocolItemState struct {
	Index          int      `json:"index"`
	Total          int      `json:"total"`
	Name           string   `json:"name"`
	Kind           string   `json:"kind"`
	Status         string   `json:"status"`
	RC             *int     `json:"rc,omitempty"`
	StartedAt      string   `json:"started_at,omitempty"`
	ElapsedSeconds *float64 `json:"elapsed_seconds,omitempty"`
}

type ProtocolProgressState struct {
	Scope          string  `json:"scope"`
	RunID          string  `json:"run_id,omitempty"`
	Index          int     `json:"index,omitempty"`
	Total          int     `json:"total,omitempty"`
	ItemName       string  `json:"item_name,omitempty"`
	ItemKind       string  `json:"item_kind,omitempty"`
	Phase          string  `json:"phase"`
	Status         string  `json:"status"`
	ElapsedSeconds float64 `json:"elapsed_seconds"`
	OutputLines    int     `json:"output_lines"`
	Detail         string  `json:"detail,omitempty"`
}

type ProtocolActionResultState struct {
	PromptID        string  `json:"prompt_id"`
	ActionID        string  `json:"action_id"`
	Action          string  `json:"action"`
	Index           int     `json:"index"`
	ItemName        string  `json:"item_name"`
	ItemKind        string  `json:"item_kind"`
	Status          string  `json:"status"`
	RC              int     `json:"rc"`
	TimedOut        bool    `json:"timed_out,omitempty"`
	ElapsedSeconds  float64 `json:"elapsed_seconds"`
	Output          string  `json:"output,omitempty"`
	OutputTruncated bool    `json:"output_truncated,omitempty"`
}
type ProtocolQueueMutationResultState struct {
	PromptID   string `json:"prompt_id"`
	MutationID string `json:"mutation_id"`
	Action     string `json:"action"`
	Index      int    `json:"index"`
	ItemName   string `json:"item_name"`
	ItemKind   string `json:"item_kind"`
	Status     string `json:"status"`
	Message    string `json:"message,omitempty"`
	Remaining  int    `json:"remaining"`
}

type ProtocolArtifactState struct {
	ArtifactKind string `json:"artifact_kind"`
	Path         string `json:"path"`
	Primary      bool   `json:"primary,omitempty"`
	ItemName     string `json:"item_name,omitempty"`
	ItemKind     string `json:"item_kind,omitempty"`
	Index        int    `json:"index,omitempty"`
	Total        int    `json:"total,omitempty"`
}

type ProtocolState struct {
	Available       bool            `json:"available"`
	Enabled         bool            `json:"enabled"`
	CommandsEnabled bool            `json:"commands_enabled"`
	EventCount      int             `json:"event_count"`
	LastSeq         int64           `json:"last_seq,omitempty"`
	LastEvent       json.RawMessage `json:"last_event,omitempty"`
	QueueSnapshot   json.RawMessage `json:"queue_snapshot,omitempty"`
	ResumeSnapshot  json.RawMessage `json:"resume_snapshot,omitempty"`
	HistorySnapshot json.RawMessage `json:"history_snapshot,omitempty"`
	HistoryReport   json.RawMessage `json:"history_report,omitempty"`
	Prompt          json.RawMessage     `json:"prompt,omitempty"`
	Items           []ProtocolItemState     `json:"items,omitempty"`
	Artifacts       []ProtocolArtifactState `json:"artifacts,omitempty"`
	Progress        *ProtocolProgressState     `json:"progress,omitempty"`
	ActionResult    *ProtocolActionResultState       `json:"action_result,omitempty"`
	QueueMutation   *ProtocolQueueMutationResultState `json:"queue_mutation_result,omitempty"`
	Error           string                           `json:"error,omitempty"`
}

type ProtocolStateProvider interface {
	ProtocolState(string) (ProtocolState, error)
}

type ProtocolCommandWriter interface {
	ProtocolCommand(string, []byte) error
}

func validateProtocolCommand(data []byte) ([]byte, error) {
	if len(data) == 0 || len(data) > maxProtocolCommandBytes {
		return nil, fmt.Errorf("Patch protocol command size is invalid")
	}
	if bytes.IndexByte(data, '\n') >= 0 || bytes.IndexByte(data, '\r') >= 0 {
		return nil, fmt.Errorf("Patch protocol command must be one JSONL record")
	}
	var envelope struct {
		Protocol string `json:"protocol"`
		Version  int    `json:"version"`
		Type     string `json:"type"`
		Seq      int64  `json:"seq"`
		Command  string `json:"command"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("invalid Patch protocol command JSON: %w", err)
	}
	if envelope.Protocol != patchProtocolName || envelope.Version != patchProtocolVersion || envelope.Type != "command" {
		return nil, fmt.Errorf("unsupported Patch protocol command envelope")
	}
	if envelope.Seq < 1 || strings.TrimSpace(envelope.Command) == "" {
		return nil, fmt.Errorf("invalid Patch protocol command identity")
	}
	var value map[string]json.RawMessage
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, fmt.Errorf("invalid Patch protocol command object: %w", err)
	}
	return json.Marshal(value)
}

func protocolItemEvent(data []byte) (ProtocolItemState, string, error) {
	var event struct {
		Type           string   `json:"type"`
		Index          int      `json:"index"`
		Total          int      `json:"total"`
		Name           string   `json:"name"`
		Kind           string   `json:"kind"`
		Status         string   `json:"status"`
		RC             *int     `json:"rc"`
		StartedAt      string   `json:"started_at"`
		ElapsedSeconds *float64 `json:"elapsed_seconds"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return ProtocolItemState{}, "", fmt.Errorf("invalid Patch item event JSON: %w", err)
	}
	if event.Type != "item_started" && event.Type != "item_finished" {
		return ProtocolItemState{}, "", fmt.Errorf("unsupported Patch item event")
	}
	if event.Index < 1 || event.Total < event.Index || event.Total > maxProtocolItems {
		return ProtocolItemState{}, "", fmt.Errorf("Patch item event index/total is out of bounds")
	}
	if strings.TrimSpace(event.Name) == "" || strings.TrimSpace(event.Kind) == "" {
		return ProtocolItemState{}, "", fmt.Errorf("Patch item event identity is incomplete")
	}
	status := strings.TrimSpace(event.Status)
	if event.Type == "item_started" {
		status = "RUNNING"
	} else if status == "" {
		return ProtocolItemState{}, "", fmt.Errorf("Patch item_finished status is required")
	}
	return ProtocolItemState{
		Index: event.Index,
		Total: event.Total,
		Name: event.Name,
		Kind: event.Kind,
		Status: status,
		RC: event.RC,
		StartedAt: event.StartedAt,
		ElapsedSeconds: event.ElapsedSeconds,
	}, event.Type, nil
}

func upsertProtocolItem(items []ProtocolItemState, item ProtocolItemState) []ProtocolItemState {
	for i := range items {
		if items[i].Index == item.Index {
			items[i] = item
			return items
		}
	}
	if len(items) >= maxProtocolItems {
		return items
	}
	items = append(items, item)
	sort.Slice(items, func(i, j int) bool { return items[i].Index < items[j].Index })
	return items
}

func protocolProgressEvent(data []byte) (ProtocolProgressState, error) {
	var event struct {
		Type           string  `json:"type"`
		Scope          string  `json:"scope"`
		RunID          string  `json:"run_id"`
		Index          int     `json:"index"`
		Total          int     `json:"total"`
		ItemName       string  `json:"item_name"`
		ItemKind       string  `json:"item_kind"`
		Phase          string  `json:"phase"`
		Status         string  `json:"status"`
		ElapsedSeconds float64 `json:"elapsed_seconds"`
		OutputLines    int     `json:"output_lines"`
		Detail         string  `json:"detail"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return ProtocolProgressState{}, fmt.Errorf("invalid Patch progress event JSON: %w", err)
	}
	if event.Type != "progress" {
		return ProtocolProgressState{}, fmt.Errorf("unsupported Patch progress event")
	}
	event.Scope = strings.TrimSpace(event.Scope)
	event.RunID = strings.TrimSpace(event.RunID)
	event.ItemName = strings.TrimSpace(event.ItemName)
	event.ItemKind = strings.TrimSpace(event.ItemKind)
	event.Phase = strings.TrimSpace(event.Phase)
	event.Status = strings.TrimSpace(event.Status)
	event.Detail = strings.TrimSpace(event.Detail)
	if event.Scope == "" || len(event.Scope) > 64 || event.Phase == "" || len(event.Phase) > 128 || event.Status == "" || len(event.Status) > 64 {
		return ProtocolProgressState{}, fmt.Errorf("Patch progress identity is invalid")
	}
	if len(event.RunID) > 256 || len(event.ItemName) > 1024 || len(event.ItemKind) > 128 || len(event.Detail) > 2048 {
		return ProtocolProgressState{}, fmt.Errorf("Patch progress payload is too large")
	}
	if event.Index < 0 || event.Total < 0 || (event.Index == 0) != (event.Total == 0) {
		return ProtocolProgressState{}, fmt.Errorf("Patch progress index/total is invalid")
	}
	if event.Index > 0 && (event.Total < event.Index || event.Total > maxProtocolItems) {
		return ProtocolProgressState{}, fmt.Errorf("Patch progress index/total is out of bounds")
	}
	if event.OutputLines < 0 || event.ElapsedSeconds < 0 || math.IsNaN(event.ElapsedSeconds) || math.IsInf(event.ElapsedSeconds, 0) {
		return ProtocolProgressState{}, fmt.Errorf("Patch progress counters are invalid")
	}
	return ProtocolProgressState{
		Scope: event.Scope,
		RunID: event.RunID,
		Index: event.Index,
		Total: event.Total,
		ItemName: event.ItemName,
		ItemKind: event.ItemKind,
		Phase: event.Phase,
		Status: event.Status,
		ElapsedSeconds: event.ElapsedSeconds,
		OutputLines: event.OutputLines,
		Detail: event.Detail,
	}, nil
}

func protocolActionResultEvent(data []byte) (ProtocolActionResultState, error) {
	var event struct {
		Type            string  `json:"type"`
		PromptID        string  `json:"prompt_id"`
		ActionID        string  `json:"action_id"`
		Action          string  `json:"action"`
		Index           int     `json:"index"`
		ItemName        string  `json:"item_name"`
		ItemKind        string  `json:"item_kind"`
		Status          string  `json:"status"`
		RC              int     `json:"rc"`
		TimedOut        bool    `json:"timed_out"`
		ElapsedSeconds  float64 `json:"elapsed_seconds"`
		Output          string  `json:"output"`
		OutputTruncated bool    `json:"output_truncated"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return ProtocolActionResultState{}, fmt.Errorf("invalid Patch action_result JSON: %w", err)
	}
	if event.Type != "action_result" {
		return ProtocolActionResultState{}, fmt.Errorf("unsupported Patch action result event")
	}
	event.PromptID = strings.TrimSpace(event.PromptID)
	event.ActionID = strings.TrimSpace(event.ActionID)
	event.Action = strings.ToLower(strings.TrimSpace(event.Action))
	event.ItemName = strings.TrimSpace(event.ItemName)
	event.ItemKind = strings.TrimSpace(event.ItemKind)
	event.Status = strings.ToUpper(strings.TrimSpace(event.Status))
	if event.PromptID == "" || len(event.PromptID) > 256 || event.ActionID == "" || len(event.ActionID) > 128 {
		return ProtocolActionResultState{}, fmt.Errorf("Patch action_result identity is invalid")
	}
	if event.Action != "inspect" && event.Action != "preview" && event.Action != "validate" {
		return ProtocolActionResultState{}, fmt.Errorf("Patch action_result action is unsupported")
	}
	if event.Index < 1 || event.Index > maxProtocolItems || event.ItemName == "" || len(event.ItemName) > 1024 || event.ItemKind == "" || len(event.ItemKind) > 128 {
		return ProtocolActionResultState{}, fmt.Errorf("Patch action_result item identity is invalid")
	}
	switch event.Status {
	case "PASS", "FAIL", "TIMEOUT", "UNSUPPORTED":
	default:
		return ProtocolActionResultState{}, fmt.Errorf("Patch action_result status is invalid")
	}
	if event.ElapsedSeconds < 0 || math.IsNaN(event.ElapsedSeconds) || math.IsInf(event.ElapsedSeconds, 0) {
		return ProtocolActionResultState{}, fmt.Errorf("Patch action_result elapsed_seconds is invalid")
	}
	if len([]byte(event.Output)) > 128<<10 || strings.ContainsRune(event.Output, '\x00') {
		return ProtocolActionResultState{}, fmt.Errorf("Patch action_result output is invalid")
	}
	return ProtocolActionResultState{
		PromptID: event.PromptID, ActionID: event.ActionID, Action: event.Action, Index: event.Index,
		ItemName: event.ItemName, ItemKind: event.ItemKind, Status: event.Status, RC: event.RC,
		TimedOut: event.TimedOut, ElapsedSeconds: event.ElapsedSeconds, Output: event.Output, OutputTruncated: event.OutputTruncated,
	}, nil
}
func protocolQueueMutationResultEvent(data []byte) (ProtocolQueueMutationResultState, error) {
	var event struct {
		Type       string `json:"type"`
		PromptID   string `json:"prompt_id"`
		MutationID string `json:"mutation_id"`
		Action     string `json:"action"`
		Index      int    `json:"index"`
		ItemName   string `json:"item_name"`
		ItemKind   string `json:"item_kind"`
		Status     string `json:"status"`
		Message    string `json:"message"`
		Remaining  int    `json:"remaining"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return ProtocolQueueMutationResultState{}, fmt.Errorf("invalid Patch queue_mutation_result JSON: %w", err)
	}
	if event.Type != "queue_mutation_result" {
		return ProtocolQueueMutationResultState{}, fmt.Errorf("unsupported Patch queue mutation event")
	}
	event.PromptID = strings.TrimSpace(event.PromptID)
	event.MutationID = strings.TrimSpace(event.MutationID)
	event.Action = strings.ToLower(strings.TrimSpace(event.Action))
	event.ItemName = strings.TrimSpace(event.ItemName)
	event.ItemKind = strings.TrimSpace(event.ItemKind)
	event.Status = strings.ToUpper(strings.TrimSpace(event.Status))
	event.Message = strings.TrimSpace(event.Message)
	if event.PromptID == "" || len(event.PromptID) > 256 || event.MutationID == "" || len(event.MutationID) > 128 {
		return ProtocolQueueMutationResultState{}, fmt.Errorf("Patch queue mutation identity is invalid")
	}
	if event.Action != "delete" {
		return ProtocolQueueMutationResultState{}, fmt.Errorf("Patch queue mutation action is unsupported")
	}
	if event.Index < 1 || event.Index > maxProtocolItems || event.ItemName == "" || len(event.ItemName) > 1024 || event.ItemKind == "" || len(event.ItemKind) > 128 {
		return ProtocolQueueMutationResultState{}, fmt.Errorf("Patch queue mutation item identity is invalid")
	}
	if event.Status != "PASS" && event.Status != "FAIL" {
		return ProtocolQueueMutationResultState{}, fmt.Errorf("Patch queue mutation status is invalid")
	}
	if event.Remaining < 0 || event.Remaining > maxProtocolItems || len([]byte(event.Message)) > 4096 || strings.ContainsRune(event.Message, '\x00') {
		return ProtocolQueueMutationResultState{}, fmt.Errorf("Patch queue mutation result payload is invalid")
	}
	return ProtocolQueueMutationResultState{
		PromptID: event.PromptID,
		MutationID: event.MutationID,
		Action: event.Action,
		Index: event.Index,
		ItemName: event.ItemName,
		ItemKind: event.ItemKind,
		Status: event.Status,
		Message: event.Message,
		Remaining: event.Remaining,
	}, nil
}

func protocolArtifactEvent(data []byte) (ProtocolArtifactState, error) {
	var event struct {
		Type         string `json:"type"`
		ArtifactKind string `json:"artifact_kind"`
		Path         string `json:"path"`
		Primary      bool   `json:"primary"`
		ItemName     string `json:"item_name"`
		ItemKind     string `json:"item_kind"`
		Index        int    `json:"index"`
		Total        int    `json:"total"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return ProtocolArtifactState{}, fmt.Errorf("invalid Patch artifact event JSON: %w", err)
	}
	if event.Type != "artifact" {
		return ProtocolArtifactState{}, fmt.Errorf("unsupported Patch artifact event")
	}
	kind := strings.TrimSpace(event.ArtifactKind)
	rel := strings.TrimSpace(event.Path)
	if kind == "" || len(kind) > 128 {
		return ProtocolArtifactState{}, fmt.Errorf("Patch artifact kind is invalid")
	}
	if rel == "" || len(rel) > 4096 || !strings.HasPrefix(rel, "artifacts/") || strings.ContainsRune(rel, '\x00') || strings.Contains(rel, "\\") {
		return ProtocolArtifactState{}, fmt.Errorf("Patch artifact path is invalid")
	}
	for _, part := range strings.Split(rel, "/") {
		if part == "" || part == "." || part == ".." {
			return ProtocolArtifactState{}, fmt.Errorf("Patch artifact path is not project-relative")
		}
	}
	if len(event.ItemName) > 1024 || len(event.ItemKind) > 128 {
		return ProtocolArtifactState{}, fmt.Errorf("Patch artifact item identity is too large")
	}
	if event.Index < 0 || event.Total < 0 || (event.Index == 0) != (event.Total == 0) {
		return ProtocolArtifactState{}, fmt.Errorf("Patch artifact index/total is invalid")
	}
	if event.Index > 0 && (event.Total < event.Index || event.Total > maxProtocolItems) {
		return ProtocolArtifactState{}, fmt.Errorf("Patch artifact index/total is out of bounds")
	}
	return ProtocolArtifactState{
		ArtifactKind: kind,
		Path: rel,
		Primary: event.Primary,
		ItemName: strings.TrimSpace(event.ItemName),
		ItemKind: strings.TrimSpace(event.ItemKind),
		Index: event.Index,
		Total: event.Total,
	}, nil
}

func upsertProtocolArtifact(items []ProtocolArtifactState, artifact ProtocolArtifactState) []ProtocolArtifactState {
	for i := range items {
		if items[i].ArtifactKind == artifact.ArtifactKind && items[i].Path == artifact.Path {
			items[i] = artifact
			return items
		}
	}
	if len(items) >= maxProtocolArtifacts {
		return items
	}
	return append(items, artifact)
}

func protocolPromptID(data []byte) (string, error) {
	var event struct {
		Type     string `json:"type"`
		PromptID string `json:"prompt_id"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return "", fmt.Errorf("invalid Patch prompt JSON: %w", err)
	}
	if event.Type != "prompt" || strings.TrimSpace(event.PromptID) == "" {
		return "", fmt.Errorf("invalid Patch prompt identity")
	}
	return event.PromptID, nil
}

func protocolItemActionPromptID(data []byte) (string, bool, error) {
	var command struct {
		Command string `json:"command"`
		Payload struct {
			PromptID string `json:"prompt_id"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(data, &command); err != nil {
		return "", false, fmt.Errorf("invalid Patch protocol command JSON: %w", err)
	}
	if command.Command != "item_action" {
		return "", false, nil
	}
	if strings.TrimSpace(command.Payload.PromptID) == "" {
		return "", true, fmt.Errorf("Patch item_action prompt_id is required")
	}
	return command.Payload.PromptID, true, nil
}
func protocolQueueDeletePromptID(data []byte) (string, bool, error) {
	var command struct {
		Command string `json:"command"`
		Payload struct {
			PromptID string `json:"prompt_id"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(data, &command); err != nil {
		return "", false, fmt.Errorf("invalid Patch protocol command JSON: %w", err)
	}
	if command.Command != "queue_delete" {
		return "", false, nil
	}
	if strings.TrimSpace(command.Payload.PromptID) == "" {
		return "", true, fmt.Errorf("Patch queue_delete prompt_id is required")
	}
	return command.Payload.PromptID, true, nil
}

func protocolHistoryDetailPromptID(data []byte) (string, bool, error) {
	var command struct {
		Command string `json:"command"`
		Payload struct {
			PromptID string `json:"prompt_id"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(data, &command); err != nil {
		return "", false, fmt.Errorf("invalid Patch protocol command JSON: %w", err)
	}
	if command.Command != "history_detail" {
		return "", false, nil
	}
	if strings.TrimSpace(command.Payload.PromptID) == "" {
		return "", true, fmt.Errorf("Patch history_detail prompt_id is required")
	}
	return command.Payload.PromptID, true, nil
}

func protocolResumeActionPromptID(data []byte) (string, bool, error) {
	var command struct {
		Command string `json:"command"`
		Payload struct {
			PromptID string `json:"prompt_id"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(data, &command); err != nil {
		return "", false, fmt.Errorf("invalid Patch protocol command JSON: %w", err)
	}
	if command.Command != "resume_action" {
		return "", false, nil
	}
	if strings.TrimSpace(command.Payload.PromptID) == "" {
		return "", true, fmt.Errorf("Patch resume_action prompt_id is required")
	}
	return command.Payload.PromptID, true, nil
}

func protocolPromptResponseID(data []byte) (string, bool, error) {
	var command struct {
		Command string `json:"command"`
		Payload struct {
			PromptID string `json:"prompt_id"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(data, &command); err != nil {
		return "", false, fmt.Errorf("invalid Patch protocol command JSON: %w", err)
	}
	if command.Command != "prompt_response" {
		return "", false, nil
	}
	if strings.TrimSpace(command.Payload.PromptID) == "" {
		return "", true, fmt.Errorf("Patch prompt_response prompt_id is required")
	}
	return command.Payload.PromptID, true, nil
}

func cloneProtocolState(in ProtocolState) ProtocolState {
	out := in
	out.LastEvent = append(json.RawMessage(nil), in.LastEvent...)
	out.QueueSnapshot = append(json.RawMessage(nil), in.QueueSnapshot...)
	out.ResumeSnapshot = append(json.RawMessage(nil), in.ResumeSnapshot...)
	out.HistorySnapshot = append(json.RawMessage(nil), in.HistorySnapshot...)
	out.HistoryReport = append(json.RawMessage(nil), in.HistoryReport...)
	out.Prompt = append(json.RawMessage(nil), in.Prompt...)
	out.Items = append([]ProtocolItemState(nil), in.Items...)
	out.Artifacts = append([]ProtocolArtifactState(nil), in.Artifacts...)
	if in.Progress != nil {
		progress := *in.Progress
		out.Progress = &progress
	}
	if in.ActionResult != nil {
		actionResult := *in.ActionResult
		out.ActionResult = &actionResult
	}
	if in.QueueMutation != nil {
		queueMutation := *in.QueueMutation
		out.QueueMutation = &queueMutation
	}
	return out
}
