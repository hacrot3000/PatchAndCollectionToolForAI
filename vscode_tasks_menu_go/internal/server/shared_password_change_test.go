package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/identity"
)

func passwordChangeRequest(cookie *http.Cookie, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "https://taskdeck.test/api/auth/password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	return req
}

func TestSharedPasswordChangeRevokesAllSessionsAndRequiresRelogin(t *testing.T) {
	s := sharedLoginTestServer(t)
	first := sharedAPILogin(t, s, "alice")
	second := sharedAPILogin(t, s, "alice")

	change := passwordChangeRequest(second, `{"current_password":"private-admin-password","new_password":"replacement-admin-password"}`)
	recorder := httptest.NewRecorder()
	s.Handler().ServeHTTP(recorder, change)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("change status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) == 0 || cookies[0].MaxAge != -1 {
		t.Fatalf("password change did not clear browser cookie: %+v", cookies)
	}
	for _, cookie := range []*http.Cookie{first, second} {
		if _, _, err := identity.AuthenticateBrowserSession(context.Background(), s.Identity, s.Config.SharedProjectID, cookie.Value, time.Now()); err == nil {
			t.Fatal("password change left an existing session valid")
		}
	}

	oldLogin := httptest.NewRecorder()
	s.Handler().ServeHTTP(oldLogin, loginRequest(`{"username":"alice","password":"private-admin-password"}`))
	if oldLogin.Code != http.StatusUnauthorized {
		t.Fatalf("old password login status=%d body=%s", oldLogin.Code, oldLogin.Body.String())
	}
	newLogin := httptest.NewRecorder()
	s.Handler().ServeHTTP(newLogin, loginRequest(`{"username":"alice","password":"replacement-admin-password"}`))
	if newLogin.Code != http.StatusOK {
		t.Fatalf("new password login status=%d body=%s", newLogin.Code, newLogin.Body.String())
	}
}

func TestSharedPasswordChangeRejectsWrongCurrentPasswordWithoutMutation(t *testing.T) {
	s := sharedLoginTestServer(t)
	cookie := sharedAPILogin(t, s, "alice")
	change := passwordChangeRequest(cookie, `{"current_password":"incorrect-current-password","new_password":"replacement-admin-password"}`)
	recorder := httptest.NewRecorder()
	s.Handler().ServeHTTP(recorder, change)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("wrong current status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if _, _, err := identity.AuthenticateBrowserSession(context.Background(), s.Identity, s.Config.SharedProjectID, cookie.Value, time.Now()); err != nil {
		t.Fatalf("wrong current password revoked valid session: %v", err)
	}

	login := httptest.NewRecorder()
	s.Handler().ServeHTTP(login, loginRequest(`{"username":"alice","password":"private-admin-password"}`))
	if login.Code != http.StatusOK {
		t.Fatalf("original password stopped working: %d %s", login.Code, login.Body.String())
	}
}

func TestSharedPasswordChangeValidatesNewPasswordAndAuditsWithoutSecrets(t *testing.T) {
	s := sharedLoginTestServer(t)
	cookie := sharedAPILogin(t, s, "alice")

	invalid := passwordChangeRequest(cookie, `{"current_password":"private-admin-password","new_password":"short"}`)
	invalidRecorder := httptest.NewRecorder()
	s.Handler().ServeHTTP(invalidRecorder, invalid)
	if invalidRecorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid new password status=%d body=%s", invalidRecorder.Code, invalidRecorder.Body.String())
	}

	change := passwordChangeRequest(cookie, `{"current_password":"private-admin-password","new_password":"replacement-admin-password"}`)
	recorder := httptest.NewRecorder()
	s.Handler().ServeHTTP(recorder, change)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("change status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	project, err := s.Identity.ProjectByKey(context.Background(), s.Config.SharedProjectID)
	if err != nil {
		t.Fatal(err)
	}
	events, err := s.Identity.ListAudit(context.Background(), identity.AuditQuery{
		ProjectID: project.ID,
		Action: "auth.password_change",
		Limit: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range events {
		if event.Result == "success" {
			found = true
		}
		if strings.Contains(event.Details, "private-admin-password") || strings.Contains(event.Details, "replacement-admin-password") {
			t.Fatalf("password change audit leaked password material: %+v", event)
		}
	}
	if !found {
		t.Fatalf("missing successful password-change audit: %+v", events)
	}
}
