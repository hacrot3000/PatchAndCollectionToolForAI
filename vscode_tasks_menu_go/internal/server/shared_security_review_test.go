package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSharedSecurityReviewPublicRouteExceptionsStayAuthenticated(t *testing.T) {
	s := sharedLoginTestServer(t)
	tests := []struct {
		method string
		path   string
		body   string
		want   int
	}{
		{http.MethodGet, "/api/browser/lease", "", http.StatusUnauthorized},
		{http.MethodGet, "/api/mutation-lock", "", http.StatusUnauthorized},
		{http.MethodPost, "/api/auth/password", `{"current_password":"x","new_password":"y"}`, http.StatusUnauthorized},
		{http.MethodGet, "/api/sessions", "", http.StatusUnauthorized},
		{http.MethodGet, "/admin/access", "", http.StatusUnauthorized},
		{http.MethodGet, "/api/broadcast", "", http.StatusUnauthorized},
	}
	for _, tc := range tests {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "https://taskdeck.test"+tc.path, strings.NewReader(tc.body))
			req.RemoteAddr = "192.0.2.50:41000"
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			recorder := httptest.NewRecorder()
			s.Handler().ServeHTTP(recorder, req)
			if recorder.Code != tc.want {
				t.Fatalf("status=%d want=%d body=%s", recorder.Code, tc.want, recorder.Body.String())
			}
		})
	}
}

func TestSharedSecurityReviewHealthAndLoginAreOnlyAnonymousExceptions(t *testing.T) {
	s := sharedLoginTestServer(t)

	login := httptest.NewRequest(http.MethodGet, "https://taskdeck.test/login", nil)
	login.RemoteAddr = "192.0.2.50:41000"
	loginRecorder := httptest.NewRecorder()
	s.Handler().ServeHTTP(loginRecorder, login)
	if loginRecorder.Code != http.StatusOK {
		t.Fatalf("login page status=%d body=%s", loginRecorder.Code, loginRecorder.Body.String())
	}

	loopbackHealth := httptest.NewRequest(http.MethodGet, "https://taskdeck.test/api/health", nil)
	loopbackHealth.RemoteAddr = "127.0.0.1:41000"
	loopbackRecorder := httptest.NewRecorder()
	s.Handler().ServeHTTP(loopbackRecorder, loopbackHealth)
	if loopbackRecorder.Code != http.StatusOK {
		t.Fatalf("loopback health status=%d body=%s", loopbackRecorder.Code, loopbackRecorder.Body.String())
	}

	remoteHealth := httptest.NewRequest(http.MethodGet, "https://taskdeck.test/api/health", nil)
	remoteHealth.RemoteAddr = "192.0.2.50:41000"
	remoteRecorder := httptest.NewRecorder()
	s.Handler().ServeHTTP(remoteRecorder, remoteHealth)
	if remoteRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("remote health status=%d body=%s", remoteRecorder.Code, remoteRecorder.Body.String())
	}
}

func TestSharedSecurityReviewInternalControlNeedsTokenEvenFromLoopback(t *testing.T) {
	s := &Server{InternalControlToken: "test-control-token"}
	req := httptest.NewRequest(http.MethodPost, "/api/state/tasks?scope=self-update&action=handoff", nil)
	req.RemoteAddr = "127.0.0.1:42000"
	if s.internalControlRequest(req) {
		t.Fatal("loopback request without control token was trusted")
	}
	req.Header.Set(InternalControlHeader, "wrong-token")
	if s.internalControlRequest(req) {
		t.Fatal("loopback request with wrong control token was trusted")
	}
	req.Header.Set(InternalControlHeader, s.InternalControlToken)
	if !s.internalControlRequest(req) {
		t.Fatal("loopback request with correct control token was rejected")
	}
	req.RemoteAddr = "192.0.2.10:42000"
	if s.internalControlRequest(req) {
		t.Fatal("remote request with control token was trusted")
	}
}
