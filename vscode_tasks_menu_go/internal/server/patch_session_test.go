package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPatchModeArgumentsAreBounded(t *testing.T) {
	tests := []struct {
		mode string
		want []string
	}{
		{"", nil},
		{"queue", nil},
		{"resume", []string{"resume"}},
		{"history", []string{"report"}},
		{"plan", []string{"plan"}},
	}
	for _, tc := range tests {
		got, _, err := patchModeArguments(tc.mode)
		if err != nil {
			t.Fatalf("mode %q: %v", tc.mode, err)
		}
		if strings.Join(got, "\x00") != strings.Join(tc.want, "\x00") {
			t.Fatalf("mode %q args=%v want %v", tc.mode, got, tc.want)
		}
	}
	if _, _, err := patchModeArguments("arbitrary --flag"); err == nil {
		t.Fatal("web Patch mode must reject arbitrary arguments")
	}
}

func TestPatchToolExecutionUsesBuiltinRuntimeAndReservedTaskID(t *testing.T) {
	workspace := t.TempDir()
	runtimeRoot := t.TempDir()
	entry := filepath.Join(runtimeRoot, "python_patch_entry.py")
	python := filepath.Join(runtimeRoot, "python3")
	for _, path := range []string{entry, python} {
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("TASKDECK_PATCH_RUNTIME", entry)
	t.Setenv("TASKDECK_PATCH_PYTHON", python)

	spec, err := patchToolExecution(workspace, "resume")
	if err != nil {
		t.Fatal(err)
	}
	if spec.TaskID != -1 {
		t.Fatalf("Patch Tool task id=%d want -1", spec.TaskID)
	}
	if !spec.ProtocolEvents {
		t.Fatal("built-in Patch session must request the optional protocol event channel")
	}
	if !spec.ProtocolCommands {
		t.Fatal("built-in Patch session must enable protocol commands now that queue_selection prompt events are defined")
	}
	if spec.Label != "Patch Tool · Resume" {
		t.Fatalf("label=%q", spec.Label)
	}
	if spec.Command != python {
		t.Fatalf("command=%q want %q", spec.Command, python)
	}
	joined := strings.Join(spec.Args, "\n")
	for _, want := range []string{entry, "--project-root", filepath.Clean(workspace), "--", "resume"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args=%v missing %q", spec.Args, want)
		}
	}
	if spec.Cwd != filepath.Clean(workspace) {
		t.Fatalf("cwd=%q want %q", spec.Cwd, workspace)
	}
}

func TestSessionsAPIHasBuiltinPatchKind(t *testing.T) {
	data, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	for _, want := range []string{
		"case \"patch\":",
		"patchToolExecution(s.Workspace, req.PatchMode)",
		"PatchMode string",
		"json:\"patch_mode,omitempty\"",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("Patch session API missing %q", want)
		}
	}
}


func TestSessionProtocolEndpointIsOptional(t *testing.T) {
	data, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	for _, want := range []string{
		"case \"protocol\":",
		"session.ProtocolStateProvider",
		"session.ProtocolState{Available: false}",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("optional protocol endpoint missing %q", want)
		}
	}
}
