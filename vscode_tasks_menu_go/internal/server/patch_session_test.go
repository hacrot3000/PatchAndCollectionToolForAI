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
		{"health", []string{"health"}},
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
	nativeResume := false
	for _, item := range spec.Env {
		if item == "TASKDECK_PATCH_NATIVE_RESUME=1" {
			nativeResume = true
			break
		}
	}
	if !nativeResume {
		t.Fatal("built-in Resume session must explicitly opt into native Resume")
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
		"patchToolExecutionForUI(s.Workspace, req.PatchMode, req.PatchUI)",
		"PatchMode string",
		"json:\"patch_mode,omitempty\"",
		"PatchUI   string",
		"json:\"patch_ui,omitempty\"",
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


func TestPatchQueueDoesNotOptIntoNativeResume(t *testing.T) {
	workspace := t.TempDir()
	runtimeRoot := t.TempDir()
	entry := filepath.Join(runtimeRoot, "python_patch_entry.py")
	python := filepath.Join(runtimeRoot, "python3")
	for _, path := range []string{entry, python} {
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil { t.Fatal(err) }
	}
	t.Setenv("TASKDECK_PATCH_RUNTIME", entry)
	t.Setenv("TASKDECK_PATCH_PYTHON", python)
	t.Setenv("TASKDECK_PATCH_NATIVE_RESUME", "0")
	spec, err := patchToolExecution(workspace, "queue")
	if err != nil { t.Fatal(err) }
	for _, item := range spec.Env {
		if item == "TASKDECK_PATCH_NATIVE_RESUME=1" {
			t.Fatal("ordinary Queue session must not opt into native Resume")
		}
	}
}


func TestPatchHistoryOptsIntoNativeHistoryOnly(t *testing.T) {
	workspace := t.TempDir()
	runtimeRoot := t.TempDir()
	entry := filepath.Join(runtimeRoot, "python_patch_entry.py")
	python := filepath.Join(runtimeRoot, "python3")
	for _, path := range []string{entry, python} {
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil { t.Fatal(err) }
	}
	t.Setenv("TASKDECK_PATCH_RUNTIME", entry)
	t.Setenv("TASKDECK_PATCH_PYTHON", python)

	history, err := patchToolExecution(workspace, "history")
	if err != nil { t.Fatal(err) }
	found := false
	for _, item := range history.Env {
		if item == "TASKDECK_PATCH_NATIVE_HISTORY=1" { found = true }
	}
	if !found { t.Fatal("built-in History session must explicitly opt into native History") }

	queue, err := patchToolExecution(workspace, "queue")
	if err != nil { t.Fatal(err) }
	for _, item := range queue.Env {
		if item == "TASKDECK_PATCH_NATIVE_HISTORY=1" {
			t.Fatal("ordinary Queue session must not opt into native History")
		}
	}
}


func TestPatchHealthIsReadOnlyProtocolSession(t *testing.T) {
	workspace := t.TempDir()
	runtimeRoot := t.TempDir()
	entry := filepath.Join(runtimeRoot, "python_patch_entry.py")
	python := filepath.Join(runtimeRoot, "python3")
	for _, path := range []string{entry, python} {
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil { t.Fatal(err) }
	}
	t.Setenv("TASKDECK_PATCH_RUNTIME", entry)
	t.Setenv("TASKDECK_PATCH_PYTHON", python)
	spec, err := patchToolExecution(workspace, "health")
	if err != nil { t.Fatal(err) }
	if !spec.ProtocolEvents {
		t.Fatal("Health must keep structured protocol events enabled")
	}
	if spec.ProtocolCommands {
		t.Fatal("Health is read-only and must not allocate the protocol command channel")
	}
	if spec.Label != "Patch Tool · Health" {
		t.Fatalf("unexpected Health label %q", spec.Label)
	}
}


func TestPatchToolTerminalUIModeDisablesNativeProtocol(t *testing.T) {
	workspace := t.TempDir()
	runtimeRoot := t.TempDir()
	entry := filepath.Join(runtimeRoot, "python_patch_entry.py")
	python := filepath.Join(runtimeRoot, "python3")
	for _, path := range []string{entry, python} {
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil { t.Fatal(err) }
	}
	t.Setenv("TASKDECK_PATCH_RUNTIME", entry)
	t.Setenv("TASKDECK_PATCH_PYTHON", python)

	spec, err := patchToolExecutionForUI(workspace, "resume", "terminal")
	if err != nil { t.Fatal(err) }
	if spec.ProtocolEvents || spec.ProtocolCommands {
		t.Fatalf("terminal legacy mode must not allocate Patch protocol channels: events=%v commands=%v", spec.ProtocolEvents, spec.ProtocolCommands)
	}
	for _, item := range spec.Env {
		if strings.HasPrefix(item, "TASKDECK_PATCH_NATIVE_") {
			t.Fatalf("terminal legacy mode must not opt into native Python routes: %q", item)
		}
	}
}

func TestPatchToolNativeUIModeKeepsProtocol(t *testing.T) {
	workspace := t.TempDir()
	runtimeRoot := t.TempDir()
	entry := filepath.Join(runtimeRoot, "python_patch_entry.py")
	python := filepath.Join(runtimeRoot, "python3")
	for _, path := range []string{entry, python} {
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil { t.Fatal(err) }
	}
	t.Setenv("TASKDECK_PATCH_RUNTIME", entry)
	t.Setenv("TASKDECK_PATCH_PYTHON", python)

	spec, err := patchToolExecutionForUI(workspace, "history", "native")
	if err != nil { t.Fatal(err) }
	if !spec.ProtocolEvents || !spec.ProtocolCommands {
		t.Fatalf("native mode must keep Patch protocol channels: events=%v commands=%v", spec.ProtocolEvents, spec.ProtocolCommands)
	}
	found := false
	for _, item := range spec.Env {
		if item == "TASKDECK_PATCH_NATIVE_HISTORY=1" { found = true }
	}
	if !found { t.Fatal("native History must opt into TASKDECK_PATCH_NATIVE_HISTORY") }
	if _, err := patchToolExecutionForUI(workspace, "queue", "unexpected"); err == nil {
		t.Fatal("unknown Patch UI mode must be rejected")
	}
}


func TestPatchCollectExecutionUsesDirectHeadlessProtocol(t *testing.T) {
	workspace := t.TempDir()
	runtimeRoot := t.TempDir()
	entry := filepath.Join(runtimeRoot, "python_patch_entry.py")
	python := filepath.Join(runtimeRoot, "python3")
	for _, path := range []string{entry, python} {
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil { t.Fatal(err) }
	}
	t.Setenv("TASKDECK_PATCH_RUNTIME", entry)
	t.Setenv("TASKDECK_PATCH_PYTHON", python)
	spec, err := patchCollectExecution(workspace, "CODE_COLLECTION_REQUEST_demo.zip")
	if err != nil { t.Fatal(err) }
	if spec.TaskID != -1 || !spec.ProtocolEvents || spec.ProtocolCommands {
		t.Fatalf("direct COLLECT execution contract lost: task=%d events=%v commands=%v", spec.TaskID, spec.ProtocolEvents, spec.ProtocolCommands)
	}
	joined := strings.Join(spec.Args, "\n")
	for _, want := range []string{"collect", "request", "patchs/CODE_COLLECTION_REQUEST_demo.zip"} {
		if !strings.Contains(joined, want) { t.Fatalf("direct COLLECT args missing %q: %v", want, spec.Args) }
	}
	env := strings.Join(spec.Env, "\n")
	for _, want := range []string{"TASKDECK_PATCH_DIRECT_COLLECT=1","TASKDECK_PATCH_PROGRESS_ITEM_NAME=CODE_COLLECTION_REQUEST_demo.zip","TASKDECK_PATCH_PROGRESS_ITEM_KIND=COLLECT"} {
		if !strings.Contains(env,want) { t.Fatalf("direct COLLECT env missing %q",want) }
	}
	for _, invalid := range []string{"../escape.zip","sub/request.zip","request.txt",""} {
		if _, err := patchCollectExecution(workspace, invalid); err == nil { t.Fatalf("unsafe direct COLLECT name accepted: %q",invalid) }
	}
}
