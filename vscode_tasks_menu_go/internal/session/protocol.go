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
	maxProtocolPlanConflicts = 1024
	maxProtocolPlanOverlap   = 64
	maxProtocolPlanDepends   = 128
	maxProtocolPlanWarnings  = 256
	maxProtocolHealthChecks   = 512
	maxProtocolHealthMessages = 256
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
	DiagnosisKind  string   `json:"diagnosis_kind,omitempty"`
	FailureReason  string   `json:"failure_reason,omitempty"`
	OutputTail     string   `json:"output_tail,omitempty"`
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
type ProtocolHistoryManagementArtifact struct {
	Label          string `json:"label"`
	Path           string `json:"path"`
	UploadRequired bool   `json:"upload_required,omitempty"`
}

type ProtocolHistoryManagementResultState struct {
	PromptID       string                             `json:"prompt_id"`
	ManagementID   string                             `json:"management_id"`
	Action         string                             `json:"action"`
	RunID          string                             `json:"run_id"`
	Status         string                             `json:"status"`
	RC             int                                `json:"rc"`
	HistoryChanged bool                               `json:"history_changed,omitempty"`
	Message        string                             `json:"message,omitempty"`
	Artifact       *ProtocolHistoryManagementArtifact `json:"artifact,omitempty"`
}

type ProtocolHistorySupportArtifact struct {
	Label          string `json:"label"`
	Path           string `json:"path"`
	UploadRequired bool   `json:"upload_required,omitempty"`
}

type ProtocolHistorySupportResultState struct {
	PromptID  string                          `json:"prompt_id"`
	SupportID string                          `json:"support_id"`
	RunID     string                          `json:"run_id"`
	ItemIndex int                             `json:"item_index"`
	ItemName  string                          `json:"item_name"`
	Status    string                          `json:"status"`
	Message   string                          `json:"message,omitempty"`
	Artifact  *ProtocolHistorySupportArtifact `json:"artifact,omitempty"`
}

type ProtocolHistoryCleanupResultState struct {
	PromptID       string `json:"prompt_id"`
	CleanupID      string `json:"cleanup_id"`
	Status         string `json:"status"`
	RC             int    `json:"rc"`
	HistoryChanged bool   `json:"history_changed,omitempty"`
	Removed        int    `json:"removed"`
	Pinned         int    `json:"pinned"`
	Remaining      int    `json:"remaining"`
	Policy         string `json:"policy"`
	EligibleBefore int    `json:"eligible_before"`
	Message        string `json:"message,omitempty"`
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

type ProtocolPlanItemState struct {
	Index          int      `json:"index"`
	Name           string   `json:"name"`
	PatchID        string   `json:"patch_id"`
	PackageSHA256  string   `json:"package_sha256,omitempty"`
	TargetCount    int      `json:"target_count"`
	DependsOn      []string `json:"depends_on,omitempty"`
	IDReuseCount   int      `json:"id_reuse_count"`
}

type ProtocolPlanPreviousActionState struct {
	Action string `json:"action"`
	Reason string `json:"reason,omitempty"`
}

type ProtocolPlanConflictState struct {
	Left              string   `json:"left"`
	LeftPatchID       string   `json:"left_patch_id,omitempty"`
	Right             string   `json:"right"`
	RightPatchID      string   `json:"right_patch_id,omitempty"`
	Relation          string   `json:"relation"`
	DependencyOrdered bool     `json:"dependency_ordered"`
	Overlap           []string `json:"overlap,omitempty"`
}

type ProtocolPlanResourceState struct {
	Status                   string `json:"status"`
	ActualProjectFreeBytes   *int64 `json:"actual_project_free_bytes,omitempty"`
	RequiredProjectFreeBytes *int64 `json:"required_project_free_bytes,omitempty"`
	ActualTempFreeBytes      *int64 `json:"actual_temp_free_bytes,omitempty"`
	RequiredTempFreeBytes    *int64 `json:"required_temp_free_bytes,omitempty"`
}

type ProtocolPlanPreviewState struct {
	Name          string `json:"name"`
	Status        string `json:"status"`
	RC            int    `json:"rc"`
	Stage         string `json:"stage"`
	DiagnosisKind string `json:"diagnosis_kind,omitempty"`
	Message       string `json:"message,omitempty"`
	TargetCount   int    `json:"target_count"`
}

type ProtocolPlanErrorState struct {
	Kind    string `json:"kind"`
	Message string `json:"message,omitempty"`
}

type ProtocolPlanSnapshotState struct {
	Status                string                           `json:"status"`
	FailurePolicy         string                           `json:"failure_policy"`
	TransactionPolicy     string                           `json:"transaction_policy"`
	Items                 []ProtocolPlanItemState          `json:"items,omitempty"`
	PreviousFailureAction *ProtocolPlanPreviousActionState `json:"previous_failure_action,omitempty"`
	StaticConflicts       []ProtocolPlanConflictState      `json:"static_conflicts,omitempty"`
	Resources             *ProtocolPlanResourceState       `json:"resources,omitempty"`
	Previews              []ProtocolPlanPreviewState       `json:"previews,omitempty"`
	Warnings              []string                         `json:"warnings,omitempty"`
	Error                 *ProtocolPlanErrorState          `json:"error,omitempty"`
}

type ProtocolHealthCheckState struct {
	Name           string `json:"name"`
	Status         string `json:"status"`
	Detail         string `json:"detail,omitempty"`
	Entries        *int   `json:"entries,omitempty"`
	Failures       *int   `json:"failures,omitempty"`
	MissingManaged *int   `json:"missing_managed,omitempty"`
	StaleManaged   *int   `json:"stale_managed,omitempty"`
	Files          *int   `json:"files,omitempty"`
	Dirs           *int   `json:"dirs,omitempty"`
	Actual         *int   `json:"actual,omitempty"`
}

type ProtocolHealthSummaryState struct {
	Pass  int `json:"pass"`
	Warn  int `json:"warn"`
	Fail  int `json:"fail"`
	Total int `json:"total"`
}

type ProtocolHealthSnapshotState struct {
	Status      string                     `json:"status"`
	ToolVersion string                     `json:"tool_version"`
	Summary     ProtocolHealthSummaryState `json:"summary"`
	Checks      []ProtocolHealthCheckState `json:"checks,omitempty"`
	Warnings    []string                   `json:"warnings,omitempty"`
	Errors      []string                   `json:"errors,omitempty"`
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
	PlanSnapshot    *ProtocolPlanSnapshotState   `json:"plan_snapshot,omitempty"`
	HealthSnapshot  *ProtocolHealthSnapshotState `json:"health_snapshot,omitempty"`
	Prompt          json.RawMessage     `json:"prompt,omitempty"`
	Items           []ProtocolItemState     `json:"items,omitempty"`
	Artifacts       []ProtocolArtifactState `json:"artifacts,omitempty"`
	Progress        *ProtocolProgressState     `json:"progress,omitempty"`
	ActionResult    *ProtocolActionResultState       `json:"action_result,omitempty"`
	QueueMutation     *ProtocolQueueMutationResultState     `json:"queue_mutation_result,omitempty"`
	HistoryManagement *ProtocolHistoryManagementResultState `json:"history_management_result,omitempty"`
	HistorySupport    *ProtocolHistorySupportResultState    `json:"history_support_result,omitempty"`
	HistoryCleanup    *ProtocolHistoryCleanupResultState    `json:"history_cleanup_result,omitempty"`
	Error             string                                `json:"error,omitempty"`
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

func protocolHealthSnapshotEvent(data []byte) (ProtocolHealthSnapshotState, error) {
	var event struct {
		Type        string                     `json:"type"`
		Status      string                     `json:"status"`
		ToolVersion string                     `json:"tool_version"`
		Summary     ProtocolHealthSummaryState `json:"summary"`
		Checks      []ProtocolHealthCheckState `json:"checks"`
		Warnings    []string                   `json:"warnings"`
		Errors      []string                   `json:"errors"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return ProtocolHealthSnapshotState{}, fmt.Errorf("invalid Patch health_snapshot JSON: %w", err)
	}
	if event.Type != "health_snapshot" {
		return ProtocolHealthSnapshotState{}, fmt.Errorf("unsupported Patch Health event")
	}
	event.Status = strings.ToUpper(strings.TrimSpace(event.Status))
	switch event.Status {
	case "PASS", "WARN", "FAIL":
	default:
		return ProtocolHealthSnapshotState{}, fmt.Errorf("Patch Health status is invalid")
	}
	event.ToolVersion = strings.TrimSpace(event.ToolVersion)
	if event.ToolVersion == "" || len(event.ToolVersion) > 128 {
		return ProtocolHealthSnapshotState{}, fmt.Errorf("Patch Health tool_version is invalid")
	}
	if len(event.Checks) > maxProtocolHealthChecks ||
		len(event.Warnings) > maxProtocolHealthMessages ||
		len(event.Errors) > maxProtocolHealthMessages {
		return ProtocolHealthSnapshotState{}, fmt.Errorf("Patch Health collection is out of bounds")
	}
	if event.Summary.Pass < 0 || event.Summary.Warn < 0 || event.Summary.Fail < 0 || event.Summary.Total < 0 {
		return ProtocolHealthSnapshotState{}, fmt.Errorf("Patch Health summary is invalid")
	}

	actualPass, actualWarn, actualFail := 0, 0, 0
	for i := range event.Checks {
		check := &event.Checks[i]
		check.Name = strings.TrimSpace(check.Name)
		check.Status = strings.ToUpper(strings.TrimSpace(check.Status))
		check.Detail = strings.TrimSpace(check.Detail)
		if check.Name == "" || len(check.Name) > 512 || len(check.Detail) > 512 {
			return ProtocolHealthSnapshotState{}, fmt.Errorf("Patch Health check is invalid")
		}
		switch check.Status {
		case "PASS":
			actualPass++
		case "WARN":
			actualWarn++
		case "FAIL":
			actualFail++
		default:
			return ProtocolHealthSnapshotState{}, fmt.Errorf("Patch Health check status is invalid")
		}
		for _, value := range []*int{
			check.Entries,
			check.Failures,
			check.MissingManaged,
			check.StaleManaged,
			check.Files,
			check.Dirs,
			check.Actual,
		} {
			if value != nil && *value < 0 {
				return ProtocolHealthSnapshotState{}, fmt.Errorf("Patch Health check counter is invalid")
			}
		}
	}
	if event.Summary.Pass != actualPass ||
		event.Summary.Warn != actualWarn ||
		event.Summary.Fail != actualFail ||
		event.Summary.Total != len(event.Checks) ||
		event.Summary.Pass+event.Summary.Warn+event.Summary.Fail != event.Summary.Total {
		return ProtocolHealthSnapshotState{}, fmt.Errorf("Patch Health summary does not match checks")
	}

	for i := range event.Warnings {
		event.Warnings[i] = strings.TrimSpace(event.Warnings[i])
		if event.Warnings[i] == "" || len(event.Warnings[i]) > 1024 {
			return ProtocolHealthSnapshotState{}, fmt.Errorf("Patch Health warning is invalid")
		}
	}
	for i := range event.Errors {
		event.Errors[i] = strings.TrimSpace(event.Errors[i])
		if event.Errors[i] == "" || len(event.Errors[i]) > 1024 {
			return ProtocolHealthSnapshotState{}, fmt.Errorf("Patch Health error is invalid")
		}
	}

	return ProtocolHealthSnapshotState{
		Status:      event.Status,
		ToolVersion: event.ToolVersion,
		Summary:     event.Summary,
		Checks:      event.Checks,
		Warnings:    event.Warnings,
		Errors:      event.Errors,
	}, nil
}

func protocolPlanSnapshotEvent(data []byte) (ProtocolPlanSnapshotState, error) {
	var event struct {
		Type                  string                           `json:"type"`
		Status                string                           `json:"status"`
		FailurePolicy         string                           `json:"failure_policy"`
		TransactionPolicy     string                           `json:"transaction_policy"`
		Items                 []ProtocolPlanItemState          `json:"items"`
		PreviousFailureAction *ProtocolPlanPreviousActionState `json:"previous_failure_action"`
		StaticConflicts       []ProtocolPlanConflictState      `json:"static_conflicts"`
		Resources             *ProtocolPlanResourceState       `json:"resources"`
		Previews              []ProtocolPlanPreviewState       `json:"previews"`
		Warnings              []string                         `json:"warnings"`
		Error                 *ProtocolPlanErrorState          `json:"error"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return ProtocolPlanSnapshotState{}, fmt.Errorf("invalid Patch plan_snapshot JSON: %w", err)
	}
	if event.Type != "plan_snapshot" {
		return ProtocolPlanSnapshotState{}, fmt.Errorf("unsupported Patch Plan event")
	}
	event.Status = strings.ToLower(strings.TrimSpace(event.Status))
	switch event.Status {
	case "ready", "empty", "blocked", "preview_failed":
	default:
		return ProtocolPlanSnapshotState{}, fmt.Errorf("Patch Plan status is invalid")
	}
	event.FailurePolicy = strings.TrimSpace(event.FailurePolicy)
	if event.FailurePolicy != "fail_fast" && event.FailurePolicy != "continue_independent" {
		return ProtocolPlanSnapshotState{}, fmt.Errorf("Patch Plan failure_policy is invalid")
	}
	event.TransactionPolicy = strings.TrimSpace(event.TransactionPolicy)
	if event.TransactionPolicy != "patch" && event.TransactionPolicy != "batch" {
		return ProtocolPlanSnapshotState{}, fmt.Errorf("Patch Plan transaction_policy is invalid")
	}
	if len(event.Items) > maxProtocolItems || len(event.Previews) > maxProtocolItems ||
		len(event.StaticConflicts) > maxProtocolPlanConflicts || len(event.Warnings) > maxProtocolPlanWarnings {
		return ProtocolPlanSnapshotState{}, fmt.Errorf("Patch Plan collection is out of bounds")
	}

	seenIndexes := make(map[int]bool, len(event.Items))
	for i := range event.Items {
		item := &event.Items[i]
		item.Name = strings.TrimSpace(item.Name)
		item.PatchID = strings.TrimSpace(item.PatchID)
		item.PackageSHA256 = strings.TrimSpace(item.PackageSHA256)
		if item.Index < 1 || item.Index > maxProtocolItems || seenIndexes[item.Index] ||
			item.Name == "" || len(item.Name) > 1024 || len(item.PatchID) > 512 ||
			len(item.PackageSHA256) > 64 || item.TargetCount < 0 || item.IDReuseCount < 0 ||
			len(item.DependsOn) > maxProtocolPlanDepends {
			return ProtocolPlanSnapshotState{}, fmt.Errorf("Patch Plan item is invalid")
		}
		seenIndexes[item.Index] = true
		for j := range item.DependsOn {
			item.DependsOn[j] = strings.TrimSpace(item.DependsOn[j])
			if item.DependsOn[j] == "" || len(item.DependsOn[j]) > 512 {
				return ProtocolPlanSnapshotState{}, fmt.Errorf("Patch Plan dependency is invalid")
			}
		}
	}

	if event.PreviousFailureAction != nil {
		event.PreviousFailureAction.Action = strings.TrimSpace(event.PreviousFailureAction.Action)
		event.PreviousFailureAction.Reason = strings.TrimSpace(event.PreviousFailureAction.Reason)
		if len(event.PreviousFailureAction.Action) > 64 || len(event.PreviousFailureAction.Reason) > 512 {
			return ProtocolPlanSnapshotState{}, fmt.Errorf("Patch Plan previous failure action is invalid")
		}
	}

	for i := range event.StaticConflicts {
		row := &event.StaticConflicts[i]
		row.Left = strings.TrimSpace(row.Left)
		row.LeftPatchID = strings.TrimSpace(row.LeftPatchID)
		row.Right = strings.TrimSpace(row.Right)
		row.RightPatchID = strings.TrimSpace(row.RightPatchID)
		row.Relation = strings.TrimSpace(row.Relation)
		if row.Left == "" || row.Right == "" || len(row.Left) > 1024 || len(row.Right) > 1024 ||
			len(row.LeftPatchID) > 512 || len(row.RightPatchID) > 512 || len(row.Relation) > 128 ||
			len(row.Overlap) > maxProtocolPlanOverlap {
			return ProtocolPlanSnapshotState{}, fmt.Errorf("Patch Plan conflict is invalid")
		}
		for j := range row.Overlap {
			row.Overlap[j] = strings.TrimSpace(row.Overlap[j])
			if row.Overlap[j] == "" || len(row.Overlap[j]) > 1024 {
				return ProtocolPlanSnapshotState{}, fmt.Errorf("Patch Plan conflict overlap is invalid")
			}
		}
	}

	if event.Resources != nil {
		event.Resources.Status = strings.TrimSpace(event.Resources.Status)
		if event.Resources.Status == "" || len(event.Resources.Status) > 64 {
			return ProtocolPlanSnapshotState{}, fmt.Errorf("Patch Plan resource status is invalid")
		}
		for _, value := range []*int64{
			event.Resources.ActualProjectFreeBytes,
			event.Resources.RequiredProjectFreeBytes,
			event.Resources.ActualTempFreeBytes,
			event.Resources.RequiredTempFreeBytes,
		} {
			if value != nil && *value < 0 {
				return ProtocolPlanSnapshotState{}, fmt.Errorf("Patch Plan resource bytes are invalid")
			}
		}
	}

	for i := range event.Previews {
		row := &event.Previews[i]
		row.Name = strings.TrimSpace(row.Name)
		row.Status = strings.TrimSpace(row.Status)
		row.Stage = strings.TrimSpace(row.Stage)
		row.DiagnosisKind = strings.TrimSpace(row.DiagnosisKind)
		row.Message = strings.TrimSpace(row.Message)
		if row.Name == "" || len(row.Name) > 1024 || row.Status == "" || len(row.Status) > 64 ||
			row.Stage == "" || len(row.Stage) > 64 || len(row.DiagnosisKind) > 128 ||
			len(row.Message) > 1024 || row.TargetCount < 0 {
			return ProtocolPlanSnapshotState{}, fmt.Errorf("Patch Plan preview is invalid")
		}
	}

	for i := range event.Warnings {
		event.Warnings[i] = strings.TrimSpace(event.Warnings[i])
		if len(event.Warnings[i]) > 1024 || strings.ContainsRune(event.Warnings[i], '\x00') {
			return ProtocolPlanSnapshotState{}, fmt.Errorf("Patch Plan warning is invalid")
		}
	}
	if event.Error != nil {
		event.Error.Kind = strings.TrimSpace(event.Error.Kind)
		event.Error.Message = strings.TrimSpace(event.Error.Message)
		if event.Error.Kind == "" || len(event.Error.Kind) > 128 || len(event.Error.Message) > 1024 {
			return ProtocolPlanSnapshotState{}, fmt.Errorf("Patch Plan error is invalid")
		}
	}

	return ProtocolPlanSnapshotState{
		Status: event.Status,
		FailurePolicy: event.FailurePolicy,
		TransactionPolicy: event.TransactionPolicy,
		Items: append([]ProtocolPlanItemState(nil), event.Items...),
		PreviousFailureAction: event.PreviousFailureAction,
		StaticConflicts: append([]ProtocolPlanConflictState(nil), event.StaticConflicts...),
		Resources: event.Resources,
		Previews: append([]ProtocolPlanPreviewState(nil), event.Previews...),
		Warnings: append([]string(nil), event.Warnings...),
		Error: event.Error,
	}, nil
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
		DiagnosisKind  string   `json:"diagnosis_kind"`
		FailureReason  string   `json:"failure_reason"`
		OutputTail     string   `json:"output_tail"`
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
	event.DiagnosisKind = strings.TrimSpace(event.DiagnosisKind)
	event.FailureReason = strings.TrimSpace(event.FailureReason)
	event.OutputTail = strings.TrimSpace(event.OutputTail)
	if len([]byte(event.DiagnosisKind)) > 512 ||
		len([]byte(event.FailureReason)) > 8192 ||
		len([]byte(event.OutputTail)) > 16384 ||
		strings.ContainsRune(event.DiagnosisKind, '\x00') ||
		strings.ContainsRune(event.FailureReason, '\x00') ||
		strings.ContainsRune(event.OutputTail, '\x00') {
		return ProtocolItemState{}, "", fmt.Errorf("Patch item failure evidence is invalid")
	}
	if event.Type == "item_started" {
		status = "RUNNING"
		event.DiagnosisKind = ""
		event.FailureReason = ""
		event.OutputTail = ""
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
		DiagnosisKind: event.DiagnosisKind,
		FailureReason: event.FailureReason,
		OutputTail: event.OutputTail,
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
func protocolHistoryCleanupResultEvent(data []byte) (ProtocolHistoryCleanupResultState, error) {
	var event struct {
		Type           string `json:"type"`
		PromptID       string `json:"prompt_id"`
		CleanupID      string `json:"cleanup_id"`
		Status         string `json:"status"`
		RC             int    `json:"rc"`
		HistoryChanged bool   `json:"history_changed"`
		Removed        int    `json:"removed"`
		Pinned         int    `json:"pinned"`
		Remaining      int    `json:"remaining"`
		Policy         string `json:"policy"`
		EligibleBefore int    `json:"eligible_before"`
		Message        string `json:"message"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return ProtocolHistoryCleanupResultState{}, fmt.Errorf("invalid Patch history_cleanup_result JSON: %w", err)
	}
	if event.Type != "history_cleanup_result" {
		return ProtocolHistoryCleanupResultState{}, fmt.Errorf("unsupported Patch History cleanup event")
	}
	event.PromptID = strings.TrimSpace(event.PromptID)
	event.CleanupID = strings.TrimSpace(event.CleanupID)
	event.Status = strings.ToUpper(strings.TrimSpace(event.Status))
	event.Policy = strings.TrimSpace(event.Policy)
	event.Message = strings.TrimSpace(event.Message)
	if event.PromptID == "" || len(event.PromptID) > 256 ||
		event.CleanupID == "" || len(event.CleanupID) > 128 {
		return ProtocolHistoryCleanupResultState{}, fmt.Errorf("Patch History cleanup identity is invalid")
	}
	if event.Status != "PASS" || event.RC != 0 {
		return ProtocolHistoryCleanupResultState{}, fmt.Errorf("Patch History cleanup result is invalid")
	}
	if event.Removed < 0 || event.Pinned < 0 || event.Remaining < 0 || event.EligibleBefore < 0 ||
		event.Removed > event.EligibleBefore || event.HistoryChanged != (event.Removed > 0) {
		return ProtocolHistoryCleanupResultState{}, fmt.Errorf("Patch History cleanup counters are inconsistent")
	}
	if event.Policy != "remove_unpinned_idle_then_oldest_unpinned_over_limit" {
		return ProtocolHistoryCleanupResultState{}, fmt.Errorf("Patch History cleanup policy is unsupported")
	}
	if len([]byte(event.Message)) > 4096 || strings.ContainsRune(event.Message, '\x00') {
		return ProtocolHistoryCleanupResultState{}, fmt.Errorf("Patch History cleanup message is invalid")
	}
	return ProtocolHistoryCleanupResultState{
		PromptID: event.PromptID, CleanupID: event.CleanupID,
		Status: event.Status, RC: event.RC, HistoryChanged: event.HistoryChanged,
		Removed: event.Removed, Pinned: event.Pinned, Remaining: event.Remaining,
		Policy: event.Policy, EligibleBefore: event.EligibleBefore, Message: event.Message,
	}, nil
}

func protocolHistorySupportResultEvent(data []byte) (ProtocolHistorySupportResultState, error) {
	var event struct {
		Type      string `json:"type"`
		PromptID  string `json:"prompt_id"`
		SupportID string `json:"support_id"`
		RunID     string `json:"run_id"`
		ItemIndex int    `json:"item_index"`
		ItemName  string `json:"item_name"`
		Status    string `json:"status"`
		Message   string `json:"message"`
		Artifact  *struct {
			Label          string `json:"label"`
			Path           string `json:"path"`
			UploadRequired bool   `json:"upload_required"`
		} `json:"artifact"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return ProtocolHistorySupportResultState{}, fmt.Errorf("invalid Patch history_support_result JSON: %w", err)
	}
	if event.Type != "history_support_result" {
		return ProtocolHistorySupportResultState{}, fmt.Errorf("unsupported Patch History support event")
	}
	event.PromptID = strings.TrimSpace(event.PromptID)
	event.SupportID = strings.TrimSpace(event.SupportID)
	event.RunID = strings.TrimSpace(event.RunID)
	event.ItemName = strings.TrimSpace(event.ItemName)
	event.Status = strings.ToUpper(strings.TrimSpace(event.Status))
	event.Message = strings.TrimSpace(event.Message)
	if event.PromptID == "" || len(event.PromptID) > 256 ||
		event.SupportID == "" || len(event.SupportID) > 128 ||
		event.RunID == "" || len(event.RunID) > 128 {
		return ProtocolHistorySupportResultState{}, fmt.Errorf("Patch History support identity is invalid")
	}
	if event.ItemIndex < 1 || event.ItemIndex > 512 ||
		event.ItemName == "" || len(event.ItemName) > 512 ||
		(event.Status != "PASS" && event.Status != "FAIL") {
		return ProtocolHistorySupportResultState{}, fmt.Errorf("Patch History support result is invalid")
	}
	if len([]byte(event.Message)) > 4096 || strings.ContainsRune(event.Message, '\x00') {
		return ProtocolHistorySupportResultState{}, fmt.Errorf("Patch History support message is invalid")
	}
	out := ProtocolHistorySupportResultState{
		PromptID: event.PromptID, SupportID: event.SupportID, RunID: event.RunID,
		ItemIndex: event.ItemIndex, ItemName: event.ItemName, Status: event.Status, Message: event.Message,
	}
	if event.Status == "PASS" {
		if event.Artifact == nil {
			return ProtocolHistorySupportResultState{}, fmt.Errorf("Patch History support PASS requires an artifact")
		}
		label := strings.TrimSpace(event.Artifact.Label)
		rel := strings.TrimSpace(event.Artifact.Path)
		if label == "" || len(label) > 128 ||
			rel == "" || len(rel) > 4096 || !strings.HasPrefix(rel, "artifacts/support/") ||
			strings.ContainsRune(rel, '\x00') || strings.Contains(rel, "\\") {
			return ProtocolHistorySupportResultState{}, fmt.Errorf("Patch History support artifact is invalid")
		}
		for _, part := range strings.Split(rel, "/") {
			if part == "" || part == "." || part == ".." {
				return ProtocolHistorySupportResultState{}, fmt.Errorf("Patch History support artifact path is invalid")
			}
		}
		out.Artifact = &ProtocolHistorySupportArtifact{
			Label: label, Path: rel, UploadRequired: event.Artifact.UploadRequired,
		}
	} else if event.Artifact != nil {
		return ProtocolHistorySupportResultState{}, fmt.Errorf("Patch History support FAIL must not carry an artifact")
	}
	return out, nil
}

func protocolHistoryManagementResultEvent(data []byte) (ProtocolHistoryManagementResultState, error) {
	var event struct {
		Type           string `json:"type"`
		PromptID       string `json:"prompt_id"`
		ManagementID   string `json:"management_id"`
		Action         string `json:"action"`
		RunID          string `json:"run_id"`
		Status         string `json:"status"`
		RC             int    `json:"rc"`
		HistoryChanged bool   `json:"history_changed"`
		Message        string `json:"message"`
		Artifact       *struct {
			Label          string `json:"label"`
			Path           string `json:"path"`
			UploadRequired bool   `json:"upload_required"`
		} `json:"artifact"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return ProtocolHistoryManagementResultState{}, fmt.Errorf("invalid Patch history_management_result JSON: %w", err)
	}
	if event.Type != "history_management_result" {
		return ProtocolHistoryManagementResultState{}, fmt.Errorf("unsupported Patch History management event")
	}
	event.PromptID = strings.TrimSpace(event.PromptID)
	event.ManagementID = strings.TrimSpace(event.ManagementID)
	event.Action = strings.ToLower(strings.TrimSpace(event.Action))
	event.RunID = strings.TrimSpace(event.RunID)
	event.Status = strings.ToUpper(strings.TrimSpace(event.Status))
	event.Message = strings.TrimSpace(event.Message)
	if event.PromptID == "" || len(event.PromptID) > 256 || event.ManagementID == "" || len(event.ManagementID) > 128 {
		return ProtocolHistoryManagementResultState{}, fmt.Errorf("Patch History management identity is invalid")
	}
	switch event.Action {
	case "pin", "unpin", "delete", "export":
	default:
		return ProtocolHistoryManagementResultState{}, fmt.Errorf("Patch History management action is unsupported")
	}
	if event.RunID == "" || len(event.RunID) > 128 || (event.Status != "PASS" && event.Status != "FAIL") {
		return ProtocolHistoryManagementResultState{}, fmt.Errorf("Patch History management result is invalid")
	}
	if len([]byte(event.Message)) > 4096 || strings.ContainsRune(event.Message, '\x00') {
		return ProtocolHistoryManagementResultState{}, fmt.Errorf("Patch History management message is invalid")
	}
	out := ProtocolHistoryManagementResultState{
		PromptID: event.PromptID, ManagementID: event.ManagementID, Action: event.Action,
		RunID: event.RunID, Status: event.Status, RC: event.RC,
		HistoryChanged: event.HistoryChanged, Message: event.Message,
	}
	if event.Artifact != nil {
		label := strings.TrimSpace(event.Artifact.Label)
		rel := strings.TrimSpace(event.Artifact.Path)
		if event.Action != "export" || event.Status != "PASS" || label == "" || len(label) > 128 ||
			rel == "" || len(rel) > 4096 || !strings.HasPrefix(rel, "artifacts/") ||
			strings.ContainsRune(rel, '\x00') || strings.Contains(rel, "\\") {
			return ProtocolHistoryManagementResultState{}, fmt.Errorf("Patch History management artifact is invalid")
		}
		for _, part := range strings.Split(rel, "/") {
			if part == "" || part == "." || part == ".." {
				return ProtocolHistoryManagementResultState{}, fmt.Errorf("Patch History management artifact path is invalid")
			}
		}
		out.Artifact = &ProtocolHistoryManagementArtifact{
			Label: label, Path: rel, UploadRequired: event.Artifact.UploadRequired,
		}
	}
	return out, nil
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

func protocolHistoryCleanupPromptID(data []byte) (string, bool, error) {
	var command struct {
		Command string `json:"command"`
		Payload struct {
			PromptID string `json:"prompt_id"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(data, &command); err != nil {
		return "", false, fmt.Errorf("invalid Patch protocol command JSON: %w", err)
	}
	if command.Command != "history_cleanup" {
		return "", false, nil
	}
	if strings.TrimSpace(command.Payload.PromptID) == "" {
		return "", true, fmt.Errorf("Patch history_cleanup prompt_id is required")
	}
	return command.Payload.PromptID, true, nil
}

func protocolHistorySupportPromptID(data []byte) (string, bool, error) {
	var command struct {
		Command string `json:"command"`
		Payload struct {
			PromptID string `json:"prompt_id"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(data, &command); err != nil {
		return "", false, fmt.Errorf("invalid Patch protocol command JSON: %w", err)
	}
	if command.Command != "history_support" {
		return "", false, nil
	}
	if strings.TrimSpace(command.Payload.PromptID) == "" {
		return "", true, fmt.Errorf("Patch history_support prompt_id is required")
	}
	return command.Payload.PromptID, true, nil
}

func protocolHistoryManagePromptID(data []byte) (string, bool, error) {
	var command struct {
		Command string `json:"command"`
		Payload struct {
			PromptID string `json:"prompt_id"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(data, &command); err != nil {
		return "", false, fmt.Errorf("invalid Patch protocol command JSON: %w", err)
	}
	if command.Command != "history_manage" {
		return "", false, nil
	}
	if strings.TrimSpace(command.Payload.PromptID) == "" {
		return "", true, fmt.Errorf("Patch history_manage prompt_id is required")
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
	if in.PlanSnapshot != nil {
		planSnapshot := *in.PlanSnapshot
		planSnapshot.Items = append([]ProtocolPlanItemState(nil), in.PlanSnapshot.Items...)
		for i := range planSnapshot.Items {
			planSnapshot.Items[i].DependsOn = append([]string(nil), in.PlanSnapshot.Items[i].DependsOn...)
		}
		planSnapshot.StaticConflicts = append([]ProtocolPlanConflictState(nil), in.PlanSnapshot.StaticConflicts...)
		for i := range planSnapshot.StaticConflicts {
			planSnapshot.StaticConflicts[i].Overlap = append([]string(nil), in.PlanSnapshot.StaticConflicts[i].Overlap...)
		}
		planSnapshot.Previews = append([]ProtocolPlanPreviewState(nil), in.PlanSnapshot.Previews...)
		planSnapshot.Warnings = append([]string(nil), in.PlanSnapshot.Warnings...)
		if in.PlanSnapshot.PreviousFailureAction != nil {
			previous := *in.PlanSnapshot.PreviousFailureAction
			planSnapshot.PreviousFailureAction = &previous
		}
		if in.PlanSnapshot.Resources != nil {
			resources := *in.PlanSnapshot.Resources
			planSnapshot.Resources = &resources
		}
		if in.PlanSnapshot.Error != nil {
			planError := *in.PlanSnapshot.Error
			planSnapshot.Error = &planError
		}
		out.PlanSnapshot = &planSnapshot
	}
	if in.HealthSnapshot != nil {
		healthSnapshot := *in.HealthSnapshot
		healthSnapshot.Checks = append([]ProtocolHealthCheckState(nil), in.HealthSnapshot.Checks...)
		healthSnapshot.Warnings = append([]string(nil), in.HealthSnapshot.Warnings...)
		healthSnapshot.Errors = append([]string(nil), in.HealthSnapshot.Errors...)
		out.HealthSnapshot = &healthSnapshot
	}
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
	if in.HistoryManagement != nil {
		historyManagement := *in.HistoryManagement
		if in.HistoryManagement.Artifact != nil {
			artifact := *in.HistoryManagement.Artifact
			historyManagement.Artifact = &artifact
		}
		out.HistoryManagement = &historyManagement
	}
	if in.HistorySupport != nil {
		historySupport := *in.HistorySupport
		if in.HistorySupport.Artifact != nil {
			artifact := *in.HistorySupport.Artifact
			historySupport.Artifact = &artifact
		}
		out.HistorySupport = &historySupport
	}
	if in.HistoryCleanup != nil {
		historyCleanup := *in.HistoryCleanup
		out.HistoryCleanup = &historyCleanup
	}
	return out
}
