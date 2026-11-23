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
	"strings"
	"time"

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
func (s *Server) migrateLegacyCoordinationJournal(legacyPath, hubPath, owner string) error {
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

	plan, err := s.buildAgentCoordinationMigration(source, owner)
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
		if _, ok := s.adoptUnmarkedCoordinationMigration(hubPath, owner); ok {
			return nil
		}
		marker = &agentCoordinationMigrationState{
			Version: 1, Owner: owner, LegacySHA256: legacyDigest,
			Status: agentCoordinationMigrationPreparing, UpdatedAt: time.Now().UTC(),
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

// adoptUnmarkedCoordinationMigration records a completion marker for a valid
// owner-scoped journal created before markers existed. Invalid or foreign
// journals are not adopted; the migration recomputes them from the legacy file.
func (s *Server) adoptUnmarkedCoordinationMigration(hubPath, owner string) (*agentCoordinationMigrationState, bool) {
	body, err := os.ReadFile(hubPath)
	if err != nil || len(body) == 0 || len(body) > 8<<20 {
		return nil, false
	}
	var state struct {
		Version int    `json:"version"`
		OwnerID string `json:"owner_id"`
	}
	if json.Unmarshal(body, &state) != nil || state.Version != 2 || state.OwnerID != owner {
		return nil, false
	}
	marker := &agentCoordinationMigrationState{
		Version: 1, Owner: owner, LegacySHA256: "",
		Status: agentCoordinationMigrationComplete, HubSHA256: sha256Hex(body),
		UpdatedAt: time.Now().UTC(),
	}
	if unclaimedBody, err := os.ReadFile(agentCoordinationUnclaimedPath(hubPath)); err == nil {
		marker.UnclaimedSHA256 = sha256Hex(unclaimedBody)
	}
	source, err := os.ReadFile(legacyCoordinationPathForHub(hubPath))
	if err == nil {
		marker.LegacySHA256 = sha256Hex(source)
	}
	if marker.LegacySHA256 == "" {
		return nil, false
	}
	if err := writeAgentCoordinationFileAtomic(agentCoordinationMigrationPath(hubPath), mustMigrationJSON(marker), 0600); err != nil {
		return nil, false
	}
	return marker, true
}

// legacyCoordinationPathForHub derives the shared legacy journal path from an
// owner-scoped hub directory name. It is only used for marker adoption.
func legacyCoordinationPathForHub(hubPath string) string {
	dir := filepath.Dir(hubPath)
	name := filepath.Base(dir)
	const prefix = "agent-coordination-"
	const suffix = "-"
	idx := strings.LastIndex(name, suffix)
	if !strings.HasPrefix(name, prefix) || idx <= len(prefix) {
		return ""
	}
	legacyDir := filepath.Join(filepath.Dir(dir), name[:idx])
	return filepath.Join(legacyDir, "coordination.json")
}

type agentCoordinationMigrationPlan struct {
	hubBody       []byte
	unclaimedBody []byte
	unclaimed     agentCoordinationUnclaimed
}

func (s *Server) buildAgentCoordinationMigration(source []byte, owner string) (agentCoordinationMigrationPlan, error) {
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
	ownerConversations, err := s.agentConversationIDsForOwner(owner)
	if err != nil {
		return agentCoordinationMigrationPlan{}, err
	}
	// OwnedGroups is verified from PrincipalOwner. AccessibleGroups (read
	// grants and shared groups) is never used as ownership evidence.
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
	for key, value := range state.Peers {
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
	owned := make([]any, 0, len(state.Messages))
	for index, raw := range state.Messages {
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

	ownedContracts := map[string]any{}
	for key, raw := range state.Contracts {
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
		if principal == owner {
			// An explicit owner principal is direct evidence and does not
			// depend on group membership (shared groups are not ownership).
			ownedContracts[key] = raw
			continue
		}
		if group != "" && ownerGroups[group] {
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
	for key, raw := range state.Artifacts {
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
		if principal == owner {
			ownedArtifacts[key] = raw
			continue
		}
		if group != "" && ownerGroups[group] {
			ownedArtifacts[key] = raw
			continue
		}
		if conversation != "" && ownerConversations[conversation] {
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
	for key, raw := range state.Deps {
		dependency, isObject := raw.(map[string]any)
		if !isObject {
			continue
		}
		group, _ := dependency["group_id"].(string)
		conversation, _ := dependency["conversation_id"].(string)
		if !sharedScope {
			ownedDependencies[key] = raw
			continue
		}
		if group != "" && ownerGroups[group] {
			ownedDependencies[key] = raw
			continue
		}
		if conversation != "" && ownerConversations[conversation] {
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
	return agentCoordinationMigrationPlan{hubBody: hubBody, unclaimedBody: unclaimedBody, unclaimed: unclaimed}, nil
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
