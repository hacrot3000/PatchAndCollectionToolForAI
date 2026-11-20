package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func terminalHistoryRequest(t *testing.T, srv *Server, method, sessionID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "/api/terminal-history?session_id="+sessionID, strings.NewReader(body))
	rr := httptest.NewRecorder()
	srv.terminalHistory(rr, req)
	return rr
}

func decodeTerminalHistorySession(t *testing.T, rr *httptest.ResponseRecorder) terminalHistorySession {
	t.Helper()
	var value terminalHistorySession
	if err := json.Unmarshal(rr.Body.Bytes(), &value); err != nil {
		t.Fatalf("decode terminal history: %v body=%s", err, rr.Body.String())
	}
	return value
}

func TestTerminalHistoryPersistsBoundsAndClears(t *testing.T) {
	workspace := t.TempDir()
	srv := &Server{Workspace: workspace, Sessions: newBroadcastTestService()}

	body := `{"note":"investigate startup","commands":[{"command":"echo ok","output":"ok","exit_code":0,"cwd":"/tmp","started_at":10,"finished_at":25,"bookmark_line":1,"bookmark_text":"ok"}]}`
	put := terminalHistoryRequest(t, srv, http.MethodPut, "1", body)
	if put.Code != http.StatusOK {
		t.Fatalf("put status=%d body=%s", put.Code, put.Body.String())
	}
	value := decodeTerminalHistorySession(t, put)
	if value.Note != "investigate startup" || len(value.Commands) != 1 {
		t.Fatalf("unexpected saved history: %#v", value)
	}
	if value.Commands[0].BookmarkLine != 1 || value.Commands[0].BookmarkText != "ok" {
		t.Fatalf("bookmark not persisted: %#v", value.Commands[0])
	}

	path, err := terminalHistoryPath(workspace)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("history mode=%o want 600", info.Mode().Perm())
	}

	replacement := &Server{Workspace: workspace, Sessions: newBroadcastTestService()}
	get := terminalHistoryRequest(t, replacement, http.MethodGet, "1", "")
	if get.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", get.Code, get.Body.String())
	}
	value = decodeTerminalHistorySession(t, get)
	if value.Note != "investigate startup" || len(value.Commands) != 1 || value.Commands[0].Command != "echo ok" {
		t.Fatalf("history did not survive server replacement: %#v", value)
	}

	clear := terminalHistoryRequest(t, replacement, http.MethodPut, "1", `{"commands":[],"note":""}`)
	if clear.Code != http.StatusOK {
		t.Fatalf("clear status=%d body=%s", clear.Code, clear.Body.String())
	}
	value = decodeTerminalHistorySession(t, clear)
	if len(value.Commands) != 0 || value.Note != "" {
		t.Fatalf("history not cleared: %#v", value)
	}
}

func TestTerminalHistoryRejectsUnknownSessionAndNormalizesBookmark(t *testing.T) {
	srv := &Server{Workspace: t.TempDir(), Sessions: newBroadcastTestService()}
	missing := terminalHistoryRequest(t, srv, http.MethodGet, "missing", "")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing session status=%d body=%s", missing.Code, missing.Body.String())
	}

	put := terminalHistoryRequest(t, srv, http.MethodPut, "1",
		`{"commands":[{"command":"printf x","output":"line one\nline two","bookmark_line":99,"bookmark_text":"stale"}]}`)
	if put.Code != http.StatusOK {
		t.Fatalf("put status=%d body=%s", put.Code, put.Body.String())
	}
	value := decodeTerminalHistorySession(t, put)
	if len(value.Commands) != 1 || value.Commands[0].BookmarkLine != 0 || value.Commands[0].BookmarkText != "" {
		t.Fatalf("invalid bookmark was not normalized: %#v", value.Commands)
	}
}

func TestTerminalHistoryRemapsRestoredSessionID(t *testing.T) {
	workspace := t.TempDir()
	_, err := mutateTerminalHistoryStore(workspace, func(store *terminalHistoryStore) error {
		store.Sessions["old-id"] = terminalHistorySession{
			Commands: []terminalHistoryCommand{{Command: "make test", Output: "PASS"}},
			Note: "before restart",
			UpdatedAt: 1,
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := remapTerminalHistorySessions(workspace, map[string]string{"old-id": "new-id"}); err != nil {
		t.Fatal(err)
	}
	store, err := readTerminalHistoryStore(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := store.Sessions["old-id"]; exists {
		t.Fatalf("old terminal history key still present: %#v", store.Sessions)
	}
	value, exists := store.Sessions["new-id"]
	if !exists || value.Note != "before restart" || len(value.Commands) != 1 || value.Commands[0].Command != "make test" {
		t.Fatalf("history remap failed: %#v", store.Sessions)
	}
}
