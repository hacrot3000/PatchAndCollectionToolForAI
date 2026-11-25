package server

import (
	"strings"
	"testing"
)

func m1711Evidence() WorkspaceDriftEvidence {
	return WorkspaceDriftEvidence{
		PinnedPackRef:       "execution-pack-sha256:" + strings.Repeat("a", 64),
		FreshPackRef:        "execution-pack-sha256:" + strings.Repeat("a", 64),
		PinnedWorkspaceID:   "ws-1",
		FreshWorkspaceID:    "ws-1",
		PinnedProjectID:     "proj-1",
		FreshProjectID:      "proj-1",
		PinnedProviderID:    "p1",
		FreshProviderID:     "p1",
		PinnedConfigVersion: strings.Repeat("b", 64),
		FreshConfigVersion:  strings.Repeat("b", 64),
	}
}

func TestClassifyWorkspaceDriftStable(t *testing.T) {
	d, reason := ClassifyWorkspaceDrift(m1711Evidence())
	if d != DriftStable || reason != "" {
		t.Fatalf("stable workspace misclassified: %s / %s", d, reason)
	}
}

func TestClassifyWorkspaceDriftBindingChange(t *testing.T) {
	fields := []struct {
		name string
		mut  func(*WorkspaceDriftEvidence)
	}{
		{"workspace", func(e *WorkspaceDriftEvidence) { e.FreshWorkspaceID = "ws-2" }},
		{"project", func(e *WorkspaceDriftEvidence) { e.FreshProjectID = "proj-2" }},
		{"provider", func(e *WorkspaceDriftEvidence) { e.FreshProviderID = "p2" }},
		{"config", func(e *WorkspaceDriftEvidence) { e.FreshConfigVersion = strings.Repeat("c", 64) }},
	}
	for _, tc := range fields {
		t.Run(tc.name, func(t *testing.T) {
			e := m1711Evidence()
			tc.mut(&e)
			d, reason := ClassifyWorkspaceDrift(e)
			if d != DriftBindingChange || reason != "workspace_or_provider_binding_changed" {
				t.Fatalf("binding change not blocked: %s / %s", d, reason)
			}
		})
	}
}

func TestClassifyWorkspaceDriftUnverifiable(t *testing.T) {
	e := m1711Evidence()
	e.PinnedProviderID = ""
	if d, _ := ClassifyWorkspaceDrift(e); d != DriftUnverifiable {
		t.Fatalf("incomplete binding must be unverifiable: %s", d)
	}
	e = m1711Evidence()
	e.FreshPackRef = ""
	if d, _ := ClassifyWorkspaceDrift(e); d != DriftUnverifiable {
		t.Fatalf("missing pack ref must be unverifiable: %s", d)
	}
}

func TestClassifyWorkspaceDriftAuthorizedOnlyWithMatchingReceipt(t *testing.T) {
	base := m1711Evidence()
	base.FreshPackRef = "execution-pack-sha256:" + strings.Repeat("d", 64)
	if d, r := ClassifyWorkspaceDrift(base); d != DriftUnrelated || r != "pack_changed_without_receipt" {
		t.Fatalf("pack change without receipt must block: %s / %s", d, r)
	}
	e := base
	e.AuthorizedReceiptRunID = "run-1"
	e.AuthorizedReceiptDigest = strings.Repeat("e", 64)
	e.ExpectedReceiptRunID = "run-2"
	e.ExpectedReceiptDigest = strings.Repeat("e", 64)
	if d, r := ClassifyWorkspaceDrift(e); d != DriftUnrelated || r != "receipt_does_not_match_expected_mutation" {
		t.Fatalf("wrong run receipt must not authorize: %s / %s", d, r)
	}
	e.ExpectedReceiptRunID = "run-1"
	e.ExpectedReceiptDigest = strings.Repeat("f", 64)
	if d, _ := ClassifyWorkspaceDrift(e); d != DriftUnrelated {
		t.Fatalf("wrong digest must not authorize: %s", d)
	}
	e.ExpectedReceiptDigest = strings.Repeat("e", 64)
	if d, r := ClassifyWorkspaceDrift(e); d != DriftAuthorizedMutation || r != "verified_receipt_permits_fresh_checkpoint" {
		t.Fatalf("exact receipt must authorize a fresh checkpoint: %s / %s", d, r)
	}
}

func TestCheckpointLineageID(t *testing.T) {
	if checkpointLineageID("", "fresh", "digest") != "" {
		t.Fatal("missing prior ref must have no lineage")
	}
	a := checkpointLineageID("old", "fresh", "digest")
	if len(a) != 64 || a != checkpointLineageID("old", "fresh", "digest") {
		t.Fatal("lineage must be a stable 64-hex digest")
	}
	if a == checkpointLineageID("old", "fresh", "other") {
		t.Fatal("different receipt must change lineage")
	}
	if a == checkpointLineageID("old", "other", "digest") {
		t.Fatal("different fresh ref must change lineage")
	}
}
