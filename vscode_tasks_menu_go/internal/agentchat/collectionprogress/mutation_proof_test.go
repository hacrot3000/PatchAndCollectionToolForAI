package collectionprogress

import (
	"strings"
	"testing"
)

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

func TestMutationProofIncompleteOrForeignFallsBack(t *testing.T) {
	good := signedProof(t, nil)
	receipt := strings.Repeat("a", 64)
	for name, tc := range map[string]struct{
		p MutationProof; owner, project, run, call, digest string
	}{
		"incomplete": {signedProof(t, func(p *MutationProof){p.Complete=false}), "w","p","run-1","call-1",receipt},
		"foreign_project": {good, "w","other","run-1","call-1",receipt},
		"foreign_run": {good, "w","p","other-run","call-1",receipt},
		"wrong_receipt": {good, "w","p","run-1","call-1",strings.Repeat("b",64)},
		"tampered_paths": {signedProof(t, func(p *MutationProof){p.IntegrityDigest=strings.Repeat("c",64)}), "w","p","run-1","call-1",receipt},
		"rename_arity": {signedProof(t, func(p *MutationProof){p.RenamedTo=nil}), "w","p","run-1","call-1",receipt},
	} {
		t.Run(name, func(t *testing.T){
			if _, ok := SelectiveMutationPaths(tc.p, tc.owner, tc.project, tc.run, tc.call, tc.digest); ok {
				t.Fatal("incomplete/foreign/tampered proof wrongly enabled selective invalidation")
			}
		})
	}
}

func TestMutationProofTraversalRejected(t *testing.T) {
	p := signedProof(t, func(p *MutationProof){p.Created=[]string{"../escape"}})
	if _, ok := SelectiveMutationPaths(p, "w", "p", "run-1", "call-1", strings.Repeat("a",64)); ok {
		t.Fatal("path traversal proof accepted")
	}
}
