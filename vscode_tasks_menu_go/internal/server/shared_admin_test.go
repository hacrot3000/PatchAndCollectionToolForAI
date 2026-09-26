package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/identity"
)

func TestSharedAdminReadAPIsNeverExposeCredentialHashes(t *testing.T) {
	s := sharedLoginTestServer(t)
	cookie := sharedAPILogin(t, s, "alice")
	for _, path := range []string{
		"/api/admin/users",
		"/api/admin/roles",
		"/api/admin/permissions",
		"/api/admin/sessions",
		"/api/admin/audit",
	} {
		response := sharedRequest(t, s, path, cookie)
		if response.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", path, response.Code, response.Body.String())
		}
		body := response.Body.String()
		if strings.Contains(body, "password_hash") || strings.Contains(body, "token_hash") || strings.Contains(body, cookie.Value) {
			t.Fatalf("%s exposed credential material: %s", path, body)
		}
	}
}

func TestSharedAdminAPIsStayUnavailableInLegacyMode(t *testing.T) {
	s := authTestServer()
	request := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	request.SetBasicAuth(s.Config.Username, s.Config.Password)
	recorder := httptest.NewRecorder()
	s.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("legacy admin status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestSharedAdminCreatesProjectUserAndMembership(t *testing.T) {
	s := sharedLoginTestServer(t)
	cookie := sharedAPILogin(t, s, "alice")
	request := httptest.NewRequest(http.MethodPost, "https://taskdeck.test/api/admin/users", strings.NewReader(`{
		"username":"bob",
		"display_name":"Bob Developer",
		"password":"bob-private-password",
		"role_id":"system:viewer"
	}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	s.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create user status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "bob-private-password") || strings.Contains(recorder.Body.String(), "password_hash") || strings.Contains(recorder.Body.String(), "token_hash") {
		t.Fatalf("create response exposed credential material: %s", recorder.Body.String())
	}
	var response struct {
		User sharedAdminUserView `json:"user"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.User.Username != "bob" || response.User.RoleID != "system:viewer" || !response.User.UserEnabled || !response.User.MemberEnabled {
		t.Fatalf("unexpected created user: %+v", response.User)
	}
	user, err := s.Identity.UserByUsername(context.Background(), "bob")
	if err != nil {
		t.Fatal(err)
	}
	verified, err := identity.VerifyPassword(context.Background(), "bob-private-password", user.PasswordHash)
	if err != nil || !verified {
		t.Fatalf("stored password verification=%v err=%v", verified, err)
	}
	project, err := s.Identity.ProjectByKey(context.Background(), s.Config.SharedProjectID)
	if err != nil {
		t.Fatal(err)
	}
	member, err := s.Identity.ProjectMember(context.Background(), project.ID, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if member.RoleID != "system:viewer" || !member.Enabled {
		t.Fatalf("unexpected membership: %+v", member)
	}
	events, err := s.Identity.ListAudit(context.Background(), identity.AuditQuery{ProjectID: project.ID, Action: "admin.user.create", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Result != "success" || events[0].ResourceID != string(user.ID) {
		t.Fatalf("missing create audit event: %+v", events)
	}

	duplicate := httptest.NewRequest(http.MethodPost, "https://taskdeck.test/api/admin/users", strings.NewReader(`{
		"username":"bob",
		"password":"another-private-password",
		"role_id":"system:viewer"
	}`))
	duplicate.Header.Set("Content-Type", "application/json")
	duplicate.AddCookie(cookie)
	duplicateRecorder := httptest.NewRecorder()
	s.Handler().ServeHTTP(duplicateRecorder, duplicate)
	if duplicateRecorder.Code != http.StatusConflict {
		t.Fatalf("duplicate status=%d body=%s", duplicateRecorder.Code, duplicateRecorder.Body.String())
	}
}
