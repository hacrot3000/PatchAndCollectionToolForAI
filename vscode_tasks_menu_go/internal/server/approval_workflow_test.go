package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/approval"
	"bletonfc/vscode_tasks_menu/internal/dbprofile"
	"bletonfc/vscode_tasks_menu/internal/dbsession"
)

func TestApprovalPolicyDefaultsOffAndDangerousChallengeIsOneTime(t *testing.T) {
	s := &Server{Workspace: t.TempDir()}
	store, err := s.dangerousApprovalStore()
	if err != nil { t.Fatal(err) }
	if _, err := store.SetPolicy(approval.Policy{Rules: []approval.Rule{{Action:"git.remote.delete",Mode:approval.ModeType}}}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/git/status", nil)
	rr := httptest.NewRecorder()
	if s.requireDangerousApproval(rr, req, "git.remote.delete", "origin/feature-x") {
		t.Fatal("approval unexpectedly allowed without grant")
	}
	if rr.Code != http.StatusPreconditionRequired || !strings.Contains(rr.Body.String(), `"approval_required":true`) {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	created, err := store.Create("git.remote.delete", "origin/feature-x", "local", "local", "origin/feature-x")
	if err != nil { t.Fatal(err) }
	req = httptest.NewRequest(http.MethodPost, "/api/git/status", nil)
	req.Header.Set("X-TaskDeck-Approval-ID", created.ID)
	rr = httptest.NewRecorder()
	if !s.requireDangerousApproval(rr, req, "git.remote.delete", "origin/feature-x") {
		t.Fatalf("approved grant rejected status=%d body=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/git/status", nil)
	req.Header.Set("X-TaskDeck-Approval-ID", created.ID)
	rr = httptest.NewRecorder()
	if s.requireDangerousApproval(rr, req, "git.remote.delete", "origin/feature-x") {
		t.Fatal("one-time approval grant was reused")
	}
	if rr.Code != http.StatusPreconditionFailed {
		t.Fatalf("reuse status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestDatabaseProductionMarkerIsExplicit(t *testing.T) {
	cases := []struct{
		name string
		profile dbprofile.Profile
		want bool
	}{
		{"name alone is not production", dbprofile.Profile{Name:"MySQL Production"}, false},
		{"environment", dbprofile.Profile{Options:map[string]string{"environment":"production"}}, true},
		{"short env", dbprofile.Profile{Options:map[string]string{"env":"prod"}}, true},
		{"boolean", dbprofile.Profile{Options:map[string]string{"production":"true"}}, true},
		{"false", dbprofile.Profile{Options:map[string]string{"production":"false"}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T){
			if got:=databaseProfileProduction(tc.profile); got!=tc.want {
				t.Fatalf("production=%v want=%v profile=%+v",got,tc.want,tc.profile)
			}
		})
	}
}

func TestProductionDatabaseApprovalOnlyForWriteAndSchema(t *testing.T) {
	workspace := t.TempDir()
	s := &Server{Workspace: workspace}
	profileStore, err := dbprofile.NewStore(filepath.Join(workspace, "profiles.json"))
	if err != nil { t.Fatal(err) }
	profile, err := profileStore.Create(dbprofile.Profile{
		Name:"Prod", AdapterID:"mysql-cli", AdapterKind:"mysql", Host:"127.0.0.1", Port:3306,
		Options:map[string]string{"environment":"production"},
	})
	if err != nil { t.Fatal(err) }
	s.DBProfiles = profileStore
	store, err := s.dangerousApprovalStore()
	if err != nil { t.Fatal(err) }
	if _, err := store.SetPolicy(approval.Policy{Rules:[]approval.Rule{{Action:"db.production.write",Mode:approval.ModeConfirm}}}); err != nil {
		t.Fatal(err)
	}
	meta := dbsession.Metadata{ProfileID:profile.ID,AdapterKind:"mysql"}

	for _, permission := range []string{"db.read"} {
		rr:=httptest.NewRecorder()
		req:=httptest.NewRequest(http.MethodPost,"/api/db/sessions/id/request",nil)
		if !s.requireProductionDatabaseApproval(rr,req,meta,permission,"app") {
			t.Fatalf("read permission unexpectedly gated: status=%d body=%s",rr.Code,rr.Body.String())
		}
	}
	for _, permission := range []string{"db.write","db.schema"} {
		rr:=httptest.NewRecorder()
		req:=httptest.NewRequest(http.MethodPost,"/api/db/sessions/id/request",nil)
		if s.requireProductionDatabaseApproval(rr,req,meta,permission,"app") {
			t.Fatalf("%s unexpectedly allowed without approval",permission)
		}
		if rr.Code!=http.StatusPreconditionRequired {
			t.Fatalf("%s status=%d body=%s",permission,rr.Code,rr.Body.String())
		}
	}
}
