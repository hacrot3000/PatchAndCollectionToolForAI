package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/patchtool"
)

func TestAgentChatProjectIsolation(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	a := &Server{Workspace: filepath.Join(root, "ABC")}
	b := &Server{Workspace: filepath.Join(root, "DEF")}
	for _, s := range []*Server{a, b} {
		if err := os.MkdirAll(s.Workspace, 0700); err != nil {
			t.Fatal(err)
		}
	}
	identityA, err := patchtool.EnsureProjectIdentity(a.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	identityB, err := patchtool.EnsureProjectIdentity(b.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	sessionA, err := agentchat.NewSession("local", a.Workspace, identityA.ID, "p", "m", agentchat.Stateless)
	if err != nil {
		t.Fatal(err)
	}
	sessionB, err := agentchat.NewSession("local", b.Workspace, identityB.ID, "p", "m", agentchat.Stateless)
	if err != nil {
		t.Fatal(err)
	}
	storeA, err := a.agentChatStore()
	if err != nil {
		t.Fatal(err)
	}
	storeB, err := b.agentChatStore()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := storeA.Save(sessionA, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := storeB.Save(sessionB, 0); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		server     *Server
		own, other string
	}{{a, sessionA.ID, sessionB.ID}, {b, sessionB.ID, sessionA.ID}} {
		list := invokeAI(t, tc.server.agentChatSessionsAPI, http.MethodGet, "/api/agent/sessions", nil)
		if list.Code != 200 || !strings.Contains(list.Body.String(), tc.own) || strings.Contains(list.Body.String(), tc.other) {
			t.Fatalf("cross-project list: %d %s", list.Code, list.Body)
		}
		get := invokeAI(t, tc.server.agentChatSessionItemAPI, http.MethodGet, "/api/agent/sessions/"+tc.other, nil)
		if get.Code != http.StatusNotFound {
			t.Fatalf("cross-project direct access: %d %s", get.Code, get.Body)
		}
	}
	// Even a misplaced journal in the correct workspace directory must not be listed or opened.
	privateFile, err := aiPrivatePath(a.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	unscoped, err := agentchat.NewStore(filepath.Join(filepath.Dir(privateFile), "agent-sessions-"+aiWorkspaceHash(a.Workspace)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = unscoped.Save(sessionB, 0); err != nil {
		t.Fatal(err)
	}
	wrongIdentity, err := agentchat.NewSession("local", a.Workspace, identityB.ID, "p", "m", agentchat.Stateless)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = unscoped.Save(wrongIdentity, 0); err != nil {
		t.Fatal(err)
	}
	list := invokeAI(t, a.agentChatSessionsAPI, http.MethodGet, "/api/agent/sessions", nil)
	var result struct {
		Sessions []struct {
			ID string `json:"id"`
		} `json:"sessions"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if list.Code != 200 || len(result.Sessions) != 1 || result.Sessions[0].ID != sessionA.ID {
		t.Fatalf("misplaced session visible: %s", list.Body)
	}
	get := invokeAI(t, a.agentChatSessionItemAPI, http.MethodGet, "/api/agent/sessions/"+sessionB.ID, nil)
	if get.Code != http.StatusNotFound {
		t.Fatalf("misplaced session accessible: %d", get.Code)
	}
	get = invokeAI(t, a.agentChatSessionItemAPI, http.MethodGet, "/api/agent/sessions/"+wrongIdentity.ID, nil)
	if get.Code != http.StatusNotFound {
		t.Fatalf("wrong-project identity accessible: %d", get.Code)
	}
}
