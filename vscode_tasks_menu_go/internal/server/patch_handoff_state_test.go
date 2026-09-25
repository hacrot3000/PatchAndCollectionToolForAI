package server

import (
	"os"
	"strings"
	"testing"
)

func TestTaskDeckPatchHandoffMatchesIncompletePhase2State(t *testing.T) {
	read := func(path string) string {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		return string(data)
	}

	handoff := read("../../../TASKDECK_PATCH_HANDOFF.md")
	plan := read("../../../TASKDECK_PATCH_ADDON_PLAN.md")
	readme := read("../../../README.md")

	for _, want := range []string{
		"Integration is NOT complete. Phase 2 is IN PROGRESS.",
		"Phase 2E.2 — Installed-runtime browser smoke — PENDING",
		"Do not mark Phase 2E DONE until this real-runtime smoke passes.",
		"Phase 2F — Cutover closeout — PENDING",
		"89d3a458",
	} {
		if !strings.Contains(handoff, want) {
			t.Fatalf("TaskDeck Patch handoff missing recovery invariant %q", want)
		}
	}

	for _, want := range []string{
		"Status: **IN PROGRESS**",
		"- [ ] Phase 2E.2 installed-runtime browser smoke after self-update",
		"- [ ] **Phase 2F — Cutover closeout.**",
		"TASKDECK_PATCH_HANDOFF.md",
	} {
		if !strings.Contains(plan, want) {
			t.Fatalf("TaskDeck Patch roadmap missing incomplete-state contract %q", want)
		}
	}

	for _, want := range []string{
		"browser smoke trên bản TaskDeck đã self-update vẫn còn pending",
		"installed-runtime browser smoke after self-update is still pending",
		"TASKDECK_PATCH_HANDOFF.md",
	} {
		if !strings.Contains(readme, want) {
			t.Fatalf("README must not overstate native Patch completion; missing %q", want)
		}
	}

	for _, forbidden := range []string{
		"Integration roadmap complete.",
		"Status: **DONE**\n\n> **Correction 2026-09-25:**",
	} {
		if strings.Contains(plan, forbidden) {
			t.Fatalf("TaskDeck Patch roadmap regressed to premature completion marker %q", forbidden)
		}
	}
}
