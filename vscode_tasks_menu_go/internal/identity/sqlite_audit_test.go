package identity

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSQLiteAuditStoreAppend(t *testing.T) {
	state := &fakeSQLiteState{currentVersion: int64(schemaVersion)}
	db := openFakeIdentitySessionStore(t, state)

	userID := ID("user-1")
	projectID := ID("project-1")
	event := AuditEvent{
		ID:           "audit-1",
		Timestamp:    time.Date(2026, 9, 25, 13, 0, 0, 0, time.UTC),
		UserID:       &userID,
		ProjectID:    &projectID,
		Action:       "terminal.create",
		ResourceType: "session",
		ResourceID:   "session-1",
		Result:       "ALLOW",
		ClientIP:     "127.0.0.1",
		Details:      "{}",
	}
	if err := db.AppendAudit(context.Background(), event); err != nil {
		t.Fatal(err)
	}

	_, execs, _ := state.snapshot()
	assertSQLLogContains(t, execs, "INSERT INTO audit_log")
}

func TestSQLiteAuditStoreListsNewestFilteredEvents(t *testing.T) {
	db := openRealSQLiteDatabase(t, filepath.Join(t.TempDir(), identityDBName))
	base := time.Date(2026, 9, 25, 13, 0, 0, 0, time.UTC)
	for i, action := range []string{"task.run", "terminal.create", "task.run"} {
		if err := db.AppendAudit(context.Background(), AuditEvent{
			ID: ID("audit-" + string(rune('1'+i))), Timestamp: base.Add(time.Duration(i) * time.Minute),
			Action: action, Result: "success", Details: "{}",
		}); err != nil {
			t.Fatal(err)
		}
	}
	events, err := db.ListAudit(context.Background(), AuditQuery{Action: "task.run", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].ID != "audit-3" || events[0].Action != "task.run" {
		t.Fatalf("unexpected audit query result: %#v", events)
	}
}

func TestSQLiteAuditStoreRejectsIncompleteEvent(t *testing.T) {
	state := &fakeSQLiteState{currentVersion: int64(schemaVersion)}
	db := openFakeIdentitySessionStore(t, state)

	err := db.AppendAudit(context.Background(), AuditEvent{
		ID:        "audit-1",
		Timestamp: time.Now().UTC(),
		Action:    "",
		Result:    "DENY",
	})
	if err == nil || !strings.Contains(err.Error(), "requires id, timestamp, action and result") {
		t.Fatalf("err=%v want audit validation", err)
	}

	empty := ID("")
	err = db.AppendAudit(context.Background(), AuditEvent{
		ID:        "audit-2",
		Timestamp: time.Now().UTC(),
		UserID:    &empty,
		Action:    "login",
		Result:    "DENY",
	})
	if err == nil || !strings.Contains(err.Error(), "user_id cannot be empty") {
		t.Fatalf("err=%v want empty user_id validation", err)
	}
}
