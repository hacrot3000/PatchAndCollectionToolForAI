package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/dbadapter"
	"bletonfc/vscode_tasks_menu/internal/dbsession"
	"bletonfc/vscode_tasks_menu/internal/identity"
)

func TestSharedActionPermissionGuardLocalAndShared(t *testing.T) {
	s := &Server{}
	localReq := httptest.NewRequest(http.MethodPost, "/action", nil)
	localRR := httptest.NewRecorder()
	if !s.requireSharedActionPermission(localRR, localReq, identity.PermissionGitPush, "push", "git") {
		t.Fatal("local single-user request should bypass shared permission guard")
	}

	deniedReq := httptest.NewRequest(http.MethodPost, "/action", nil)
	deniedReq = deniedReq.WithContext(context.WithValue(deniedReq.Context(), sharedPrincipalContextKey{}, identity.Principal{
		Permissions: map[string]bool{identity.PermissionGitWrite: true},
	}))
	deniedRR := httptest.NewRecorder()
	if s.requireSharedActionPermission(deniedRR, deniedReq, identity.PermissionGitPush, "push", "git") {
		t.Fatal("shared request without git.push was allowed")
	}
	if deniedRR.Code != http.StatusForbidden {
		t.Fatalf("denied status=%d want=%d", deniedRR.Code, http.StatusForbidden)
	}

	allowedReq := httptest.NewRequest(http.MethodPost, "/action", nil)
	allowedReq = allowedReq.WithContext(context.WithValue(allowedReq.Context(), sharedPrincipalContextKey{}, identity.Principal{
		Permissions: map[string]bool{identity.PermissionGitPush: true},
	}))
	allowedRR := httptest.NewRecorder()
	if !s.requireSharedActionPermission(allowedRR, allowedReq, identity.PermissionGitPush, "push", "git") {
		t.Fatal("shared request with git.push was denied")
	}
}

func TestDatabaseOperationPermissionClassification(t *testing.T) {
	mysql := dbsession.Metadata{AdapterKind: "mysql"}
	sqlite := dbsession.Metadata{AdapterKind: "sqlite"}
	mongo := dbsession.Metadata{AdapterKind: "mongo"}

	tests := []struct {
		name string
		meta dbsession.Metadata
		op dbadapter.Operation
		payload interface{}
		want string
	}{
		{"browse", mysql, dbadapter.OpBrowseRows, dbadapter.BrowseRowsPayload{}, identity.PermissionDBRead},
		{"mutate", mysql, dbadapter.OpMutateRows, dbadapter.MutateRowsPayload{}, identity.PermissionDBWrite},
		{"transaction", mysql, dbadapter.OpCommit, nil, identity.PermissionDBWrite},
		{"count", mysql, dbadapter.OpObjectAction, dbadapter.ObjectActionPayload{Action:"count_rows"}, identity.PermissionDBRead},
		{"truncate", mysql, dbadapter.OpObjectAction, dbadapter.ObjectActionPayload{Action:"truncate"}, identity.PermissionDBWrite},
		{"drop", mysql, dbadapter.OpObjectAction, dbadapter.ObjectActionPayload{Action:"drop"}, identity.PermissionDBSchema},
		{"select", mysql, dbadapter.OpExecute, dbadapter.ExecutePayload{Statement:"SELECT * FROM users"}, identity.PermissionDBRead},
		{"with", sqlite, dbadapter.OpExecute, dbadapter.ExecutePayload{Statement:"-- note\nWITH x AS (SELECT 1) SELECT * FROM x"}, identity.PermissionDBRead},
		{"ddl", mysql, dbadapter.OpExecute, dbadapter.ExecutePayload{Statement:"/* migration */ ALTER TABLE users ADD COLUMN active INT"}, identity.PermissionDBSchema},
		{"dml", mysql, dbadapter.OpExecute, dbadapter.ExecutePayload{Statement:"UPDATE users SET active=1"}, identity.PermissionDBWrite},
		{"mongo-safe-dsl", mongo, dbadapter.OpExecute, dbadapter.ExecutePayload{Statement:`{"op":"find"}`}, identity.PermissionDBRead},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := databaseOperationPermission(test.meta, test.op, test.payload); got != test.want {
				t.Fatalf("permission=%q want=%q", got, test.want)
			}
		})
	}
}

func TestGranularPermissionsAreRegistered(t *testing.T) {
	for _, key := range identity.PermissionUpgradeV2Keys() {
		if !identity.KnownPermission(key) {
			t.Fatalf("granular permission %q is not registered", key)
		}
	}
}
