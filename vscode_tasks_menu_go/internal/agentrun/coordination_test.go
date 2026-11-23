package agentrun

import (
	"path/filepath"
	"testing"
)

func TestCoordinationHubPeersMessagesAndContractLifecycle(t *testing.T) {
	hub, err := NewCoordinationHub(filepath.Join(t.TempDir(), "coordination.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hub.UpsertPeer(Peer{ConversationID: "conv-1", GroupID: "group-a", Status: PeerRunning, Summary: "implementing A2"}); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.UpsertPeer(Peer{ConversationID: "conv-2", GroupID: "group-a", Status: PeerIdle}); err != nil {
		t.Fatal(err)
	}
	peers, err := hub.Peers("group-a")
	if err != nil || len(peers) != 2 {
		t.Fatalf("peers: %+v %v", peers, err)
	}
	ask, err := hub.PostPeerMessage(PeerMessage{GroupID: "group-a", FromConversation: "conv-2", ToConversation: "conv-1", Kind: "ask", Body: "what is the A2 signature?"})
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := hub.PostPeerMessage(PeerMessage{GroupID: "group-a", FromConversation: "conv-2", ToConversation: "conv-1", Kind: "ask", Body: "what is the A2 signature?"})
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.ID != ask.ID {
		t.Fatal("duplicate peer question created a second event")
	}
	answer, err := hub.PostPeerMessage(PeerMessage{GroupID: "group-a", FromConversation: "conv-1", ToConversation: "conv-2", Kind: "answer", Body: "authenticate(mac: String): AuthResult", CorrelationID: ask.CorrelationID})
	if err != nil {
		t.Fatal(err)
	}
	if answer.Status != "answered" || answer.Answer == "" {
		t.Fatalf("answer was not linked: %+v", answer)
	}
	messages, err := hub.PeerMessages("conv-2", "group-a")
	if err != nil || len(messages) != 1 {
		t.Fatalf("messages: %+v %v", messages, err)
	}
	if messages[0].Status != "answered" || messages[0].Answer == "" || messages[0].AnsweredBy != "conv-1" {
		t.Fatalf("answer was not linked to the question: %+v", messages[0])
	}
	contract, err := hub.UpsertContract(Contract{Key: "BikeService.authenticate", Signature: "authenticate(mac: String): AuthResult", OwnerTask: "A1", OwnerGroup: "group-a", Status: ContractPlanned, FilePaths: []string{"src/BikeService.kt"}})
	if err != nil {
		t.Fatal(err)
	}
	if contract.Revision != 1 {
		t.Fatalf("contract revision=%d", contract.Revision)
	}
	if _, err := hub.UpsertContract(Contract{Key: contract.Key, Signature: "changed()", OwnerTask: "A1", OwnerGroup: "group-a", Status: ContractPlanned}); err == nil {
		t.Fatal("signature changed without a revision bump")
	}
	if _, err := hub.AdvanceContract(contract.Key, contract.Revision, ContractIntegrated, "", ""); err == nil {
		t.Fatal("integrated contract accepted without an artifact reference")
	}
	artifact, err := hub.PublishArtifact(Artifact{Kind: "diff", Ref: "worktree-diff:conv-1", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ProducerRun: "run-1", Conversation: "conv-1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hub.AdvanceContract(contract.Key, contract.Revision, ContractIntegrated, artifact.ID, ""); err != nil {
		t.Fatal(err)
	}
	dependency, err := hub.RegisterDependency(Dependency{ConsumerTask: "A2", ProducerTask: "A1", ContractKey: contract.Key})
	if err != nil {
		t.Fatal(err)
	}
	if dependency.Status != DependencyPlanned || dependency.ContractRevision != contract.Revision {
		t.Fatalf("dependency: %+v", dependency)
	}
	if _, err := hub.RegisterDependency(Dependency{ConsumerTask: "A3", ProducerTask: "A4", ContractKey: contract.Key, Status: DependencyVerified}); err == nil {
		t.Fatal("verified dependency accepted without evidence")
	}
	events, err := hub.Events(0, 50)
	if err != nil || len(events) < 6 {
		t.Fatalf("events: %d %v", len(events), err)
	}
	for i := 1; i < len(events); i++ {
		if events[i].Sequence <= events[i-1].Sequence {
			t.Fatal("coordination events are not strictly ordered")
		}
	}
}

func TestCoordinationHubRejectsUnknownContractAndPeerLoop(t *testing.T) {
	hub, err := NewCoordinationHub(filepath.Join(t.TempDir(), "coordination.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hub.RegisterDependency(Dependency{ConsumerTask: "A2", ProducerTask: "A1", ContractKey: "missing"}); err == nil {
		t.Fatal("dependency referenced an unpublished contract")
	}
	if _, err := hub.PostPeerMessage(PeerMessage{FromConversation: "conv-1", ToConversation: "conv-1", Kind: "ask", Body: "self"}); err == nil {
		t.Fatal("peer message to itself accepted")
	}
	if _, err := hub.PostPeerMessage(PeerMessage{FromConversation: "conv-1", ToConversation: "conv-2", Kind: "ask", Body: "loop", HopCount: 9}); err == nil {
		t.Fatal("unbounded peer hop accepted")
	}
}
