package server

import (
	"os"
	"path/filepath"
	"testing"
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
