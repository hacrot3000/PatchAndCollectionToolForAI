package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/identity"
	webassets "bletonfc/vscode_tasks_menu/web"
)

func requestProjectNotes(s *Server, method, target, body string, principal *identity.Principal) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if principal != nil {
		req = req.WithContext(context.WithValue(req.Context(), sharedPrincipalContextKey{}, *principal))
	}
	rec := httptest.NewRecorder()
	s.projectNotes(rec, req)
	return rec
}

func decodeNote(t *testing.T, rr *httptest.ResponseRecorder) projectNote {
	t.Helper()
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected HTTP %d: %s", rr.Code, rr.Body.String())
	}
	var note projectNote
	if err := json.Unmarshal(rr.Body.Bytes(), &note); err != nil {
		t.Fatal(err)
	}
	return note
}

func TestProjectNotesCRUDPreservesPromptAndMetadata(t *testing.T) {
	workspace := t.TempDir()
	s := &Server{Workspace: workspace}
	prompt := "Hãy kiểm tra:\n- BLE\n- Git\n\nKhông sửa trực tiếp."
	body, _ := json.Marshal(projectNoteSaveRequest{Category: "Prompt mẫu", Title: "Kiểm tra dự án", Content: prompt})
	note := decodeNote(t, requestProjectNotes(s, http.MethodPost, "/api/project-notes", string(body), nil))
	if note.ID == "" || note.CreatedAt == "" || note.UpdatedAt == "" || note.Content != prompt || note.Category != "Prompt mẫu" {
		t.Fatalf("created note corrupted: %+v", note)
	}
	list := requestProjectNotes(s, http.MethodGet, "/api/project-notes", "", nil)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), note.ID) || !strings.Contains(list.Body.String(), "Prompt mẫu") {
		t.Fatalf("GET notes failed: %d %s", list.Code, list.Body.String())
	}
	update, _ := json.Marshal(projectNoteSaveRequest{ID: note.ID, Category: "Lưu ý", Title: "New title", Content: "New content"})
	updated := decodeNote(t, requestProjectNotes(s, http.MethodPut, "/api/project-notes", string(update), nil))
	if updated.CreatedAt != note.CreatedAt || updated.Content != "New content" || updated.Category != "Lưu ý" {
		t.Fatalf("update changed creation metadata or failed: %+v", updated)
	}
	path, err := projectNotesPath(workspace)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("note file must be private: %v %v", info, err)
	}
	if !strings.Contains(path, ".vscode") {
		t.Fatalf("notes must be stored in project .vscode: %s", path)
	}
	deleteRR := requestProjectNotes(s, http.MethodDelete, "/api/project-notes?id="+note.ID, "", nil)
	if deleteRR.Code != http.StatusOK {
		t.Fatalf("DELETE failed: %d %s", deleteRR.Code, deleteRR.Body.String())
	}
	after := requestProjectNotes(s, http.MethodGet, "/api/project-notes", "", nil)
	if strings.Contains(after.Body.String(), note.ID) {
		t.Fatalf("deleted note is still listed: %s", after.Body.String())
	}
}

func TestProjectNotesIsolatedByWorkspaceAndSharedProject(t *testing.T) {
	work1, work2 := t.TempDir(), t.TempDir()
	one, two := &Server{Workspace: work1}, &Server{Workspace: work2}
	payload := `{"title":"Only one workspace","content":"secret"}`
	if rr := requestProjectNotes(one, http.MethodPost, "/api/project-notes", payload, nil); rr.Code != http.StatusOK {
		t.Fatal(rr.Body.String())
	}
	list := requestProjectNotes(two, http.MethodGet, "/api/project-notes", "", nil)
	if strings.Contains(list.Body.String(), "Only one workspace") {
		t.Fatal("notes leaked across workspaces")
	}
	one.Config.SharedServerEnabled = true
	alice := identity.Principal{UserID: "alice", ProjectID: "p1"}
	bob := identity.Principal{UserID: "bob", ProjectID: "p1"}
	other := identity.Principal{UserID: "carol", ProjectID: "p2"}
	own := decodeNote(t, requestProjectNotes(one, http.MethodPost, "/api/project-notes",
		`{"title":"Team note","category":"Prompt","content":"team"}`, &alice))
	if rr := requestProjectNotes(one, http.MethodGet, "/api/project-notes", "", &bob); !strings.Contains(rr.Body.String(), own.ID) {
		t.Fatal("members of same project cannot read shared note")
	}
	if rr := requestProjectNotes(one, http.MethodGet, "/api/project-notes", "", &other); strings.Contains(rr.Body.String(), own.ID) {
		t.Fatal("notes leaked across shared project identities")
	}
	if rr := requestProjectNotes(one, http.MethodDelete, "/api/project-notes?id="+own.ID, "", &other); rr.Code != http.StatusNotFound {
		t.Fatalf("cross-project delete returned %d", rr.Code)
	}
	if rr := requestProjectNotes(one, http.MethodPut, "/api/project-notes",
		`{"id":"`+own.ID+`","title":"Steal","content":"bad"}`, &other); rr.Code != http.StatusNotFound {
		t.Fatalf("cross-project update returned %d", rr.Code)
	}
	if rr := requestProjectNotes(one, http.MethodGet, "/api/project-notes", "", nil); rr.Code != http.StatusUnauthorized {
		t.Fatalf("shared mode should reject missing principal: %d", rr.Code)
	}
}

func TestProjectNotesRejectsInvalidInputAndUnknownIDs(t *testing.T) {
	s := &Server{Workspace: t.TempDir()}
	bad := []string{
		`{"title":"","content":"x"}`,
		`{"title":"No content","content":"  "}`,
		`{"title":"a\nb","content":"x"}`,
		`{"title":"x","content":"ok","unknown":1}`,
		`{"title":"x","content":"ok"}{"title":"y","content":"z"}`,
		`{"id":"custom","title":"x","content":"ok"}`,
		`{"title":"x","category":"bad\ncategory","content":"ok"}`,
	}
	for _, body := range bad {
		if rr := requestProjectNotes(s, http.MethodPost, "/api/project-notes", body, nil); rr.Code != http.StatusBadRequest {
			t.Fatalf("POST accepted invalid note %q: %d %s", body, rr.Code, rr.Body.String())
		}
	}
	tooLong, _ := json.Marshal(projectNoteSaveRequest{Title: "Huge", Content: strings.Repeat("x", projectNotesMaxContent+1)})
	if rr := requestProjectNotes(s, http.MethodPost, "/api/project-notes", string(tooLong), nil); rr.Code != http.StatusBadRequest {
		t.Fatalf("accepted oversized content: %d", rr.Code)
	}
	for _, tc := range []struct{method, url, body string; status int}{
		{http.MethodPut, "/api/project-notes", `{"title":"x","content":"x"}`, http.StatusBadRequest},
		{http.MethodPut, "/api/project-notes", `{"id":"missing","title":"x","content":"x"}`, http.StatusNotFound},
		{http.MethodDelete, "/api/project-notes?id=missing", "", http.StatusNotFound},
		{http.MethodDelete, "/api/project-notes", "", http.StatusBadRequest},
		{http.MethodPatch, "/api/project-notes", "", http.StatusMethodNotAllowed},
	} {
		if rr := requestProjectNotes(s, tc.method, tc.url, tc.body, nil); rr.Code != tc.status {
			t.Fatalf("%s %s expected %d got %d: %s", tc.method, tc.url, tc.status, rr.Code, rr.Body.String())
		}
	}
}

func TestProjectNotesPermissionAndFrontendContracts(t *testing.T) {
	for _, tc := range []struct{method, permission string}{
		{http.MethodGet, identity.PermissionSettingsRead},
		{http.MethodPost, identity.PermissionSettingsWrite},
		{http.MethodPut, identity.PermissionSettingsWrite},
		{http.MethodDelete, identity.PermissionSettingsWrite},
	} {
		req := httptest.NewRequest(tc.method, "/api/project-notes", nil)
		if got := sharedRoutePermissions(req); len(got) != 1 || got[0] != tc.permission {
			t.Fatalf("notes permission %s: %v", tc.method, got)
		}
	}
	for _, tc := range []struct{file string; fragments []string}{
		{"featuremods/next.js", []string{"featuremods/projectnotes.js", "featuremods/workspaceutilities.js"}},
		{"featuremods/workspaceutilities.js", []string{"name:'Project notes'", "TaskMenuProjectNotes?.open?.()"}},
		{"featuremods/mobile.js", []string{"notes:()=>globalThis.TaskMenuProjectNotes?.open?.()", "['notes'"}},
		{"featuremods/projectnotes.js", []string{
			"/api/project-notes", "globalThis.TaskMenuProjectNotes={open,close,refresh",
			"if(navigator.clipboard?.writeText)", "navigator.clipboard.writeText(content)", "document.execCommand('copy')", "makeButton('Edit'", "makeButton('Copy'", "makeButton('Delete'",
			"method:id?'PUT':'POST'", "method:'DELETE'", "textContent",
		}},
	} {
		content, err := webassets.Files.ReadFile(tc.file)
		if err != nil {
			t.Fatal(err)
		}
		for _, fragment := range tc.fragments {
			if !strings.Contains(string(content), fragment) {
				t.Errorf("%s missing %q", tc.file, fragment)
			}
		}
	}
}
