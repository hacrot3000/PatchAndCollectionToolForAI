package server

import (
	"errors"

	"bletonfc/vscode_tasks_menu/internal/agentrun"
)

func (s *Server) resumeWorkflowForContinuedRun(owner, runID string) {
	store, err := s.agentWorkflowStore(owner)
	if err != nil { return }
	plans, err := store.List()
	if err != nil { return }
	for _, plan := range plans {
		for _, state := range plan.DAG.States {
			if state.RunID != runID || (state.Status != agentrun.TaskUnknown && state.Status != agentrun.TaskInterrupted) { continue }
			coordinator, err := s.workflowCoordinator(owner, plan.ID, false)
			if err == nil { _ = coordinator.resumeAfterToolContinuation(runID) }
			return
		}
	}
}

func (c *taskWorkflowCoordinator) resumeAfterToolContinuation(runID string) error {
	c.mu.Lock()
	if c.record.Status == agentrun.WorkflowStopped { c.mu.Unlock(); return errors.New("stopped workflow requires a new explicit plan review") }
	stateFound := false
	for taskID, state := range c.dag.Snapshot().States {
		if state.RunID != runID { continue }
		stateFound = true
		run, ok := c.manager.Get(runID)
		if !ok || (run.Status != agentrun.Queued && run.Status != agentrun.Running && run.Status != agentrun.WaitingTool) || hasUnresolvedRunToolCalls(run) {
			c.mu.Unlock()
			return errors.New("continued run is not safely resumable in workflow")
		}
		if _, err := c.dag.ResumeAfterToolContinuation(taskID); err != nil { c.mu.Unlock(); return err }
		c.record.DAG = c.dag.Snapshot()
		c.record.Status = agentrun.WorkflowRunning
		if err := c.persistLocked(); err != nil { c.mu.Unlock(); return err }
		shouldStart := !c.running
		if !shouldStart { select { case c.wake <- struct{}{}: default: } }
		c.mu.Unlock()
		if shouldStart { c.start() }
		return nil
	}
	c.mu.Unlock()
	if !stateFound { return errors.New("workflow task for continued run not found") }
	return errors.New("workflow run continuation failed")
}
