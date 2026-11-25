package agentchat

import (
	"encoding/json"
	"testing"
)

func collectionProofTestSpec() RunSpec {
	return RunSpec{RunID: "run-collect-1", ConversationID: "conv-1", OwnerWorkspaceID: "owner-1",
		ExecutionWorkspaceID: "exec-1", ProviderID: "provider-1", ModelID: "model-1",
		ProviderConfigVersion: "config-1", ContextSnapshotID: "snap-1",
		AttemptID: "attempt-1", IdempotencyKey: "key-1"}
}

func collectionProofHistory(callID, runID, providerID, modelID, filePath, content string) []HistoryItem {
	requestBytes, _ := json.Marshal(map[string]any{"actions": []map[string]any{{"type": "read", "path": filePath, "start_line": 1}}})
	outputBytes, _ := json.Marshal(map[string]any{"response_truncated": false, "results": []map[string]any{{
		"type": "read", "output_truncated": false, "files": []map[string]any{{
			"path": filePath, "start_line": "1", "total_lines": "2", "content": content,
			"error": "", "truncated": "false", "content_truncated": "false", "next_line": "",
		}},
	}}})
	request, output := string(requestBytes), string(outputBytes)
	return []HistoryItem{
		{Type: "user", Content: "read the file"},
		{Type: "function_call", CallID: callID, Name: "taskdeck_collect_workspace", Arguments: request,
			OriginRunID: runID, ProviderID: providerID, ModelID: modelID},
		{Type: "function_call_output", CallID: callID, Content: output,
			OriginRunID: runID, ProviderID: providerID, ModelID: modelID},
	}
}

func TestVerifyNativeCollectionReadProofAcceptsExactFullRead(t *testing.T) {
	spec := collectionProofTestSpec()
	history := collectionProofHistory("c1", spec.RunID, spec.ProviderID, spec.ModelID, "src/main.go", "a\nb\n")
	proof, err := verifyNativeCollectionReadProof(history, spec, "c1", "src/main.go")
	if err != nil {
		t.Fatal(err)
	}
	if proof.Path != "src/main.go" || proof.CallID != "c1" || proof.OriginRunID != spec.RunID {
		t.Fatalf("unexpected proof identity: %+v", proof)
	}
	if proof.RequestDigest != taskPlanDigest(history[1].Arguments) || proof.ResultDigest != taskPlanDigest(history[2].Content) {
		t.Fatalf("proof digests are not canonical: %+v", proof)
	}
}

func TestVerifyNativeCollectionReadProofRejectsPartialAndWrongPath(t *testing.T) {
	spec := collectionProofTestSpec()
	partial := collectionProofHistory("c1", spec.RunID, spec.ProviderID, spec.ModelID, "src/main.go", "b\n")
	partial[2].Content = `{"response_truncated":false,"results":[{"type":"read","output_truncated":false,"files":[{"path":"src/main.go","start_line":"2","total_lines":"2","content":"b\n","error":"","truncated":"false","content_truncated":"false","next_line":""}]}]}`
	if _, err := verifyNativeCollectionReadProof(partial, spec, "c1", "src/main.go"); err == nil {
		t.Fatal("a tail-only read was accepted as a complete full-file read")
	}
	full := collectionProofHistory("c1", spec.RunID, spec.ProviderID, spec.ModelID, "src/main.go", "a\nb\n")
	if _, err := verifyNativeCollectionReadProof(full, spec, "c1", "src/other.go"); err == nil {
		t.Fatal("a read of a different path was accepted")
	}
}

func TestVerifyNativeCollectionReadProofRejectsWrongRunAndAmbiguity(t *testing.T) {
	spec := collectionProofTestSpec()
	history := collectionProofHistory("c1", "other-run", spec.ProviderID, spec.ModelID, "src/main.go", "a\nb\n")
	if _, err := verifyNativeCollectionReadProof(history, spec, "c1", "src/main.go"); err == nil {
		t.Fatal("a call from another run was accepted")
	}
	dup := collectionProofHistory("c1", spec.RunID, spec.ProviderID, spec.ModelID, "src/main.go", "a\nb\n")
	dup = append(dup, dup[1], dup[2])
	if _, err := verifyNativeCollectionReadProof(dup, spec, "c1", "src/main.go"); err == nil {
		t.Fatal("an ambiguous duplicate call was accepted")
	}
}

func TestVerifyNativeCollectionReadProofUnsafePath(t *testing.T) {
	spec := collectionProofTestSpec()
	for _, badPath := range []string{"../secret.txt", "/etc/passwd", "src\\evil", "src/./file"} {
		history := collectionProofHistory("c1", spec.RunID, spec.ProviderID, spec.ModelID, badPath, "x\n")
		if _, err := verifyNativeCollectionReadProof(history, spec, "c1", badPath); err == nil {
			t.Fatalf("unsafe proof path accepted: %q", badPath)
		}
	}
}

func TestVerifyNativeCollectionReadProofAllowsExplicitEmptyFile(t *testing.T) {
	spec := collectionProofTestSpec()
	call := HistoryItem{Type: "function_call", CallID: "c1", Name: "taskdeck_collect_workspace",
		Arguments: `{"actions":[{"type":"read","path":"empty.txt","start_line":1}]}`,
		OriginRunID: spec.RunID, ProviderID: spec.ProviderID, ModelID: spec.ModelID}
	output := HistoryItem{Type: "function_call_output", CallID: "c1",
		Content: `{"response_truncated":false,"results":[{"type":"read","output_truncated":false,"files":[{"path":"empty.txt","start_line":"1","total_lines":"0","content":"","error":"","truncated":"false","content_truncated":"false","next_line":""}]}]}`,
		OriginRunID: spec.RunID, ProviderID: spec.ProviderID, ModelID: spec.ModelID}
	if _, err := verifyNativeCollectionReadProof([]HistoryItem{call, output}, spec, "c1", "empty.txt"); err != nil {
		t.Fatalf("explicit complete empty-file read was rejected: %v", err)
	}
}
