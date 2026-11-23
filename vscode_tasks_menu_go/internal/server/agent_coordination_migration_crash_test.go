package server

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/agentrun"
)

// TestCoordinationMigrationRecoversFromWriteFaults injects a failure at each
// durable write step. The legacy journal is never removed, so a later run can
// finish the migration from the marker and produce exactly one copy.
func TestCoordinationMigrationRecoversFromWriteFaults(t *testing.T) {
	for _, step := range []string{"marker", "quarantine", "hub"} {
		t.Run(step, func(t *testing.T) {
			s, _, _, legacyPath := agentLegacyCoordinationFixture(t)
			hubPath := agentOwnerHubPath(t, s, "user:alice")
			dir := filepath.Dir(hubPath)
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			var blocked string
			switch step {
			case "marker":
				blocked = agentCoordinationMigrationPath(hubPath)
			case "quarantine":
				blocked = agentCoordinationUnclaimedPath(hubPath)
			case "hub":
				blocked = hubPath
			}
			if err := os.Mkdir(blocked, 0700); err != nil {
				t.Fatal(err)
			}
			if _, err := s.agentCoordinationHub("user:alice"); err == nil {
				t.Fatalf("%s write fault did not fail the migration", step)
			}
			if err := os.Remove(blocked); err != nil {
				t.Fatal(err)
			}
			// A fresh call simulates a process restart. It must finish the
			// pending marker and never create duplicate records.
			if _, err := s.agentCoordinationHub("user:alice"); err != nil {
				t.Fatalf("restart after %s fault: %v", step, err)
			}
			marker, err := loadAgentCoordinationMigration(agentCoordinationMigrationPath(hubPath))
			if err != nil || marker == nil || marker.Status != agentCoordinationMigrationComplete {
				t.Fatalf("migration marker is not complete after %s fault: %+v %v", step, marker, err)
			}
			hub, err := s.agentCoordinationHub("user:alice")
			if err != nil {
				t.Fatal(err)
			}
			peers, err := hub.Peers("")
			if err != nil {
				t.Fatal(err)
			}
			if len(peers) != 1 {
				t.Fatalf("restart after %s fault duplicated peers: %+v", step, peers)
			}
			unclaimed, err := s.agentCoordinationUnclaimedRecords(hubPath)
			if err != nil {
				t.Fatal(err)
			}
			if len(unclaimed) == 0 {
				t.Fatalf("restart after %s fault lost the quarantine", step)
			}
			if _, err := os.Stat(legacyPath); err != nil {
				t.Fatalf("legacy journal was removed after %s fault: %v", step, err)
			}
		})
	}
}

// TestCoordinationMigrationDoesNotTrustHubExistence verifies that an existing
// coordination.json alone is not accepted as completion evidence. A foreign or
// unreadable journal without a valid marker is recomputed from the legacy file.
func TestCoordinationMigrationDoesNotTrustHubExistence(t *testing.T) {
	s, conversations, _, _ := agentLegacyCoordinationFixture(t)
	hubPath := agentOwnerHubPath(t, s, "user:alice")
	if _, err := s.agentCoordinationHub("user:alice"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hubPath, []byte("{\"version\":2,\"owner_id\":\"user:bob\"}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(agentCoordinationMigrationPath(hubPath)); err != nil {
		t.Fatal(err)
	}
	hub, err := s.agentCoordinationHub("user:alice")
	if err != nil {
		t.Fatal(err)
	}
	peers, err := hub.Peers("")
	if err != nil {
		t.Fatal(err)
	}
	if len(peers) != 1 || peers[0].ConversationID != conversations["user:alice"].ID {
		t.Fatalf("foreign journal was not rebuilt from the legacy source: %+v", peers)
	}
}

// TestUnmarkedJournalIsRepairedRecordByRecord verifies that a v2 journal from
// the buggy migration is not adopted just because version/owner_id match.
func TestUnmarkedJournalIsRepairedRecordByRecord(t *testing.T) {
	s, conversations, groups, legacyPath := agentLegacyCoordinationFixture(t)
	hubPath := agentOwnerHubPath(t, s, "user:alice")
	// First create a correct journal, then simulate the older buggy migration:
	// foreign records copied in, valid post-migration additions, no marker.
	if _, err := s.agentCoordinationHub("user:alice"); err != nil {
		t.Fatal(err)
	}
	var state map[string]any
	body, err := os.ReadFile(hubPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body, &state); err != nil {
		t.Fatal(err)
	}
	peers, _ := state["peers"].(map[string]any)
	peers["foreign-peer"] = map[string]any{"conversation_id": conversations["user:bob"].ID, "status": "idle"}
	peers["new-valid-peer"] = map[string]any{"conversation_id": conversations["user:alice"].ID, "status": "running", "summary": "added after migration"}
	contracts, _ := state["contracts"].(map[string]any)
	contracts["foreign.api"] = map[string]any{"key": "foreign.api", "owner_principal": "user:bob", "owner_group": groups["user:bob"].ID, "signature": "foreign(): Result", "revision": 1, "status": "planned"}
	contracts["new-valid.api"] = map[string]any{"key": "new-valid.api", "owner_principal": "user:alice", "signature": "new(): Result", "revision": 1, "status": "planned"}
	state["events"] = []any{
		map[string]any{"sequence": 1, "kind": "valid", "owner_principal": "user:alice", "summary": "added after migration", "occurred_at": time.Now().UTC()},
		map[string]any{"sequence": 2, "kind": "foreign", "owner_principal": "user:bob", "summary": "SECRET_EVENT", "occurred_at": time.Now().UTC()},
	}
	state["peers"] = peers
	state["contracts"] = contracts
	corrupted, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hubPath, append(corrupted, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(agentCoordinationMigrationPath(hubPath)); err != nil {
		t.Fatal(err)
	}
	hub, err := s.agentCoordinationHub("user:alice")
	if err != nil {
		t.Fatal(err)
	}
	resultPeers, err := hub.Peers("")
	if err != nil {
		t.Fatal(err)
	}
	foundOld, foundNew, foundForeign := false, false, false
	for _, peer := range resultPeers {
		switch peer.ConversationID {
		case conversations["user:alice"].ID:
			if peer.Summary == "added after migration" {
				foundNew = true
			} else {
				foundOld = true
			}
		case conversations["user:bob"].ID:
			foundForeign = true
		}
	}
	if !foundOld || !foundNew || foundForeign {
		t.Fatalf("repair lost valid records or kept foreign peer: old=%v new=%v foreign=%v %+v", foundOld, foundNew, foundForeign, resultPeers)
	}
	resultContracts, err := hub.Contracts("")
	if err != nil {
		t.Fatal(err)
	}
	validContract, foreignContract := false, false
	for _, contract := range resultContracts {
		if contract.Key == "new-valid.api" || contract.Key == "alice.api" {
			validContract = true
		}
		if contract.Key == "foreign.api" {
			foreignContract = true
		}
	}
	if !validContract || foreignContract {
		t.Fatalf("repair kept foreign contract or lost valid contract: %+v", resultContracts)
	}
	events, err := hub.Events(0, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if strings.Contains(event.Summary, "SECRET_EVENT") {
			t.Fatalf("foreign event survived repair: %+v", event)
		}
	}
	unclaimed, err := s.agentCoordinationUnclaimedRecords(hubPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(unclaimed) == 0 {
		t.Fatal("foreign records were not quarantined during repair")
	}
	for _, record := range unclaimed {
		if record.Kind != "event" {
			continue
		}
		if !strings.Contains(record.Reason, "another principal") {
			t.Fatalf("foreign event has no clear quarantine reason: %+v", record)
		}
	}
	backup, err := os.ReadFile(filepath.Join(filepath.Dir(hubPath), "coordination.unmarked-backup.json"))
	if err != nil || len(backup) == 0 {
		t.Fatalf("pre-repair backup is missing: %v", err)
	}
	// Repair is idempotent and marker status is complete.
	if _, err := s.agentCoordinationHub("user:alice"); err != nil {
		t.Fatal(err)
	}
	marker, err := loadAgentCoordinationMigration(agentCoordinationMigrationPath(hubPath))
	if err != nil || marker == nil || marker.Status != agentCoordinationMigrationComplete {
		t.Fatalf("marker not complete after repair: %+v %v", marker, err)
	}
	if _, err := os.Stat(legacyPath); err != nil {
		t.Fatalf("legacy journal removed: %v", err)
	}
}

// TestConcurrentFirstMigrationKeepsSingleJournal verifies that two goroutines
// opening the same owner hub at once migrate exactly once and never expose a
// partially written journal.
func TestConcurrentFirstMigrationKeepsSingleJournal(t *testing.T) {
	s, conversations, _, _ := agentLegacyCoordinationFixture(t)
	hubPath := agentOwnerHubPath(t, s, "user:alice")
	var wg sync.WaitGroup
	results := make([]*agentrun.CoordinationHub, 2)
	errs := make([]error, 2)
	for i := range results {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			results[index], errs[index] = s.agentCoordinationHub("user:alice")
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent migration %d failed: %v", i, err)
		}
	}
	if results[0] == nil || results[1] == nil {
		t.Fatal("concurrent migration returned a nil hub")
	}
	peers, err := results[0].Peers("")
	if err != nil {
		t.Fatal(err)
	}
	if len(peers) != 1 || peers[0].ConversationID != conversations["user:alice"].ID {
		t.Fatalf("concurrent migration duplicated or lost peers: %+v", peers)
	}
	unclaimed, err := s.agentCoordinationUnclaimedRecords(hubPath)
	if err != nil || len(unclaimed) == 0 {
		t.Fatalf("concurrent migration lost quarantine: %+v %v", unclaimed, err)
	}
}

// TestMigrationDoesNotOverwriteConcurrentHubUpdate verifies that a hub update
// and a migration on the same journal cannot interleave. The operation that
// obtains the journal lock first completes before the other starts.
func TestMigrationDoesNotOverwriteConcurrentHubUpdate(t *testing.T) {
	s, conversations, _, _ := agentLegacyCoordinationFixture(t)
	hubPath := agentOwnerHubPath(t, s, "user:alice")
	// Create the migrated journal first so a later migration call only
	// verifies it, while the concurrent operation writes a new peer.
	if _, err := s.agentCoordinationHub("user:alice"); err != nil {
		t.Fatal(err)
	}
	hub, err := s.agentCoordinationHub("user:alice")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var updateErr, migrateErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, updateErr = hub.UpsertPeer(agentrun.Peer{ConversationID: "post-migration-peer", GroupID: "", Status: agentrun.PeerRunning, Summary: "concurrent update", UpdatedAt: time.Now().UTC()})
	}()
	go func() {
		defer wg.Done()
		_, migrateErr = s.agentCoordinationHub("user:alice")
	}()
	wg.Wait()
	if updateErr != nil {
		t.Fatalf("concurrent peer update failed: %v", updateErr)
	}
	if migrateErr != nil {
		t.Fatalf("concurrent migration verification failed: %v", migrateErr)
	}
	peers, err := hub.Peers("")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, peer := range peers {
		if peer.ConversationID == "post-migration-peer" && peer.Summary == "concurrent update" {
			found = true
		}
	}
	if !found || len(peers) != 2 {
		t.Fatalf("concurrent update was lost: %+v", peers)
	}
	_ = conversations
	_ = hubPath
}

const (
	coordinationProcessHelperEnv    = "TASKDECK_COORDINATION_TEST_HELPER"
	coordinationProcessWorkspaceEnv = "TASKDECK_COORDINATION_TEST_WORKSPACE"
	coordinationProcessOwnerEnv     = "TASKDECK_COORDINATION_TEST_OWNER"
	coordinationProcessMarkerEnv    = "TASKDECK_COORDINATION_TEST_MARKER"
	coordinationProcessSharedEnv    = "TASKDECK_COORDINATION_TEST_SHARED"
)

// TestCoordinationMigrationHelperProcess is a helper process that opens an
// owner hub and writes a marker. It is not a test case.
func TestCoordinationMigrationHelperProcess(t *testing.T) {
	if os.Getenv(coordinationProcessHelperEnv) != "1" {
		t.Skip("helper process")
	}
	s := &Server{Workspace: os.Getenv(coordinationProcessWorkspaceEnv), ConnectionSecrets: &mockAISecretStore{}}
	s.Config.SharedServerEnabled = os.Getenv(coordinationProcessSharedEnv) == "1"
	owner := os.Getenv(coordinationProcessOwnerEnv)
	hub, err := s.agentCoordinationHub(owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hub.Peers(""); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv(coordinationProcessMarkerEnv), []byte(owner+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
}

// TestConcurrentMigrationAcrossProcesses verifies that two independent server
// processes opening the same owner journal cannot both migrate it or serve a
// partially migrated journal.
func TestConcurrentMigrationAcrossProcesses(t *testing.T) {
	s, conversations, _, _ := agentLegacyCoordinationFixture(t)
	env := append(os.Environ(),
		coordinationProcessHelperEnv+"=1",
		coordinationProcessWorkspaceEnv+"="+s.Workspace,
		coordinationProcessSharedEnv+"=1",
	)
	type processCase struct {
		owner  string
		marker string
		cmd    *exec.Cmd
		out    bytes.Buffer
	}
	var cases []processCase
	for _, owner := range []string{"user:alice", "user:bob"} {
		for i := 0; i < 2; i++ {
			marker := filepath.Join(t.TempDir(), "ready-"+itoa(i))
			command := exec.Command(os.Args[0], "-test.run=TestCoordinationMigrationHelperProcess", "-test.v")
			command.Env = append(append([]string{}, env...),
				coordinationProcessOwnerEnv+"="+owner,
				coordinationProcessMarkerEnv+"="+marker,
			)
			cases = append(cases, processCase{owner: owner, marker: marker, cmd: command})
		}
	}
	for i := range cases {
		cases[i].cmd.Stdout = &cases[i].out
		cases[i].cmd.Stderr = &cases[i].out
		if err := cases[i].cmd.Start(); err != nil {
			t.Fatalf("start migration helper process %d: %v", i, err)
		}
	}
	errs := make([]error, len(cases))
	for i := range cases {
		errs[i] = cases[i].cmd.Wait()
	}
	for i, err := range errs {
		if err != nil {
			t.Fatalf("migration helper process %d failed: %v %s", i, err, cases[i].out.String())
		}
	}
	for _, item := range cases {
		body, err := os.ReadFile(item.marker)
		if err != nil || string(body) != item.owner+"\n" {
			t.Fatalf("process %s did not read the migrated journal: %q %v", item.owner, body, err)
		}
	}
	// Verify strict owner isolation: Alice's journal has only her peer.
	for owner, expected := range map[string]string{
		"user:alice": conversations["user:alice"].ID,
		"user:bob":   conversations["user:bob"].ID,
	} {
		hub, err := s.agentCoordinationHub(owner)
		if err != nil {
			t.Fatal(err)
		}
		peers, err := hub.Peers("")
		if err != nil {
			t.Fatal(err)
		}
		if len(peers) != 1 || peers[0].ConversationID != expected {
			t.Fatalf("%s journal contains foreign or missing peers: %+v", owner, peers)
		}
		marker, err := loadAgentCoordinationMigration(agentCoordinationMigrationPath(agentOwnerHubPath(t, s, owner)))
		if err != nil || marker == nil || marker.Status != agentCoordinationMigrationComplete || marker.PolicyVersion != agentCoordinationMigrationPolicyVersion || !marker.SharedMode {
			t.Fatalf("%s marker missing policy binding: %+v %v", owner, marker, err)
		}
		quarantine, err := s.agentCoordinationUnclaimedRecords(agentOwnerHubPath(t, s, owner))
		if err != nil {
			t.Fatal(err)
		}
		if len(quarantine) == 0 {
			t.Fatalf("%s quarantine lost unscoped legacy records", owner)
		}
		quarantineBody, err := os.ReadFile(agentCoordinationUnclaimedPath(agentOwnerHubPath(t, s, owner)))
		if err != nil || !strings.Contains(string(quarantineBody), owner) {
			t.Fatalf("%s quarantine is not owner-scoped: %q %v", owner, quarantineBody, err)
		}
	}
}

// TestUnmarkedJournalRepairPreservesEventCursor verifies that repair keeps a
// valid event cursor, filters foreign events, and lets polling continue from
// the old cursor without missing newly published events.
func TestUnmarkedJournalRepairPreservesEventCursor(t *testing.T) {
	s, _, groups, _ := agentLegacyCoordinationFixture(t)
	hubPath := agentOwnerHubPath(t, s, "user:alice")
	if _, err := s.agentCoordinationHub("user:alice"); err != nil {
		t.Fatal(err)
	}
	groupID := groups["user:alice"].ID
	var state map[string]any
	body, err := os.ReadFile(hubPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body, &state); err != nil {
		t.Fatal(err)
	}
	events := make([]any, 0, 100)
	for i := 1; i <= 100; i++ {
		events = append(events, map[string]any{
			"sequence": i, "id": "evt-" + itoa(i), "kind": "valid",
			"group_id": groupID, "actor": "peer", "summary": "valid event " + itoa(i),
			"occurred_at": time.Now().UTC(),
		})
	}
	events = append(events, map[string]any{
		"sequence": 101, "id": "evt-foreign", "kind": "foreign",
		"owner_principal": "user:bob", "summary": "SECRET_FOREIGN_EVENT", "occurred_at": time.Now().UTC(),
	})
	state["events"] = events
	state["event_seq"] = 101
	modified, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hubPath, append(modified, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(agentCoordinationMigrationPath(hubPath)); err != nil {
		t.Fatal(err)
	}
	hub, err := s.agentCoordinationHub("user:alice")
	if err != nil {
		t.Fatal(err)
	}
	after, err := hub.EventsForScope(groupID, "", 95, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 5 {
		t.Fatalf("polling from old cursor returned %d events, want 5: %+v", len(after), after)
	}
	last := after[len(after)-1].Sequence
	if _, err := hub.UpsertPeer(agentrun.Peer{ConversationID: "cursor-probe", GroupID: groupID, Status: agentrun.PeerRunning, Summary: "post repair"}); err != nil {
		t.Fatal(err)
	}
	afterRepair, err := hub.EventsForScope(groupID, "", last, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterRepair) != 1 || afterRepair[0].Sequence <= last || afterRepair[0].ID == "" {
		t.Fatalf("new event did not continue cursor monotonically: %+v", afterRepair)
	}
	all, err := hub.EventsForScope(groupID, "", 0, 200)
	if err != nil {
		t.Fatal(err)
	}
	seenSeq, seenID := map[uint64]bool{}, map[string]bool{}
	for _, event := range all {
		if seenSeq[event.Sequence] || seenID[event.ID] {
			t.Fatalf("duplicate event sequence or ID: %+v", event)
		}
		seenSeq[event.Sequence] = true
		seenID[event.ID] = true
		if strings.Contains(event.Summary, "SECRET_FOREIGN_EVENT") {
			t.Fatalf("foreign event survived repair: %+v", event)
		}
	}
}

// TestRepairMergesExistingQuarantine verifies that old quarantine records are
// preserved and deduplicated when repair detects new unverified records.
func TestRepairMergesExistingQuarantine(t *testing.T) {
	s, _, _, _ := agentLegacyCoordinationFixture(t)
	hubPath := agentOwnerHubPath(t, s, "user:alice")
	if _, err := s.agentCoordinationHub("user:alice"); err != nil {
		t.Fatal(err)
	}
	quarantinePath := agentCoordinationUnclaimedPath(hubPath)
	existing := agentCoordinationUnclaimed{Version: 1, Owner: "user:alice", UpdatedAt: time.Now().UTC()}
	for i := 0; i < 5; i++ {
		existing.Records = append(existing.Records, agentCoordinationUnclaimedRecord{
			Kind: "contract", Key: "old-" + itoa(i), Reason: "previously unverified",
			Record: map[string]any{"key": "old-" + itoa(i), "owner": "unknown"},
		})
	}
	body, err := json.Marshal(existing)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(quarantinePath, append(body, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	var state map[string]any
	hubBody, err := os.ReadFile(hubPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(hubBody, &state); err != nil {
		t.Fatal(err)
	}
	contracts, _ := state["contracts"].(map[string]any)
	for i := 0; i < 3; i++ {
		contracts["new-"+itoa(i)] = map[string]any{"key": "new-" + itoa(i), "owner_principal": "user:bob", "signature": "foreign", "revision": 1, "status": "planned"}
	}
	state["contracts"] = contracts
	corrupted, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hubPath, append(corrupted, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(agentCoordinationMigrationPath(hubPath)); err != nil {
		t.Fatal(err)
	}
	_, err = s.agentCoordinationHub("user:alice")
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.agentCoordinationUnclaimedRecords(hubPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 8 {
		t.Fatalf("repair did not merge 5 old + 3 new quarantine records: %+v", first)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(hubPath), "unclaimed.unmarked-backup.json")); err != nil {
		t.Fatalf("quarantine backup missing: %v", err)
	}
	_, err = s.agentCoordinationHub("user:alice")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.agentCoordinationUnclaimedRecords(hubPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != len(first) {
		t.Fatalf("retry duplicated quarantine records: %d -> %d", len(first), len(second))
	}
}

// TestModeSwitchReverifiesOwnership verifies that switching from single-user
// to shared mode does not silently reuse a complete marker.
func TestModeSwitchReverifiesOwnership(t *testing.T) {
	s, conversations, groups, legacyPath := agentLegacyCoordinationFixture(t)
	hubPath := agentOwnerHubPath(t, s, "user:alice")
	s.Config.SharedServerEnabled = false
	singleHub, err := s.agentCoordinationHub("user:alice")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := singleHub.UpsertPeer(agentrun.Peer{ConversationID: conversations["user:alice"].ID, GroupID: groups["user:alice"].ID, Status: agentrun.PeerRunning, Summary: "added before mode switch"}); err != nil {
		t.Fatal(err)
	}
	marker, err := loadAgentCoordinationMigration(agentCoordinationMigrationPath(hubPath))
	if err != nil || marker == nil || marker.SharedMode {
		t.Fatalf("single-user marker not recorded: %+v %v", marker, err)
	}
	s.Config.SharedServerEnabled = true
	hub, err := s.agentCoordinationHub("user:alice")
	if err != nil {
		t.Fatal(err)
	}
	marker, err = loadAgentCoordinationMigration(agentCoordinationMigrationPath(hubPath))
	if err != nil || marker == nil || !marker.SharedMode || marker.PolicyVersion != agentCoordinationMigrationPolicyVersion {
		t.Fatalf("mode switch did not rebind policy: %+v %v", marker, err)
	}
	peers, err := hub.Peers("")
	foundPostMigration := false
	for _, peer := range peers {
		if peer.ConversationID == conversations["user:alice"].ID && peer.Summary == "added before mode switch" {
			foundPostMigration = true
		}
	}
	if err != nil || !foundPostMigration {
		t.Fatalf("mode switch lost valid peer added before switch: %+v %v", peers, err)
	}
	if _, err := os.Stat(legacyPath); err != nil {
		t.Fatalf("legacy journal removed during mode switch: %v", err)
	}
	bobHub, err := s.agentCoordinationHub("user:bob")
	if err != nil {
		t.Fatal(err)
	}
	bobContracts, err := bobHub.Contracts("")
	if err != nil {
		t.Fatal(err)
	}
	for _, contract := range bobContracts {
		if contract.OwnerGroup == groups["user:alice"].ID {
			t.Fatalf("Bob received Alice's contract after mode switch: %+v", contract)
		}
	}
}

// TestFirstMigrationBlocksReadersUntilJournalIsComplete verifies that a reader
// cannot observe a half-migrated journal while the first migration is running.
// Barriers make the ordering deterministic instead of relying on sleeps.
func TestFirstMigrationBlocksReadersUntilJournalIsComplete(t *testing.T) {
	s, conversations, _, legacyPath := agentLegacyCoordinationFixture(t)
	hubPath := agentOwnerHubPath(t, s, "user:alice")

	lockAcquired := make(chan struct{})
	releaseLock := make(chan struct{})
	readerDone := make(chan *agentrun.CoordinationHub, 1)
	readerErr := make(chan error, 1)

	go func() {
		unlock, err := s.lockCoordinationMigration(hubPath)
		if err != nil {
			readerErr <- err
			return
		}
		close(lockAcquired)
		<-releaseLock
		unlock()
	}()
	<-lockAcquired

	go func() {
		hub, err := s.agentCoordinationHub("user:alice")
		if err != nil {
			readerErr <- err
			return
		}
		readerDone <- hub
	}()

	select {
	case hub := <-readerDone:
		t.Fatalf("reader served journal before migration completed: %+v", hub)
	case err := <-readerErr:
		t.Fatalf("reader failed while migration held the lock: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	// Migrate under the held lock, then release the reader.
	if err := s.migrateLegacyCoordinationJournalLocked(legacyPath, hubPath, "user:alice"); err != nil {
		t.Fatalf("migration under held lock failed: %v", err)
	}
	close(releaseLock)

	select {
	case hub := <-readerDone:
		peers, err := hub.Peers("")
		if err != nil || len(peers) == 0 {
			t.Fatalf("reader saw incomplete journal after migration: %+v %v", peers, err)
		}
		found := false
		for _, peer := range peers {
			if peer.ConversationID == conversations["user:alice"].ID {
				found = true
			}
		}
		if !found {
			t.Fatalf("reader missed the owner's peer: %+v", peers)
		}
	case err := <-readerErr:
		t.Fatalf("reader failed after migration: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("reader did not complete after migration released the lock")
	}
}
