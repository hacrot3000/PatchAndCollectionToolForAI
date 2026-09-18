package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/session"
)

type legacyTitleSessionService struct {
	session.Service
	meta          session.Metadata
	setTitleCalls int
}

func (s *legacyTitleSessionService) List() []session.Metadata {
	return []session.Metadata{s.meta}
}

func (s *legacyTitleSessionService) Metadata(id string) (session.Metadata, bool) {
	if id != s.meta.ID {
		return session.Metadata{}, false
	}
	return s.meta, true
}

func (s *legacyTitleSessionService) SetTitle(string, string) (session.Metadata, error) {
	s.setTitleCalls++
	return session.Metadata{}, errors.New("legacy broker title endpoint must not be called")
}

func (s *legacyTitleSessionService) SupportsSessionTitle() bool { return false }

func TestLegacyBrokerRenameUsesDurableDaemonTitleState(t *testing.T) {
	workspace := t.TempDir()
	service := &legacyTitleSessionService{
		meta: session.Metadata{
			ID:     "legacy-session-1",
			TaskID: 42,
			Label:  "Original task",
			Status: "running",
		},
	}
	s := &Server{Workspace: workspace, Sessions: service}

	post := httptest.NewRequest(http.MethodPost, "/api/sessions/legacy-session-1/title", strings.NewReader(`{"title":"  Renamed   task  "}`))
	postRR := httptest.NewRecorder()
	s.sessionItem(postRR, post)
	if postRR.Code != http.StatusOK {
		t.Fatalf("rename status=%d body=%s", postRR.Code, postRR.Body.String())
	}
	if service.setTitleCalls != 0 {
		t.Fatalf("legacy broker SetTitle called %d times", service.setTitleCalls)
	}
	var renamed session.Metadata
	if err := json.Unmarshal(postRR.Body.Bytes(), &renamed); err != nil {
		t.Fatal(err)
	}
	if renamed.Title != "Renamed task" {
		t.Fatalf("renamed title=%q want %q", renamed.Title, "Renamed task")
	}

	stat, err := os.Stat(sessionTitlesPath(workspace))
	if err != nil {
		t.Fatal(err)
	}
	if stat.Mode().Perm() != 0o600 {
		t.Fatalf("session title state mode=%o want 600", stat.Mode().Perm())
	}

	// Simulate a replacement daemon reconnecting to the same legacy broker.
	replacement := &Server{Workspace: workspace, Sessions: service}
	get := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	getRR := httptest.NewRecorder()
	replacement.sessionsRoot(getRR, get)
	if getRR.Code != http.StatusOK {
		t.Fatalf("session list status=%d body=%s", getRR.Code, getRR.Body.String())
	}
	var payload struct {
		Sessions []session.Metadata `json:"sessions"`
	}
	if err := json.Unmarshal(getRR.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Sessions) != 1 || payload.Sessions[0].Title != "Renamed task" {
		t.Fatalf("replacement daemon lost durable title: %#v", payload.Sessions)
	}

	// An explicit reset must override even stale broker metadata.
	service.meta.Title = "stale-broker-title"
	reset := httptest.NewRequest(http.MethodPost, "/api/sessions/legacy-session-1/title", strings.NewReader(`{"title":""}`))
	resetRR := httptest.NewRecorder()
	replacement.sessionItem(resetRR, reset)
	if resetRR.Code != http.StatusOK {
		t.Fatalf("reset status=%d body=%s", resetRR.Code, resetRR.Body.String())
	}
	get = httptest.NewRequest(http.MethodGet, "/api/sessions/legacy-session-1", nil)
	getRR = httptest.NewRecorder()
	replacement.sessionItem(getRR, get)
	if getRR.Code != http.StatusOK {
		t.Fatalf("metadata status=%d body=%s", getRR.Code, getRR.Body.String())
	}
	var resetMeta session.Metadata
	if err := json.Unmarshal(getRR.Body.Bytes(), &resetMeta); err != nil {
		t.Fatal(err)
	}
	if resetMeta.Title != "" {
		t.Fatalf("durable empty title did not override stale broker title: %q", resetMeta.Title)
	}
}
