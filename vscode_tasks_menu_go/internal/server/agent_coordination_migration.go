package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"bletonfc/vscode_tasks_menu/internal/agentrun"
	"bletonfc/vscode_tasks_menu/internal/patchtool"
)

// agentCoordinationUnclaimedFile is the file that holds legacy coordination
// records whose owner could not be verified. The records are never copied into
// an owner-scoped journal, so an ambiguous record can never leak into another
// owner's coordination data.
const agentCoordinationUnclaimedFile = "unclaimed.json"

const (
	agentCoordinationMigrationFile      = "migration.json"
	agentCoordinationMigrationPreparing = "preparing"
	agentCoordinationMigrationComplete  = "complete"
	// policyVersion changes when ownership/quarantine rules change. A marker
	// with an older policy is never silently reused.
	agentCoordinationMigrationPolicyVersion = 3
)

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

// agentCoordinationMigrationState is the crash-recovery marker for the
// migration. It records the exact bytes to write so a restart can finish an
// interrupted migration without relying on the existence of coordination.json.
type agentCoordinationMigrationState struct {
	Version         int       `json:"version"`
	Owner           string    `json:"owner"`
	LegacySHA256    string    `json:"legacy_sha256"`
	Status          string    `json:"status"`
	HubSHA256       string    `json:"hub_sha256,omitempty"`
	UnclaimedSHA256 string    `json:"unclaimed_sha256,omitempty"`
	SharedMode      bool      `json:"shared_mode"`
	PolicyVersion   int       `json:"policy_version"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// agentCoordinationUnclaimedPath returns the quarantine file that belongs to
// an owner-scoped coordination journal.
func agentCoordinationUnclaimedPath(hubPath string) string {
	return filepath.Join(filepath.Dir(hubPath), agentCoordinationUnclaimedFile)
}

func agentCoordinationMigrationPath(hubPath string) string {
	return filepath.Join(filepath.Dir(hubPath), agentCoordinationMigrationFile)
}

// migrateLegacyCoordinationJournal copies the pre-tenant coordination journal
// into an owner-scoped journal. The legacy journal was shared by every shared
// user, so only the records this owner can prove are copied; the legacy file
// is never deleted, so a rollback keeps its data.
//
// The migration writes a durable marker before any owner-scoped output. If the
// process dies at any step, the next call reads the marker, recomputes the
// output from the untouched legacy journal and finishes the pending writes.
// An existing coordination.json alone is not considered proof of completion.
//
// The whole migration runs while the owner journal is exclusively locked, so a
// concurrent request can neither observe nor overwrite a half-migrated
// journal.
func (s *Server) migrateLegacyCoordinationJournal(legacyPath, hubPath, owner string) error {
	unlockProcess, err := s.lockCoordinationMigration(hubPath)
	if err != nil {
		return err
	}
	defer unlockProcess()
	return s.migrateLegacyCoordinationJournalLocked(legacyPath, hubPath, owner)
}

// lockCoordinationMigration serializes the migration of one owner journal
// inside this process and takes the cross-process journal lock.
func (s *Server) lockCoordinationMigration(hubPath string) (func(), error) {
	unlockProcess := agentrun.LockCoordinationJournalProcess(hubPath)
	unlockJournal, err := agentrun.LockCoordinationJournal(hubPath)
	if err != nil {
		unlockProcess()
		return nil, err
	}
	return func() {
		unlockJournal()
		unlockProcess()
	}, nil
}

func (s *Server) migrateLegacyCoordinationJournalLocked(legacyPath, hubPath, owner string) error {
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
	legacyDigest := sha256Hex(source)
	markerPath := agentCoordinationMigrationPath(hubPath)
	marker, markerErr := loadAgentCoordinationMigration(markerPath)
	if markerErr != nil {
		return markerErr
	}
	if marker != nil {
		if marker.PolicyVersion != agentCoordinationMigrationPolicyVersion || marker.SharedMode != s.Config.SharedServerEnabled {
			// A marker written under a different authorization policy cannot
			// be reused; re-verification is required.
			return s.reverifyMarkedCoordinationJournal(legacyPath, hubPath, owner, source, legacyDigest)
		}
		if marker.Owner != owner || marker.LegacySHA256 != legacyDigest {
			return errors.New("coordination migration marker does not match the legacy journal")
		}
		if marker.Status == agentCoordinationMigrationComplete {
			return s.verifyCompletedCoordinationMigration(hubPath, marker)
		}
		if marker.Status != agentCoordinationMigrationPreparing {
			return errors.New("invalid coordination migration marker status")
		}
	}

	plan, err := s.buildAgentCoordinationMigration(source, owner, hubPath)
	if err != nil {
		return err
	}
	unclaimedBody := append(append([]byte(nil), plan.unclaimedBody...), '\n')
	hubBody := append(append([]byte(nil), plan.hubBody...), '\n')
	unclaimedDigest := ""
	if len(plan.unclaimed.Records) > 0 {
		unclaimedDigest = sha256Hex(unclaimedBody)
	}
	hubDigest := sha256Hex(hubBody)
	if marker == nil {
		// A journal written by an older build has no marker. Preserve it if it
		// is a valid owner-scoped journal: later coordination updates must not
		// be lost just because the marker was introduced after the fact.
		if adopted, err := s.adoptUnmarkedCoordinationMigration(legacyPath, hubPath, owner); err != nil {
			return err
		} else if adopted {
			return nil
		}
		marker = &agentCoordinationMigrationState{
			Version: 1, Owner: owner, LegacySHA256: legacyDigest,
			Status: agentCoordinationMigrationPreparing, SharedMode: s.Config.SharedServerEnabled,
			PolicyVersion: agentCoordinationMigrationPolicyVersion, UpdatedAt: time.Now().UTC(),
		}
	}
	marker.UnclaimedSHA256 = unclaimedDigest
	marker.HubSHA256 = hubDigest
	marker.UpdatedAt = time.Now().UTC()
	if err := writeAgentCoordinationFileAtomic(markerPath, mustMigrationJSON(marker), 0600); err != nil {
		return err
	}
	// Quarantine first: an ambiguous record is never silently promoted into an
	// owner journal, even when the process dies mid-migration.
	unclaimedPath := agentCoordinationUnclaimedPath(hubPath)
	if len(plan.unclaimed.Records) == 0 {
		if err := removeIfExists(unclaimedPath); err != nil {
			return err
		}
	} else if err := writeAgentCoordinationFileAtomic(unclaimedPath, unclaimedBody, 0600); err != nil {
		return err
	}
	if err := writeAgentCoordinationFileAtomic(hubPath, hubBody, 0600); err != nil {
		return err
	}
	marker.Status = agentCoordinationMigrationComplete
	marker.UpdatedAt = time.Now().UTC()
	return writeAgentCoordinationFileAtomic(markerPath, mustMigrationJSON(marker), 0600)
}

// adoptUnmarkedCoordinationMigration repairs an owner-scoped journal that was
// written before migration markers existed. A version/owner_id match is not
// authority: every record is rechecked against the current owner evidence and
// only verified records stay in the journal; foreign and unscoped records are
// backed up and quarantined. Records added after the earlier migration are
// preserved, including when they no longer exist in the legacy file.
func (s *Server) adoptUnmarkedCoordinationMigration(legacyPath, hubPath, owner string) (bool, error) {
	body, err := os.ReadFile(hubPath)
	if err != nil || len(body) == 0 || len(body) > 8<<20 {
		return false, nil
	}
	var state struct {
		Version   int            `json:"version"`
		OwnerID   string         `json:"owner_id"`
		Peers     map[string]any `json:"peers"`
		Messages  []any          `json:"messages"`
		Contracts map[string]any `json:"contracts"`
		Artifacts map[string]any `json:"artifacts"`
		Deps      map[string]any `json:"dependencies"`
		Events    []any          `json:"events"`
		EventSeq  uint64         `json:"event_seq"`
	}
	if json.Unmarshal(body, &state) != nil || state.Version != 2 || state.OwnerID != owner {
		return false, nil
	}
	previousUnclaimed, err := loadAgentCoordinationUnclaimed(agentCoordinationUnclaimedPath(hubPath))
	if err != nil {
		return false, err
	}
	plan, err := s.buildAgentCoordinationMigrationFromRecords(owner, state.Peers, state.Messages, state.Contracts, state.Artifacts, state.Deps, state.Events, state.EventSeq, previousUnclaimed)
	if err != nil {
		return false, err
	}
	if err := s.backupUnmarkedCoordinationJournal(hubPath, body); err != nil {
		return false, err
	}
	if err := s.backupAgentCoordinationUnclaimed(hubPath); err != nil {
		return false, err
	}
	unclaimedBody := append(append([]byte(nil), plan.unclaimedBody...), '\n')
	hubBody := append(append([]byte(nil), plan.hubBody...), '\n')
	source, err := os.ReadFile(legacyPath)
	if err != nil {
		return false, err
	}
	marker := &agentCoordinationMigrationState{
		Version: 1, Owner: owner, LegacySHA256: sha256Hex(source),
		Status: agentCoordinationMigrationPreparing, HubSHA256: sha256Hex(hubBody),
		UnclaimedSHA256: sha256Hex(unclaimedBody), SharedMode: s.Config.SharedServerEnabled,
		PolicyVersion: agentCoordinationMigrationPolicyVersion, UpdatedAt: time.Now().UTC(),
	}
	if len(plan.unclaimed.Records) == 0 {
		marker.UnclaimedSHA256 = ""
	}
	if err := writeAgentCoordinationFileAtomic(agentCoordinationMigrationPath(hubPath), mustMigrationJSON(marker), 0600); err != nil {
		return false, err
	}
	// Quarantine is never deleted during repair: old unverified records stay
	// available for review even when the current journal yields none.
	if err := writeAgentCoordinationFileAtomic(agentCoordinationUnclaimedPath(hubPath), unclaimedBody, 0600); err != nil {
		return false, err
	}
	if err := writeAgentCoordinationFileAtomic(hubPath, hubBody, 0600); err != nil {
		return false, err
	}
	marker.Status = agentCoordinationMigrationComplete
	return true, writeAgentCoordinationFileAtomic(agentCoordinationMigrationPath(hubPath), mustMigrationJSON(marker), 0600)
}

// backupUnmarkedCoordinationJournal keeps the exact pre-repair bytes. The
// backup lives next to the owner journal and is never used as migration input.
func (s *Server) backupUnmarkedCoordinationJournal(hubPath string, body []byte) error {
	backup := filepath.Join(filepath.Dir(hubPath), "coordination.unmarked-backup.json")
	if _, err := os.Stat(backup); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return writeAgentCoordinationFileAtomic(backup, body, 0600)
}

type agentCoordinationMigrationPlan struct {
	hubBody       []byte
	unclaimedBody []byte
	unclaimed     agentCoordinationUnclaimed
	eventSeq      uint64
}

func (s *Server) buildAgentCoordinationMigration(source []byte, owner, hubPath string) (agentCoordinationMigrationPlan, error) {
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
		return agentCoordinationMigrationPlan{}, errors.New("legacy coordination journal is invalid")
	}
	if state.OwnerID != "" && state.OwnerID != owner {
		return agentCoordinationMigrationPlan{}, errors.New("legacy coordination journal belongs to another owner")
	}
	return s.buildAgentCoordinationMigrationFromRecords(owner, state.Peers, state.Messages, state.Contracts, state.Artifacts, state.Deps, nil, 0, agentCoordinationUnclaimed{})
}

// buildAgentCoordinationMigrationFromRecords verifies ownership of every
// record, regardless of whether it came from the legacy journal or from a v2
// journal created by an older buggy migration.
func (s *Server) buildAgentCoordinationMigrationFromRecords(owner string, sourcePeers map[string]any, sourceMessages []any, sourceContracts, sourceArtifacts, sourceDeps map[string]any, sourceEvents []any, sourceEventSeq uint64, previousUnclaimed agentCoordinationUnclaimed) (agentCoordinationMigrationPlan, error) {
	ownerConversations, err := s.agentConversationIDsForOwner(owner)
	if err != nil {
		return agentCoordinationMigrationPlan{}, err
	}
	ownerGroups, err := s.agentConversationGroupIDsForOwner(owner)
	if err != nil {
		return agentCoordinationMigrationPlan{}, err
	}
	sharedScope := s.Config.SharedServerEnabled
	unclaimed := agentCoordinationUnclaimed{Version: 1, Owner: owner, UpdatedAt: time.Now().UTC()}
	quarantine := func(kind, key, reason string, record map[string]any) {
		unclaimed.Records = append(unclaimed.Records, agentCoordinationUnclaimedRecord{
			Kind: kind, Key: key, Reason: reason, Record: record, MigratedAt: time.Now().UTC(),
		})
	}
	filtered := map[string]any{}
	for key, value := range sourcePeers {
		peer, ok := value.(map[string]any)
		if !ok {
			continue
		}
		conversation, _ := peer["conversation_id"].(string)
		if conversation == "" {
			quarantine("peer", key, "peer record has no conversation scope", peer)
			continue
		}
		if !sharedScope || ownerConversations[conversation] {
			filtered[key] = value
			continue
		}
		quarantine("peer", key, "peer conversation is not owned by this owner", peer)
	}
	migrated := map[string]any{
		"version": 2, "owner_id": owner, "peers": filtered,
		"messages": []any{}, "contracts": map[string]any{},
		"artifacts": map[string]any{}, "dependencies": map[string]any{},
		"events": []any{},
	}
	// Events can contain payload text from other owners. During repair of a v2
	// journal they are rechecked like any other record; legacy v1 journals do
	// not carry events in the format migrated here.
	ownedEvents := make([]any, 0, len(sourceEvents))
	for index, raw := range sourceEvents {
		event, isObject := raw.(map[string]any)
		if !isObject {
			continue
		}
		principal, _ := event["owner_principal"].(string)
		group, _ := event["group_id"].(string)
		conversation, _ := event["conversation_id"].(string)
		actor, _ := event["actor"].(string)
		// CoordinationEvent has no owner_principal field. Do not quarantine a
		// valid event just because that optional field is absent. Actor alone
		// is not authority: only group/conversation ownership or an explicit
		// foreign principal can decide. Unknown scope stays in the journal.
		if !sharedScope || principal == owner || group != "" && ownerGroups[group] || conversation != "" && ownerConversations[conversation] {
			ownedEvents = append(ownedEvents, raw)
			continue
		}
		if principal != "" && principal != owner {
			quarantine("event", eventQuarantineKey(event, index), "event belongs to another principal", event)
			continue
		}
		_ = actor
		ownedEvents = append(ownedEvents, raw)
	}
	migrated["events"] = ownedEvents
	maxEventSeq := sourceEventSeq
	seenSequences := map[uint64]bool{}
	seenIDs := map[string]bool{}
	for _, raw := range ownedEvents {
		event, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if sequence, ok := event["sequence"].(float64); ok && uint64(sequence) > maxEventSeq {
			maxEventSeq = uint64(sequence)
		}
		if id, ok := event["id"].(string); ok && id != "" {
			if seenIDs[id] {
				// A duplicate ID is assigned a fresh cursor below.
				delete(event, "id")
			} else {
				seenIDs[id] = true
			}
		}
		if sequence, ok := event["sequence"].(float64); ok && sequence > 0 {
			if seenSequences[uint64(sequence)] {
				delete(event, "sequence")
			} else {
				seenSequences[uint64(sequence)] = true
			}
		}
	}
	for _, raw := range ownedEvents {
		event, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if _, ok := event["sequence"]; !ok {
			maxEventSeq++
			event["sequence"] = maxEventSeq
			seenSequences[maxEventSeq] = true
		}
		if _, ok := event["id"]; !ok {
			event["id"] = "evt-repaired-" + strconv.FormatUint(uint64(event["sequence"].(float64)), 10)
		}
	}
	migrated["event_seq"] = maxEventSeq

	owned := make([]any, 0, len(sourceMessages))
	for index, raw := range sourceMessages {
		message, isObject := raw.(map[string]any)
		if !isObject {
			continue
		}
		from, _ := message["from_conversation"].(string)
		to, _ := message["to_conversation"].(string)
		group, _ := message["group_id"].(string)
		principal, _ := message["owner_principal"].(string)
		if !sharedScope {
			owned = append(owned, raw)
			continue
		}
		if principal != "" && principal != owner {
			quarantine("message", "message-"+itoa(index), "message belongs to another principal", message)
			continue
		}
		if principal == owner || ownerConversations[from] || ownerConversations[to] {
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

	ownedContracts := map[string]any{}
	for key, raw := range sourceContracts {
		contract, isObject := raw.(map[string]any)
		if !isObject {
			continue
		}
		group, _ := contract["owner_group"].(string)
		principal, _ := contract["owner_principal"].(string)
		if !sharedScope {
			ownedContracts[key] = raw
			continue
		}
		if principal != "" && principal != owner {
			quarantine("contract", key, "contract belongs to another principal", contract)
			continue
		}
		if principal == owner || group != "" && ownerGroups[group] {
			ownedContracts[key] = raw
			continue
		}
		reason := "contract has no owner group"
		if group != "" {
			reason = "contract group is not owned by this owner"
		}
		quarantine("contract", key, reason, contract)
	}
	migrated["contracts"] = ownedContracts

	ownedArtifacts := map[string]any{}
	for key, raw := range sourceArtifacts {
		artifact, isObject := raw.(map[string]any)
		if !isObject {
			continue
		}
		conversation, _ := artifact["conversation_id"].(string)
		group, _ := artifact["group_id"].(string)
		principal, _ := artifact["owner_principal"].(string)
		if !sharedScope {
			ownedArtifacts[key] = raw
			continue
		}
		if principal != "" && principal != owner {
			quarantine("artifact", key, "artifact belongs to another principal", artifact)
			continue
		}
		if principal == owner || group != "" && ownerGroups[group] || conversation != "" && ownerConversations[conversation] {
			ownedArtifacts[key] = raw
			continue
		}
		reason := "artifact has no conversation scope"
		if conversation != "" {
			reason = "artifact conversation is not owned by this owner"
		}
		quarantine("artifact", key, reason, artifact)
	}
	migrated["artifacts"] = ownedArtifacts

	ownedDependencies := map[string]any{}
	for key, raw := range sourceDeps {
		dependency, isObject := raw.(map[string]any)
		if !isObject {
			continue
		}
		group, _ := dependency["group_id"].(string)
		conversation, _ := dependency["conversation_id"].(string)
		principal, _ := dependency["owner_principal"].(string)
		if !sharedScope {
			ownedDependencies[key] = raw
			continue
		}
		if principal != "" && principal != owner {
			quarantine("dependency", key, "dependency belongs to another principal", dependency)
			continue
		}
		if principal == owner || group != "" && ownerGroups[group] || conversation != "" && ownerConversations[conversation] {
			ownedDependencies[key] = raw
			continue
		}
		reason := "dependency has no resolvable owner scope"
		if group != "" {
			reason = "dependency group is not owned by this owner"
		}
		quarantine("dependency", key, reason, dependency)
	}
	migrated["dependencies"] = ownedDependencies

	if project, err := patchtool.ReadProjectIdentity(s.agentOwnerWorkspace()); err == nil {
		migrated["project_id"] = project.ID
	}
	hubBody, err := json.Marshal(migrated)
	if err != nil {
		return agentCoordinationMigrationPlan{}, err
	}
	// Existing quarantine records are merged with newly detected ones. Old
	// records are never re-promoted into the journal and are never lost.
	unclaimed.Records = mergeAgentCoordinationUnclaimed(previousUnclaimed.Records, unclaimed.Records)
	sort.Slice(unclaimed.Records, func(i, j int) bool {
		if unclaimed.Records[i].Kind != unclaimed.Records[j].Kind {
			return unclaimed.Records[i].Kind < unclaimed.Records[j].Kind
		}
		return unclaimed.Records[i].Key < unclaimed.Records[j].Key
	})
	unclaimedBody, err := json.Marshal(unclaimed)
	if err != nil {
		return agentCoordinationMigrationPlan{}, err
	}
	return agentCoordinationMigrationPlan{hubBody: hubBody, unclaimedBody: unclaimedBody, unclaimed: unclaimed, eventSeq: maxEventSeq}, nil
}

// verifyCompletedCoordinationMigration checks that a completed migration's
// owner-scoped output still exists and is structurally valid. It deliberately
// does not compare hashes: the coordination journal is mutable after migration
// and legitimate peer/contract updates change its bytes.
func (s *Server) verifyCompletedCoordinationMigration(hubPath string, marker *agentCoordinationMigrationState) error {
	body, err := os.ReadFile(hubPath)
	if err != nil {
		return fmt.Errorf("migrated coordination journal is unavailable: %w", err)
	}
	var state struct {
		Version int    `json:"version"`
		OwnerID string `json:"owner_id"`
	}
	if json.Unmarshal(body, &state) != nil || state.Version != 2 || state.OwnerID != marker.Owner {
		return errors.New("migrated coordination journal is invalid")
	}
	if marker.UnclaimedSHA256 != "" {
		if _, err := os.Stat(agentCoordinationUnclaimedPath(hubPath)); err != nil {
			return fmt.Errorf("coordination quarantine is missing: %w", err)
		}
	}
	return nil
}

func loadAgentCoordinationMigration(path string) (*agentCoordinationMigrationState, error) {
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var marker agentCoordinationMigrationState
	if err := json.Unmarshal(body, &marker); err != nil {
		return nil, errors.New("invalid coordination migration marker")
	}
	if marker.Version != 1 || marker.Owner == "" || marker.LegacySHA256 == "" {
		return nil, errors.New("invalid coordination migration marker")
	}
	return &marker, nil
}

// writeAgentCoordinationFileAtomic writes a complete migration file through a
// temp file, fsync, atomic rename and best-effort directory fsync. Short writes
// are rejected and temporary files are removed on every failure path.
func writeAgentCoordinationFileAtomic(path string, body []byte, mode os.FileMode) error {
	if len(body) == 0 {
		return errors.New("empty coordination migration file")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(name)
		}
	}()
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	written, err := tmp.Write(body)
	if err == nil && written != len(body) {
		err = io.ErrShortWrite
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	committed = true
	return syncDirectoryBestEffort(dir)
}

// syncDirectoryBestEffort flushes the rename on filesystems that support
// directory fsync. Unsupported filesystems keep the atomic rename guarantee.
func syncDirectoryBestEffort(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return nil
	}
	defer f.Close()
	_ = f.Sync()
	return nil
}

func sha256Hex(body []byte) string {
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func mustMigrationJSON(value any) []byte {
	body, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return append(body, '\n')
}

func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// writeAgentCoordinationUnclaimed persists the quarantine of ambiguous legacy
// records. The file is written next to the owner journal so the records stay
// reviewable without ever being served as owner-scoped coordination data.
func (s *Server) writeAgentCoordinationUnclaimed(hubPath string, unclaimed agentCoordinationUnclaimed) error {
	return writeAgentCoordinationFileAtomic(agentCoordinationUnclaimedPath(hubPath), mustMigrationJSON(unclaimed), 0600)
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

// eventQuarantineKey returns a stable fingerprint for a quarantined event.
func eventQuarantineKey(event map[string]any, index int) string {
	if id, ok := event["id"].(string); ok && id != "" {
		return "event:" + id
	}
	return "event-index:" + itoa(index)
}

// agentQuarantineFingerprint is the stable identity of a quarantine record.
// It uses the record content rather than timestamps so retries cannot duplicate
// the same logical record.
func agentQuarantineFingerprint(record agentCoordinationUnclaimedRecord) string {
	body, err := json.Marshal(struct {
		Kind   string         `json:"kind"`
		Key    string         `json:"key"`
		Record map[string]any `json:"record"`
	}{record.Kind, record.Key, record.Record})
	if err != nil {
		return record.Kind + "\x00" + record.Key
	}
	return sha256Hex(body)
}

// mergeAgentCoordinationUnclaimed merges previous and newly detected records,
// deduplicating by stable fingerprint while preserving the original record.
func mergeAgentCoordinationUnclaimed(previous, detected []agentCoordinationUnclaimedRecord) []agentCoordinationUnclaimedRecord {
	seen := map[string]bool{}
	out := make([]agentCoordinationUnclaimedRecord, 0, len(previous)+len(detected))
	for _, group := range [][]agentCoordinationUnclaimedRecord{previous, detected} {
		for _, record := range group {
			fingerprint := agentQuarantineFingerprint(record)
			if seen[fingerprint] {
				continue
			}
			seen[fingerprint] = true
			out = append(out, record)
		}
	}
	return out
}

// loadAgentCoordinationUnclaimed reads an existing quarantine before repair.
func loadAgentCoordinationUnclaimed(path string) (agentCoordinationUnclaimed, error) {
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return agentCoordinationUnclaimed{Version: 1, Records: []agentCoordinationUnclaimedRecord{}}, nil
	}
	if err != nil {
		return agentCoordinationUnclaimed{}, err
	}
	var unclaimed agentCoordinationUnclaimed
	if err := json.Unmarshal(body, &unclaimed); err != nil {
		return agentCoordinationUnclaimed{}, errors.New("invalid coordination quarantine file")
	}
	if unclaimed.Records == nil {
		unclaimed.Records = []agentCoordinationUnclaimedRecord{}
	}
	return unclaimed, nil
}

// backupAgentCoordinationUnclaimed keeps the exact pre-repair quarantine.
func (s *Server) backupAgentCoordinationUnclaimed(hubPath string) error {
	path := agentCoordinationUnclaimedPath(hubPath)
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	backup := filepath.Join(filepath.Dir(hubPath), "unclaimed.unmarked-backup.json")
	if _, err := os.Stat(backup); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return writeAgentCoordinationFileAtomic(backup, body, 0600)
}

// reverifyMarkedCoordinationJournal handles a mode/policy change: a marker is
// removed only after its journal and quarantine are backed up, then the whole
// migration is recomputed under the current policy. Valid records survive and
// previously quarantined records are merged.
func (s *Server) reverifyMarkedCoordinationJournal(legacyPath, hubPath, owner string, source []byte, legacyDigest string) error {
	if err := s.backupUnmarkedCoordinationJournal(hubPath, mustMigrationJSON(map[string]any{"reason": "policy change", "owner": owner})); err != nil {
		return err
	}
	body, err := os.ReadFile(hubPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err == nil && len(body) > 0 {
		backup := filepath.Join(filepath.Dir(hubPath), "coordination.policy-backup.json")
		if _, statErr := os.Stat(backup); errors.Is(statErr, os.ErrNotExist) {
			if err := writeAgentCoordinationFileAtomic(backup, body, 0600); err != nil {
				return err
			}
		}
	}
	if err := s.backupAgentCoordinationUnclaimed(hubPath); err != nil {
		return err
	}
	if err := removeIfExists(agentCoordinationMigrationPath(hubPath)); err != nil {
		return err
	}
	// A policy change must re-verify the existing v2 journal record by record,
	// so valid records added since the old migration survive. The unmarked
	// repair path does exactly that and also merges the old quarantine.
	if _, err := s.adoptUnmarkedCoordinationMigration(legacyPath, hubPath, owner); err != nil {
		return err
	}
	return s.migrateLegacyCoordinationJournalLocked(legacyPath, hubPath, owner)
}
