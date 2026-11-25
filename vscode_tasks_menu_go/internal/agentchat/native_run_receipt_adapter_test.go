package agentchat

import (
	"strings"
	"testing"
)

func TestAdoptNativeRunReceiptProducesStableNamespacedEvidence(t *testing.T) {
	spec := nativeReceiptTestSpec()
	env, err := newNativeRunReceipt(spec, nativeReceiptTestCall(), "project-1")
	if err != nil {
		t.Fatal(err)
	}
	first, err := adoptNativeRunReceipt(env, spec, "project-1")
	if err != nil {
		t.Fatal(err)
	}
	again, err := adoptNativeRunReceipt(env, spec, "project-1")
	if err != nil || again != first {
		t.Fatalf("receipt projection is not deterministic: %+v vs %+v, %v", first, again, err)
	}
	if first.Kind != "patch" || len(first.Ref) != 64 || first.Digest != env.Digest {
		t.Fatalf("unexpected evidence projection: %+v", first)
	}
	if strings.Trim(first.Ref, "0123456789abcdef") != "" {
		t.Fatalf("receipt reference is not a canonical digest: %q", first.Ref)
	}
	if !strings.Contains(first.Note, "independent tests are not implied") {
		t.Fatalf("evidence note overstates verification: %q", first.Note)
	}
}

func TestAdoptNativeRunReceiptRefusesTamperedOrWrongScope(t *testing.T) {
	spec := nativeReceiptTestSpec()
	env, err := newNativeRunReceipt(spec, nativeReceiptTestCall(), "project-1")
	if err != nil {
		t.Fatal(err)
	}
	tampered := env
	tampered.Payload.ResultDigest = strings.Repeat("d", 64)
	if _, err := adoptNativeRunReceipt(tampered, spec, "project-1"); err == nil {
		t.Fatal("tampered receipt was adopted")
	}
	wrongRun := spec
	wrongRun.RunID = "run-2"
	if _, err := adoptNativeRunReceipt(env, wrongRun, "project-1"); err == nil {
		t.Fatal("receipt from another run was adopted")
	}
	if _, err := adoptNativeRunReceipt(env, spec, "other-project"); err == nil {
		t.Fatal("receipt from another project was adopted")
	}
}

func TestAdoptNativeRunReceiptRefIsNamespacedPerRun(t *testing.T) {
	firstSpec := nativeReceiptTestSpec()
	firstCall := nativeReceiptTestCall()
	firstEnv, err := newNativeRunReceipt(firstSpec, firstCall, "project-1")
	if err != nil {
		t.Fatal(err)
	}
	// The same CallID bound to a different run must produce a different
	// canonical identity; it must never resolve to the first run's receipt.
	secondSpec := firstSpec
	secondSpec.RunID = "run-2"
	secondCall := firstCall
	secondCall.AgentRunID = "run-2"
	secondEnv, err := newNativeRunReceipt(secondSpec, secondCall, "project-1")
	if err != nil {
		t.Fatal(err)
	}
	first, err := adoptNativeRunReceipt(firstEnv, firstSpec, "project-1")
	if err != nil {
		t.Fatal(err)
	}
	second, err := adoptNativeRunReceipt(secondEnv, secondSpec, "project-1")
	if err != nil {
		t.Fatal(err)
	}
	if first.Ref == second.Ref {
		t.Fatalf("same CallID across runs produced one canonical identity: %s", first.Ref)
	}
}

func TestHasTaskPlanEvidenceRefIsIdempotent(t *testing.T) {
	if hasTaskPlanEvidenceRef(nil, "ref") {
		t.Fatal("empty evidence list reported an adopted receipt")
	}
	if hasTaskPlanEvidenceRef([]TaskPlanEvidence{{Kind: "patch", Ref: "ref"}}, "ref") {
		t.Fatal("entry without a digest counted as an adopted receipt")
	}
	adopted := []TaskPlanEvidence{{Kind: "patch", Ref: "ref", Digest: strings.Repeat("a", 64)}}
	if !hasTaskPlanEvidenceRef(adopted, "ref") {
		t.Fatal("adopted receipt was not detected")
	}
	if hasTaskPlanEvidenceRef(adopted, "other") {
		t.Fatal("different receipt was treated as already adopted")
	}
}
