package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func callProjectProfiles(t *testing.T, s *Server, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" { req.Header.Set("Content-Type", "application/json") }
	rr := httptest.NewRecorder()
	s.projectProfiles(rr, req)
	return rr
}

func TestNormalizeProjectProfileBoundsAndPaths(t *testing.T) {
	refs := make([]string, 0, projectProfileMaxRefs+10)
	for i := 0; i < projectProfileMaxRefs+10; i++ { refs = append(refs, "ref-"+strings.Repeat("x", i%4)) }
	tasks := make([]int, 0, projectProfileMaxTasks+20)
	for i := 1; i <= projectProfileMaxTasks+20; i++ { tasks = append(tasks, i) }
	terminals := make([]projectProfileTerminal, 0, projectProfileMaxTerminals+5)
	for i := 0; i < projectProfileMaxTerminals+5; i++ { terminals = append(terminals, projectProfileTerminal{Cwd:"tools", Title:"Term"}) }
	terminals[0].Cwd = "../escape"
	terminals[1].Cwd = "/absolute/path"
	got := normalizeProjectProfile(projectProfile{
		ID:"  p1 ", Name:" Development ", EnvironmentProfile:" Debug ",
		CommandPresetIDs:append(refs,"ref-x"), DatabaseProfileIDs:refs, SSHProfileIDs:refs, TransferProfileIDs:refs,
		DefaultGitRepo:" projects/client ", TaskIDs:append(tasks,1,2), Terminals:terminals,
	})
	if got.ID!="p1" || got.Name!="Development" || got.EnvironmentProfile!="Debug" || got.DefaultGitRepo!="projects/client" { t.Fatalf("normalized=%+v",got) }
	if len(got.CommandPresetIDs)>projectProfileMaxRefs || len(got.DatabaseProfileIDs)>projectProfileMaxRefs || len(got.SSHProfileIDs)>projectProfileMaxRefs || len(got.TransferProfileIDs)>projectProfileMaxRefs { t.Fatalf("reference limits=%+v",got) }
	if len(got.TaskIDs)!=projectProfileMaxTasks { t.Fatalf("task count=%d",len(got.TaskIDs)) }
	if len(got.Terminals)!=projectProfileMaxTerminals { t.Fatalf("terminal count=%d",len(got.Terminals)) }
	if got.Terminals[0].Cwd!="." || got.Terminals[1].Cwd!="." || got.Terminals[2].Cwd!="tools" { t.Fatalf("terminal cwd=%+v",got.Terminals[:3]) }
}

func TestProjectProfilesCRUDAndPrivateStorage(t *testing.T) {
	workspace := t.TempDir()
	s := &Server{Workspace:workspace}
	body := `{"name":"Development","environment_profile":"Debug","command_preset_ids":["preset-build","preset-test"],"terminals":[{"cwd":"projects/client","title":"Client"},{"cwd":"projects/server","title":"Server"}],"database_profile_ids":["db-main"],"ssh_profile_ids":["ssh-prod"],"transfer_profile_ids":["sftp-prod"],"default_git_repository":"projects/client","task_ids":[3,1,3,2]}`
	create := callProjectProfiles(t,s,http.MethodPost,"/api/project-profiles",body)
	if create.Code!=http.StatusOK { t.Fatalf("create status=%d body=%s",create.Code,create.Body.String()) }
	var created projectProfile
	if err:=json.Unmarshal(create.Body.Bytes(),&created); err!=nil { t.Fatal(err) }
	if created.ID=="" || created.Name!="Development" || created.EnvironmentProfile!="Debug" { t.Fatalf("created=%+v",created) }
	if got:=created.TaskIDs; len(got)!=3 || got[0]!=1 || got[1]!=2 || got[2]!=3 { t.Fatalf("task ids=%v",got) }
	list:=callProjectProfiles(t,s,http.MethodGet,"/api/project-profiles","")
	if list.Code!=http.StatusOK || !strings.Contains(list.Body.String(),"Development") { t.Fatalf("list status=%d body=%s",list.Code,list.Body.String()) }
	updateBody := `{"id":"`+created.ID+`","name":"Debug","default_git_repository":"."}`
	update:=callProjectProfiles(t,s,http.MethodPut,"/api/project-profiles",updateBody)
	if update.Code!=http.StatusOK || !strings.Contains(update.Body.String(),`"name":"Debug"`) { t.Fatalf("update status=%d body=%s",update.Code,update.Body.String()) }
	var updated projectProfile
	if err:=json.Unmarshal(update.Body.Bytes(),&updated); err!=nil { t.Fatal(err) }
	if updated.CreatedAt!=created.CreatedAt || updated.UpdatedAt=="" { t.Fatalf("timestamps created=%+v updated=%+v",created,updated) }
	path,err:=projectProfilesPath(workspace); if err!=nil { t.Fatal(err) }
	info,err:=os.Stat(path); if err!=nil { t.Fatal(err) }
	if info.Mode().Perm()&0o077!=0 { t.Fatalf("permissions=%o",info.Mode().Perm()) }
	raw,err:=os.ReadFile(path); if err!=nil { t.Fatal(err) }
	lower:=strings.ToLower(string(raw))
	for _,forbidden:=range []string{"password","secret_ref","auth_token"} { if strings.Contains(lower,forbidden) { t.Fatalf("store contains forbidden field %q: %s",forbidden,raw) } }
	del:=callProjectProfiles(t,s,http.MethodDelete,"/api/project-profiles?id="+created.ID,"")
	if del.Code!=http.StatusOK { t.Fatalf("delete status=%d body=%s",del.Code,del.Body.String()) }
	list=callProjectProfiles(t,s,http.MethodGet,"/api/project-profiles","")
	if list.Code!=http.StatusOK || strings.Contains(list.Body.String(),`"name":"Debug"`) { t.Fatalf("post-delete list=%s",list.Body.String()) }
}

func TestProjectProfilesRejectUnknownFieldsAndMissingRows(t *testing.T) {
	s:=&Server{Workspace:t.TempDir()}
	bad:=callProjectProfiles(t,s,http.MethodPost,"/api/project-profiles",`{"name":"Dev","password":"nope"}`)
	if bad.Code!=http.StatusBadRequest { t.Fatalf("unknown field status=%d body=%s",bad.Code,bad.Body.String()) }
	missing:=callProjectProfiles(t,s,http.MethodPut,"/api/project-profiles",`{"id":"missing","name":"Dev"}`)
	if missing.Code!=http.StatusNotFound { t.Fatalf("missing update status=%d body=%s",missing.Code,missing.Body.String()) }
	del:=callProjectProfiles(t,s,http.MethodDelete,"/api/project-profiles?id=missing","")
	if del.Code!=http.StatusNotFound { t.Fatalf("missing delete status=%d body=%s",del.Code,del.Body.String()) }
}

func TestProjectProfilesLimit(t *testing.T) {
	workspace:=t.TempDir()
	store:=projectProfileStore{Version:1}
	for i:=0;i<projectProfilesMaxCount;i++ { store.Profiles=append(store.Profiles,projectProfile{ID:"p-"+strings.Repeat("x",i%3),Name:"Existing"}) }
	if err:=writeProjectProfileStore(workspace,store); err!=nil { t.Fatal(err) }
	s:=&Server{Workspace:workspace}
	rr:=callProjectProfiles(t,s,http.MethodPost,"/api/project-profiles",`{"name":"Overflow"}`)
	if rr.Code!=http.StatusConflict { t.Fatalf("limit status=%d body=%s",rr.Code,rr.Body.String()) }
}
