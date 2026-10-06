package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/identity"
)

func TestRequirePermissionDeniesMissingAndAllowsKnownGrant(t *testing.T) {
	called := 0
	handler := requirePermission(identity.PermissionFilesRead, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, test := range []struct {
		name       string
		principal  *identity.Principal
		wantStatus int
	}{
		{name: "unauthenticated", wantStatus: http.StatusUnauthorized},
		{name: "missing grant", principal: &identity.Principal{}, wantStatus: http.StatusForbidden},
		{name: "unknown grant cannot imply access", principal: &identity.Principal{Permissions: map[string]bool{"files.*": true}}, wantStatus: http.StatusForbidden},
		{name: "allowed", principal: &identity.Principal{Permissions: map[string]bool{identity.PermissionFilesRead: true}}, wantStatus: http.StatusNoContent},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/project/tree", nil)
			if test.principal != nil {
				req = req.WithContext(context.WithValue(req.Context(), sharedPrincipalContextKey{}, *test.principal))
			}
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			if rr.Code != test.wantStatus {
				t.Fatalf("status=%d want=%d", rr.Code, test.wantStatus)
			}
		})
	}
	if called != 1 {
		t.Fatalf("handler called %d times", called)
	}
}

func TestRequirePermissionRejectsUnregisteredPolicy(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("unknown permission did not fail at construction")
		}
	}()
	_ = requirePermission("invented.allow_all", http.NotFoundHandler())
}

func TestSharedRoutePermissionsSeparateReadWriteAndGitViews(t *testing.T) {
	tests := []struct {
		method string
		path   string
		want   []string
	}{
		{http.MethodGet, "/api/task-runs", []string{identity.PermissionTasksView}},
		{http.MethodPost, "/api/task-runs", []string{identity.PermissionTasksRun}},
		{http.MethodGet, "/api/task-runs/log?id=run-1", []string{identity.PermissionTasksView}},
		{http.MethodGet, "/api/project/file", []string{identity.PermissionFilesRead}},
		{http.MethodGet, "/api/project/health", []string{identity.PermissionFilesRead}},
		{http.MethodGet, "/api/project/file-history?path=sample.txt", []string{identity.PermissionFilesRead}},
		{http.MethodGet, "/api/project/bytes", []string{identity.PermissionFilesRead}},
		{http.MethodGet, "/api/project/integrity?path=sample.bin", []string{identity.PermissionFilesRead}},
		{http.MethodPost, "/api/project/integrity", []string{identity.PermissionFilesRead}},
		{http.MethodGet, "/api/project/preview?path=sample.md", []string{identity.PermissionFilesRead}},
		{http.MethodGet, "/api/project/download?path=sample.bin", []string{identity.PermissionFilesDownload}},
		{http.MethodPost, "/api/project/open-host?path=sample.txt", nil},
		{http.MethodGet, "/api/project/archive/preview?path=sample.zip", []string{identity.PermissionFilesRead}},
		{http.MethodPost, "/api/project/archive/create", []string{identity.PermissionFilesWrite}},
		{http.MethodPost, "/api/project/archive/extract", []string{identity.PermissionFilesWrite}},
		{http.MethodPost, "/api/project/archive/download", []string{identity.PermissionFilesRead, identity.PermissionFilesDownload}},
		{http.MethodGet, "/api/project/symbols?q=main", []string{identity.PermissionFilesRead}},
		{http.MethodPut, "/api/project/file", []string{identity.PermissionFilesWrite}},
		{http.MethodPost, "/api/project/content/replace", []string{identity.PermissionFilesWrite}},
		{http.MethodGet, "/api/project/git-status", []string{identity.PermissionFilesRead, identity.PermissionGitStatus}},
		{http.MethodPost, "/api/files/upload", []string{identity.PermissionFilesUpload}},
		{http.MethodPost, "/api/files/upload?overwrite=1", []string{identity.PermissionFilesUpload, identity.PermissionFilesWrite}},
		{http.MethodGet, "/api/file-transfer/text", []string{identity.PermissionTransferRead, identity.PermissionFilesDownload}},
		{http.MethodPut, "/api/file-transfer/text", []string{identity.PermissionTransferUpload, identity.PermissionFilesUpload, identity.PermissionFilesWrite}},
		{http.MethodPost, "/api/file-transfer/archive-upload-extract", []string{identity.PermissionTransferRead, identity.PermissionTransferUpload, identity.PermissionFilesRead, identity.PermissionFilesUpload, identity.PermissionFilesWrite}},
		{http.MethodGet, "/api/file-transfer/profiles", []string{identity.PermissionTransferRead}},
		{http.MethodPut, "/api/file-transfer/profiles/example", []string{identity.PermissionTransferRead, identity.PermissionSettingsWrite}},
		{http.MethodPost, "/api/file-transfer/test", []string{identity.PermissionTransferRead}},
		{http.MethodGet, "/api/file-transfer/list", []string{identity.PermissionTransferRead}},
		{http.MethodPost, "/api/file-transfer/upload", []string{identity.PermissionTransferRead, identity.PermissionTransferUpload}},
		{http.MethodPost, "/api/file-transfer/mutate", []string{identity.PermissionTransferRead}},
		{http.MethodGet, "/api/file-transfer/jobs", []string{identity.PermissionTransferRead}},
		{http.MethodPost, "/api/file-transfer/jobs/control", []string{identity.PermissionTransferRead}},
		{http.MethodGet, "/api/git/status?view=log", []string{identity.PermissionGitLog}},
		{http.MethodGet, "/api/git/status?view=diff", []string{identity.PermissionGitDiff}},
		{http.MethodGet, "/api/git/status?view=file-content", []string{identity.PermissionGitDiff}},
		{http.MethodGet, "/api/git/status?view=graph", []string{identity.PermissionGitLog}},
		{http.MethodGet, "/api/git/status?view=reflog", []string{identity.PermissionGitLog}},
		{http.MethodGet, "/api/git/status?view=worktrees", []string{identity.PermissionGitStatus}},
		{http.MethodGet, "/api/git/status?view=submodules", []string{identity.PermissionGitStatus}},
		{http.MethodGet, "/api/git/status?view=rebase-plan", []string{identity.PermissionGitLog}},
		{http.MethodGet, "/api/git/status?view=lost-commits", []string{identity.PermissionGitLog}},
		{http.MethodGet, "/api/git/status?view=commit-files", []string{identity.PermissionGitDiff}},
		{http.MethodGet, "/api/git/status?view=graph-compare-head", []string{identity.PermissionGitDiff}},
		{http.MethodGet, "/api/git/status?view=semantics-preview", []string{identity.PermissionGitDiff}},
		{http.MethodGet, "/api/git/conflict-file", []string{identity.PermissionGitDiff, identity.PermissionFilesRead}},
		{http.MethodPut, "/api/git/conflict-file", []string{identity.PermissionFilesWrite}},
		{http.MethodPost, "/api/git/status", []string{identity.PermissionGitWrite}},
		{http.MethodGet, "/api/git/jobs", []string{identity.PermissionGitStatus}},
		{http.MethodPost, "/api/git/jobs/control", []string{identity.PermissionGitWrite}},
		{http.MethodGet, "/api/workspace-snapshots", []string{identity.PermissionSettingsRead}},
		{http.MethodPost, "/api/workspace-snapshots", []string{identity.PermissionSettingsWrite}},
		{http.MethodPut, "/api/workspace-snapshots", []string{identity.PermissionSettingsWrite}},
		{http.MethodDelete, "/api/workspace-snapshots?id=snapshot", []string{identity.PermissionSettingsWrite}},
		{http.MethodGet, "/api/project-profiles", []string{identity.PermissionSettingsRead}},
		{http.MethodPost, "/api/project-profiles", []string{identity.PermissionSettingsWrite}},
		{http.MethodPut, "/api/project-profiles", []string{identity.PermissionSettingsWrite}},
		{http.MethodDelete, "/api/project-profiles?id=profile", []string{identity.PermissionSettingsWrite}},
		{http.MethodGet, "/api/config/page-title", []string{identity.PermissionSettingsRead}},
		{http.MethodPut, "/api/config/page-title", []string{identity.PermissionSettingsWrite}},
		{http.MethodGet, "/api/config/running-indicator", []string{identity.PermissionSettingsRead}},
		{http.MethodPut, "/api/config/running-indicator", []string{identity.PermissionSettingsWrite}},
		{http.MethodGet, "/api/config/self-update", []string{identity.PermissionSettingsRead}},
		{http.MethodPut, "/api/config/self-update", []string{identity.PermissionSettingsWrite}},
		{http.MethodGet, "/api/ssh/profiles", []string{identity.PermissionSSHUse}},
		{http.MethodPut, "/api/ssh/profiles/example", []string{identity.PermissionSSHUse, identity.PermissionSettingsWrite}},
		{http.MethodPost, "/api/ssh/test", []string{identity.PermissionSSHUse}},
		{http.MethodPost, "/api/ssh/host-key", []string{identity.PermissionSSHUse}},
		{http.MethodGet, "/api/db/adapters", []string{identity.PermissionDBRead}},
		{http.MethodGet, "/api/db/profiles", []string{identity.PermissionDBRead}},
		{http.MethodPut, "/api/db/profiles/example", []string{identity.PermissionDBRead, identity.PermissionSettingsWrite}},
		{http.MethodPost, "/api/db/test", []string{identity.PermissionDBRead}},
		{http.MethodGet, "/api/db/sessions", []string{identity.PermissionDBRead}},
		{http.MethodPost, "/api/db/sessions", []string{identity.PermissionDBRead}},
		{http.MethodPost, "/api/db/sessions/session/request", []string{identity.PermissionDBRead}},
		{http.MethodGet, "/api/config/auth-mode", []string{identity.PermissionSettingsRead}},
		{http.MethodPost, "/api/config/auth-mode", []string{identity.PermissionProjectAdmin}},
		{http.MethodGet, "/api/sessions", nil},
		{http.MethodGet, "/api/terminal-history?session_id=terminal-1", nil},
		{http.MethodPut, "/api/terminal-history?session_id=terminal-1", nil},
		{http.MethodGet, "/api/admin/users", []string{identity.PermissionUsersView}},
		{http.MethodPost, "/api/admin/users", []string{identity.PermissionUsersManage}},
		{http.MethodPatch, "/api/admin/users/access", []string{identity.PermissionUsersManage}},
		{http.MethodPut, "/api/admin/users/permission", []string{identity.PermissionUsersManage}},
		{http.MethodDelete, "/api/admin/users/permission", []string{identity.PermissionUsersManage}},
		{http.MethodGet, "/api/admin/roles", []string{identity.PermissionRolesView}},
		{http.MethodGet, "/api/admin/permissions", []string{identity.PermissionRolesView}},
		{http.MethodPost, "/api/admin/permissions", []string{identity.PermissionRolesManage}},
		{http.MethodGet, "/api/admin/sessions", []string{identity.PermissionSessionsManage}},
		{http.MethodDelete, "/api/admin/sessions", []string{identity.PermissionSessionsManage}},
		{http.MethodGet, "/api/admin/audit", []string{identity.PermissionAuditView}},
		{http.MethodGet, "/api/broadcast", nil},
		{http.MethodPost, "/api/broadcast", nil},
		{http.MethodGet, "/api/unknown", nil},
	}
	for _, test := range tests {
		req := httptest.NewRequest(test.method, test.path, nil)
		got := sharedRoutePermissions(req)
		if len(got) != len(test.want) {
			t.Fatalf("%s %s permissions=%v want=%v", test.method, test.path, got, test.want)
		}
		for i := range got {
			if got[i] != test.want[i] {
				t.Fatalf("%s %s permissions=%v want=%v", test.method, test.path, got, test.want)
			}
		}
	}
}

func TestSharedAuthorizationChecksBeforeHandlerAndRequiresAll(t *testing.T) {
	called := 0
	s := &Server{}
	handler := s.sharedAuthorize(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		w.WriteHeader(http.StatusNoContent)
	}))
	principal := identity.Principal{Permissions: map[string]bool{identity.PermissionFilesUpload: true}}
	for _, test := range []struct {
		path string
		want int
	}{
		{"/api/files/upload", http.StatusNoContent},
		{"/api/files/upload?overwrite=1", http.StatusForbidden},
		{"/api/sessions", http.StatusNoContent},
		{"/api/terminal-history?session_id=example", http.StatusNoContent},
		{"/api/sessions/example/stop", http.StatusNoContent},
		{"/api/sessions/force-kill", http.StatusNoContent},
	} {
		req := httptest.NewRequest(http.MethodPost, test.path, nil)
		req = req.WithContext(context.WithValue(req.Context(), sharedPrincipalContextKey{}, principal))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != test.want {
			t.Fatalf("%s status=%d want=%d", test.path, rr.Code, test.want)
		}
	}
	if called != 5 {
		t.Fatalf("handler called %d times", called)
	}
}

func TestSharedAuthorizationAuditsRoutePermissionDenial(t *testing.T) {
	s := sharedLoginTestServer(t)
	ctx := context.Background()
	alice, err := s.Identity.UserByUsername(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	project, err := s.Identity.ProjectByKey(ctx, s.Config.SharedProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Identity.SetMemberPermission(ctx, identity.MemberPermission{
		ProjectID: project.ID,
		UserID: alice.ID,
		PermissionID: identity.PermissionSettingsWrite,
		Effect: identity.PermissionDeny,
	}); err != nil {
		t.Fatal(err)
	}
	cookie := sharedAPILogin(t, s, "alice")
	request := httptest.NewRequest(http.MethodPut, "https://taskdeck.test/api/config/page-title", strings.NewReader(`{"title":"blocked"}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	s.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("denied route status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	events, err := s.Identity.ListAudit(ctx, identity.AuditQuery{
		ProjectID: project.ID,
		UserID: alice.ID,
		Action: "authorization.denied",
		Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].ResourceType != "http_route" || events[0].ResourceID != "/api/config/page-title" || events[0].Result != "denied" {
		t.Fatalf("unexpected authorization audit: %+v", events)
	}
	if !strings.Contains(events[0].Details, identity.PermissionSettingsWrite) || strings.Contains(events[0].Details, "blocked") {
		t.Fatalf("unexpected authorization audit details: %s", events[0].Details)
	}
}
