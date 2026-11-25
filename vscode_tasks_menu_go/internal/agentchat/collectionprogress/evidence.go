package collectionprogress

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path"
	"strings"
)

const MaxEvidenceEntries = 128
const maxEvidenceProofs = 32
const maxEvidenceReceiptBytes = 512 << 10

type EvidenceStatus string

const (
	EvidenceComplete EvidenceStatus = "complete"
	EvidencePartial  EvidenceStatus = "partial"
	EvidenceUnknown  EvidenceStatus = "unknown"
	EvidenceError    EvidenceStatus = "error"
)

// Evidence contains only metadata derived from the server-owned native
// observation. It is NOT an acceptance receipt or a permission to skip a
// read. Do not use provider-authored text as the observation input.
type Evidence struct {
	Status      EvidenceStatus `json:"status"`
	ProofDigest string         `json:"proof_digest,omitempty"`
}

type nativeProof struct {
	Path         string `json:"path"`
	ActionIndex  int    `json:"action_index"`
	TotalLines   int    `json:"total_lines"`
	ReportSHA256 string `json:"report_sha256"`
}

func validEvidencePath(p string) bool {
	return p != "" && len(p) <= 4096 && !strings.ContainsAny(p, "\x00\\") &&
		!path.IsAbs(p) && p == path.Clean(p) && p != "." && p != ".." && !strings.HasPrefix(p, "../")
}

// InspectNativeEvidence consumes the already-verified native receipt emitted
// by agentNativeCollectionObservation. It never opens source or artifacts.
// The caller must supply the exact request and receipt from its authenticated
// native execution checkpoint. Historical or provider-supplied JSON is not
// sufficient provenance. Only the supported complete full-file report format
// can establish a content digest today; unsupported actions stay UNKNOWN.
func InspectNativeEvidence(scope ScopeDescriptor, projectKey, projectID string, request []byte, receipt string) Evidence {
	unknown := Evidence{Status: EvidenceUnknown}
	if !validDigest(scope.ObjectiveKey) || !validDigest(scope.RequestDigest) ||
		(scope.SourceRevisionDigest != "" && !validDigest(scope.SourceRevisionDigest)) ||
		projectKey == "" || projectID == "" || len(receipt) == 0 || len(receipt) > maxEvidenceReceiptBytes {
		return unknown
	}
	obj, err := canonicalRequest(request)
	if err != nil {
		return unknown
	}
	project, ok := obj["project"].(map[string]any)
	if !ok || len(project) != 2 || project["key"] != projectKey || project["id"] != projectID {
		return unknown
	}
	digest, err := fingerprintObject(obj)
	if err != nil || digest != scope.RequestDigest {
		return unknown
	}
	var observed struct {
		Source       string `json:"source"`
		Project      struct { Key string `json:"key"`; ID string `json:"id"` } `json:"project"`
		SessionID    string `json:"native_session_id"`
		RunID        string `json:"run_id"`
		Status       string `json:"status"`
		NativeStatus string `json:"native_status"`
		RC           *int   `json:"rc"`
		Verified     bool   `json:"evidence_verified"`
		Partial      bool   `json:"partial_evidence_available"`
		Truncated    *bool  `json:"text_excerpt_truncated"`
		Proofs       []nativeProof `json:"native_full_file_reads"`
		Error        *struct { Code string `json:"code"` } `json:"error"`
	}
	if json.Unmarshal([]byte(receipt), &observed) != nil {
		return unknown
	}
	if observed.Error != nil && observed.Error.Code != "" {
		return Evidence{Status: EvidenceError}
	}
	if observed.Source != "native_patch_tool_collect" || observed.Project.Key != projectKey ||
		observed.Project.ID != projectID || observed.SessionID == "" || observed.RunID == "" {
		return unknown
	}
	if observed.Status == "FAILED" || observed.Status == "FAIL" {
		return Evidence{Status: EvidenceError}
	}
	if observed.Verified && (observed.Status == "INCOMPLETE" || observed.Partial || (observed.Truncated != nil && *observed.Truncated)) {
		return Evidence{Status: EvidencePartial}
	}
	if !observed.Verified || observed.Status != "PASS" || observed.NativeStatus != "PASS" ||
		observed.RC == nil || *observed.RC != 0 || observed.Truncated == nil || *observed.Truncated {
		return unknown
	}
	actions, ok := obj["actions"].([]any)
	if !ok || len(actions) == 0 || len(actions) > maxEvidenceProofs || len(observed.Proofs) != len(actions) {
		return unknown
	}
	// Every action must be covered; one full-file report cannot prove an
	// unrelated search, range, command or additional file was inspected.
	ordered := make([]nativeProof, len(actions))
	seen := make([]bool, len(actions))
	for _, proof := range observed.Proofs {
		i := proof.ActionIndex - 1
		if i < 0 || i >= len(actions) || seen[i] || !validEvidencePath(proof.Path) ||
			proof.TotalLines < 0 || !validDigest(proof.ReportSHA256) {
			return unknown
		}
		a, ok := actions[i].(map[string]any)
		if !ok || a["type"] != "file" || a["path"] != proof.Path {
			return unknown
		}
		seen[i] = true
		ordered[i] = proof
	}
	raw, err := json.Marshal(ordered)
	if err != nil {
		return unknown
	}
	sum := sha256.Sum256(raw)
	return Evidence{Status: EvidenceComplete, ProofDigest: hex.EncodeToString(sum[:])}
}

type EvidenceEntry struct {
	Scope       ScopeDescriptor `json:"scope"`
	ProofDigest string          `json:"proof_digest"`
}

// EvidenceIndex records bounded post-observation comparisons within a
// single objective. It stores no source, paths, prompts or artifact names.
// Once evicted, absence of an entry is not proof that evidence is new.
type EvidenceIndex struct {
	ObjectiveKey string          `json:"objective_key"`
	Evicted      bool            `json:"evicted,omitempty"`
	Entries      []EvidenceEntry `json:"entries,omitempty"`
}

func NewEvidenceIndex(b ObjectiveBinding) EvidenceIndex {
	if !b.Valid() {
		return EvidenceIndex{}
	}
	return EvidenceIndex{ObjectiveKey: b.Key}
}

// ObserveEvidence compares only a just-completed verified observation.
// Equality of hashes after a read is NOT permission to reuse cached evidence
// before a later read, particularly with an unknown workspace revision.
func ObserveEvidence(index EvidenceIndex, scope ScopeDescriptor, evidence Evidence) (EvidenceIndex, Novelty) {
	if !validDigest(index.ObjectiveKey) || index.ObjectiveKey != scope.ObjectiveKey ||
		!validDigest(scope.RequestDigest) || (scope.SourceRevisionDigest != "" && !validDigest(scope.SourceRevisionDigest)) ||
		len(index.Entries) > MaxEvidenceEntries {
		return index, UnknownNovelty
	}
	if evidence.Status == EvidenceError {
		return index, ObservationError
	}
	if evidence.Status != EvidenceComplete || !validDigest(evidence.ProofDigest) {
		return index, UnknownNovelty
	}
	next := index
	next.Entries = append([]EvidenceEntry(nil), index.Entries...)
	for i, item := range next.Entries {
		if item.Scope != scope {
			continue
		}
		novelty := VerifiedNew
		if item.ProofDigest == evidence.ProofDigest {
			novelty = VerifiedDuplicate
		}
		next.Entries = append(next.Entries[:i], next.Entries[i+1:]...)
		next.Entries = append(next.Entries, EvidenceEntry{Scope: scope, ProofDigest: evidence.ProofDigest})
		return next, novelty
	}
	novelty := VerifiedNew
	if next.Evicted {
		novelty = UnknownNovelty
	}
	if len(next.Entries) == MaxEvidenceEntries {
		next.Entries = next.Entries[1:]
		next.Evicted = true
		novelty = UnknownNovelty
	}
	next.Entries = append(next.Entries, EvidenceEntry{Scope: scope, ProofDigest: evidence.ProofDigest})
	return next, novelty
}
