package agentchat

import (
	"errors"
	"path"
	"strings"
	"unicode"
)

// NativeCollectionReadProof is bounded metadata describing one verified exact
// full-file COLLECT read from a run-managed conversation. It deliberately
// carries no file content, archive bytes or raw provider text: only the
// identifiers and canonical digests needed to attribute one objective proof
// to an owner-authorized task milestone.
//
// Acceptance establishes only the mechanically verified fact that one exact,
// untruncated, complete file read happened for the named project-relative path
// under the pinned run binding. It never claims semantic correctness of the
// file or that any independent test passed.
type NativeCollectionReadProof struct {
	Path          string `json:"path"`
	CallID        string `json:"call_id"`
	OriginRunID   string `json:"origin_run_id"`
	RequestDigest string `json:"request_digest"`
	ResultDigest  string `json:"result_digest"`
}

// normalizeCollectionProofPath validates a project-relative proof path without
// touching the filesystem. Symlink resolution and byte completeness remain the
// native COLLECT executor's responsibility; this adapter repeats the lexical
// safety rules of the legacy full_file_read auto-check and never widens them.
func normalizeCollectionProofPath(candidate string) error {
	if candidate == "" || len(candidate) > 512 || path.IsAbs(candidate) ||
		path.Clean(candidate) != candidate || candidate == "." || candidate == ".." ||
		strings.HasPrefix(candidate, "../") ||
		strings.ContainsAny(candidate, "\\\r\n\x00") ||
		strings.IndexFunc(candidate, unicode.IsControl) >= 0 {
		return errors.New("unsafe collection proof path")
	}
	return nil
}

// verifyNativeCollectionReadProof validates one canonical function_call and its
// following function_call_output in a run transcript and returns bounded proof
// metadata. It reuses the existing complete-read and request-path validators
// instead of reimplementing (and weakening) them. It fails closed on a missing,
// duplicate/ambiguous, cross-run, truncated, errored or partial read. It reads
// no files and contacts no provider.
func verifyNativeCollectionReadProof(history []HistoryItem, spec RunSpec, callID, requiredPath string) (NativeCollectionReadProof, error) {
	if err := spec.Validate(); err != nil {
		return NativeCollectionReadProof{}, err
	}
	if callID == "" || len(callID) > 256 {
		return NativeCollectionReadProof{}, errors.New("collection proof call identity is missing or invalid")
	}
	if err := normalizeCollectionProofPath(requiredPath); err != nil {
		return NativeCollectionReadProof{}, err
	}
	var call *HistoryItem
	var output *HistoryItem
	matches := 0
	for i := range history {
		item := &history[i]
		if item.Type != "function_call" || item.CallID != callID || item.Name != "taskdeck_collect_workspace" {
			continue
		}
		matches++
		if matches > 1 {
			return NativeCollectionReadProof{}, errors.New("ambiguous collection proof call")
		}
		if item.OriginRunID != spec.RunID || item.ProviderID != spec.ProviderID || item.ModelID != spec.ModelID {
			return NativeCollectionReadProof{}, errors.New("collection proof call belongs to a different run binding")
		}
		call = item
		if i+1 < len(history) {
			next := &history[i+1]
			if next.Type == "function_call_output" && next.CallID == callID {
				output = next
			}
		}
	}
	if matches == 0 || call == nil {
		return NativeCollectionReadProof{}, errors.New("collection proof call is not present")
	}
	if output == nil {
		return NativeCollectionReadProof{}, errors.New("collection proof output is not paired with its call")
	}
	if output.OriginRunID != spec.RunID || output.ProviderID != spec.ProviderID || output.ModelID != spec.ModelID {
		return NativeCollectionReadProof{}, errors.New("collection proof output belongs to a different run binding")
	}
	if !collectionCallRequestedPath(call.Arguments, requiredPath) {
		return NativeCollectionReadProof{}, errors.New("collection proof request did not name the required file")
	}
	if err := validateTaskCompleteRead(output.Content, requiredPath); err != nil {
		return NativeCollectionReadProof{}, err
	}
	return NativeCollectionReadProof{
		Path:          requiredPath,
		CallID:        callID,
		OriginRunID:   spec.RunID,
		RequestDigest: taskPlanDigest(call.Arguments),
		ResultDigest:  taskPlanDigest(output.Content),
	}, nil
}

// ArchivedCollectionReadProof repeats the same bounded verification over
// archived COLLECT pages (newest first), then the live transcript. A retained
// native proof stays independently verifiable after compaction without
// restoring raw archive content into the session. It is additive: no existing
// auto-check or archive semantics are changed.
func (s *Store) ArchivedCollectionReadProof(x Session, spec RunSpec, callID, requiredPath string) (NativeCollectionReadProof, error) {
	if proof, err := verifyNativeCollectionReadProof(x.History, spec, callID, requiredPath); err == nil {
		return proof, nil
	}
	if s == nil || x.HistoryArchiveParts == 0 {
		return NativeCollectionReadProof{}, errors.New("native collection proof is unavailable")
	}
	var lastErr error
	for part := x.HistoryArchiveParts; part > 0; part-- {
		items, err := s.ArchivedCollectionHistory(x.ID, x.Owner, part)
		if err != nil {
			lastErr = err
			continue
		}
		if proof, err := verifyNativeCollectionReadProof(items, spec, callID, requiredPath); err == nil {
			return proof, nil
		} else {
			lastErr = err
		}
	}
	if lastErr != nil {
		return NativeCollectionReadProof{}, lastErr
	}
	return NativeCollectionReadProof{}, errors.New("native collection proof is unavailable")
}
