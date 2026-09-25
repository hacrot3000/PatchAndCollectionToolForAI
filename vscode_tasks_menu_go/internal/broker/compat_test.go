package broker

import (
	"runtime"
	"testing"
	"time"

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


func TestCompatibilityServiceLegacyBrokerDeliversPatchProtocolState(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Patch protocol event FD is Unix-only")
	}
	legacy := &Client{info: Info{ProtocolVersion: ProtocolVersion}}
	service := NewCompatibilityService(legacy)
	defer service.Close()

	event := "{"protocol":"taskdeck.patch","version":1,"type":"health_snapshot","seq":1,"status":"PASS","tool_version":"compat-test","summary":{"pass":1,"warn":0,"fail":0,"total":1},"checks":[{"name":"fallback","status":"PASS"}]}"
	meta, err := service.Start(tasks.Execution{
		TaskID: -1,
		Label: "Patch Tool · Health test",
		Command: "/bin/sh",
		Args: []string{"-c", "printf '%s\\n' '" + event + "' >&3"},
		Cwd: t.TempDir(),
		ProtocolEvents: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		state, err := service.ProtocolState(meta.ID)
		if err != nil {
			t.Fatal(err)
		}
		if state.HealthSnapshot != nil {
			if !state.Available || !state.Enabled {
				t.Fatalf("fallback protocol state unavailable: %#v", state)
			}
			if state.HealthSnapshot.ToolVersion != "compat-test" || state.HealthSnapshot.Status != "PASS" {
				t.Fatalf("unexpected health snapshot: %#v", state.HealthSnapshot)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	state, err := service.ProtocolState(meta.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Fatalf("legacy-broker fallback did not deliver health_snapshot: %#v", state)
}
