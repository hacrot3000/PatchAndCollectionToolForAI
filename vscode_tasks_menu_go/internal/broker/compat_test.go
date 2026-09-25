package broker

import (
	"testing"

	"bletonfc/vscode_tasks_menu/internal/tasks"
)

func TestCompatibilityServiceRoutesProtocolSessionsAroundLegacyBroker(t *testing.T) {
	legacy := &Client{info: Info{ProtocolVersion: ProtocolVersion}}
	service := NewCompatibilityService(legacy)
	defer service.Close()

	if !service.NeedsPatchProtocolFallback() {
		t.Fatal("legacy broker without Patch protocol capabilities must require compatibility fallback")
	}
	if !service.useFallback(tasks.Execution{ProtocolEvents: true}) {
		t.Fatal("Patch event session must bypass legacy broker")
	}
	if !service.useFallback(tasks.Execution{ProtocolEvents: true, ProtocolCommands: true}) {
		t.Fatal("interactive Patch protocol session must bypass legacy broker")
	}
	if service.useFallback(tasks.Execution{}) {
		t.Fatal("ordinary terminal/task sessions must remain on the long-lived broker")
	}
}

func TestCompatibilityServiceUsesModernBrokerProtocol(t *testing.T) {
	modern := &Client{info: Info{
		ProtocolVersion: ProtocolVersion,
		Capabilities: []string{
			CapabilitySessionTitle,
			CapabilityPatchProtocolEvents,
			CapabilityPatchProtocolCommands,
		},
	}}
	service := NewCompatibilityService(modern)
	defer service.Close()

	if service.NeedsPatchProtocolFallback() {
		t.Fatal("modern broker must not require Patch protocol fallback")
	}
	if service.useFallback(tasks.Execution{ProtocolEvents: true, ProtocolCommands: true}) {
		t.Fatal("modern broker should own Patch protocol sessions")
	}
}

func TestCompatibilityServiceFallsBackWhenOnlyCommandCapabilityIsMissing(t *testing.T) {
	partial := &Client{info: Info{
		ProtocolVersion: ProtocolVersion,
		Capabilities: []string{
			CapabilityPatchProtocolEvents,
		},
	}}
	service := NewCompatibilityService(partial)
	defer service.Close()

	if service.useFallback(tasks.Execution{ProtocolEvents: true}) {
		t.Fatal("read-only Patch event session can use a broker with event support")
	}
	if !service.useFallback(tasks.Execution{ProtocolEvents: true, ProtocolCommands: true}) {
		t.Fatal("interactive Patch session must fall back when broker command support is missing")
	}
}
