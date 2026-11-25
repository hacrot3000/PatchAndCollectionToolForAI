package agentrun

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoordinationEventLogCoversRequiredKindsAndStaysGroupScoped(t *testing.T) {
	hub, err := NewCoordinationHub(filepath.Join(t.TempDir(), "coordination.json"), "owner-a", "project-a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hub.UpsertPeer(Peer{ConversationID: "conv-a", GroupID: "group-a", Status: PeerRunning}); err != nil {
		t.Fatal(err)
	}
	question, err := hub.PostPeerMessage(PeerMessage{GroupID: "group-a", FromConversation: "conv-a", ToConversation: "conv-b", Kind: "ask", Body: "question", CorrelationID: "corr-1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hub.PostPeerMessage(PeerMessage{GroupID: "group-a", FromConversation: "conv-b", ToConversation: "conv-a", Kind: "answer", Body: "answer", CorrelationID: question.CorrelationID}); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.UpsertContract(Contract{Key: "api.auth", OwnerGroup: "group-a", Signature: "auth(): Result", Status: ContractPlanned}); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.PublishArtifact(Artifact{ID: "artifact-1", GroupID: "group-a", Conversation: "conv-a", Kind: "diff", Ref: "diff:1", SHA256: strings.Repeat("a", 64)}); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.RegisterDependency(Dependency{GroupID: "group-a", ConversationID: "conv-a", ConsumerTask: "consumer", ProducerTask: "producer", ContractKey: "api.auth"}); err != nil {
		t.Fatal(err)
	}
	events, err := hub.EventsForScope("group-a", "", 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	kinds := make(map[string]bool, len(events))
	for _, event := range events {
		kinds[event.Kind] = true
		if event.GroupID != "group-a" {
			t.Fatalf("event escaped its group: %+v", event)
		}
		if event.Sequence == 0 {
			t.Fatalf("event lost its monotonic sequence: %+v", event)
		}
	}
	for _, kind := range []string{"peer_progress", "peer_ask", "peer_answered", "contract_published", "artifact_published", "dependency_registered"} {
		if !kinds[kind] {
			t.Fatalf("coordination event log is missing the %q kind: %v", kind, kinds)
		}
	}
	for i := 1; i < len(events); i++ {
		if events[i].Sequence <= events[i-1].Sequence {
			t.Fatal("coordination event sequence is not strictly increasing")
		}
	}
	otherGroup, err := hub.EventsForScope("group-b", "", 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(otherGroup) != 0 {
		t.Fatalf("group-b observed group-a events: %+v", otherGroup)
	}
}

func TestCoordinationHubScopesDuplicateContractKeysByGroup(t *testing.T) {
	hub, err := NewCoordinationHub(filepath.Join(t.TempDir(), "coordination.json"), "owner-a", "project-a")
	if err != nil {
		t.Fatal(err)
	}
	contractA, err := hub.UpsertContract(Contract{Key: "api.auth", OwnerGroup: "group-a", Signature: "authA(token): ResultA", Status: ContractPlanned})
	if err != nil {
		t.Fatal(err)
	}
	contractB, err := hub.UpsertContract(Contract{Key: "api.auth", OwnerGroup: "group-b", Signature: "authB(token): ResultB", Status: ContractPlanned})
	if err != nil {
		t.Fatal(err)
	}
	for groupID, want := range map[string]string{"group-a": contractA.Signature, "group-b": contractB.Signature} {
		contracts, err := hub.Contracts(groupID)
		if err != nil || len(contracts) != 1 || contracts[0].Signature != want {
			t.Fatalf("contracts for %s = %+v, err=%v; want signature %q", groupID, contracts, err, want)
		}
	}
	if _, err := hub.AdvanceContractInGroup("group-b", "api.auth", contractB.Revision, ContractIntegrated, "artifact-b", ""); err != nil {
		t.Fatal(err)
	}
	contractsA, err := hub.Contracts("group-a")
	if err != nil || len(contractsA) != 1 || contractsA[0].Status != ContractPlanned {
		t.Fatalf("advancing group-b contract mutated group-a: %+v err=%v", contractsA, err)
	}

	depA, err := hub.RegisterDependency(Dependency{GroupID: "group-a", ConversationID: "conv-a", ConsumerTask: "consumer", ProducerTask: "producer", ContractKey: "api.auth"})
	if err != nil {
		t.Fatal(err)
	}
	depB, err := hub.RegisterDependency(Dependency{GroupID: "group-b", ConversationID: "conv-b", ConsumerTask: "consumer", ProducerTask: "producer", ContractKey: "api.auth"})
	if err != nil {
		t.Fatal(err)
	}
	if depA.ID == depB.ID {
		t.Fatalf("group dependencies collided: %+v %+v", depA, depB)
	}
	depsA, err := hub.DependenciesForScope("group-a", "")
	if err != nil || len(depsA) != 1 || depsA[0].GroupID != "group-a" {
		t.Fatalf("dependencies for group-a leaked: %+v err=%v", depsA, err)
	}

	if _, err := hub.PublishArtifact(Artifact{ID: "shared-id", GroupID: "group-a", Conversation: "conv-a", Kind: "diff", Ref: "diff:group-a", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.PublishArtifact(Artifact{ID: "shared-id", GroupID: "group-b", Conversation: "conv-b", Kind: "diff", Ref: "diff:group-b", SHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}); err == nil {
		t.Fatal("artifact ID was reused across group ownership scopes")
	}
	artifactsA, err := hub.ArtifactsForScope("group-a", "")
	if err != nil || len(artifactsA) != 1 || artifactsA[0].GroupID != "group-a" {
		t.Fatalf("artifacts for group-a leaked: %+v err=%v", artifactsA, err)
	}
	eventsA, err := hub.EventsForScope("group-a", "", 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range eventsA {
		if event.GroupID != "group-a" {
			t.Fatalf("event escaped group scope: %+v", event)
		}
	}
}

func TestCoordinationHubV1MigrationPreservesDataAndBindsTenant(t *testing.T) {
	path := filepath.Join(t.TempDir(), "coordination.json")
	legacy := coordinationState{
		Version:      1,
		Peers:        map[string]Peer{"legacy-conv": {ConversationID: "legacy-conv", GroupID: "legacy-group", Status: PeerIdle}},
		Messages:     []PeerMessage{{ID: "legacy-message", GroupID: "legacy-group", FromConversation: "legacy-conv", ToConversation: "peer", Kind: "ask", Body: "legacy question", Status: "pending"}},
		Contracts:    map[string]Contract{"legacy.api": {Key: "legacy.api", OwnerGroup: "legacy-group", Signature: "legacy(): Result", Revision: 3, Status: ContractImplemented}},
		Artifacts:    map[string]Artifact{"legacy-artifact": {ID: "legacy-artifact", Kind: "diff", Ref: "legacy-ref", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Conversation: "legacy-conv"}},
		Dependencies: map[string]Dependency{"legacy-dep": {ID: "legacy-dep", ConsumerTask: "consumer", ProducerTask: "producer", ContractKey: "legacy.api", ContractRevision: 3}},
		Events:       []CoordinationEvent{{Sequence: 1, ID: "legacy-event", GroupID: "legacy-group", Kind: "legacy", Actor: "legacy-conv"}},
		EventSeq:     1,
	}
	body, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(body, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	hub, err := NewCoordinationHub(path, "owner-a", "project-a")
	if err != nil {
		t.Fatal(err)
	}
	contracts, err := hub.Contracts("legacy-group")
	if err != nil || len(contracts) != 1 || contracts[0].Revision != 3 {
		t.Fatalf("legacy contract migration failed: %+v err=%v", contracts, err)
	}
	messages, err := hub.PeerMessages("legacy-conv", "legacy-group")
	if err != nil || len(messages) != 1 || messages[0].ID != "legacy-message" {
		t.Fatalf("legacy messages were not preserved: %+v err=%v", messages, err)
	}
	artifacts, err := hub.ArtifactsForScope("legacy-group", "legacy-conv")
	if err != nil || len(artifacts) != 1 || artifacts[0].ID != "legacy-artifact" {
		t.Fatalf("legacy artifact was not available in its conversation scope: %+v err=%v", artifacts, err)
	}
	dependencies, err := hub.DependenciesForScope("legacy-group", "")
	if err != nil || len(dependencies) != 1 || dependencies[0].GroupID != "legacy-group" {
		t.Fatalf("legacy dependency ownership was not migrated: %+v err=%v", dependencies, err)
	}
	events, err := hub.EventsForScope("legacy-group", "", 0, 10)
	if err != nil || len(events) != 1 || events[0].GroupID != "legacy-group" {
		t.Fatalf("legacy events were not preserved in the group scope: %+v err=%v", events, err)
	}
	if _, err := hub.UpsertPeer(Peer{ConversationID: "next-conv", GroupID: "legacy-group", Status: PeerRunning}); err != nil {
		t.Fatal(err)
	}
	persisted, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var migrated coordinationState
	if err := json.Unmarshal(persisted, &migrated); err != nil {
		t.Fatal(err)
	}
	if migrated.Version != 2 || migrated.OwnerID != "owner-a" || migrated.ProjectID != "project-a" {
		t.Fatalf("v1 data did not persist as tenant-bound v2 state: %+v", migrated)
	}
	other, err := NewCoordinationHub(path, "owner-b", "project-a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Peers("legacy-group"); err == nil {
		t.Fatal("coordination journal was readable under another owner")
	}
	wrongProject, err := NewCoordinationHub(path, "owner-a", "project-b")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wrongProject.Peers("legacy-group"); err == nil {
		t.Fatal("coordination journal was readable under another project")
	}
}
