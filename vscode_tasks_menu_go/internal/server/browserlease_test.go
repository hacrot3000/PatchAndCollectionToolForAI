package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
	"time"
)

func TestBrowserLeaseLastAcquireRevokesPrevious(t *testing.T) {
	lease := newBrowserLease()
	first, err := lease.acquire()
	if err != nil {
		t.Fatal(err)
	}
	revoked, ok := lease.watch(first)
	if !ok || revoked == nil {
		t.Fatal("first lease must be watchable")
	}
	second, err := lease.acquire()
	if err != nil {
		t.Fatal(err)
	}
	if second == first || second == "" {
		t.Fatalf("second lease = %q, first = %q", second, first)
	}
	select {
	case <-revoked:
	case <-time.After(time.Second):
		t.Fatal("previous lease was not revoked")
	}
	if lease.valid(first) {
		t.Fatal("old lease unexpectedly remained valid")
	}
	if !lease.valid(second) {
		t.Fatal("latest lease must be valid")
	}
}

func TestBrowserLeaseAPILatestBrowserWins(t *testing.T) {
	s := &Server{}
	h := s.Handler()

	acquire := func() string {
		req := httptest.NewRequest(http.MethodPost, "/api/browser/lease", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("acquire status=%d body=%s", rr.Code, rr.Body.String())
		}
		body := rr.Body.String()
		const marker = `"lease":"`
		start := strings.Index(body, marker)
		if start < 0 {
			t.Fatalf("lease missing from %q", body)
		}
		start += len(marker)
		end := strings.Index(body[start:], `"`)
		if end < 0 {
			t.Fatalf("invalid lease response %q", body)
		}
		return body[start : start+end]
	}

	first := acquire()
	second := acquire()

	check := func(token string, want int) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/browser/lease", nil)
		req.Header.Set(browserLeaseHeader, token)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != want {
			t.Fatalf("lease check token=%q status=%d want=%d body=%s", token, rr.Code, want, rr.Body.String())
		}
		return rr
	}
	old := check(first, http.StatusConflict)
	if old.Header().Get("X-TaskMenu-Lease-Revoked") != "1" {
		t.Fatal("revoked lease response must advertise revocation")
	}
	check(second, http.StatusOK)
}

func TestSharedBrowserLeaseDoesNotRevokeConcurrentUsers(t *testing.T) {
	s := &Server{}
	s.Config.SharedServerEnabled = true
	for i := 0; i < 2; i++ {
		rr := httptest.NewRecorder()
		s.browserLeaseAPI(rr, httptest.NewRequest(http.MethodPost, "/api/browser/lease", nil))
		if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"lease":"shared"`) {
			t.Fatalf("shared lease acquire status=%d body=%s", rr.Code, rr.Body.String())
		}
	}
	if s.browserLease != nil {
		t.Fatal("shared lease endpoint created a single-browser lease")
	}
	rr := httptest.NewRecorder()
	s.browserLeaseAPI(rr, httptest.NewRequest(http.MethodGet, "/api/browser/lease", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("shared lease heartbeat status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestBrowserLeaseBlocksStaleMutationsButNotLegacyBeforeFirstAcquire(t *testing.T) {
	s := &Server{}
	called := 0
	h := s.requireBrowserLease(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		w.WriteHeader(http.StatusNoContent)
	}))

	legacy := httptest.NewRecorder()
	h.ServeHTTP(legacy, httptest.NewRequest(http.MethodPost, "/api/test", nil))
	if legacy.Code != http.StatusNoContent || called != 1 {
		t.Fatalf("pre-lease mutation status=%d called=%d", legacy.Code, called)
	}

	first, err := s.browserLeaseState().acquire()
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.browserLeaseState().acquire()
	if err != nil {
		t.Fatal(err)
	}

	staleReq := httptest.NewRequest(http.MethodPost, "/api/test", nil)
	staleReq.Header.Set(browserLeaseHeader, first)
	stale := httptest.NewRecorder()
	h.ServeHTTP(stale, staleReq)
	if stale.Code != http.StatusConflict || called != 1 {
		t.Fatalf("stale mutation status=%d called=%d", stale.Code, called)
	}

	currentReq := httptest.NewRequest(http.MethodPost, "/api/test", nil)
	currentReq.Header.Set(browserLeaseHeader, second)
	current := httptest.NewRecorder()
	h.ServeHTTP(current, currentReq)
	if current.Code != http.StatusNoContent || called != 2 {
		t.Fatalf("current mutation status=%d called=%d", current.Code, called)
	}
}

func TestBrowserLeaseUIClaimsControlAndStopsReconnectAfterRevocation(t *testing.T) {
	for _, want := range []string{
		"await acquireBrowserLease();await loadTasks();await syncSessions();",
		"X-TaskMenu-Lease",
		"showLeaseLost",
		"Reload and take control",
		"browserLeaseLost",
		"?lease=",
		"if(view.closed||browserLeaseLost)return",
		"browserLeaseHeartbeat=setInterval",
	} {
		if !strings.Contains(appJS, want) {
			t.Fatalf("app JS missing browser lease behavior %q", want)
		}
	}
	if !strings.Contains(appCSS, ".browser-lease-overlay") {
		t.Fatal("browser lease takeover UI overlay is missing")
	}
}

func TestBrowserLeaseAllowsOnlyLoopbackSelfUpdateTransportActions(t *testing.T) {
	s := &Server{}
	if _, err := s.browserLeaseState().acquire(); err != nil {
		t.Fatal(err)
	}
	called := 0
	h := s.requireBrowserLease(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		w.WriteHeader(http.StatusAccepted)
	}))

	tests := []struct {
		name       string
		target     string
		remoteAddr string
		want       int
	}{
		{"loopback handoff", "/api/state/tasks?scope=self-update&action=handoff", "127.0.0.1:43120", http.StatusAccepted},
		{"loopback detach", "/api/state/tasks?scope=self-update&action=detach", "[::1]:43120", http.StatusAccepted},
		{"remote handoff", "/api/state/tasks?scope=self-update&action=handoff", "192.168.1.20:43120", http.StatusConflict},
		{"browser confirm still protected", "/api/state/tasks?scope=self-update&action=confirm", "127.0.0.1:43120", http.StatusConflict},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tc.target, nil)
			req.RemoteAddr = tc.remoteAddr
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != tc.want {
				t.Fatalf("status=%d want=%d body=%s", rr.Code, tc.want, rr.Body.String())
			}
		})
	}
	if called != 2 {
		t.Fatalf("inner handler called=%d want 2", called)
	}
}

func TestBrowserLeaseHealthCheckRejectsTokenWhenDaemonHasNoActiveLease(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/browser/lease", nil)
	req.Header.Set(browserLeaseHeader, "token-from-previous-daemon")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusConflict, rr.Body.String())
	}
	if rr.Header().Get("X-TaskMenu-Lease-Revoked") != "1" {
		t.Fatal("stale token after daemon restart must be marked revoked")
	}
}

func TestDirectMutationFeaturesUseLeaseAwareFetch(t *testing.T) {
	for _, path := range []string{"featuremods/upload.js", "featuremods/editor.js", "featuremods/terminalrestore.js"} {
		data, err := webassets.Files.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "app.fetchWithLease(") {
			t.Fatalf("%s must use the lease-aware fetch helper for direct mutations", path)
		}
	}
	for _, want := range []string{
		"async function fetchWithLease",
		"X-TaskMenu-Lease-Revoked",
		"showLeaseLost();",
	} {
		if !strings.Contains(appJS, want) {
			t.Fatalf("app JS missing direct-fetch lease handling %q", want)
		}
	}
}
