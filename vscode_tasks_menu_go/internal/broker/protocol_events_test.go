package broker

import (
	"runtime"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/session"
	"bletonfc/vscode_tasks_menu/internal/tasks"
)

func TestPatchProtocolCapabilityAndWireFallback(t *testing.T) {
	legacy := &Client{info: Info{ProtocolVersion: ProtocolVersion}}
	if legacy.SupportsPatchProtocolEvents() {
		t.Fatal("legacy broker must not advertise Patch protocol events")
	}
	state, err := legacy.ProtocolState("missing")
	if err != nil || state.Available {
		t.Fatalf("legacy protocol state should be unavailable without network call: state=%#v err=%v", state, err)
	}

	wire := executionToWire(tasks.Execution{TaskID: -1, ProtocolEvents: true})
	if !wire.ProtocolEvents {
		t.Fatal("new wire format lost ProtocolEvents")
	}
	roundTrip := executionFromWire(wire)
	if !roundTrip.ProtocolEvents {
		t.Fatal("wire round-trip lost ProtocolEvents")
	}

	modern := &Client{info: NewInfo(t.TempDir())}
	if runtime.GOOS != "windows" && !modern.SupportsPatchProtocolEvents() {
		t.Fatal("new POSIX broker must advertise Patch protocol events")
	}
}

func TestClientImplementsOptionalProtocolStateProvider(t *testing.T) {
	var _ session.ProtocolStateProvider = (*Client)(nil)
}
