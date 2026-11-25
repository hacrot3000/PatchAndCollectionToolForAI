package agentrun

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
)

const (
	MaxConcurrentRuns = 5
	maxStoredRuns     = 1000
	maxRunEvents      = 512
	maxEventPayload   = 512 << 10
)

var globalRunSlots = make(chan struct{}, MaxConcurrentRuns)

type globalSlotContextKey struct{}
type principalOwnerContextKey struct{}

type globalRunLease struct{ active atomic.Bool }

func WithPrincipalOwner(ctx context.Context, owner string) context.Context {
	return context.WithValue(ctx, principalOwnerContextKey{}, owner)
}

func PrincipalOwner(ctx context.Context) (string, bool) {
	owner, ok := ctx.Value(principalOwnerContextKey{}).(string)
	return owner, ok && owner != ""
}

func GlobalSlotHeld(ctx context.Context) bool {
	lease, ok := ctx.Value(globalSlotContextKey{}).(*globalRunLease)
	return ok && lease != nil && lease.active.Load()
}

func AcquireGlobalSlotContext(ctx context.Context) (context.Context, func(), bool) {
	release, ok := AcquireGlobalSlot(ctx)
	if !ok {
		return nil, nil, false
	}
	lease := &globalRunLease{}
	lease.active.Store(true)
	withLease := context.WithValue(ctx, globalSlotContextKey{}, lease)
	var once sync.Once
	return withLease, func() { once.Do(func() { lease.active.Store(false); release() }) }, true
}

func AcquireGlobalSlot(ctx context.Context) (func(), bool) {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case globalRunSlots <- struct{}{}:
		var once sync.Once
		return func() { once.Do(func() { <-globalRunSlots }) }, true
	case <-ctx.Done():
		return nil, false
	}
}

type Status string

const (
	Queued      Status = "queued"
	Running     Status = "running"
	WaitingTool Status = "waiting_tool"
	Completed   Status = "completed"
	Failed      Status = "failed"
	Stopped     Status = "stopped"
	Interrupted Status = "interrupted"
)

type Event struct {
	RunID            string                 `json:"run_id"`
	AttemptID        string                 `json:"attempt_id"`
	ProviderID       string                 `json:"provider_id"`
	Sequence         uint64                 `json:"sequence"`
	ProviderSequence uint64                 `json:"provider_sequence,omitempty"`
	Type             agentchat.RunEventType `json:"type"`
	Payload          []byte                 `json:"payload,omitempty"`
	At               time.Time              `json:"at"`
}

type Record struct {
	Spec        agentchat.RunSpec       `json:"spec"`
	Status      Status                  `json:"status"`
	Attempt     int                     `json:"attempt"`
	History     []agentchat.HistoryItem `json:"history"`
	Events      []Event                 `json:"events,omitempty"`
	LastError   string                  `json:"last_error,omitempty"`
	RetrySafe   bool                    `json:"retry_safe,omitempty"`
	CreatedAt   time.Time               `json:"created_at"`
	UpdatedAt   time.Time               `json:"updated_at"`
	FinishedAt  *time.Time              `json:"finished_at,omitempty"`
	WorkerEpoch string                  `json:"worker_epoch,omitempty"`
	Continuation *agentchat.TurnContinuation `json:"continuation,omitempty"`
}

type Executor interface {
	Execute(context.Context, agentchat.RunSpec, []agentchat.HistoryItem, agentchat.EventSink) error
	Cancel(context.Context, agentchat.RunSpec) error
}

// WaitingToolResolver optionally resolves a read-only tool using the already
// persisted run record. A nil result leaves the run awaiting manual action.
// A resolver must not grant Patch approval or replay interrupted operations.
type WaitingToolResolver interface {
	ResolveWaitingTool(context.Context, Record) ([]agentchat.HistoryItem, string, error)
}

// CompletedTurnResolver supplies a bounded, server-authorized continuation
// for a confirmed provider turn. It does not accept a task or a tool result.
type CompletedTurnResolver interface {
	ResolveCompletedTurn(context.Context, Record) (TurnDecision, error)
}

type TurnDecision struct {
	Progress *agentchat.TurnContinuation
	Next []agentchat.HistoryItem
	SnapshotID string
}

type RetrySafeError struct{ Err error }

func (e RetrySafeError) Error() string {
	if e.Err == nil {
		return "safe pre-execution failure"
	}
	return e.Err.Error()
}
func (e RetrySafeError) Unwrap() error { return e.Err }

func SafeToRetry(err error) bool {
	var safe RetrySafeError
	return errors.As(err, &safe)
}

type Manager struct {
	mu        sync.RWMutex
	dir       string
	executor  Executor
	epoch     string
	queue     chan string
	closed    chan struct{}
	ctx       context.Context
	cancelAll context.CancelFunc
	closeOnce sync.Once
	runs      map[string]Record
	cancel    map[string]context.CancelFunc
	waiting   map[string]chan struct{}
	scheduled map[string]bool
	workers   sync.WaitGroup
}

func NewManager(dir string, concurrency int, executor Executor) (*Manager, error) {
	if dir == "" || !filepath.IsAbs(dir) || executor == nil {
		return nil, errors.New("absolute run directory and executor are required")
	}
	if concurrency < 1 || concurrency > MaxConcurrentRuns {
		return nil, fmt.Errorf("run concurrency must be between 1 and %d", MaxConcurrentRuns)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("unsafe run journal directory")
	}
	epoch, err := newID()
	if err != nil {
		return nil, err
	}
	managerCtx, cancelAll := context.WithCancel(context.Background())
	m := &Manager{dir: filepath.Clean(dir), executor: executor, epoch: epoch, queue: make(chan string, maxStoredRuns), closed: make(chan struct{}), ctx: managerCtx, cancelAll: cancelAll, runs: map[string]Record{}, cancel: map[string]context.CancelFunc{}, waiting: map[string]chan struct{}{}, scheduled: map[string]bool{}}
	if err := m.load(); err != nil {
		return nil, err
	}
	for i := 0; i < concurrency; i++ {
		m.workers.Add(1)
		go m.worker()
	}
	for _, record := range m.List() {
		if record.Status != Queued {
			continue
		}
		m.mu.Lock()
		m.scheduled[record.Spec.RunID] = true
		m.mu.Unlock()
		m.queue <- record.Spec.RunID
	}
	return m, nil
}

func NewID() (string, error) { return newID() }

func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func (m *Manager) Submit(spec agentchat.RunSpec, history []agentchat.HistoryItem) (Record, error) {
	if err := spec.Validate(); err != nil {
		return Record{}, err
	}
	if len(history) > 512 {
		return Record{}, errors.New("run history exceeds limit")
	}
	for _, item := range history {
		if len(item.Content)+len(item.Arguments) > maxEventPayload {
			return Record{}, errors.New("run history item exceeds limit")
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.runs[spec.RunID]; exists {
		return Record{}, errors.New("run ID already exists")
	}
	if len(m.runs) >= maxStoredRuns {
		return Record{}, errors.New("run journal capacity reached")
	}
	now := time.Now().UTC()
	record := Record{Spec: spec, Status: Queued, Attempt: 1, History: append([]agentchat.HistoryItem(nil), history...), CreatedAt: now, UpdatedAt: now}
	if err := m.saveLocked(record); err != nil {
		return Record{}, err
	}
	m.runs[spec.RunID] = record
	m.scheduled[spec.RunID] = true
	select {
	case m.queue <- spec.RunID:
		return record, nil
	default:
		delete(m.scheduled, spec.RunID)
		delete(m.runs, spec.RunID)
		_ = os.Remove(m.file(spec.RunID))
		return Record{}, errors.New("run queue is full")
	}
}

func (m *Manager) IsCurrentEpoch(runID string) bool {
	record, ok := m.Get(runID)
	return ok && record.WorkerEpoch == m.epoch && (record.Status == Running || record.Status == WaitingTool)
}

func (m *Manager) Get(runID string) (Record, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	record, ok := m.runs[runID]
	return cloneRecord(record), ok
}

func (m *Manager) List() []Record {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Record, 0, len(m.runs))
	for _, record := range m.runs {
		out = append(out, cloneRecord(record))
	}
	return out
}

func (m *Manager) Stop(ctx context.Context, runID string) error {
	m.mu.Lock()
	record, ok := m.runs[runID]
	if !ok {
		m.mu.Unlock()
		return os.ErrNotExist
	}
	if record.Status == Completed || record.Status == Failed || record.Status == Stopped || record.Status == Interrupted {
		m.mu.Unlock()
		return errors.New("run is already terminal")
	}
	if cancel := m.cancel[runID]; cancel != nil {
		cancel()
	}
	record.Status = Stopped
	now := time.Now().UTC()
	record.UpdatedAt, record.FinishedAt = now, &now
	if err := m.saveLocked(record); err != nil {
		m.mu.Unlock()
		return err
	}
	m.runs[runID] = record
	wait := m.waiting[runID]
	m.mu.Unlock()
	if wait != nil {
		select {
		case wait <- struct{}{}:
		default:
		}
	}
	return m.executor.Cancel(ctx, record.Spec)
}

func (m *Manager) ResumeQueued(runID string) (Record, error) {
	m.mu.Lock()
	record, ok := m.runs[runID]
	if !ok {
		m.mu.Unlock()
		return Record{}, os.ErrNotExist
	}
	if record.Status != Queued {
		m.mu.Unlock()
		return Record{}, errors.New("run is not queued")
	}
	if m.scheduled[runID] {
		m.mu.Unlock()
		return cloneRecord(record), nil
	}
	m.scheduled[runID] = true
	m.mu.Unlock()
	select {
	case m.queue <- runID:
		return cloneRecord(record), nil
	case <-m.closed:
		m.mu.Lock()
		delete(m.scheduled, runID)
		m.mu.Unlock()
		return Record{}, errors.New("run manager is closed")
	default:
		m.mu.Lock()
		delete(m.scheduled, runID)
		m.mu.Unlock()
		return Record{}, errors.New("run queue is full")
	}
}

func (m *Manager) Retry(runID string) (Record, error) { return m.retry(runID, false) }

func (m *Manager) ResumeAfterReview(runID string, confirmedNoSideEffect bool) (Record, error) {
	if !confirmedNoSideEffect {
		return Record{}, errors.New("explicit confirmation that prior side effects were inspected is required")
	}
	return m.retry(runID, true)
}

func (m *Manager) ContinueWithToolResults(runID string, results []agentchat.HistoryItem, snapshotID string) (Record, error) {
	return m.continueWithToolResults(runID, results, snapshotID, "",nil)
}

// A nonempty expectedAttempt identifies an automatic, still-live continuation.
// It must never resurrect a stopped/interrupted run or continue a newer attempt.
func (m *Manager) continueWithToolResults(runID string, results []agentchat.HistoryItem, snapshotID, expectedAttempt string, terminalErr error) (Record, error) {
	if snapshotID == "" || len(snapshotID) > 256 || len(results) == 0 {
		return Record{}, errors.New("tool continuation requires result items and a new context snapshot")
	}
	m.mu.Lock()
	record, ok := m.runs[runID]
	if !ok {
		m.mu.Unlock()
		return Record{}, os.ErrNotExist
	}
	wait := m.waiting[runID]
	waitingActive := record.Status == WaitingTool && wait != nil
	resumeAfterStop := (record.Status == Stopped || record.Status == Interrupted) && !m.scheduled[runID]
	if expectedAttempt != "" && (!waitingActive || record.Spec.AttemptID != expectedAttempt) {
		m.mu.Unlock()
		return Record{}, errors.New("automatic tool continuation no longer owns the active attempt")
	}
	if !waitingActive && !resumeAfterStop {
		m.mu.Unlock()
		return Record{}, errors.New("run has no safe tool continuation state")
	}
	select {
	case <-m.closed:
		m.mu.Unlock()
		return Record{}, errors.New("run manager is closed")
	default:
	}
	pending := map[string]bool{}
	for _, item := range record.History {
		if item.Type == "function_call" {
			pending[item.CallID] = true
		}
		if item.Type == "function_call_output" {
			delete(pending, item.CallID)
		}
	}
	seen := map[string]bool{}
	for _, result := range results {
		if result.Type != "function_call_output" || !validSnapshotHistoryItem(result) || !pending[result.CallID] || seen[result.CallID] {
			m.mu.Unlock()
			return Record{}, errors.New("tool result does not match a pending run call")
		}
		seen[result.CallID] = true
	}
	if len(seen) != len(pending) {
		m.mu.Unlock()
		return Record{}, errors.New("all pending tool calls must be reconciled before continuation")
	}
	attemptID, err := newID()
	if err != nil {
		m.mu.Unlock()
		return Record{}, err
	}
	idempotencyKey, err := newID()
	if err != nil {
		m.mu.Unlock()
		return Record{}, err
	}
	for _, result := range results {
		if len(record.History) >= 256 {
			m.mu.Unlock()
			return Record{}, errors.New("run history capacity reached")
		}
		record.History = append(record.History, result)
		payload, _ := json.Marshal(map[string]string{"call_id": result.CallID, "status": "reconciled"})
		if len(record.Events) >= maxRunEvents {
			m.mu.Unlock()
			return Record{}, errors.New("run event capacity reached")
		}
		record.Events = append(record.Events, Event{RunID: runID, AttemptID: record.Spec.AttemptID, ProviderID: record.Spec.ProviderID, Sequence: uint64(len(record.Events) + 1), Type: agentchat.RunEventToolResult, Payload: payload, At: time.Now().UTC()})
	}
	record.Spec.ContextSnapshotID = snapshotID
	if terminalErr!=nil {
		now:=time.Now().UTC()
		record.Status,record.RetrySafe,record.LastError=Failed,false,safeRunError(terminalErr)
		record.FinishedAt=&now;record.UpdatedAt=now
		if err:=m.saveLocked(record);err!=nil{m.mu.Unlock();return Record{},err}
		m.runs[runID]=record
		m.mu.Unlock()
		return cloneRecord(record),nil
	}
	record.Spec.AttemptID, record.Spec.IdempotencyKey = attemptID, idempotencyKey
	record.Attempt++
	record.Status, record.RetrySafe, record.LastError = Queued, false, ""
	record.WorkerEpoch = ""
	record.FinishedAt = nil
	record.UpdatedAt = time.Now().UTC()
	if err := m.saveLocked(record); err != nil {
		m.mu.Unlock()
		return Record{}, err
	}
	m.runs[runID] = record
	if waitingActive {
		// Publish the wake-up before releasing the mutex; the worker can
		// observe the queued state and drain this exact signal atomically.
		select {
		case wait <- struct{}{}:
		default:
		}
		m.mu.Unlock()
		return cloneRecord(record), nil
	}
	m.scheduled[runID] = true
	select {
	case m.queue <- runID:
		m.mu.Unlock()
		return cloneRecord(record), nil
	default:
		delete(m.scheduled, runID)
		m.mu.Unlock()
		return cloneRecord(record), errors.New("run continuation persisted but queue is full; explicitly resume the queued run later")
	}
}

func (m *Manager) retry(runID string, reviewed bool) (Record, error) {
	m.mu.Lock()
	record, ok := m.runs[runID]
	if !ok {
		m.mu.Unlock()
		return Record{}, os.ErrNotExist
	}
	safeFailure := record.Status == Failed && record.RetrySafe
	reviewedUncertain := reviewed && (record.Status == Interrupted || record.Status == Failed || record.Status == Stopped)
	if !safeFailure && !reviewedUncertain {
		m.mu.Unlock()
		return Record{}, errors.New("run is not proven safe to retry; explicit review is required")
	}
	attemptID, err := newID()
	if err != nil {
		m.mu.Unlock()
		return Record{}, err
	}
	idem, err := newID()
	if err != nil {
		m.mu.Unlock()
		return Record{}, err
	}
	record.Spec.AttemptID = attemptID
	record.Spec.IdempotencyKey = idem
	record.Attempt++
	record.Status = Queued
	record.LastError = ""
	record.WorkerEpoch = ""
	record.FinishedAt = nil
	record.UpdatedAt = time.Now().UTC()
	if err := m.saveLocked(record); err != nil {
		m.mu.Unlock()
		return Record{}, err
	}
	m.runs[runID] = record
	m.scheduled[runID] = true
	m.mu.Unlock()
	select {
	case m.queue <- runID:
		return cloneRecord(record), nil
	case <-m.closed:
		m.mu.Lock()
		delete(m.scheduled, runID)
		record.Status = Failed
		record.LastError = "run manager closed before queue admission"
		_ = m.saveLocked(record)
		m.runs[runID] = record
		m.mu.Unlock()
		return Record{}, errors.New("run manager is closed")
	default:
		m.mu.Lock()
		delete(m.scheduled, runID)
		record.Status = Failed
		record.LastError = "run queue is full"
		_ = m.saveLocked(record)
		m.runs[runID] = record
		m.mu.Unlock()
		return Record{}, errors.New("run queue is full")
	}
}

func (m *Manager) Close() {
	m.closeOnce.Do(func() { close(m.closed); m.cancelAll() })
	m.mu.Lock()
	for _, cancel := range m.cancel {
		cancel()
	}
	m.mu.Unlock()
	m.workers.Wait()
}

func (m *Manager) worker() {
	defer m.workers.Done()
	for {
		select {
		case <-m.closed:
			return
		case runID := <-m.queue:
			select {
			case <-m.closed:
				return
			default:
			}
			runSlotCtx, releaseGlobal, admitted := AcquireGlobalSlotContext(m.ctx)
			if !admitted {
				return
			}
			select {
			case <-m.closed:
				releaseGlobal()
				return
			default:
			}
			wait := make(chan struct{}, 1)
			m.mu.Lock()
			m.waiting[runID] = wait
			m.mu.Unlock()
			for {
				m.execute(runID, runSlotCtx)
				record, ok := m.Get(runID)
				if ok && record.Status == Queued {
					// Automatic native results may already be persisted before
					// execute returns. Do not drop their queued continuation.
					select {
					case <-wait:
					default:
					}
					continue
				}
				if !ok || record.Status != WaitingTool {
					break
				}
				select {
				case <-wait:
					select {
					case <-m.closed:
						m.markInterrupted(runID)
						break
					default:
						continue
					}
				case <-m.closed:
					m.markInterrupted(runID)
				}
				break
			}
			m.mu.Lock()
			delete(m.waiting, runID)
			delete(m.scheduled, runID)
			m.mu.Unlock()
			releaseGlobal()
		}
	}
}

func (m *Manager) markInterrupted(runID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	record, ok := m.runs[runID]
	if !ok || (record.Status != Running && record.Status != WaitingTool) {
		return
	}
	record.Status, record.RetrySafe = Interrupted, false
	record.LastError = "TaskDeck stopped while run was active; inspect side effects before explicit resume"
	now := time.Now().UTC()
	record.UpdatedAt, record.FinishedAt = now, &now
	_ = m.saveLocked(record)
	m.runs[runID] = record
}

func (m *Manager) execute(runID string, slotContext context.Context) {
	m.mu.Lock()
	record, ok := m.runs[runID]
	if !ok || record.Status != Queued {
		m.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(slotContext)
	m.cancel[runID] = cancel
	record.Status = Running
	record.WorkerEpoch = m.epoch
	record.UpdatedAt = time.Now().UTC()
	if err := m.saveLocked(record); err != nil {
		record.Status, record.LastError = Failed, "run state could not be persisted"
		cancel()
		delete(m.cancel, runID)
		m.runs[runID] = record
		m.mu.Unlock()
		return
	}
	m.runs[runID] = record
	spec := record.Spec
	history := append([]agentchat.HistoryItem(nil), record.History...)
	m.mu.Unlock()

	err := m.executor.Execute(ctx, spec, history, runEventSink{manager: m, runID: runID})
	if resolver, supported := m.executor.(WaitingToolResolver); supported && err == nil && ctx.Err() == nil {
		if pending, exists := m.Get(runID); exists && pending.Status == WaitingTool && pending.Spec.AttemptID == spec.AttemptID {
			results, snapshotID, resolveErr := resolver.ResolveWaitingTool(ctx, pending)
			if len(results)!=0&&ctx.Err()==nil {
				_,err=m.continueWithToolResults(runID,results,snapshotID,spec.AttemptID,resolveErr)
				if err==nil&&resolveErr!=nil{err=resolveErr}
			}else if resolveErr!=nil{err=resolveErr}
		}
	}
	var decision TurnDecision
	if resolver, supported := m.executor.(CompletedTurnResolver); supported && err == nil && ctx.Err() == nil {
		if current, exists := m.Get(runID); exists && current.Status == Running && current.Spec.AttemptID == spec.AttemptID {
			decision, err = resolver.ResolveCompletedTurn(ctx, current)
		}
	}
	canContinue := ctx.Err() == nil
	cancel()
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.cancel, runID)
	record = m.runs[runID]
	if record.Status == Stopped || record.Status==Failed || (record.Status == Queued && record.Spec.AttemptID != spec.AttemptID) {
		return
	}
	if record.Status == WaitingTool && err == nil {
		record.UpdatedAt = time.Now().UTC()
		_ = m.saveLocked(record)
		m.runs[runID] = record
		return
	}
	if decision.Progress != nil && agentchat.ValidTurnContinuation(decision.Progress) {
		record.Continuation = decision.Progress
	}
	if err == nil && len(decision.Next) != 0 && canContinue && record.Status == Running &&
		record.Spec.AttemptID == spec.AttemptID {
		// The conversation may have received another run while the provider
		// was answering. Never choose an ambiguous automatic continuation.
		otherRun := false
		for _, other := range m.runs {
			if other.Spec.RunID != runID && other.Spec.ConversationID == spec.ConversationID &&
				(other.CreatedAt.After(record.CreatedAt) || other.Status == Queued ||
					other.Status == Running || other.Status == WaitingTool) {
				otherRun = true
				break
			}
		}
		if otherRun {
			if record.Continuation != nil {
				record.Continuation.Reason = "another_run_owns_conversation"
			}
		} else if queued, queueErr := m.queueCompletedTurnLocked(record, decision); queueErr == nil {
			m.runs[runID] = queued
			return
		} else {
			err = queueErr
		}
	}
	now := time.Now().UTC()
	record.UpdatedAt, record.FinishedAt = now, &now
	if err != nil {
		record.Status = Failed
		record.RetrySafe = SafeToRetry(err)
		record.LastError = safeRunError(err)
	} else {
		record.Status = Completed
		record.RetrySafe = false
	}
	_ = m.saveLocked(record)
	m.runs[runID] = record
}

type runEventSink struct {
	manager *Manager
	runID   string
}

func (s runEventSink) Publish(ctx context.Context, event agentchat.RunEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(event.Payload) > maxEventPayload {
		return errors.New("run event payload exceeds limit")
	}
	if event.RunID != "" && event.RunID != s.runID {
		return errors.New("run event belongs to a different run")
	}
	return s.manager.appendEvent(s.runID, event)
}

func (m *Manager) appendEvent(runID string, event agentchat.RunEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	record, ok := m.runs[runID]
	if !ok || (record.Status != Running && record.Status != WaitingTool) {
		return errors.New("run is not active")
	}
	if len(record.Events) >= maxRunEvents {
		return errors.New("run event limit reached")
	}
	if event.AttemptID != "" && event.AttemptID != record.Spec.AttemptID || event.ProviderID != "" && event.ProviderID != record.Spec.ProviderID {
		return errors.New("run event does not match immutable attempt binding")
	}
	if event.Type == agentchat.RunEventToolCall {
		var call agentchat.FunctionCall
		if err := json.Unmarshal(event.Payload, &call); err != nil || call.CallID == "" || call.Name == "" || len(call.Arguments) == 0 || !json.Valid([]byte(call.Arguments)) {
			return errors.New("provider tool-call event is malformed")
		}
		if len(record.History) >= 256 {
			return errors.New("run history capacity reached")
		}
		for _, previous := range record.History {
			if previous.Type == "function_call" && previous.CallID == call.CallID {
				return errors.New("duplicate provider tool-call ID")
			}
		}
		record.History = append(record.History, agentchat.HistoryItem{Type: "function_call", CallID: call.CallID,
			Name: call.Name, Arguments: call.Arguments, OriginRunID: runID, ProviderID: record.Spec.ProviderID,
			ModelID: record.Spec.ModelID, ParentTurnID: record.Spec.ParentTurnID})
	}
	record.Events = append(record.Events, Event{
		RunID: runID, AttemptID: record.Spec.AttemptID, ProviderID: record.Spec.ProviderID,
		Sequence: uint64(len(record.Events) + 1), ProviderSequence: event.Sequence,
		Type: event.Type, Payload: append([]byte(nil), event.Payload...), At: time.Now().UTC(),
	})
	record.UpdatedAt = time.Now().UTC()
	switch event.Type {
	case agentchat.RunEventToolCall:
		record.Status = WaitingTool
	case agentchat.RunEventToolResult, agentchat.RunEventTextDelta:
		record.Status = Running
	}
	if err := m.saveLocked(record); err != nil {
		return err
	}
	m.runs[runID] = record
	return nil
}

// Called only by execute while holding m.mu. The worker which owns the
// current slot observes the new queued attempt and continues it. No second
// worker, provider selection, or legacy session request is created.
func (m *Manager) queueCompletedTurnLocked(record Record, decision TurnDecision) (Record, error) {
	select {
	case <-m.closed:
		return Record{}, errors.New("run manager is closed")
	default:
	}
	if decision.Progress == nil || !agentchat.ValidTurnContinuation(decision.Progress) ||
		decision.SnapshotID == "" || len(decision.Next) != 2 ||
		decision.Next[0].Type != "assistant" || decision.Next[1].Type != "user" ||
		len(record.History)+len(decision.Next) > 256 {
		return Record{}, errors.New("invalid automatic continuation")
	}
	for _, item := range decision.Next {
		if !validSnapshotHistoryItem(item) || item.OriginRunID != record.Spec.RunID ||
			item.ProviderID != record.Spec.ProviderID || item.ModelID != record.Spec.ModelID ||
			item.ParentTurnID != record.Spec.ParentTurnID {
			return Record{}, errors.New("automatic continuation history is not run-bound")
		}
	}
	attemptID, err := newID()
	if err != nil { return Record{}, err }
	key, err := newID()
	if err != nil { return Record{}, err }
	record.History = append(append([]agentchat.HistoryItem(nil), record.History...), decision.Next...)
	record.Spec.ContextSnapshotID = decision.SnapshotID
	record.Spec.AttemptID, record.Spec.IdempotencyKey = attemptID, key
	record.Attempt++
	record.Status, record.LastError, record.RetrySafe = Queued, "", false
	record.WorkerEpoch, record.FinishedAt = "", nil
	record.UpdatedAt = time.Now().UTC()
	if err := m.saveLocked(record); err != nil { return Record{}, err }
	return record, nil
}

func cloneRecord(record Record) Record {
	if record.Continuation != nil {
		copy := *record.Continuation
		copy.EvidenceRefs = append([]string(nil), copy.EvidenceRefs...)
		if copy.NextRetryAt != nil {
			at := *copy.NextRetryAt
			copy.NextRetryAt = &at
		}
		record.Continuation = &copy
	}
	record.History = append([]agentchat.HistoryItem(nil), record.History...)
	record.Events = append([]Event(nil), record.Events...)
	for i := range record.Events {
		record.Events[i].Payload = append([]byte(nil), record.Events[i].Payload...)
	}
	if record.FinishedAt != nil {
		at := *record.FinishedAt
		record.FinishedAt = &at
	}
	return record
}

func safeRunError(error) string { return "provider execution failed; inspect redacted diagnostics" }
