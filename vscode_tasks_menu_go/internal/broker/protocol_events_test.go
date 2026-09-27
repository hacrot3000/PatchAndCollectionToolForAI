package broker

import (
	"runtime"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/session"
	"bletonfc/vscode_tasks_menu/internal/tasks"
)

func TestPatchProtocolCapabilityAndWireFallback(t *testing.T) {
	legacy := &Client{info: Info{ProtocolVersion: ProtocolVersion}}
	if legacy.SupportsPatchProtocolEvents() {
		t.Fatal("legacy broker must not advertise Patch protocol events")
	}
	if legacy.SupportsPatchProtocolCommands() {
		t.Fatal("legacy broker must not advertise Patch protocol commands")
	}
	state, err := legacy.ProtocolState("missing")
	if err != nil || state.Available {
		t.Fatalf("legacy protocol state should be unavailable without network call: state=%#v err=%v", state, err)
	}

	wire := executionToWire(tasks.Execution{TaskID: -1, ProtocolEvents: true, ProtocolCommands: true})
	if !wire.ProtocolEvents || !wire.ProtocolCommands {
		t.Fatal("new wire format lost Patch protocol flags")
	}
	roundTrip := executionFromWire(wire)
	if !roundTrip.ProtocolEvents || !roundTrip.ProtocolCommands {
		t.Fatal("wire round-trip lost Patch protocol flags")
	}

	modern := &Client{info: NewInfo(t.TempDir())}
	if runtime.GOOS != "windows" && (!modern.SupportsPatchProtocolEvents() || !modern.SupportsPatchProtocolCommands()) {
		t.Fatal("new POSIX broker must advertise Patch protocol event and command capabilities")
	}
}

func TestLegacyBrokerRejectsOwnedSessionBeforeNetworkCall(t *testing.T) {
	legacy := &Client{info: Info{ProtocolVersion: ProtocolVersion}}
	_, err := legacy.Start(tasks.Execution{
		SessionKind: tasks.SessionKindTerminal,
		OwnerUserID: "user-1",
		ProjectID:   "project-1",
	})
	if err == nil || !strings.Contains(err.Error(), CapabilitySessionOwnership) {
		t.Fatalf("owned session error = %v, want missing ownership capability", err)
	}
}

func TestClientImplementsOptionalProtocolInterfaces(t *testing.T) {
	var _ session.ProtocolStateProvider = (*Client)(nil)
	var _ session.ProtocolCommandWriter = (*Client)(nil)
}
