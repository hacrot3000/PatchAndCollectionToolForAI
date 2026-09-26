package server

import (
	"context"
	"net/http"
	"net/http/httptest"
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
		{http.MethodGet, "/api/project/file", []string{identity.PermissionFilesRead}},
		{http.MethodPut, "/api/project/file", []string{identity.PermissionFilesWrite}},
		{http.MethodPost, "/api/files/upload", []string{identity.PermissionFilesUpload}},
		{http.MethodPost, "/api/files/upload?overwrite=1", []string{identity.PermissionFilesUpload, identity.PermissionFilesWrite}},
		{http.MethodGet, "/api/git/status?view=log", []string{identity.PermissionGitLog}},
		{http.MethodGet, "/api/git/status?view=diff", []string{identity.PermissionGitDiff}},
		{http.MethodPost, "/api/git/status", nil},
		{http.MethodGet, "/api/config/page-title", []string{identity.PermissionSettingsRead}},
		{http.MethodPut, "/api/config/page-title", []string{identity.PermissionSettingsWrite}},
		{http.MethodGet, "/api/sessions", nil},
		{http.MethodGet, "/api/admin/users", []string{identity.PermissionUsersView}},
		{http.MethodPost, "/api/admin/users", []string{identity.PermissionUsersManage}},
		{http.MethodPatch, "/api/admin/users/access", []string{identity.PermissionUsersManage}},
		{http.MethodGet, "/api/admin/roles", []string{identity.PermissionRolesView}},
		{http.MethodGet, "/api/admin/sessions", []string{identity.PermissionSessionsManage}},
		{http.MethodGet, "/api/admin/audit", []string{identity.PermissionAuditView}},
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
	if called != 4 {
		t.Fatalf("handler called %d times", called)
	}
}
