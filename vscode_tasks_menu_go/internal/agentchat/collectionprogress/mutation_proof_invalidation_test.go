package collectionprogress

import (
	"strings"
	"testing"
)

// completeProofFor builds a VALID, complete, self-consistent proof bound to the
// standard test identity. It is used here only to exercise the consumer's
// selective path; producer trust is a separate host-owned responsibility.
func completeProofFor(t *testing.T, created []string) MutationProof {
	t.Helper()
	p := MutationProof{
		Version: MutationProofVersion, OwnerWorkspaceID: "w", ProjectID: "p",
		RunID: "run-1", CallID: "call-1", ReceiptDigest: strings.Repeat("a", 64),
		Complete: true, Created: append([]string(nil), created...),
	}
	d, err := MutationProofDigest(p)
	if err != nil {
		t.Fatal(err)
	}
	p.IntegrityDigest = d
	return p
}

func mutationCoverageFixture(t *testing.T) CoverageIndex {
	t.Helper()
	b := counterFixture(t).Objective
	a, okA := NewCoverageKey(b, "src/a.go", strings.Repeat("a", 64))
	z, okZ := NewCoverageKey(b, "src/b.go", strings.Repeat("a", 64))
	if !okA || !okZ {
		t.Fatal("coverage keys")
	}
	idx := NewCoverageIndex(b)
	idx, _ = ObserveCoverage(idx, a, LineRange{1, 100})
	idx, _ = ObserveCoverage(idx, z, LineRange{1, 100})
	return idx
}

const mutationReceipt = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestInvalidateForMutationSelectiveWithCompleteProof(t *testing.T) {
	b := counterFixture(t).Objective
	evidence := NewEvidenceIndex(b)
	coverage := mutationCoverageFixture(t)
	if len(coverage.Entries) != 2 {
		t.Fatal("fixture coverage entries")
	}
	proof := completeProofFor(t, []string{"src/a.go"})

	ev, cov := InvalidateForMutation(evidence, &coverage, &proof, "w", "p", "run-1", "call-1", mutationReceipt)
	if !ev.Evicted {
		t.Fatal("patch must always broadly invalidate exact-request evidence")
	}
	if cov == nil || len(cov.Entries) != 1 {
		t.Fatalf("selective coverage invalidation wrong: %+v", cov)
	}
	// The unaffected path's verified proof survives.
	z, _ := NewCoverageKey(b, "src/b.go", strings.Repeat("a", 64))
	_, cmp := ObserveCoverage(*cov, z, LineRange{1, 100})
	if cmp.Novelty != VerifiedDuplicate {
		t.Fatal("unrelated coverage proof lost under selective invalidation")
	}
}

func TestInvalidateForMutationBroadWithoutProof(t *testing.T) {
	b := counterFixture(t).Objective
	evidence := NewEvidenceIndex(b)
	coverage := mutationCoverageFixture(t)
	ev, cov := InvalidateForMutation(evidence, &coverage, nil, "w", "p", "run-1", "call-1", mutationReceipt)
	if !ev.Evicted || cov == nil || !cov.Evicted || len(cov.Entries) != 0 {
		t.Fatal("missing proof must fall back to broad invalidation")
	}
}

func TestInvalidateForMutationBroadForInvalidProof(t *testing.T) {
	b := counterFixture(t).Objective
	evidence := NewEvidenceIndex(b)
	proof := completeProofFor(t, []string{"src/a.go"})
	for name, tc := range map[string]struct {
		proof                             *MutationProof
		owner, project, run, call, digest string
	}{
		"incomplete": {func() *MutationProof { p := proof; p.Complete = false; return &p }(), "w", "p", "run-1", "call-1", mutationReceipt},
		"foreign_run": {&proof, "w", "p", "other-run", "call-1", mutationReceipt},
		"wrong_receipt": {&proof, "w", "p", "run-1", "call-1", strings.Repeat("b", 64)},
		"nil_coverage": {&proof, "w", "p", "run-1", "call-1", mutationReceipt},
	} {
		t.Run(name, func(t *testing.T) {
			coverage := mutationCoverageFixture(t)
			ev, cov := InvalidateForMutation(evidence, &coverage, tc.proof, tc.owner, tc.project, tc.run, tc.call, tc.digest)
			if !ev.Evicted || cov == nil || !cov.Evicted || len(cov.Entries) != 0 {
				t.Fatal("invalid proof must fall back to broad invalidation")
			}
		})
	}
}

func TestInvalidateForMutationNilCoverage(t *testing.T) {
	b := counterFixture(t).Objective
	ev, cov := InvalidateForMutation(NewEvidenceIndex(b), nil, nil, "w", "p", "run-1", "call-1", mutationReceipt)
	if !ev.Evicted || cov != nil {
		t.Fatal("nil coverage must remain nil with broad evidence invalidation")
	}
}
