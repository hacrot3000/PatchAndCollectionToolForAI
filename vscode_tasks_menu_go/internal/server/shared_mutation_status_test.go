package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/identity"
)

func TestSharedMutationStatusExposesHolderWithoutLeaseToken(t *testing.T) {
	s, principal := sharedFileAuditServer(t)
	lease, conflict, err := s.sharedMutation.acquire(principal, "patch.run", "patch-session", time.Now().UTC())
	if err != nil || conflict != nil || lease.token == "" {
		t.Fatalf("prepare lock lease=%+v conflict=%+v err=%v", lease, conflict, err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/mutation-lock", nil)
	request = sharedAuditRequest(request, principal)
	recorder := httptest.NewRecorder()
	s.sharedMutationStatus(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), lease.token) || strings.Contains(recorder.Body.String(), "token") {
		t.Fatalf("lock status exposed internal token: %s", recorder.Body.String())
	}
	var status sharedMutationStatusResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if !status.Locked || status.Holder == nil || status.Holder.Operation != "patch.run" || status.Holder.ResourceID != "patch-session" {
		t.Fatalf("unexpected lock status: %+v", status)
	}
}

func TestSharedMutationStatusRouteRequiresAuthenticationButNoModulePermission(t *testing.T) {
	s := &Server{}
	called := 0
	handler := s.sharedAuthorize(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/api/mutation-lock", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d", recorder.Code)
	}
	principal := identity.Principal{UserID: "alice", ProjectID: "project"}
	request = httptest.NewRequest(http.MethodGet, "/api/mutation-lock", nil)
	request = request.WithContext(context.WithValue(request.Context(), sharedPrincipalContextKey{}, principal))
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent || called != 1 {
		t.Fatalf("authenticated status=%d called=%d", recorder.Code, called)
	}
}

func TestMainUIShowsSharedMutationLockStatus(t *testing.T) {
	for _, want := range []string{
		`id="mutation-lock"`,
		"/api/mutation-lock",
		"Workspace locked · ",
		"mutationLocked=true",
	} {
		if !strings.Contains(indexHTML+appJS, want) {
			t.Fatalf("shared mutation lock UI missing %q", want)
		}
	}
}
