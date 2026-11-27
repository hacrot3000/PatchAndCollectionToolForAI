package collectionprogress

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
)

// MutationProof is an authoritative, bounded description of the COMPLETE file
// mutation set of one native PATCH execution. It is server-owned: a UI
// changed-files report or model text can never populate it. Selective evidence
// invalidation is allowed ONLY when a proof validates under ValidateMutationProof;
// every other case falls back to conservative broad invalidation.
type MutationProof struct {
	Version          int      `json:"version"`
	OwnerWorkspaceID string   `json:"owner_workspace_id"`
	ProjectID        string   `json:"project_id"`
	RunID            string   `json:"run_id"`
	CallID           string   `json:"call_id"`
	ReceiptDigest    string   `json:"receipt_digest"`
	Complete         bool     `json:"complete"`
	Created          []string `json:"created,omitempty"`
	Deleted          []string `json:"deleted,omitempty"`
	Replaced         []string `json:"replaced,omitempty"`
	RenamedFrom      []string `json:"renamed_from,omitempty"`
	RenamedTo        []string `json:"renamed_to,omitempty"`
	IntegrityDigest  string   `json:"integrity_digest"`
}

const MutationProofVersion = 1
const maxMutationPaths = 4096
const maxMutationPathsTotal = 8192

// validMutationProofPath accepts only canonical, project-relative paths. It
// mirrors collectionprogress.validEvidencePath (rejecting absolute paths,
// traversal, backslash/ambiguous separators, dot components and NUL) and adds
// rejection of other ASCII control characters. Keeping one path policy for
// evidence and mutation proofs prevents a proof from naming scopes that the
// coverage validator itself would refuse.
func validMutationProofPath(p string) bool {
	if !validEvidencePath(p) {
		return false
	}
	for i := 0; i < len(p); i++ {
		if p[i] < 0x20 || p[i] == 0x7f {
			return false
		}
	}
	return true
}

// mutationProofCanonical normalizes every path set so the integrity digest is
// independent of slice order and rejects duplicate/empty/path-traversal values.
func mutationProofCanonical(p MutationProof) ([][]string, error) {
	sets := [][]string{p.Created, p.Deleted, p.Replaced, p.RenamedFrom, p.RenamedTo}
	out := make([][]string, 0, len(sets))
	total := 0
	for _, set := range sets {
		total += len(set)
		if len(set) > maxMutationPaths || total > maxMutationPathsTotal {
			return nil, errors.New("mutation proof path set too large")
		}
		clone := append([]string(nil), set...)
		sort.Strings(clone)
		seen := map[string]bool{}
		for _, path := range clone {
			if !validMutationProofPath(path) || seen[path] {
				return nil, errors.New("invalid mutation path")
			}
			seen[path] = true
		}
		out = append(out, clone)
	}
	if len(out[3]) != len(out[4]) {
		return nil, errors.New("rename source/destination arity mismatch")
	}
	// Rename source and destination are ordered independently on purpose: the
	// affected-set semantics name BOTH sides as scopes, so pairing identity is
	// not part of the digest. Arity above still bounds the overall rename set.
	return out, nil
}

// MutationProofDigest computes the integrity digest over canonical path sets
// and all binding fields. It never includes model text or UI reports.
func MutationProofDigest(p MutationProof) (string, error) {
	sets, err := mutationProofCanonical(p)
	if err != nil {
		return "", err
	}
	canonical := struct {
		Version, Complete                       int
		OwnerWorkspaceID, ProjectID, RunID      string
		CallID, ReceiptDigest                   string
		Created, Deleted, Replaced              []string
		RenamedFrom, RenamedTo                  []string
	}{
		Version: p.Version, Complete: 0,
		OwnerWorkspaceID: p.OwnerWorkspaceID, ProjectID: p.ProjectID,
		RunID: p.RunID, CallID: p.CallID, ReceiptDigest: p.ReceiptDigest,
		Created: sets[0], Deleted: sets[1], Replaced: sets[2],
		RenamedFrom: sets[3], RenamedTo: sets[4],
	}
	if p.Complete { canonical.Complete = 1 }
	body, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}

// ValidateMutationProof returns nil ONLY for a complete, integrity-checked
// proof bound to the exact owner workspace, project, run, call and receipt.
// Anything incomplete, tampered or foreign fails closed (selective invalidation
// must not be used).
func ValidateMutationProof(p MutationProof, ownerWorkspaceID, projectID, runID, callID, receiptDigest string) error {
	if p.Version != MutationProofVersion {
		return errors.New("unsupported mutation proof version")
	}
	if !p.Complete {
		return errors.New("mutation proof is not complete")
	}
	if p.OwnerWorkspaceID != ownerWorkspaceID || p.ProjectID != projectID ||
		p.RunID != runID || p.CallID != callID {
		return errors.New("mutation proof run binding mismatch")
	}
	if !validDigest(p.ReceiptDigest) || p.ReceiptDigest != receiptDigest {
		return errors.New("mutation proof receipt digest mismatch")
	}
	digest, err := MutationProofDigest(p)
	if err != nil || !validDigest(p.IntegrityDigest) || digest != p.IntegrityDigest {
		return errors.New("mutation proof integrity failure")
	}
	return nil
}

// SelectiveMutationPaths returns the complete affected path set ONLY when the
// proof validates. Otherwise it returns (nil,false) and the caller MUST use
// broad invalidation. Rename sources and destinations are both affected scopes.
func SelectiveMutationPaths(p MutationProof, ownerWorkspaceID, projectID, runID, callID, receiptDigest string) ([]string, bool) {
	if err := ValidateMutationProof(p, ownerWorkspaceID, projectID, runID, callID, receiptDigest); err != nil {
		return nil, false
	}
	seen := map[string]bool{}
	var paths []string
	for _, set := range [][]string{p.Created, p.Deleted, p.Replaced, p.RenamedFrom, p.RenamedTo} {
		for _, path := range set {
			if !seen[path] {
				seen[path] = true
				paths = append(paths, path)
			}
		}
	}
	if len(paths) == 0 {
		return nil, false
	}
	return paths, true
}

// InvalidateForMutation is the fail-closed consumer for a PATCH mutation. It
// applies selective COVERAGE invalidation only when a server-owned, complete,
// validated MutationProof supplies the exact affected path set. Evidence has no
// per-path proof mapping, so a patch always broadly invalidates it. Any missing,
// incomplete, foreign or tampered proof falls back to broad invalidation of both
// indexes. The proof must be produced by trusted host verification of the exact
// native execution: a UI changed-files report, model text, or a digest computed
// by the caller is NOT authority. This is an additive primitive (like
// adoptNativeRunReceipt): production mutating paths keep broad invalidation
// until a trusted complete-proof producer is wired in, which is unsafe today
// because the native run receipt carries no complete changed-files manifest.
func InvalidateForMutation(evidence EvidenceIndex, coverage *CoverageIndex, proof *MutationProof, ownerWorkspaceID, projectID, runID, callID, receiptDigest string) (EvidenceIndex, *CoverageIndex) {
	invalidatedEvidence := InvalidateEvidence(evidence, false)
	if coverage == nil {
		return invalidatedEvidence, nil
	}
	broad := InvalidateCoverage(*coverage, nil, false)
	result := &broad
	if proof != nil {
		if paths, ok := SelectiveMutationPaths(*proof, ownerWorkspaceID, projectID, runID, callID, receiptDigest); ok {
			selective := InvalidateCoverage(*coverage, paths, true)
			result = &selective
		}
	}
	return invalidatedEvidence, result
}
