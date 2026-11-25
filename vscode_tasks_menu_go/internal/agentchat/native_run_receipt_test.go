package agentchat

import (
	"strings"
	"testing"
)

func nativeReceiptTestSpec() RunSpec {
	return RunSpec{RunID: "run-1", ConversationID: "conv-1", OwnerWorkspaceID: "owner-1",
		ExecutionWorkspaceID: "exec-1", ProviderID: "provider-1", ModelID: "model-1",
		ProviderConfigVersion: "config-1", ContextSnapshotID: "snap-1",
		AttemptID: "attempt-1", IdempotencyKey: "key-1"}
}

func nativeReceiptTestCall() ToolCallRecord {
	return ToolCallRecord{AgentRunID: "run-1", CallID: "call-1", Name: "taskdeck_submit_patch",
		Status: "done", FeedbackSent: true, FeedbackStatus: "acknowledged",
		ArgumentsSHA256: strings.Repeat("a", 64),
		Result:          `{"status":"PASS","run_id":"native-1","rc":0}`,
		PatchSessionID:  "native-1", QueuePath: "patchs/patch.zip",
		PackageSHA256: strings.Repeat("b", 64), ExpectedHead: "deadbeef",
		ExpectedBranch: "main", WorkspaceStatusSHA256: strings.Repeat("c", 64)}
}

func TestNativeRunReceiptBuildAndVerify(t *testing.T) {
	spec, call := nativeReceiptTestSpec(), nativeReceiptTestCall()
	env, err := newNativeRunReceipt(spec, call, "project-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyNativeRunReceipt(env, spec, "project-1"); err != nil {
		t.Fatal(err)
	}
	again, err := newNativeRunReceipt(spec, call, "project-1")
	if err != nil || again.Digest != env.Digest {
		t.Fatalf("receipt digest is not deterministic: %v", err)
	}
}

func TestNativeRunReceiptRejectsUnverifiedCall(t *testing.T) {
	spec := nativeReceiptTestSpec()
	call := nativeReceiptTestCall()
	call.FeedbackStatus = "prepared"
	if _, err := newNativeRunReceipt(spec, call, "project-1"); err == nil {
		t.Fatal("unacknowledged native result produced a receipt")
	}
	call = nativeReceiptTestCall()
	call.AgentRunID = "other-run"
	if _, err := newNativeRunReceipt(spec, call, "project-1"); err == nil {
		t.Fatal("call from another run produced a receipt")
	}
}

func TestNativeRunReceiptFailsClosedOnTamperAndScope(t *testing.T) {
	spec := nativeReceiptTestSpec()
	env, err := newNativeRunReceipt(spec, nativeReceiptTestCall(), "project-1")
	if err != nil {
		t.Fatal(err)
	}
	tampered := env
	tampered.Payload.ResultDigest = strings.Repeat("d", 64)
	if err := verifyNativeRunReceipt(tampered, spec, "project-1"); err == nil {
		t.Fatal("tampered receipt passed")
	}
	wrongRun := spec
	wrongRun.RunID = "run-2"
	if err := verifyNativeRunReceipt(env, wrongRun, "project-1"); err == nil {
		t.Fatal("wrong run accepted")
	}
	if err := verifyNativeRunReceipt(env, spec, "other-project"); err == nil {
		t.Fatal("wrong project accepted")
	}
	badDigest := env
	badDigest.Digest = strings.Repeat("e", 64)
	if err := verifyNativeRunReceipt(badDigest, spec, "project-1"); err == nil {
		t.Fatal("forged digest accepted")
	}
}

func TestNativeRunReceiptLegacyCallWithoutRunBinding(t *testing.T) {
	spec, call := nativeReceiptTestSpec(), nativeReceiptTestCall()
	call.AgentRunID = ""
	env, err := newNativeRunReceipt(spec, call, "project-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyNativeRunReceipt(env, spec, "project-1"); err != nil {
		t.Fatal(err)
	}
}
