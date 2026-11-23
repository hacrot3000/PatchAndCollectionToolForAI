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
)

// TestCoordinationMigrationHelperProcess is a helper process that opens an
// owner hub and writes a marker. It is not a test case.
func TestCoordinationMigrationHelperProcess(t *testing.T) {
	if os.Getenv(coordinationProcessHelperEnv) != "1" {
		t.Skip("helper process")
	}
	s := &Server{Workspace: os.Getenv(coordinationProcessWorkspaceEnv), ConnectionSecrets: &mockAISecretStore{}}
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
	s, _, _, _ := agentLegacyCoordinationFixture(t)
	env := append(os.Environ(),
		coordinationProcessHelperEnv+"=1",
		coordinationProcessWorkspaceEnv+"="+s.Workspace,
		coordinationProcessOwnerEnv+"=user:alice",
	)
	commands := make([]*exec.Cmd, 2)
	markers := make([]string, 2)
	outputs := make([]bytes.Buffer, 2)
	for i := range commands {
		markers[i] = filepath.Join(t.TempDir(), "ready-"+itoa(i))
		command := exec.Command(os.Args[0], "-test.run=TestCoordinationMigrationHelperProcess", "-test.v")
		command.Env = append(append([]string{}, env...), coordinationProcessMarkerEnv+"="+markers[i])
		command.Stdout = &outputs[i]
		command.Stderr = &outputs[i]
		if err := command.Start(); err != nil {
			t.Fatalf("start migration helper process %d: %v", i, err)
		}
		commands[i] = command
	}
	for i, command := range commands {
		if err := command.Wait(); err != nil {
			t.Fatalf("migration helper process %d failed: %v %s", i, err, outputs[i].String())
		}
	}
	for i, marker := range markers {
		body, err := os.ReadFile(marker)
		if err != nil || string(body) != "user:alice\n" {
			t.Fatalf("process %d did not read the migrated journal: %q %v", i, body, err)
		}
	}
	// The parent process still sees exactly one migrated journal afterwards.
	hub, err := s.agentCoordinationHub("user:alice")
	if err != nil {
		t.Fatal(err)
	}
	peers, err := hub.Peers("")
	if err != nil || len(peers) < 1 {
		t.Fatalf("multi-process migration lost peers: %+v %v", peers, err)
	}
	seen := map[string]bool{}
	for _, peer := range peers {
		if seen[peer.ConversationID] {
			t.Fatalf("multi-process migration duplicated peer %s", peer.ConversationID)
		}
		seen[peer.ConversationID] = true
	}
}
