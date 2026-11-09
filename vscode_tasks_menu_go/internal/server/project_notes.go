package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"bletonfc/vscode_tasks_menu/internal/projectfiles"
)

const (
	projectNotesFile        = "vscode_tasks_menu.notes.json"
	projectNotesMaxFile     = 8 << 20
	projectNotesMaxBody     = 256 << 10
	projectNotesMaxContent  = 128 << 10
	projectNotesMaxCount    = 250
	projectNotesMaxTitle    = 180
	projectNotesMaxCategory = 80
)

type projectNote struct {
	ID        string `json:"id"`
	Category  string `json:"category,omitempty"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type projectNoteRecord struct {
	ProjectKey string      `json:"project_key"`
	Note       projectNote `json:"note"`
}

type projectNoteStore struct {
	Version int                 `json:"version"`
	Notes   []projectNoteRecord `json:"notes"`
}

type projectNoteSaveRequest struct {
	ID       string `json:"id,omitempty"`
	Category string `json:"category"`
	Title    string `json:"title"`
	Content  string `json:"content"`
}

// Notes are shared inside one TaskDeck project, rather than globally or per browser.
// Include the authenticated project ID in shared mode: a shared deployment may
// map several project identities to the same physical workspace.
func projectNoteScope(s *Server, r *http.Request) (string, error) {
	if !s.Config.SharedServerEnabled {
		return "local", nil
	}
	principal, ok := PrincipalFromContext(r.Context())
	if !ok || principal.ProjectID == "" || principal.UserID == "" {
		return "", errors.New("project identity is unavailable")
	}
	return "project:" + string(principal.ProjectID), nil
}

func projectNotesPath(workspace string) (string, error) {
	return projectfiles.Resolve(workspace, projectNotesFile)
}

func readProjectNotes(workspace string) (projectNoteStore, error) {
	empty := projectNoteStore{Version: 1, Notes: []projectNoteRecord{}}
	path, err := projectNotesPath(workspace)
	if err != nil {
		return empty, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return empty, nil
	}
	if err != nil {
		return empty, fmt.Errorf("read project notes: %w", err)
	}
	if len(data) > projectNotesMaxFile {
		return empty, errors.New("project notes file exceeds size limit")
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return empty, nil
	}
	var store projectNoteStore
	if err := json.Unmarshal(data, &store); err != nil {
		return empty, fmt.Errorf("parse project notes: %w", err)
	}
	if store.Version != 1 {
		return empty, errors.New("unsupported project notes version")
	}
	if store.Notes == nil {
		store.Notes = []projectNoteRecord{}
	}
	return store, nil
}

func writeProjectNotes(workspace string, store projectNoteStore) error {
	store.Version = 1
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("encode project notes: %w", err)
	}
	data = append(data, '\n')
	if len(data) > projectNotesMaxFile {
		return errors.New("project notes exceed size limit")
	}
	path, err := projectNotesPath(workspace)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".vscode_tasks_menu.notes.*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func validateProjectNoteFields(req projectNoteSaveRequest) (projectNoteSaveRequest, error) {
	req.Category = strings.TrimSpace(req.Category)
	req.Title = strings.TrimSpace(req.Title)
	if !utf8.ValidString(req.Category) || !utf8.ValidString(req.Title) || !utf8.ValidString(req.Content) {
		return req, errors.New("invalid UTF-8 in project note")
	}
	if req.Title == "" || utf8.RuneCountInString(req.Title) > projectNotesMaxTitle ||
		strings.ContainsAny(req.Title, "\r\n\x00") {
		return req, errors.New("note title is required and must be one line (max 180 characters)")
	}
	if utf8.RuneCountInString(req.Category) > projectNotesMaxCategory ||
		strings.ContainsAny(req.Category, "\r\n\x00") {
		return req, errors.New("note category must be one line (max 80 characters)")
	}
	if strings.TrimSpace(req.Content) == "" || len(req.Content) > projectNotesMaxContent ||
		strings.ContainsRune(req.Content, '\x00') {
		return req, errors.New("note content is required (max 128 KiB)")
	}
	return req, nil
}

func nextProjectNoteID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func notesForProject(store projectNoteStore, key string) []projectNote {
	result := []projectNote{}
	for _, item := range store.Notes {
		if item.ProjectKey == key {
			result = append(result, item.Note)
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].UpdatedAt > result[j].UpdatedAt
	})
	return result
}

func decodeProjectNote(w http.ResponseWriter, r *http.Request) (projectNoteSaveRequest, error) {
	var req projectNoteSaveRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, projectNotesMaxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		return req, errors.New("invalid project note JSON")
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return req, errors.New("project note JSON must contain exactly one object")
	}
	return validateProjectNoteFields(req)
}

func (s *Server) projectNotes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete:
	default:
		w.Header().Set("Allow", "GET, POST, PUT, DELETE")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	key, err := projectNoteScope(s, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	s.projectNotesMu.Lock()
	defer s.projectNotesMu.Unlock()
	store, err := readProjectNotes(s.Workspace)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, map[string]any{"notes": notesForProject(store, key)})
	case http.MethodPost, http.MethodPut:
		req, err := decodeProjectNote(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		id := strings.TrimSpace(req.ID)
		if r.Method == http.MethodPost && id != "" || r.Method == http.MethodPut && id == "" {
			http.Error(w, "POST must omit id; PUT requires id", http.StatusBadRequest)
			return
		}
		index := -1
		if r.Method == http.MethodPut {
			for i := range store.Notes {
				if store.Notes[i].ProjectKey == key && store.Notes[i].Note.ID == id {
					index = i
					break
				}
			}
			if index == -1 {
				http.Error(w, "project note not found", http.StatusNotFound)
				return
			}
		} else {
			if len(notesForProject(store, key)) >= projectNotesMaxCount {
				http.Error(w, "project note limit reached", http.StatusConflict)
				return
			}
			id, err = nextProjectNoteID()
			if err != nil {
				http.Error(w, "cannot allocate project note id", http.StatusInternalServerError)
				return
			}
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		note := projectNote{
			ID: id, Category: req.Category, Title: req.Title,
			Content: req.Content, CreatedAt: now, UpdatedAt: now,
		}
		if index >= 0 {
			note.CreatedAt = store.Notes[index].Note.CreatedAt
			if note.CreatedAt == "" {
				note.CreatedAt = now
			}
			store.Notes[index].Note = note
		} else {
			store.Notes = append(store.Notes, projectNoteRecord{ProjectKey: key, Note: note})
		}
		if err := writeProjectNotes(s.Workspace, store); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.auditSharedSuccess(r, "project_note.save", "project_note", id, nil)
		writeJSON(w, http.StatusOK, note)
	case http.MethodDelete:
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		if id == "" {
			http.Error(w, "project note id is required", http.StatusBadRequest)
			return
		}
		found := false
		next := make([]projectNoteRecord, 0, len(store.Notes))
		for _, item := range store.Notes {
			if item.ProjectKey == key && item.Note.ID == id {
				found = true
				continue
			}
			next = append(next, item)
		}
		if !found {
			http.Error(w, "project note not found", http.StatusNotFound)
			return
		}
		store.Notes = next
		if err := writeProjectNotes(s.Workspace, store); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.auditSharedSuccess(r, "project_note.delete", "project_note", id, nil)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
	}
}
