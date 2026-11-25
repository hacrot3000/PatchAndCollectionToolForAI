package agentrun

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"bletonfc/vscode_tasks_menu/internal/taskmanifest"
)

type TaskStatus string

const (
	TaskQueued      TaskStatus = "queued"
	TaskRunning     TaskStatus = "running"
	TaskSucceeded   TaskStatus = "succeeded"
	TaskFailed      TaskStatus = "failed"
	TaskUnknown     TaskStatus = "unknown"
	TaskBlocked     TaskStatus = "blocked"
	TaskInterrupted TaskStatus = "interrupted"
	TaskReview      TaskStatus = "waiting_review"
	TaskWaitingSlot TaskStatus = "waiting_slot"
	TaskCancelled   TaskStatus = "cancelled"
)

type TaskState struct {
	TaskID               string     `json:"task_id"`
	Status               TaskStatus `json:"status"`
	Fence                uint64     `json:"fence"`
	Attempt              int        `json:"attempt"`
	RunID                string     `json:"run_id,omitempty"`
	ExecutionWorkspaceID string     `json:"execution_workspace_id,omitempty"`
	ConversationID       string     `json:"conversation_id,omitempty"`
	ContextSnapshotID    string     `json:"context_snapshot_id,omitempty"`
	WaitReason           string     `json:"wait_reason,omitempty"`
	Report               string     `json:"report,omitempty"`
	LastError            string     `json:"last_error,omitempty"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

type DAGSnapshot struct {
	Version     int                  `json:"version"`
	ManifestID  string               `json:"manifest_id"`
	ProjectID   string               `json:"project_id"`
	Concurrency int                  `json:"concurrency"`
	Revision    uint64               `json:"revision"`
	States      map[string]TaskState `json:"states"`
	UpdatedAt   time.Time            `json:"updated_at"`
}

type DAGScheduler struct {
	mu       sync.Mutex
	manifest taskmanifest.Manifest
	state    DAGSnapshot
}

func NewDAGScheduler(manifest taskmanifest.Manifest, concurrency int) (*DAGScheduler, error) {
	if !manifest.Valid || manifest.PlanningOnly != true || len(manifest.Tasks) == 0 || len(manifest.Tasks) > taskmanifest.MaxTasks {
		return nil, errors.New("valid planning manifest required")
	}
	if concurrency < 1 || concurrency > MaxConcurrentRuns {
		return nil, errors.New("DAG concurrency must be between 1 and 5")
	}
	states := make(map[string]TaskState, len(manifest.Tasks))
	now := time.Now().UTC()
	for _, task := range manifest.Tasks {
		states[task.ID] = TaskState{TaskID: task.ID, Status: TaskQueued, Attempt: 0, UpdatedAt: now}
	}
	return &DAGScheduler{manifest: manifest, state: DAGSnapshot{
		Version: 1, ManifestID: manifest.ManifestID, ProjectID: manifest.ProjectID,
		Concurrency: concurrency, States: states, UpdatedAt: now,
	}}, nil
}

func RestoreDAGScheduler(manifest taskmanifest.Manifest, snapshot DAGSnapshot) (*DAGScheduler, error) {
	if !manifest.Valid || snapshot.Version != 1 || snapshot.ManifestID != manifest.ManifestID || snapshot.ProjectID != manifest.ProjectID || snapshot.Concurrency < 1 || snapshot.Concurrency > MaxConcurrentRuns || len(snapshot.States) != len(manifest.Tasks) {
		return nil, errors.New("DAG snapshot does not match validated manifest")
	}
	for _, task := range manifest.Tasks {
		state, exists := snapshot.States[task.ID]
		if !exists || state.TaskID != task.ID {
			return nil, errors.New("DAG snapshot task mismatch")
		}
		switch state.Status {
		case TaskQueued, TaskRunning, TaskWaitingSlot, TaskSucceeded, TaskFailed, TaskUnknown, TaskBlocked, TaskInterrupted, TaskReview, TaskCancelled:
		default:
			return nil, errors.New("invalid DAG task status")
		}
		if state.Status == TaskRunning {
			state.Status = TaskInterrupted
			snapshot.States[task.ID] = state
		} else if state.Status == TaskWaitingSlot {
			state.Status, state.WaitReason = TaskQueued, "concurrency_slot"
			snapshot.States[task.ID] = state
		}
	}
	return &DAGScheduler{manifest: manifest, state: snapshot}, nil
}

func (d *DAGScheduler) ClaimReady() (TaskState, taskmanifest.Task, bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.refreshWaitReasonsLocked()
	active := 0
	for _, state := range d.state.States {
		if state.Status == TaskRunning {
			active++
		}
	}
	if active >= d.state.Concurrency {
		return TaskState{}, taskmanifest.Task{}, false, nil
	}
	for _, taskID := range d.manifest.Graph.Topological {
		state := d.state.States[taskID]
		if state.Status != TaskQueued {
			continue
		}
		task, ok := taskByID(d.manifest.Tasks, taskID)
		if !ok {
			return TaskState{}, taskmanifest.Task{}, false, errors.New("manifest graph task is missing")
		}
		ready := true
		for _, dep := range task.Dependencies {
			depState := d.state.States[dep]
			if depState.Status != TaskSucceeded {
				ready = false
				break
			}
		}
		if !ready {
			continue
		}
		state.Status = TaskRunning
		state.WaitReason = ""
		state.Fence++
		state.Attempt++
		state.UpdatedAt = time.Now().UTC()
		d.state.States[taskID] = state
		d.bumpLocked()
		return state, task, true, nil
	}
	d.markBlockedLocked()
	return TaskState{}, taskmanifest.Task{}, false, nil
}

func (d *DAGScheduler) BindTask(taskID string, fence uint64, runID, executionWorkspaceID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	state, ok := d.state.States[taskID]
	if !ok || state.Status != TaskRunning || state.Fence != fence || runID == "" || executionWorkspaceID == "" {
		return errors.New("stale or incomplete DAG task binding")
	}
	state.RunID, state.ExecutionWorkspaceID, state.UpdatedAt = runID, executionWorkspaceID, time.Now().UTC()
	d.state.States[taskID] = state
	d.bumpLocked()
	return nil
}

func (d *DAGScheduler) DeferForGlobalSlot(taskID string, fence uint64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	state, ok := d.state.States[taskID]
	if !ok || state.Status != TaskRunning || state.Fence != fence {
		return errors.New("stale task slot deferral")
	}
	state.Status, state.WaitReason, state.UpdatedAt = TaskWaitingSlot, "global_concurrency_slot", time.Now().UTC()
	d.state.States[taskID] = state
	d.bumpLocked()
	return nil
}

func (d *DAGScheduler) ActivateGlobalSlot(taskID string) (TaskState, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	state, ok := d.state.States[taskID]
	if !ok || state.Status != TaskWaitingSlot {
		return TaskState{}, errors.New("task is not waiting for a global run slot")
	}
	state.Status, state.WaitReason, state.Fence, state.UpdatedAt = TaskRunning, "", state.Fence+1, time.Now().UTC()
	d.state.States[taskID] = state
	d.bumpLocked()
	return state, nil
}

func (d *DAGScheduler) BindTaskContext(taskID string, fence uint64, conversationID, snapshotID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	state, ok := d.state.States[taskID]
	if !ok || state.Status != TaskRunning || state.Fence != fence || conversationID == "" || snapshotID == "" {
		return errors.New("stale or incomplete task context binding")
	}
	state.ConversationID, state.ContextSnapshotID, state.UpdatedAt = conversationID, snapshotID, time.Now().UTC()
	d.state.States[taskID] = state
	d.bumpLocked()
	return nil
}

func (d *DAGScheduler) AwaitReview(taskID string, fence uint64, report string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	state, ok := d.state.States[taskID]
	if !ok || state.Status != TaskRunning || state.Fence != fence {
		return errors.New("stale task report")
	}
	if len(report) > maxEventPayload {
		return errors.New("task report exceeds limit")
	}
	state.Status, state.Report, state.UpdatedAt = TaskReview, report, time.Now().UTC()
	d.state.States[taskID] = state
	d.markBlockedLocked()
	d.bumpLocked()
	return nil
}

func (d *DAGScheduler) ReviewDecision(taskID string, fence uint64, approve bool, reason string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	state, ok := d.state.States[taskID]
	if !ok || state.Status != TaskReview || state.Fence != fence {
		return errors.New("task is not awaiting this review decision")
	}
	if len(reason) > 1000 {
		return errors.New("review reason exceeds limit")
	}
	state.UpdatedAt = time.Now().UTC()
	if approve {
		state.Status = TaskSucceeded
	} else {
		state.Status, state.LastError = TaskFailed, reason
	}
	d.state.States[taskID] = state
	if approve {
		d.restoreUnblockedLocked()
	}
	d.markBlockedLocked()
	d.bumpLocked()
	return nil
}

func (d *DAGScheduler) Complete(taskID string, fence uint64, result TaskStatus, report string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	state, ok := d.state.States[taskID]
	if !ok || state.Status != TaskRunning || state.Fence != fence {
		return errors.New("stale DAG task completion")
	}
	if result != TaskSucceeded && result != TaskFailed && result != TaskUnknown {
		return errors.New("invalid terminal task result")
	}
	if len(report) > maxEventPayload {
		return errors.New("task report exceeds limit")
	}
	state.Status, state.Report, state.UpdatedAt = result, report, time.Now().UTC()
	d.state.States[taskID] = state
	if result == TaskSucceeded {
		d.restoreUnblockedLocked()
	}
	d.markBlockedLocked()
	d.bumpLocked()
	return nil
}

func (d *DAGScheduler) MarkInterrupted(taskID string, fence uint64, reason string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	state, ok := d.state.States[taskID]
	if !ok || state.Status != TaskRunning || state.Fence != fence {
		return errors.New("stale task interruption")
	}
	state.Status, state.LastError, state.UpdatedAt = TaskInterrupted, reason, time.Now().UTC()
	d.state.States[taskID] = state
	d.markBlockedLocked()
	d.bumpLocked()
	return nil
}

func (d *DAGScheduler) CancelTask(taskID string, fence uint64, reason string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	state, ok := d.state.States[taskID]
	if !ok || state.Fence != fence || (state.Status != TaskRunning && state.Status != TaskWaitingSlot && state.Status != TaskQueued && state.Status != TaskReview && state.Status != TaskInterrupted) {
		return errors.New("stale task cancellation")
	}
	state.Status, state.WaitReason, state.LastError, state.UpdatedAt = TaskCancelled, "", reason, time.Now().UTC()
	d.state.States[taskID] = state
	d.markBlockedLocked()
	d.bumpLocked()
	return nil
}

func (d *DAGScheduler) ResumeActive(taskID string) (TaskState, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	state, ok := d.state.States[taskID]
	if !ok || state.Status != TaskInterrupted || state.RunID == "" {
		return TaskState{}, errors.New("interrupted task has no active run binding")
	}
	state.Status, state.WaitReason, state.Fence, state.UpdatedAt = TaskRunning, "", state.Fence+1, time.Now().UTC()
	d.state.States[taskID] = state
	d.bumpLocked()
	return state, nil
}

func (d *DAGScheduler) ResumeInterrupted(taskID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	state, ok := d.state.States[taskID]
	if !ok || state.Status != TaskInterrupted {
		return errors.New("task is not interrupted")
	}
	state.Status, state.WaitReason, state.LastError, state.UpdatedAt = TaskQueued, "", "", time.Now().UTC()
	d.state.States[taskID] = state
	for id, child := range d.state.States {
		if child.Status == TaskBlocked && dependsTransitively(d.manifest.Tasks, id, taskID) {
			child.Status, child.UpdatedAt = TaskQueued, time.Now().UTC()
			d.state.States[id] = child
		}
	}
	d.bumpLocked()
	return nil
}

func (d *DAGScheduler) ResumeAfterReview(taskID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	state, ok := d.state.States[taskID]
	if !ok || (state.Status != TaskFailed && state.Status != TaskUnknown && state.Status != TaskInterrupted) {
		return errors.New("task is not eligible for reviewed resume")
	}
	state.Status, state.WaitReason, state.LastError, state.UpdatedAt = TaskQueued, "", "", time.Now().UTC()
	d.state.States[taskID] = state
	for id, child := range d.state.States {
		if child.Status == TaskBlocked && dependsTransitively(d.manifest.Tasks, id, taskID) {
			child.Status, child.UpdatedAt = TaskQueued, time.Now().UTC()
			d.state.States[id] = child
		}
	}
	d.bumpLocked()
	return nil
}

func (d *DAGScheduler) ResumeAfterToolContinuation(taskID string) (TaskState, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	state, ok := d.state.States[taskID]
	if !ok || (state.Status != TaskUnknown && state.Status != TaskInterrupted) || state.RunID == "" {
		return TaskState{}, errors.New("task has no reconciled run to continue")
	}
	state.Status, state.WaitReason, state.LastError, state.Fence, state.UpdatedAt = TaskRunning, "", "", state.Fence+1, time.Now().UTC()
	d.state.States[taskID] = state
	d.bumpLocked()
	return state, nil
}

func (d *DAGScheduler) Retry(taskID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	state, ok := d.state.States[taskID]
	if !ok || state.Status != TaskFailed {
		return errors.New("only a known failed task can be retried")
	}
	state.Status, state.WaitReason = TaskQueued, ""
	state.Report, state.LastError, state.UpdatedAt = "", "", time.Now().UTC()
	d.state.States[taskID] = state
	for id, child := range d.state.States {
		if child.Status == TaskBlocked && dependsTransitively(d.manifest.Tasks, id, taskID) {
			child.Status, child.UpdatedAt = TaskQueued, time.Now().UTC()
			d.state.States[id] = child
		}
	}
	d.bumpLocked()
	return nil
}

func (d *DAGScheduler) Snapshot() DAGSnapshot {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.refreshWaitReasonsLocked()
	states := make(map[string]TaskState, len(d.state.States))
	for id, state := range d.state.States {
		states[id] = state
	}
	snapshot := d.state
	snapshot.States = states
	return snapshot
}

func (d *DAGScheduler) refreshWaitReasonsLocked() {
	d.markBlockedLocked()
	active := 0
	for _, state := range d.state.States {
		if state.Status == TaskRunning {
			active++
		}
	}
	changed := false
	for id, state := range d.state.States {
		if state.Status != TaskQueued {
			continue
		}
		task, ok := taskByID(d.manifest.Tasks, id)
		if !ok {
			continue
		}
		waitingDependency := false
		for _, dependency := range task.Dependencies {
			if d.state.States[dependency].Status != TaskSucceeded {
				waitingDependency = true
				break
			}
		}
		reason := "ready"
		if waitingDependency {
			reason = "dependency"
		} else if active >= d.state.Concurrency {
			reason = "concurrency_slot"
		}
		if state.WaitReason != reason {
			state.WaitReason, state.UpdatedAt = reason, time.Now().UTC()
			d.state.States[id] = state
			changed = true
		}
	}
	if changed {
		d.bumpLocked()
	}
}

func (d *DAGScheduler) restoreUnblockedLocked() {
	for id, state := range d.state.States {
		if state.Status != TaskBlocked {
			continue
		}
		task, ok := taskByID(d.manifest.Tasks, id)
		if !ok {
			continue
		}
		allSucceeded := true
		for _, dependency := range task.Dependencies {
			if d.state.States[dependency].Status != TaskSucceeded {
				allSucceeded = false
				break
			}
		}
		if allSucceeded {
			state.Status, state.UpdatedAt = TaskQueued, time.Now().UTC()
			d.state.States[id] = state
		}
	}
}

func (d *DAGScheduler) markBlockedLocked() {
	changed := false
	for id, state := range d.state.States {
		if state.Status != TaskQueued {
			continue
		}
		task, ok := taskByID(d.manifest.Tasks, id)
		if !ok {
			continue
		}
		for _, dep := range task.Dependencies {
			depStatus := d.state.States[dep].Status
			if depStatus == TaskFailed || depStatus == TaskUnknown || depStatus == TaskInterrupted || depStatus == TaskBlocked || depStatus == TaskCancelled {
				state.Status, state.UpdatedAt = TaskBlocked, time.Now().UTC()
				d.state.States[id] = state
				changed = true
				break
			}
		}
	}
	if changed {
		d.state.Revision++
	}
}

func (d *DAGScheduler) bumpLocked() {
	d.state.Revision++
	d.state.UpdatedAt = time.Now().UTC()
}

func SaveDAGSnapshot(path string, snapshot DAGSnapshot) error {
	if path == "" || !filepath.IsAbs(path) {
		return errors.New("absolute DAG snapshot path required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	body, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	if len(body) > 1<<20 {
		return errors.New("DAG snapshot exceeds limit")
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".dag-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(append(body, '\n'))
	}
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, path)
}

func LoadDAGSnapshot(path string) (DAGSnapshot, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return DAGSnapshot{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 || info.Size() > 1<<20 {
		return DAGSnapshot{}, errors.New("unsafe DAG snapshot")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return DAGSnapshot{}, err
	}
	var snapshot DAGSnapshot
	if err := json.Unmarshal(body, &snapshot); err != nil {
		return DAGSnapshot{}, err
	}
	return snapshot, nil
}

func taskByID(tasks []taskmanifest.Task, id string) (taskmanifest.Task, bool) {
	for _, task := range tasks {
		if task.ID == id {
			return task, true
		}
	}
	return taskmanifest.Task{}, false
}

func dependsTransitively(tasks []taskmanifest.Task, taskID, dependencyID string) bool {
	seen := map[string]bool{}
	var visit func(string) bool
	visit = func(current string) bool {
		if seen[current] {
			return false
		}
		seen[current] = true
		task, ok := taskByID(tasks, current)
		if !ok {
			return false
		}
		for _, dependency := range task.Dependencies {
			if dependency == dependencyID || visit(dependency) {
				return true
			}
		}
		return false
	}
	return visit(taskID)
}
