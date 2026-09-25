package session

import (
	"os"
	"strings"
	"testing"
)

func TestProtocolHistoryCleanupLegacyResultIsValidatedRetainedAndCloned(t *testing.T) {
	s := &managedSession{protocol: ProtocolState{Available:true, Enabled:true}}
	s.applyProtocolLine([]byte(`{"protocol":"taskdeck.patch","version":1,"type":"history_cleanup_result","seq":2,"prompt_id":"p1","cleanup_id":"c1","status":"PASS","rc":0,"history_changed":true,"removed":3,"pinned":2,"remaining":30,"policy":"remove_unpinned_idle_then_oldest_unpinned_over_limit","eligible_before":4,"message":"History cleanup removed 3 entries"}`))
	if s.protocol.Error != "" || s.protocol.HistoryCleanup == nil {
		t.Fatalf("valid legacy History cleanup rejected: %#v", s.protocol)
	}
	got := s.protocol.HistoryCleanup
	if got.CleanupID!="c1" || got.Mode!="retention" || got.Removed!=3 || got.Pinned!=2 || got.Remaining!=30 || !got.HistoryChanged {
		t.Fatalf("unexpected legacy History cleanup state: %#v", got)
	}
	clone := cloneProtocolState(s.protocol)
	if clone.HistoryCleanup == nil || clone.HistoryCleanup.CleanupID != "c1" {
		t.Fatal("cloned protocol state lost legacy History cleanup result")
	}
}

func TestProtocolHistoryAgeCleanupPreviewIsValidatedRetainedAndDeepCloned(t *testing.T) {
	digest := strings.Repeat("a",64)
	line := `{"protocol":"taskdeck.patch","version":1,"type":"history_cleanup_result","seq":2,"prompt_id":"p1","cleanup_id":"preview-1","mode":"preview","status":"PASS","rc":0,"history_changed":false,"removed":0,"pinned":1,"remaining":4,"policy":"remove_unpinned_older_than_days","eligible_before":2,"older_than_days":30,"cutoff_at":"2026-08-26T10:00:00+00:00","candidate_digest":"`+digest+`","candidates":[{"run_id":"r1","status":"PASS","started_at":"2026-07-01T00:00:00+00:00","display_time":"2026-07-01 07:00:00","primary_name":"patch_a.zip","item_count":1},{"run_id":"r2","status":"FAIL","started_at":"2026-07-02T00:00:00+00:00","display_time":"2026-07-02 07:00:00","primary_name":"CODE_COLLECTION_REQUEST_b.zip","item_count":2}],"message":"2 unpinned History entries are older than 30 days"}`
	s := &managedSession{protocol: ProtocolState{Available:true, Enabled:true}}
	s.applyProtocolLine([]byte(line))
	if s.protocol.Error != "" || s.protocol.HistoryCleanup == nil {
		t.Fatalf("valid age cleanup preview rejected: %#v", s.protocol)
	}
	got := s.protocol.HistoryCleanup
	if got.Mode!="preview" || got.OlderThanDays!=30 || got.EligibleBefore!=2 ||
		got.CandidateDigest!=digest || len(got.Candidates)!=2 || got.Candidates[1].PrimaryName!="CODE_COLLECTION_REQUEST_b.zip" {
		t.Fatalf("unexpected age cleanup preview state: %#v", got)
	}

	clone := cloneProtocolState(s.protocol)
	if clone.HistoryCleanup == nil || len(clone.HistoryCleanup.Candidates)!=2 {
		t.Fatal("cloned protocol state lost cleanup candidates")
	}
	clone.HistoryCleanup.Candidates[0].PrimaryName = "changed"
	if s.protocol.HistoryCleanup.Candidates[0].PrimaryName != "patch_a.zip" {
		t.Fatal("cleanup candidate slice was shallow-cloned")
	}
}

func TestProtocolHistoryAgeCleanupDeleteIsValidated(t *testing.T) {
	digest := strings.Repeat("b",64)
	line := `{"protocol":"taskdeck.patch","version":1,"type":"history_cleanup_result","seq":3,"prompt_id":"p1","cleanup_id":"delete-1","preview_cleanup_id":"preview-1","mode":"delete","status":"PASS","rc":0,"history_changed":true,"removed":2,"pinned":1,"remaining":2,"policy":"remove_unpinned_older_than_days","eligible_before":2,"older_than_days":30,"cutoff_at":"2026-08-26T10:00:00+00:00","candidate_digest":"`+digest+`","message":"History cleanup removed 2 entries older than 30 days"}`
	s := &managedSession{protocol: ProtocolState{Available:true, Enabled:true}}
	s.applyProtocolLine([]byte(line))
	if s.protocol.Error != "" || s.protocol.HistoryCleanup == nil {
		t.Fatalf("valid age cleanup delete rejected: %#v", s.protocol)
	}
	got := s.protocol.HistoryCleanup
	if got.Mode!="delete" || got.PreviewCleanupID!="preview-1" || got.Removed!=2 || !got.HistoryChanged {
		t.Fatalf("unexpected age cleanup delete state: %#v", got)
	}
}

func TestProtocolHistoryAgeCleanupStalePreviewFailureIsRetainedWithoutMutation(t *testing.T) {
	digest := strings.Repeat("d",64)
	line := `{"protocol":"taskdeck.patch","version":1,"type":"history_cleanup_result","seq":4,"prompt_id":"p1","cleanup_id":"delete-2","preview_cleanup_id":"preview-1","mode":"delete","status":"FAIL","rc":2,"history_changed":false,"removed":0,"pinned":1,"remaining":4,"policy":"remove_unpinned_older_than_days","eligible_before":1,"older_than_days":30,"cutoff_at":"2026-08-26T10:00:00+00:00","candidate_digest":"`+digest+`","message":"History changed after cleanup preview; preview again before deleting"}`
	s := &managedSession{protocol: ProtocolState{Available:true, Enabled:true}}
	s.applyProtocolLine([]byte(line))
	if s.protocol.Error != "" || s.protocol.HistoryCleanup == nil {
		t.Fatalf("valid stale-preview cleanup failure rejected: %#v", s.protocol)
	}
	got := s.protocol.HistoryCleanup
	if got.Status!="FAIL" || got.RC!=2 || got.Removed!=0 || got.HistoryChanged {
		t.Fatalf("stale-preview cleanup failure mutated state: %#v", got)
	}
}

func TestProtocolHistoryCleanupResultRejectsInconsistentPayload(t *testing.T) {
	digest := strings.Repeat("c",64)
	for _, line := range []string{
		`{"protocol":"taskdeck.patch","version":1,"type":"history_cleanup_result","seq":1,"prompt_id":"","cleanup_id":"c","status":"PASS","rc":0,"removed":0,"pinned":0,"remaining":0,"policy":"remove_unpinned_idle_then_oldest_unpinned_over_limit","eligible_before":0}`,
		`{"protocol":"taskdeck.patch","version":1,"type":"history_cleanup_result","seq":1,"prompt_id":"p","cleanup_id":"c","status":"PASS","rc":0,"history_changed":false,"removed":2,"pinned":0,"remaining":0,"policy":"remove_unpinned_idle_then_oldest_unpinned_over_limit","eligible_before":1}`,
		`{"protocol":"taskdeck.patch","version":1,"type":"history_cleanup_result","seq":1,"prompt_id":"p","cleanup_id":"c","status":"PASS","rc":0,"history_changed":true,"removed":1,"pinned":0,"remaining":0,"policy":"other","eligible_before":1}`,
		`{"protocol":"taskdeck.patch","version":1,"type":"history_cleanup_result","seq":1,"prompt_id":"p","cleanup_id":"c","mode":"preview","status":"PASS","rc":0,"history_changed":true,"removed":1,"pinned":0,"remaining":0,"policy":"remove_unpinned_older_than_days","eligible_before":1,"older_than_days":30,"cutoff_at":"2026-08-26T10:00:00+00:00","candidate_digest":"`+digest+`","candidates":[{"run_id":"r","status":"PASS","primary_name":"a.zip","item_count":1}]}`,
		`{"protocol":"taskdeck.patch","version":1,"type":"history_cleanup_result","seq":1,"prompt_id":"p","cleanup_id":"c","mode":"preview","status":"PASS","rc":0,"history_changed":false,"removed":0,"pinned":0,"remaining":0,"policy":"remove_unpinned_older_than_days","eligible_before":2,"older_than_days":30,"cutoff_at":"2026-08-26T10:00:00+00:00","candidate_digest":"`+digest+`","candidates":[{"run_id":"r","status":"PASS","primary_name":"a.zip","item_count":1}]}`,
		`{"protocol":"taskdeck.patch","version":1,"type":"history_cleanup_result","seq":1,"prompt_id":"p","cleanup_id":"c","mode":"delete","status":"PASS","rc":0,"history_changed":true,"removed":1,"pinned":0,"remaining":0,"policy":"remove_unpinned_older_than_days","eligible_before":1,"older_than_days":30,"cutoff_at":"2026-08-26T10:00:00+00:00","candidate_digest":"`+digest+`"}`,
		`{"protocol":"taskdeck.patch","version":1,"type":"history_cleanup_result","seq":1,"prompt_id":"p","cleanup_id":"c","mode":"preview","status":"PASS","rc":0,"history_changed":false,"removed":0,"pinned":0,"remaining":0,"policy":"remove_unpinned_older_than_days","eligible_before":0,"older_than_days":30,"cutoff_at":"2026-08-26T10:00:00+00:00","candidate_digest":"bad"}`,
	} {
		s := &managedSession{protocol: ProtocolState{Available:true, Enabled:true}}
		s.applyProtocolLine([]byte(line))
		if s.protocol.HistoryCleanup != nil || s.protocol.Error == "" {
			t.Fatalf("invalid History cleanup accepted: %s state=%#v", line, s.protocol)
		}
	}
}

func TestProtocolCommandHistoryCleanupUsesFinalActivePromptGateAndKeepsPrompt(t *testing.T) {
	readFD, writeFD, err := os.Pipe()
	if err != nil { t.Fatal(err) }
	defer readFD.Close()
	defer writeFD.Close()
	manager := NewManager(1 << 20)
	prompt := []byte(`{"protocol":"taskdeck.patch","version":1,"type":"prompt","prompt_id":"active","prompt_kind":"history_action","actions":["cleanup"],"constraints":{"destructive_actions":["cleanup"],"cleanup":{"age_policy":"remove_unpinned_older_than_days","min_days":1,"max_days":3650}}}`)
	s := &managedSession{
		meta: Metadata{ID:"history-cleanup-test", Status:"running"},
		protocol: ProtocolState{Available:true, Enabled:true, CommandsEnabled:true, Prompt:append([]byte(nil), prompt...)},
		protocolCommand: writeFD,
	}
	manager.sessions["history-cleanup-test"] = s

	stale := []byte(`{"protocol":"taskdeck.patch","version":1,"type":"command","seq":1,"command":"history_cleanup","payload":{"prompt_id":"stale","cleanup_id":"c1","older_than_days":30,"confirmed":false}}`)
	if err := manager.ProtocolCommand("history-cleanup-test", stale); err == nil || !strings.Contains(err.Error(), "active prompt") {
		t.Fatalf("stale history_cleanup should fail closed, err=%v", err)
	}
	current := []byte(`{"protocol":"taskdeck.patch","version":1,"type":"command","seq":2,"command":"history_cleanup","payload":{"prompt_id":"active","cleanup_id":"c2","older_than_days":30,"confirmed":false}}`)
	if err := manager.ProtocolCommand("history-cleanup-test", current); err != nil { t.Fatal(err) }
	buf := make([]byte, 4096)
	n, err := readFD.Read(buf)
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(buf[:n]), `"command":"history_cleanup"`) {
		t.Fatalf("history_cleanup was not written to FD4: %q", buf[:n])
	}
	state, err := manager.ProtocolState("history-cleanup-test")
	if err != nil { t.Fatal(err) }
	if len(state.Prompt) == 0 {
		t.Fatal("history_cleanup must keep current prompt until Python refreshes it")
	}
}
