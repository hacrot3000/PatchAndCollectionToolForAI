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
