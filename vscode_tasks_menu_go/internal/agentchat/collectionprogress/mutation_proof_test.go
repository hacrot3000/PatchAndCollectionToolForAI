package collectionprogress

import (
	"strings"
	"testing"
)

// signedProof builds a VALID signed proof. Mutations here must keep the
// proof canonicalizable; use tamperAfterSigning for negative cases.
func signedProof(t *testing.T, mutate func(*MutationProof)) MutationProof {
	t.Helper()
	p := MutationProof{
		Version: MutationProofVersion, OwnerWorkspaceID: "w", ProjectID: "p",
		RunID: "run-1", CallID: "call-1", ReceiptDigest: strings.Repeat("a", 64),
		Complete: true, Created: []string{"new.go"}, Deleted: []string{"old.go"},
		Replaced: []string{"main.go"}, RenamedFrom: []string{"a.go"}, RenamedTo: []string{"b.go"},
	}
	if mutate != nil { mutate(&p) }
	d, err := MutationProofDigest(p)
	if err != nil { t.Fatal(err) }
	p.IntegrityDigest = d
	return p
}

func TestMutationProofCompleteProofAllowsSelective(t *testing.T) {
	p := signedProof(t, nil)
	paths, ok := SelectiveMutationPaths(p, "w", "p", "run-1", "call-1", strings.Repeat("a", 64))
	if !ok || len(paths) != 5 { t.Fatalf("complete proof rejected: ok=%t paths=%v", ok, paths) }
	// Rename source AND destination are both affected scopes.
	foundA, foundB := false, false
	for _, path := range paths { if path == "a.go" { foundA = true }; if path == "b.go" { foundB = true } }
	if !foundA || !foundB { t.Fatal("rename source/destination scope missing") }
}

// tamperAfterSigning mutates an already-signed proof without recomputing its
// integrity digest. Negative cases must be built this way: signing a proof whose
// path set is malformed fails in MutationProofDigest before any assertion.
func tamperAfterSigning(p MutationProof, mutate func(*MutationProof)) MutationProof {
	mutate(&p)
	return p
}

func TestMutationProofIncompleteOrForeignFallsBack(t *testing.T) {
	good := signedProof(t, nil)
	receipt := strings.Repeat("a", 64)
	incomplete := signedProof(t, func(p *MutationProof) { p.Complete = false })
	tamperedPaths := tamperAfterSigning(good, func(p *MutationProof) { p.Created = []string{"evil.go"} })
	renameArity := tamperAfterSigning(good, func(p *MutationProof) { p.RenamedTo = nil })
	for name, tc := range map[string]struct {
		p                                    MutationProof
		owner, project, run, call, digest string
	}{
		"incomplete":      {incomplete, "w", "p", "run-1", "call-1", receipt},
		"foreign_project": {good, "w", "other", "run-1", "call-1", receipt},
		"foreign_run":     {good, "w", "p", "other-run", "call-1", receipt},
		"wrong_receipt":   {good, "w", "p", "run-1", "call-1", strings.Repeat("b", 64)},
		"tampered_paths":  {tamperedPaths, "w", "p", "run-1", "call-1", receipt},
		"rename_arity":    {renameArity, "w", "p", "run-1", "call-1", receipt},
	} {
		t.Run(name, func(t *testing.T) {
			if _, ok := SelectiveMutationPaths(tc.p, tc.owner, tc.project, tc.run, tc.call, tc.digest); ok {
				t.Fatal("incomplete/foreign/tampered proof wrongly enabled selective invalidation")
			}
		})
	}
	// Digest construction itself must reject a malformed rename set.
	if _, err := MutationProofDigest(renameArity); err == nil {
		t.Fatal("digest accepted a malformed rename set")
	}
}

func TestMutationProofTraversalRejected(t *testing.T) {
	good := signedProof(t, nil)
	traversal := tamperAfterSigning(good, func(p *MutationProof) { p.Created = []string{"../escape"} })
	if _, ok := SelectiveMutationPaths(traversal, "w", "p", "run-1", "call-1", strings.Repeat("a", 64)); ok {
		t.Fatal("path traversal proof accepted")
	}
	if _, err := MutationProofDigest(traversal); err == nil {
		t.Fatal("digest accepted a traversal path")
	}
}

func TestMutationProofCanonicalPathPolicy(t *testing.T) {
	for _, tc := range []struct {
		path  string
		valid bool
	}{
		{"src/main.go", true},
		{"/absolute.go", false},
		{"../escape", false},
		{"./relative.go", false},
		{".", false},
		{"..", false},
		{"a\\b.go", false},
		{"a/../../b", false},
		{"bad\x00.go", false},
		{"bad\x01.go", false},
	} {
		p := signedProof(t, nil)
		p.Created = []string{tc.path}
		_, err := MutationProofDigest(p)
		if tc.valid && err != nil {
			t.Fatalf("valid path %q rejected: %v", tc.path, err)
		}
		if !tc.valid && err == nil {
			t.Fatalf("unsafe path %q accepted", tc.path)
		}
	}
}
