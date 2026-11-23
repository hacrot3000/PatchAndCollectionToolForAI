package server

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"

	"bletonfc/vscode_tasks_menu/internal/patchtool"
)

// agentCoordinationUnclaimedFile is the file that holds legacy coordination
// records whose owner could not be verified. The records are never copied into
// an owner-scoped journal, so an ambiguous record can never leak into another
// owner's coordination data.
const agentCoordinationUnclaimedFile = "unclaimed.json"

// agentCoordinationUnclaimedRecord is one legacy record that could not be
// attributed to a single owner. It is kept so no data is lost and an operator
// can review the record and assign it deliberately.
type agentCoordinationUnclaimedRecord struct {
	Kind       string         `json:"kind"`
	Key        string         `json:"key"`
	Reason     string         `json:"reason"`
	Record     map[string]any `json:"record"`
	MigratedAt time.Time      `json:"migrated_at"`
}

// agentCoordinationUnclaimed is the persisted quarantine of ambiguous records.
type agentCoordinationUnclaimed struct {
	Version   int                                `json:"version"`
	Owner     string                             `json:"owner"`
	Records   []agentCoordinationUnclaimedRecord `json:"records"`
	UpdatedAt time.Time                          `json:"updated_at"`
}

// agentCoordinationUnclaimedPath returns the quarantine file that belongs to
// an owner-scoped coordination journal.
func agentCoordinationUnclaimedPath(hubPath string) string {
	return filepath.Join(filepath.Dir(hubPath), agentCoordinationUnclaimedFile)
}

// migrateLegacyCoordinationJournal copies the pre-tenant coordination journal
// into an owner-scoped journal. The legacy journal was shared by every shared
// user, so only the records this owner can prove are copied; the legacy file
// is never deleted, so a rollback keeps its data.
//
// Records whose owner cannot be verified are written to an explicit quarantine
// file next to the owner journal instead of being assigned to an owner. The
// migration is idempotent: a journal that already exists is not rewritten.
func (s *Server) migrateLegacyCoordinationJournal(legacyPath, hubPath, owner string) error {
	if _, err := os.Stat(hubPath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	source, err := os.ReadFile(legacyPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(source) > 8<<20 {
		return errors.New("legacy coordination journal exceeds the size limit")
	}
	var state struct {
		Version   int            `json:"version"`
		OwnerID   string         `json:"owner_id"`
		ProjectID string         `json:"project_id"`
		Peers     map[string]any `json:"peers"`
		Messages  []any          `json:"messages"`
		Contracts map[string]any `json:"contracts"`
		Artifacts map[string]any `json:"artifacts"`
		Deps      map[string]any `json:"dependencies"`
	}
	if err := json.Unmarshal(source, &state); err != nil {
		return errors.New("legacy coordination journal is invalid")
	}
	if state.OwnerID != "" && state.OwnerID != owner {
		return errors.New("legacy coordination journal belongs to another owner")
	}
	ownerConversations, err := s.agentConversationIDsForOwner(owner)
	if err != nil {
		return err
	}
	ownerGroups, err := s.agentConversationGroupIDsForOwner(owner)
	if err != nil {
		return err
	}
	// A single-user installation owns the whole legacy journal. A shared
	// installation must only migrate the records this owner owns.
	sharedScope := s.Config.SharedServerEnabled
	unclaimed := agentCoordinationUnclaimed{Version: 1, Owner: owner, UpdatedAt: time.Now().UTC()}
	quarantine := func(kind, key, reason string, record map[string]any) {
		unclaimed.Records = append(unclaimed.Records, agentCoordinationUnclaimedRecord{
			Kind: kind, Key: key, Reason: reason, Record: record, MigratedAt: time.Now().UTC(),
		})
	}
	filtered := map[string]any{}
	for key, value := range state.Peers {
		peer, ok := value.(map[string]any)
		if !ok {
			continue
		}
		conversation, _ := peer["conversation_id"].(string)
		if conversation == "" {
			// Without a conversation the owner cannot be verified, so the
			// record is quarantined instead of being guessed.
			quarantine("peer", key, "peer record has no conversation scope", peer)
			continue
		}
		if !sharedScope || ownerConversations[conversation] {
			filtered[key] = value
			continue
		}
		if sharedScope {
			quarantine("peer", key, "peer conversation is not owned by this owner", peer)
		}
	}
	migrated := map[string]any{
		"version": 2, "owner_id": owner, "peers": filtered,
		"messages": []any{}, "contracts": map[string]any{},
		"artifacts": map[string]any{}, "dependencies": map[string]any{},
		"events": []any{},
	}
	if messages, ok := state.Messages, true; ok {
		owned := make([]any, 0, len(messages))
		for index, raw := range messages {
			message, isObject := raw.(map[string]any)
			if !isObject {
				continue
			}
			from, _ := message["from_conversation"].(string)
			to, _ := message["to_conversation"].(string)
			group, _ := message["group_id"].(string)
			if !sharedScope {
				owned = append(owned, raw)
				continue
			}
			// A peer message carries content produced by its participants, so
			// group ownership alone is not enough to claim it. It belongs to
			// this owner only when the owner owns one of the participants.
			if ownerConversations[from] || ownerConversations[to] {
				owned = append(owned, raw)
				continue
			}
			reason := "message participants are not owned by this owner"
			if group != "" {
				reason = "message participants are not owned by this owner (group " + group + ")"
			}
			quarantine("message", "message-"+itoa(index), reason, message)
		}
		migrated["messages"] = owned
	}
	if contracts, ok := state.Contracts, true; ok {
		owned := map[string]any{}
		for key, raw := range contracts {
			contract, isObject := raw.(map[string]any)
			if !isObject {
				continue
			}
			group, _ := contract["owner_group"].(string)
			if !sharedScope {
				owned[key] = raw
				continue
			}
			if group != "" && ownerGroups[group] {
				owned[key] = raw
				continue
			}
			// A contract without a group cannot be attributed to one owner in
			// a shared installation, so it is quarantined rather than copied.
			reason := "contract has no owner group"
			if group != "" {
				reason = "contract group is not owned by this owner"
			}
			quarantine("contract", key, reason, contract)
		}
		migrated["contracts"] = owned
	}
	if artifacts, ok := state.Artifacts, true; ok {
		owned := map[string]any{}
		for key, raw := range artifacts {
			artifact, isObject := raw.(map[string]any)
			if !isObject {
				continue
			}
			conversation, _ := artifact["conversation_id"].(string)
			group, _ := artifact["group_id"].(string)
			if !sharedScope {
				owned[key] = raw
				continue
			}
			if group != "" && ownerGroups[group] {
				owned[key] = raw
				continue
			}
			if conversation != "" && ownerConversations[conversation] {
				owned[key] = raw
				continue
			}
			reason := "artifact has no conversation scope"
			if conversation != "" {
				reason = "artifact conversation is not owned by this owner"
			}
			quarantine("artifact", key, reason, artifact)
		}
		migrated["artifacts"] = owned
	}
	if dependencies, ok := state.Deps, true; ok {
		owned := map[string]any{}
		for key, raw := range dependencies {
			dependency, isObject := raw.(map[string]any)
			if !isObject {
				continue
			}
			group, _ := dependency["group_id"].(string)
			conversation, _ := dependency["conversation_id"].(string)
			if !sharedScope {
				owned[key] = raw
				continue
			}
			if group != "" && ownerGroups[group] {
				owned[key] = raw
				continue
			}
			if conversation != "" && ownerConversations[conversation] {
				owned[key] = raw
				continue
			}
			reason := "dependency has no resolvable owner scope"
			if group != "" {
				reason = "dependency group is not owned by this owner"
			}
			quarantine("dependency", key, reason, dependency)
		}
		migrated["dependencies"] = owned
	}
	if project, err := patchtool.ReadProjectIdentity(s.agentOwnerWorkspace()); err == nil {
		migrated["project_id"] = project.ID
	}
	body, err := json.Marshal(migrated)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(hubPath), 0700); err != nil {
		return err
	}
	if err := os.WriteFile(hubPath, append(body, '\n'), 0600); err != nil {
		return err
	}
	return s.writeAgentCoordinationUnclaimed(hubPath, unclaimed)
}

// writeAgentCoordinationUnclaimed persists the quarantine of ambiguous legacy
// records. The file is written next to the owner journal so the records stay
// reviewable without ever being served as owner-scoped coordination data.
func (s *Server) writeAgentCoordinationUnclaimed(hubPath string, unclaimed agentCoordinationUnclaimed) error {
	path := agentCoordinationUnclaimedPath(hubPath)
	if len(unclaimed.Records) == 0 {
		// An empty quarantine removes a stale file from a previous migration
		// attempt, so the state always reflects the last migration run.
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	sort.Slice(unclaimed.Records, func(i, j int) bool {
		if unclaimed.Records[i].Kind != unclaimed.Records[j].Kind {
			return unclaimed.Records[i].Kind < unclaimed.Records[j].Kind
		}
		return unclaimed.Records[i].Key < unclaimed.Records[j].Key
	})
	body, err := json.Marshal(unclaimed)
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(body, '\n'), 0600)
}

// agentCoordinationUnclaimedRecords returns the quarantined legacy records of
// an owner-scoped journal.
func (s *Server) agentCoordinationUnclaimedRecords(hubPath string) ([]agentCoordinationUnclaimedRecord, error) {
	body, err := os.ReadFile(agentCoordinationUnclaimedPath(hubPath))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var unclaimed agentCoordinationUnclaimed
	if err := json.Unmarshal(body, &unclaimed); err != nil {
		return nil, errors.New("invalid coordination quarantine file")
	}
	return unclaimed.Records, nil
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := ""
	for value > 0 {
		digits = string(rune('0'+value%10)) + digits
		value /= 10
	}
	return digits
}
