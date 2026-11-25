package agentchat

import (
	"errors"
	"strings"
)

// This file adds the pure adapter from a verified canonical native receipt to
// the owner-owned Task Plan evidence ledger. It is deliberately additive: no
// existing call site is changed, nothing is persisted, and no provider or
// workspace read occurs. It is the deterministic, idempotency-ready projection
// primitive required before a run receipt can be adopted exactly once.

// adoptNativeRunReceipt converts an already-built canonical receipt into exactly
// one objective Task Plan evidence entry. Verification is repeated here so a
// caller cannot bypass the immutable run binding by passing a tampered or
// foreign envelope. The Ref is a namespaced identity derived from the run,
// attempt, tool call and project; a repeated CallID from a sibling retry cannot
// be mistaken for this receipt. The Note names only the mechanically proven
// fact and explicitly does not claim independent test success.
func adoptNativeRunReceipt(env nativeRunReceiptEnvelope, spec RunSpec, projectID string) (TaskPlanEvidence, error) {
	if err := verifyNativeRunReceipt(env, spec, projectID); err != nil {
		return TaskPlanEvidence{}, err
	}
	p := env.Payload
	ref := nativeRunReceiptSHA256(p.RunID, p.AttemptID, p.CallID, p.AgentRunID, p.ProjectID)
	if len(ref) != 64 || strings.Trim(ref, "0123456789abcdef") != "" {
		return TaskPlanEvidence{}, errors.New("native receipt projection produced an invalid reference")
	}
	return TaskPlanEvidence{
		Kind:   "patch",
		Ref:    ref,
		Digest: env.Digest,
		Note:   "Verified native receipt for run " + p.RunID + "; independent tests are not implied",
	}, nil
}

// hasTaskPlanEvidenceRef is the idempotency predicate for receipt adoption: a
// repeat sync must never append a second entry for the same namespaced proof.
// An entry with an empty digest does not count as an adopted receipt.
func hasTaskPlanEvidenceRef(evidence []TaskPlanEvidence, ref string) bool {
	if ref == "" {
		return false
	}
	for _, item := range evidence {
		if item.Ref == ref && item.Digest != "" {
			return true
		}
	}
	return false
}
