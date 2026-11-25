package collectionprogress

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"
)

// ObjectiveScope must be assembled from authenticated owner/workspace bindings
// and the owner-authored goal, never provider messages or synthetic prompts.
// Generation changes on a genuine new objective or accepted plan transition,
// not on a provider/model switch, attempt, or call ID.
type ObjectiveScope struct {
	Owner              string
	ProjectID          string
	OwnerWorkspace     string
	ExecutionWorkspace string
	ConversationID     string
	Goal               string
	StepID             string
	Generation         uint64
}

// ObjectiveBinding contains no raw prompt or authorization grant.
type ObjectiveBinding struct {
	Key    string `json:"key"`
	Intent Intent `json:"intent"`
}

func BindObjective(scope ObjectiveScope) (ObjectiveBinding, error) {
	for _, value := range []string{scope.Owner, scope.ProjectID, scope.OwnerWorkspace, scope.ExecutionWorkspace, scope.ConversationID} {
		if strings.TrimSpace(value) == "" || len(value) > 4096 || strings.ContainsRune(value, 0) || !utf8.ValidString(value) {
			return ObjectiveBinding{}, errors.New("incomplete or invalid collection objective scope")
		}
	}
	if strings.TrimSpace(scope.Goal) == "" || len(scope.Goal) > 1<<20 || len(scope.StepID) > 256 || strings.ContainsRune(scope.StepID, 0) || scope.Generation == 0 || !utf8.ValidString(scope.Goal) || !utf8.ValidString(scope.StepID) {
		return ObjectiveBinding{}, errors.New("invalid collection objective generation or goal")
	}
	// Encode field boundaries unambiguously. Do not lowercase case-sensitive
	// paths or assume that different goal texts are semantically equivalent.
	raw, err := json.Marshal(scope)
	if err != nil {
		return ObjectiveBinding{}, err
	}
	sum := sha256.Sum256(raw)
	return ObjectiveBinding{Key: hex.EncodeToString(sum[:]), Intent: ClassifyOwnerGoal(scope.Goal)}, nil
}

func (b ObjectiveBinding) Valid() bool {
	digest, err := hex.DecodeString(b.Key)
	if err != nil || len(digest) != sha256.Size || strings.ToLower(b.Key) != b.Key {
		return false
	}
	switch b.Intent {
	case Implementation, Mixed, ReadOnly, UnknownIntent:
		return true
	default:
		return false
	}
}

// AdvisoryIntent never provides or changes permission. A disabled Patch
// proposal path must not pressure the user into a mutation.
func (b ObjectiveBinding) AdvisoryIntent(patchProposalAvailable bool) Intent {
	if !b.Valid() {
		return UnknownIntent
	}
	if !patchProposalAvailable && (b.Intent == Implementation || b.Intent == Mixed) {
		return UnknownIntent
	}
	return b.Intent
}

type ObjectiveTransition string

const (
	ObjectiveUnbound   ObjectiveTransition = "unbound"
	ObjectiveStarted   ObjectiveTransition = "started"
	ObjectiveUnchanged ObjectiveTransition = "unchanged"
	ObjectiveChanged   ObjectiveTransition = "changed"
	ObjectiveSuspended ObjectiveTransition = "suspended"
)

// CompareObjective is pure. Stop preserves the previous scope; an unbound
// legacy objective does not manufacture progress. Callers keep old journals.
func CompareObjective(previous, next ObjectiveBinding, stopped bool) ObjectiveTransition {
	if stopped {
		return ObjectiveSuspended
	}
	if !next.Valid() {
		return ObjectiveUnbound
	}
	if !previous.Valid() {
		return ObjectiveStarted
	}
	if previous.Key == next.Key {
		return ObjectiveUnchanged
	}
	return ObjectiveChanged
}

// ClassifyOwnerGoal is a conservative, deliberately incomplete language
// heuristic for advisory text ONLY. Supply actual owner-authored text, not
// transcript user-role items containing synthetic tool/continuation output.
// Unrecognized, quoted and conditional requests remain neutral.
func ClassifyOwnerGoal(goal string) Intent {
	goal = strings.Join(strings.Fields(strings.ToLower(goal)), " ")
	goal = strings.TrimPrefix(goal, "hãy ")
	goal = strings.TrimPrefix(goal, "please ")
	for _, restriction := range []string{"read-only", "read only", "do not modify", "do not change", "no code changes", "without changing", "không sửa", "không thay đổi", "chỉ đọc", "chỉ phân tích"} {
		if strings.Contains(goal, restriction) {
			return ReadOnly
		}
	}
	for _, uncertain := range []string{"?", "\"", "`", " if ", " nếu ", "how to ", "cách "} {
		if strings.Contains(goal, uncertain) {
			return UnknownIntent
		}
	}
	if strings.HasPrefix(goal, "if ") || strings.HasPrefix(goal, "nếu ") {
		return UnknownIntent
	}
	read := hasObjectivePrefix(goal, []string{"audit ", "inspect ", "investigate ", "review ", "analyze ", "analyse ", "research ", "kiểm tra ", "điều tra ", "rà soát ", "phân tích ", "nghiên cứu "})
	implement := hasObjectivePrefix(goal, []string{"implement ", "fix ", "refactor ", "update code ", "change code ", "write code ", "triển khai ", "sửa lỗi ", "sửa code ", "viết code ", "cập nhật code "})
	if read {
		for _, conjunction := range []string{" and fix ", " and implement ", " and refactor ", " và sửa lỗi ", " và sửa code ", " và triển khai "} {
			if strings.Contains(goal, conjunction) {
				return Mixed
			}
		}
		return ReadOnly
	}
	if implement {
		return Implementation
	}
	return UnknownIntent
}

func hasObjectivePrefix(goal string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(goal, prefix) && strings.TrimSpace(strings.TrimPrefix(goal, prefix)) != "" {
			return true
		}
	}
	return false
}
