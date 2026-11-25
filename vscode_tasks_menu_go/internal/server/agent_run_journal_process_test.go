package server

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/agentrun"
)

const agentRunJournalHelperEnv = "TASKDECK_AGENT_RUN_JOURNAL_HELPER"

// TestAgentRunJournalHelperProcess is not a real test. It runs as a separate
// OS process so the parent can verify that a persisted Agent Run journal is
// readable by a different daemon instance.
func TestAgentRunJournalHelperProcess(t *testing.T) {
	if os.Getenv(agentRunJournalHelperEnv) != "1" {
		t.Skip("helper process")
	}
	workspace := os.Getenv("TASKDECK_AGENT_RUN_JOURNAL_WORKSPACE")
	runDir := os.Getenv("TASKDECK_AGENT_RUN_JOURNAL_DIR")
	owner := os.Getenv("TASKDECK_AGENT_RUN_JOURNAL_OWNER")
	if workspace == "" || runDir == "" || owner == "" {
		t.Fatal("helper process is missing its environment")
	}
	manager, err := agentrun.NewManager(runDir, 1, &journalHelperExecutor{})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	record, ok := manager.Get(os.Getenv("TASKDECK_AGENT_RUN_JOURNAL_RUN_ID"))
	if !ok {
		t.Fatal("persisted run was not recovered by a separate process")
	}
	payload, err := json.Marshal(map[string]any{
		"status": string(record.Status), "run_id": record.Spec.RunID,
		"conversation_id": record.Spec.ConversationID, "provider_id": record.Spec.ProviderID,
		"history_items": len(record.History),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stdout.Write(append(payload, '\n')); err != nil {
		t.Fatal(err)
	}
}

type journalHelperExecutor struct{}

func (*journalHelperExecutor) Execute(context.Context, agentchat.RunSpec, []agentchat.HistoryItem, agentchat.EventSink) error {
	return nil
}
func (*journalHelperExecutor) Cancel(context.Context, agentchat.RunSpec) error { return nil }

func TestAgentRunJournalIsRecoveredBySeparateProcess(t *testing.T) {
	s, session, provider := prepareRunAPIServer(t, false)
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	ownerWorkspaceID, err := agentrun.WorkspaceID(s.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	s.AgentRunExecutor = &journalHelperExecutor{}
	manager, err := s.agentRunManager("local")
	if err != nil {
		t.Fatal(err)
	}
	spec := runSpecFor(s, session, provider)
	spec.RunID = "cross-process-run"
	if _, err := manager.Submit(spec, []agentchat.HistoryItem{{Type: "user", Content: "durable prompt"}}); err != nil {
		t.Fatal(err)
	}
	waitForAgentRunStatus(t, manager, spec.RunID, agentrun.Completed)
	runDir, err := s.agentRunJournalDir("local")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(runDir, spec.RunID+".json")); err != nil {
		t.Fatalf("run journal file is missing: %v", err)
	}
	manager.Close()

	command := exec.Command(os.Args[0], "-test.run=TestAgentRunJournalHelperProcess", "-test.v")
	command.Env = append(os.Environ(),
		agentRunJournalHelperEnv+"=1",
		"TASKDECK_AGENT_RUN_JOURNAL_WORKSPACE="+s.Workspace,
		"TASKDECK_AGENT_RUN_JOURNAL_DIR="+runDir,
		"TASKDECK_AGENT_RUN_JOURNAL_OWNER="+ownerWorkspaceID,
		"TASKDECK_AGENT_RUN_JOURNAL_RUN_ID="+spec.RunID,
		"XDG_CONFIG_HOME="+configHome,
	)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("separate process could not recover the run journal: %v\n%s", err, output)
	}
	var recovered struct {
		Status         string `json:"status"`
		RunID          string `json:"run_id"`
		ConversationID string `json:"conversation_id"`
		ProviderID     string `json:"provider_id"`
		HistoryItems   int    `json:"history_items"`
	}
	line := helperJSONLine(string(output))
	if err := json.Unmarshal([]byte(line), &recovered); err != nil {
		t.Fatalf("separate process did not report the recovered run: %v\n%s", err, output)
	}
	if recovered.RunID != spec.RunID || recovered.ConversationID != session.ID || recovered.ProviderID != provider.ID || recovered.HistoryItems < 1 {
		t.Fatalf("separate process recovered the wrong run state: %+v", recovered)
	}
	if recovered.Status != string(agentrun.Completed) {
		t.Fatalf("separate process recovered status %q, want completed", recovered.Status)
	}
}

func TestAgentRunJournalRecoversInterruptedRunAcrossProcesses(t *testing.T) {
	s, session, provider := prepareRunAPIServer(t, false)
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	s.AgentRunExecutor = &journalHelperExecutor{}
	manager, err := s.agentRunManager("local")
	if err != nil {
		t.Fatal(err)
	}
	spec := runSpecFor(s, session, provider)
	spec.RunID = "cross-process-interrupted"
	if _, err := manager.Submit(spec, []agentchat.HistoryItem{{Type: "user", Content: "interrupted prompt"}}); err != nil {
		t.Fatal(err)
	}
	waitForAgentRunStatus(t, manager, spec.RunID, agentrun.Completed)
	runDir, err := s.agentRunJournalDir("local")
	if err != nil {
		t.Fatal(err)
	}
	manager.Close()

	// Simulate a daemon that died while the run was still active: the durable
	// journal still claims the run was running.
	journalPath := filepath.Join(runDir, spec.RunID+".json")
	body, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	var record map[string]any
	if err := json.Unmarshal(body, &record); err != nil {
		t.Fatal(err)
	}
	record["status"] = string(agentrun.Running)
	crashed, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(journalPath, append(crashed, '\n'), 0600); err != nil {
		t.Fatal(err)
	}

	command := exec.Command(os.Args[0], "-test.run=TestAgentRunJournalHelperProcess", "-test.v")
	command.Env = append(os.Environ(),
		agentRunJournalHelperEnv+"=1",
		"TASKDECK_AGENT_RUN_JOURNAL_WORKSPACE="+s.Workspace,
		"TASKDECK_AGENT_RUN_JOURNAL_DIR="+runDir,
		"TASKDECK_AGENT_RUN_JOURNAL_OWNER=helper",
		"TASKDECK_AGENT_RUN_JOURNAL_RUN_ID="+spec.RunID,
		"XDG_CONFIG_HOME="+configHome,
	)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("separate process could not recover the interrupted run: %v\n%s", err, output)
	}
	var recovered struct {
		Status       string `json:"status"`
		HistoryItems int    `json:"history_items"`
	}
	line := helperJSONLine(string(output))
	if err := json.Unmarshal([]byte(line), &recovered); err != nil {
		t.Fatalf("separate process did not report the recovered run: %v\n%s", err, output)
	}
	if recovered.Status != string(agentrun.Interrupted) {
		t.Fatalf("separate process recovered status %q, want interrupted after a daemon restart", recovered.Status)
	}
	if recovered.HistoryItems < 1 {
		t.Fatal("recovered interrupted run lost its durable history")
	}
}

// helperJSONLine extracts the single JSON object the helper process prints,
// ignoring the go test harness lines around it.
func helperJSONLine(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "{") && strings.HasSuffix(line, "}") {
			return line
		}
	}
	return strings.TrimSpace(output)
}
