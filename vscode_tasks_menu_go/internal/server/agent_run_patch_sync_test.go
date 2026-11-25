package server

import (
    "testing"

    "bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestAgentRunHistorySnapshotIsStale(t *testing.T) {
    tests := []struct {
        name      string
        session   []agentchat.HistoryItem
        run       []agentchat.HistoryItem
        runID     string
        wantStale bool
    }{
        {
            name:  "empty session is not stale",
            run:   []agentchat.HistoryItem{{Type: "user", Content: "prompt"}},
            runID: "run-a",
        },
        {
            name:    "own final answer is not stale",
            session: []agentchat.HistoryItem{{Type: "assistant", Content: "done", OriginRunID: "run-a"}},
            run:     []agentchat.HistoryItem{{Type: "user", Content: "prompt"}},
            runID:   "run-a",
        },
        {
            name:      "superseding run final answer is stale",
            session:   []agentchat.HistoryItem{{Type: "assistant", Content: "newer answer", OriginRunID: "run-b"}},
            run:       []agentchat.HistoryItem{{Type: "user", Content: "prompt"}},
            runID:     "run-a",
            wantStale: true,
        },
        {
            name:      "superseding tool result is stale",
            session:   []agentchat.HistoryItem{{Type: "function_call_output", CallID: "c1", Content: "result", OriginRunID: "run-b"}},
            run:       []agentchat.HistoryItem{{Type: "user", Content: "prompt"}},
            runID:     "run-a",
            wantStale: true,
        },
        {
            name:    "prior run already in snapshot is not stale",
            session: []agentchat.HistoryItem{{Type: "assistant", Content: "older answer", OriginRunID: "run-0"}},
            run:     []agentchat.HistoryItem{{Type: "assistant", Content: "older answer", OriginRunID: "run-0"}, {Type: "user", Content: "prompt"}},
            runID:   "run-a",
        },
        {
            name:    "foreign user prompt alone is not stale",
            session: []agentchat.HistoryItem{{Type: "user", Content: "newer prompt", OriginRunID: "run-b"}},
            run:     []agentchat.HistoryItem{{Type: "user", Content: "prompt"}},
            runID:   "run-a",
        },
    }
    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            got := agentRunHistorySnapshotIsStale(tc.session, tc.run, tc.runID)
            if got != tc.wantStale {
                t.Fatalf("stale=%v, want %v", got, tc.wantStale)
            }
        })
    }
}
