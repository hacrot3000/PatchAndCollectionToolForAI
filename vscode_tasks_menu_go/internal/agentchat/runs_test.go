package agentchat

import "testing"

func TestRunSpecRequiresCompleteImmutableBinding(t *testing.T) {
	spec := RunSpec{
		RunID: "run-102", ConversationID: "conv-1", ParentTurnID: "turn-101",
		OwnerWorkspaceID: "owner-ws", ExecutionWorkspaceID: "exec-ws",
		ProviderID: "gpt", ModelID: "model-x", ProviderConfigVersion: "v1",
		ContextSnapshotID: "ctx-102", AttemptID: "run-102-a1", IdempotencyKey: "req-102",
	}
	if err := spec.Validate(); err != nil {
		t.Fatalf("valid binding rejected: %v", err)
	}
	spec.ProviderID = " \n "
	if err := spec.Validate(); err == nil {
		t.Fatal("empty/unsafe provider binding accepted")
	}
}

func TestHistoryItemCarriesRunProvenance(t *testing.T) {
	item := HistoryItem{Type: "assistant", Content: "answer", OriginRunID: "run-102", ProviderID: "gpt", ModelID: "model-x", ParentTurnID: "turn-101"}
	if err := validHistoryItem(item); err != nil {
		t.Fatalf("provenance-bearing visible history rejected: %v", err)
	}
}
