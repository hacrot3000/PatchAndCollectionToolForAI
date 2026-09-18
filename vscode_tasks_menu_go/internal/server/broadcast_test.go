package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/session"
	"bletonfc/vscode_tasks_menu/internal/tasks"
)

type broadcastTestService struct {
	session.Service
	metas  map[string]session.Metadata
	inputs map[string][]string
}

func newBroadcastTestService() *broadcastTestService {
	return &broadcastTestService{
		metas: map[string]session.Metadata{
			"1": {ID: "1", TaskID: 0, Label: "Terminal 1", Status: "running"},
			"2": {ID: "2", TaskID: 42, Label: "Task 2", Status: "running"},
			"3": {ID: "3", TaskID: 0, Label: "Terminal 3", Status: "running"},
			"4": {ID: "4", TaskID: 0, Label: "Terminal 4", Status: "running"},
			"5": {ID: "5", TaskID: 0, Label: "Terminal 5", Status: "running"},
			"6": {ID: "6", TaskID: 0, Label: "Terminal 6", Status: "exited"},
		},
		inputs: map[string][]string{},
	}
}

func (s *broadcastTestService) List() []session.Metadata {
	out := make([]session.Metadata, 0, len(s.metas))
	for _, meta := range s.metas {
		out = append(out, meta)
	}
	return out
}

func (s *broadcastTestService) Metadata(id string) (session.Metadata, bool) {
	meta, ok := s.metas[id]
	return meta, ok
}

func (s *broadcastTestService) Input(id string, data []byte) error {
	s.inputs[id] = append(s.inputs[id], string(data))
	return nil
}

func (s *broadcastTestService) Start(tasks.Execution) (session.Metadata, error) {
	panic("unexpected Start")
}

func postBroadcast(t *testing.T, srv *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/broadcast", strings.NewReader(body))
	rr := httptest.NewRecorder()
	srv.broadcastStateAPI(rr, req)
	return rr
}

func decodeBroadcastState(t *testing.T, rr *httptest.ResponseRecorder) broadcastState {
	t.Helper()
	var state broadcastState
	if err := json.Unmarshal(rr.Body.Bytes(), &state); err != nil {
		t.Fatalf("decode broadcast state: %v body=%s", err, rr.Body.String())
	}
	return state
}

func createBroadcastGroupForTest(t *testing.T, srv *Server, name, preset string) string {
	t.Helper()
	rr := postBroadcast(t, srv, `{"action":"create_group","name":"`+name+`","preset":"`+preset+`"}`)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create group status=%d body=%s", rr.Code, rr.Body.String())
	}
	state := decodeBroadcastState(t, rr)
	if len(state.Groups) == 0 {
		t.Fatal("group not created")
	}
	return state.Groups[len(state.Groups)-1].ID
}

func assignBroadcastGroupForTest(t *testing.T, srv *Server, sessionID, groupID string) {
	t.Helper()
	rr := postBroadcast(t, srv, `{"action":"assign","session_id":"`+sessionID+`","group_id":"`+groupID+`"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("assign session=%s status=%d body=%s", sessionID, rr.Code, rr.Body.String())
	}
}

func setBroadcastModeForTest(t *testing.T, srv *Server, mode string) {
	t.Helper()
	rr := postBroadcast(t, srv, `{"action":"set_mode","mode":"`+mode+`"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("set mode=%s status=%d body=%s", mode, rr.Code, rr.Body.String())
	}
}

func resetBroadcastInputs(service *broadcastTestService) {
	service.inputs = map[string][]string{}
}

func TestBroadcastModesAndGroups(t *testing.T) {
	workspace := t.TempDir()
	service := newBroadcastTestService()
	srv := &Server{Workspace: workspace, Sessions: service}

	groupA := createBroadcastGroupForTest(t, srv, "Group A", "ocean")
	groupB := createBroadcastGroupForTest(t, srv, "Group B", "forest")
	assignBroadcastGroupForTest(t, srv, "1", groupA)
	assignBroadcastGroupForTest(t, srv, "2", groupA)
	assignBroadcastGroupForTest(t, srv, "3", groupB)
	assignBroadcastGroupForTest(t, srv, "4", groupB)

	setBroadcastModeForTest(t, srv, broadcastModeAll)
	rr := postBroadcast(t, srv, `{"action":"input","source_id":"1","data":"Y"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("broadcast all status=%d body=%s", rr.Code, rr.Body.String())
	}
	if len(service.inputs["1"]) != 0 {
		t.Fatal("source session received duplicate broadcast input")
	}
	for _, id := range []string{"2", "3", "4", "5"} {
		if got := strings.Join(service.inputs[id], ""); got != "Y" {
			t.Fatalf("broadcast all session %s got %q want Y", id, got)
		}
	}
	if len(service.inputs["6"]) != 0 {
		t.Fatal("exited session received broadcast input")
	}

	resetBroadcastInputs(service)
	setBroadcastModeForTest(t, srv, broadcastModeGroup)
	rr = postBroadcast(t, srv, `{"action":"input","source_id":"1","data":"Y"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("broadcast group A status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := strings.Join(service.inputs["2"], ""); got != "Y" {
		t.Fatalf("group peer got %q want Y", got)
	}
	for _, id := range []string{"1", "3", "4", "5", "6"} {
		if len(service.inputs[id]) != 0 {
			t.Fatalf("non-peer session %s received group input %#v", id, service.inputs[id])
		}
	}

	resetBroadcastInputs(service)
	rr = postBroadcast(t, srv, `{"action":"input","source_id":"3","data":"\r"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("broadcast group B Enter status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := strings.Join(service.inputs["4"], ""); got != "\r" {
		t.Fatalf("group B peer got raw input %q want carriage return", got)
	}
	for _, id := range []string{"1", "2", "3", "5", "6"} {
		if len(service.inputs[id]) != 0 {
			t.Fatalf("session %s unexpectedly received group B Enter", id)
		}
	}

	resetBroadcastInputs(service)
	rr = postBroadcast(t, srv, `{"action":"input","source_id":"5","data":"N"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("ungrouped source status=%d body=%s", rr.Code, rr.Body.String())
	}
	if len(service.inputs) != 0 {
		t.Fatalf("ungrouped source broadcast to peers: %#v", service.inputs)
	}

	resetBroadcastInputs(service)
	setBroadcastModeForTest(t, srv, broadcastModeNone)
	rr = postBroadcast(t, srv, `{"action":"input","source_id":"1","data":"Z"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("broadcast none status=%d body=%s", rr.Code, rr.Body.String())
	}
	if len(service.inputs) != 0 {
		t.Fatalf("broadcast none delivered input: %#v", service.inputs)
	}
}

func TestBroadcastStatePersistsAndGroupRemovalCleansAssignments(t *testing.T) {
	workspace := t.TempDir()
	service := newBroadcastTestService()
	srv := &Server{Workspace: workspace, Sessions: service}
	groupID := createBroadcastGroupForTest(t, srv, "Operators", "violet")
	assignBroadcastGroupForTest(t, srv, "1", groupID)
	setBroadcastModeForTest(t, srv, broadcastModeGroup)

	stat, err := os.Stat(broadcastStatePath(workspace))
	if err != nil {
		t.Fatal(err)
	}
	if stat.Mode().Perm() != 0o600 {
		t.Fatalf("broadcast state mode=%o want 600", stat.Mode().Perm())
	}

	replacement := &Server{Workspace: workspace, Sessions: service}
	get := httptest.NewRequest(http.MethodGet, "/api/broadcast", nil)
	getRR := httptest.NewRecorder()
	replacement.broadcastStateAPI(getRR, get)
	if getRR.Code != http.StatusOK {
		t.Fatalf("reload state status=%d body=%s", getRR.Code, getRR.Body.String())
	}
	state := decodeBroadcastState(t, getRR)
	if state.Mode != broadcastModeGroup || state.Assignments["1"] != groupID {
		t.Fatalf("broadcast state not restored: %#v", state)
	}

	rr := postBroadcast(t, replacement, `{"action":"delete_group","group_id":"`+groupID+`"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("delete group status=%d body=%s", rr.Code, rr.Body.String())
	}
	state = decodeBroadcastState(t, rr)
	if len(state.Groups) != 0 || state.Assignments["1"] != "" {
		t.Fatalf("delete group did not clean assignment: %#v", state)
	}
}

func TestBroadcastRejectsInvalidPreset(t *testing.T) {
	srv := &Server{Workspace: t.TempDir(), Sessions: newBroadcastTestService()}
	rr := postBroadcast(t, srv, `{"action":"create_group","name":"Unsafe","preset":"custom-css"}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("invalid preset status=%d body=%s", rr.Code, rr.Body.String())
	}
}
