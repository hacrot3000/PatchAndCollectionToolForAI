package server

import (
	"crypto/sha256"
	"encoding/hex"
)

// WorkspaceDriftDisposition is the bounded outcome of comparing a fresh
// execution-workspace/pack observation against an immutable run snapshot.
// It is content-free: identifiers and digests only. It never reads files,
// Git, provider state, credentials or model output, and never executes a
// native tool. Adopting a fresh checkpoint from AuthorizedMutation remains a
// separate, owner-reviewed wiring change; this primitive only classifies.
type WorkspaceDriftDisposition string

const (
	DriftStable             WorkspaceDriftDisposition = "stable"
	DriftAuthorizedMutation WorkspaceDriftDisposition = "authorized_mutation"
	DriftUnrelated          WorkspaceDriftDisposition = "unrelated_drift"
	DriftBindingChange      WorkspaceDriftDisposition = "binding_change"
	DriftUnverifiable       WorkspaceDriftDisposition = "unverifiable"
)

// WorkspaceDriftEvidence is the minimal server-derived evidence needed for
// the decision. Raw AI Pack text, file contents and provider output are
// deliberately not fields: only digests and identifiers are accepted.
type WorkspaceDriftEvidence struct {
	PinnedPackRef       string
	FreshPackRef        string
	PinnedWorkspaceID   string
	FreshWorkspaceID    string
	PinnedProjectID     string
	FreshProjectID      string
	PinnedProviderID    string
	FreshProviderID     string
	PinnedConfigVersion string
	FreshConfigVersion  string

	// An authorized mutation is proven only by an already-verified native
	// receipt whose identity and digest match the operation that changed the
	// pack. The caller derives Expected* from that exact approved operation;
	// model text, titles and raw stdout can never fill these fields.
	AuthorizedReceiptRunID  string
	AuthorizedReceiptDigest string
	ExpectedReceiptRunID    string
	ExpectedReceiptDigest   string
}

// ClassifyWorkspaceDrift fails closed. A changed workspace/project/provider/
// config binding is always a hard block (no silent workspace or provider
// switch). A changed pack is adopted only when an exact verified receipt for
// this run authorizes the mutation; otherwise it is unrelated drift.
func ClassifyWorkspaceDrift(e WorkspaceDriftEvidence) (WorkspaceDriftDisposition, string) {
	if e.PinnedWorkspaceID == "" || e.PinnedProjectID == "" ||
		e.PinnedProviderID == "" || e.PinnedConfigVersion == "" {
		return DriftUnverifiable, "incomplete_pinned_binding"
	}
	if e.FreshWorkspaceID != e.PinnedWorkspaceID ||
		e.FreshProjectID != e.PinnedProjectID ||
		e.FreshProviderID != e.PinnedProviderID ||
		e.FreshConfigVersion != e.PinnedConfigVersion {
		return DriftBindingChange, "workspace_or_provider_binding_changed"
	}
	if e.PinnedPackRef == "" || e.FreshPackRef == "" {
		return DriftUnverifiable, "missing_pack_reference"
	}
	if e.PinnedPackRef == e.FreshPackRef {
		return DriftStable, ""
	}
	if e.AuthorizedReceiptRunID == "" || e.AuthorizedReceiptDigest == "" ||
		e.ExpectedReceiptRunID == "" || e.ExpectedReceiptDigest == "" {
		return DriftUnrelated, "pack_changed_without_receipt"
	}
	if e.AuthorizedReceiptRunID != e.ExpectedReceiptRunID ||
		e.AuthorizedReceiptDigest != e.ExpectedReceiptDigest {
		return DriftUnrelated, "receipt_does_not_match_expected_mutation"
	}
	return DriftAuthorizedMutation, "verified_receipt_permits_fresh_checkpoint"
}

// checkpointLineageID binds a fresh checkpoint to the exact prior pack ref and
// the authorizing receipt digest. The old snapshot and provider binding stay
// independently verifiable; an empty result means the lineage is not proven.
func checkpointLineageID(oldRef, freshRef, receiptDigest string) string {
	if oldRef == "" || freshRef == "" || receiptDigest == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(oldRef + "\n" + freshRef + "\n" + receiptDigest))
	return hex.EncodeToString(sum[:])
}
