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

func TestSharedAdminUpdatesProjectMembershipAccess(t *testing.T) {
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
	now := time.Now().UTC()
	bobID, err := identity.NewID()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Identity.CreateProjectUser(ctx,
		identity.User{ID: bobID, Username: "bob", PasswordHash: alice.PasswordHash, Enabled: true, CreatedAt: now, UpdatedAt: now},
		identity.ProjectMember{ProjectID: project.ID, UserID: bobID, RoleID: "system:viewer", Enabled: true, CreatedAt: now, UpdatedAt: now},
	); err != nil {
		t.Fatal(err)
	}
	bobCookie := sharedAPILogin(t, s, "bob")
	adminCookie := sharedAPILogin(t, s, "alice")

	request := httptest.NewRequest(http.MethodPatch, "https://taskdeck.test/api/admin/users/access", strings.NewReader(`{
		"user_id":"`+string(bobID)+`",
		"role_id":"system:developer",
		"enabled":false
	}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(adminCookie)
	recorder := httptest.NewRecorder()
	s.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("update access status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	member, err := s.Identity.ProjectMember(ctx, project.ID, bobID)
	if err != nil {
		t.Fatal(err)
	}
	if member.RoleID != "system:developer" || member.Enabled {
		t.Fatalf("unexpected updated membership: %+v", member)
	}
	bob, err := s.Identity.UserByID(ctx, bobID)
	if err != nil {
		t.Fatal(err)
	}
	if !bob.Enabled {
		t.Fatal("project membership update disabled the global user")
	}

	me := httptest.NewRequest(http.MethodGet, "https://taskdeck.test/api/auth/me", nil)
	me.AddCookie(bobCookie)
	meRecorder := httptest.NewRecorder()
	s.Handler().ServeHTTP(meRecorder, me)
	if meRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("disabled member still authenticated: %d %s", meRecorder.Code, meRecorder.Body.String())
	}
	events, err := s.Identity.ListAudit(ctx, identity.AuditQuery{ProjectID: project.ID, UserID: alice.ID, Action: "admin.member.update", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Result != "success" || events[0].ResourceID != string(bobID) {
		t.Fatalf("missing membership audit event: %+v", events)
	}
}

func TestSharedAdminMemberPermissionOverridesTakeEffect(t *testing.T) {
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
	now := time.Now().UTC()
	bobID, err := identity.NewID()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Identity.CreateProjectUser(ctx,
		identity.User{ID: bobID, Username: "bob", PasswordHash: alice.PasswordHash, Enabled: true, CreatedAt: now, UpdatedAt: now},
		identity.ProjectMember{ProjectID: project.ID, UserID: bobID, RoleID: "system:viewer", Enabled: true, CreatedAt: now, UpdatedAt: now},
	); err != nil {
		t.Fatal(err)
	}
	adminCookie := sharedAPILogin(t, s, "alice")
	bobCookie := sharedAPILogin(t, s, "bob")

	setOverride := func(method, permission, effect string, want int) {
		t.Helper()
		body := `{"user_id":"`+string(bobID)+`","permission_key":"`+permission+`"`
		if method == http.MethodPut {
			body += `,"effect":"`+effect+`"`
		}
		body += `}`
		request := httptest.NewRequest(method, "https://taskdeck.test/api/admin/users/permission", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.AddCookie(adminCookie)
		recorder := httptest.NewRecorder()
		s.Handler().ServeHTTP(recorder, request)
		if recorder.Code != want {
			t.Fatalf("%s %s status=%d body=%s", method, permission, recorder.Code, recorder.Body.String())
		}
	}

	setOverride(http.MethodPut, identity.PermissionFilesRead, string(identity.PermissionDeny), http.StatusNoContent)
	setOverride(http.MethodPut, identity.PermissionSettingsWrite, string(identity.PermissionAllow), http.StatusNoContent)

	me := httptest.NewRequest(http.MethodGet, "https://taskdeck.test/api/auth/me", nil)
	me.AddCookie(bobCookie)
	meRecorder := httptest.NewRecorder()
	s.Handler().ServeHTTP(meRecorder, me)
	if meRecorder.Code != http.StatusOK {
		t.Fatalf("current user status=%d body=%s", meRecorder.Code, meRecorder.Body.String())
	}
	var current sharedPrincipalResponse
	if err := json.Unmarshal(meRecorder.Body.Bytes(), &current); err != nil {
		t.Fatal(err)
	}
	permissions := map[string]bool{}
	for _, key := range current.Permissions {
		permissions[key] = true
	}
	if permissions[identity.PermissionFilesRead] || !permissions[identity.PermissionSettingsWrite] {
		t.Fatalf("override permissions not applied: %v", current.Permissions)
	}

	setOverride(http.MethodDelete, identity.PermissionFilesRead, "", http.StatusNoContent)
	effective, err := s.Identity.EffectivePermissions(ctx, project.ID, bobID)
	if err != nil {
		t.Fatal(err)
	}
	if !effective[identity.PermissionFilesRead] {
		t.Fatalf("deleting DENY did not restore role permission: %v", effective)
	}
	events, err := s.Identity.ListAudit(ctx, identity.AuditQuery{ProjectID: project.ID, UserID: alice.ID, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	var setCount, deleteCount int
	for _, event := range events {
		switch event.Action {
		case "admin.member_permission.set":
			setCount++
		case "admin.member_permission.delete":
			deleteCount++
		}
	}
	if setCount != 2 || deleteCount != 1 {
		t.Fatalf("unexpected permission audit counts set=%d delete=%d", setCount, deleteCount)
	}
}

func TestSharedAdminRevokesProjectMemberSession(t *testing.T) {
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
	now := time.Now().UTC()
	bobID, err := identity.NewID()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Identity.CreateProjectUser(ctx,
		identity.User{ID: bobID, Username: "bob", PasswordHash: alice.PasswordHash, Enabled: true, CreatedAt: now, UpdatedAt: now},
		identity.ProjectMember{ProjectID: project.ID, UserID: bobID, RoleID: "system:viewer", Enabled: true, CreatedAt: now, UpdatedAt: now},
	); err != nil {
		t.Fatal(err)
	}
	bobCookie := sharedAPILogin(t, s, "bob")
	adminCookie := sharedAPILogin(t, s, "alice")
	sessions, err := s.Identity.ListAuthSessions(ctx, identity.AuthSessionQuery{ProjectID: project.ID, UserID: bobID, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("bob session count=%d", len(sessions))
	}
	request := httptest.NewRequest(http.MethodDelete, "https://taskdeck.test/api/admin/sessions", strings.NewReader(`{"session_id":"`+string(sessions[0].ID)+`"}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(adminCookie)
	recorder := httptest.NewRecorder()
	s.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("revoke session status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	me := httptest.NewRequest(http.MethodGet, "https://taskdeck.test/api/auth/me", nil)
	me.AddCookie(bobCookie)
	meRecorder := httptest.NewRecorder()
	s.Handler().ServeHTTP(meRecorder, me)
	if meRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("revoked session still authenticated: %d %s", meRecorder.Code, meRecorder.Body.String())
	}
	events, err := s.Identity.ListAudit(ctx, identity.AuditQuery{ProjectID: project.ID, UserID: alice.ID, Action: "admin.session.revoke", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Result != "success" || events[0].ResourceID != string(sessions[0].ID) {
		t.Fatalf("missing revoke audit event: %+v", events)
	}
}

func TestSharedAdminRejectsSelfAccessAndPermissionModification(t *testing.T) {
	s := sharedLoginTestServer(t)
	ctx := context.Background()
	alice, err := s.Identity.UserByUsername(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	cookie := sharedAPILogin(t, s, "alice")

	access := httptest.NewRequest(http.MethodPatch, "https://taskdeck.test/api/admin/users/access", strings.NewReader(`{
		"user_id":"`+string(alice.ID)+`",
		"role_id":"system:viewer",
		"enabled":false
	}`))
	access.Header.Set("Content-Type", "application/json")
	access.AddCookie(cookie)
	accessRecorder := httptest.NewRecorder()
	s.Handler().ServeHTTP(accessRecorder, access)
	if accessRecorder.Code != http.StatusConflict {
		t.Fatalf("self access status=%d body=%s", accessRecorder.Code, accessRecorder.Body.String())
	}

	override := httptest.NewRequest(http.MethodPut, "https://taskdeck.test/api/admin/users/permission", strings.NewReader(`{
		"user_id":"`+string(alice.ID)+`",
		"permission_key":"users.manage",
		"effect":"DENY"
	}`))
	override.Header.Set("Content-Type", "application/json")
	override.AddCookie(cookie)
	overrideRecorder := httptest.NewRecorder()
	s.Handler().ServeHTTP(overrideRecorder, override)
	if overrideRecorder.Code != http.StatusConflict {
		t.Fatalf("self override status=%d body=%s", overrideRecorder.Code, overrideRecorder.Body.String())
	}
	project, err := s.Identity.ProjectByKey(ctx, s.Config.SharedProjectID)
	if err != nil {
		t.Fatal(err)
	}
	member, err := s.Identity.ProjectMember(ctx, project.ID, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !member.Enabled || member.RoleID != "system:admin" {
		t.Fatalf("self modification changed membership: %+v", member)
	}
	effective, err := s.Identity.EffectivePermissions(ctx, project.ID, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !effective[identity.PermissionUsersManage] {
		t.Fatalf("self modification removed users.manage: %v", effective)
	}
}
