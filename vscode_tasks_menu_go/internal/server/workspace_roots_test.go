package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/config"
)

func callWorkspaceRoots(t *testing.T, s *Server, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	req:=httptest.NewRequest(method,target,strings.NewReader(body))
	if body!="" { req.Header.Set("Content-Type","application/json") }
	rr:=httptest.NewRecorder(); s.workspaceRoots(rr,req); return rr
}

func TestWorkspaceRootsCRUDAndResolve(t *testing.T) {
	base:=t.TempDir()
	primary:=filepath.Join(base,"main")
	attached:=filepath.Join(base,"client")
	if err:=os.MkdirAll(primary,0o755); err!=nil { t.Fatal(err) }
	if err:=os.MkdirAll(attached,0o755); err!=nil { t.Fatal(err) }
	s:=&Server{Workspace:primary}

	list:=callWorkspaceRoots(t,s,http.MethodGet,"/api/workspace-roots","")
	if list.Code!=http.StatusOK { t.Fatalf("list status=%d body=%s",list.Code,list.Body.String()) }
	var initial struct{ Roots []workspaceRootView `json:"roots"`; AttachedEnabled bool `json:"attached_enabled"` }
	if err:=json.Unmarshal(list.Body.Bytes(),&initial); err!=nil { t.Fatal(err) }
	if !initial.AttachedEnabled || len(initial.Roots)!=1 || !initial.Roots[0].Primary || initial.Roots[0].ID!=workspacePrimaryRootID { t.Fatalf("initial=%+v",initial) }

	create:=callWorkspaceRoots(t,s,http.MethodPost,"/api/workspace-roots",`{"path":"../client","name":"M3 Client"}`)
	if create.Code!=http.StatusCreated { t.Fatalf("create status=%d body=%s",create.Code,create.Body.String()) }
	var created workspaceRootView
	if err:=json.Unmarshal(create.Body.Bytes(),&created); err!=nil { t.Fatal(err) }
	if created.ID=="" || created.ID==workspacePrimaryRootID || created.Name!="M3 Client" || !created.Available || !created.Attached { t.Fatalf("created=%+v",created) }
	resolved,err:=s.resolveWorkspaceRoot(created.ID); if err!=nil { t.Fatal(err) }
	canonical,err:=canonicalWorkspaceRoot(attached); if err!=nil { t.Fatal(err) }
	if resolved.Path!=canonical { t.Fatalf("resolved=%+v want=%q",resolved,canonical) }

	rename:=callWorkspaceRoots(t,s,http.MethodPut,"/api/workspace-roots",`{"id":"`+created.ID+`","name":"Client App"}`)
	if rename.Code!=http.StatusOK || !strings.Contains(rename.Body.String(),`"name":"Client App"`) { t.Fatalf("rename status=%d body=%s",rename.Code,rename.Body.String()) }

	dup:=callWorkspaceRoots(t,s,http.MethodPost,"/api/workspace-roots",`{"path":"../client"}`)
	if dup.Code!=http.StatusConflict { t.Fatalf("duplicate status=%d body=%s",dup.Code,dup.Body.String()) }

	del:=callWorkspaceRoots(t,s,http.MethodDelete,"/api/workspace-roots?id="+created.ID,"")
	if del.Code!=http.StatusOK { t.Fatalf("delete status=%d body=%s",del.Code,del.Body.String()) }
	if _,err:=s.resolveWorkspaceRoot(created.ID); err==nil { t.Fatal("detached root still resolves") }

	path,err:=workspaceRootsPath(primary); if err!=nil { t.Fatal(err) }
	info,err:=os.Stat(path); if err!=nil { t.Fatal(err) }
	if info.Mode().Perm()&0o077!=0 { t.Fatalf("workspace root store permissions=%o",info.Mode().Perm()) }
}

func TestWorkspaceRootsCanonicalizesSymlinkAndRejectsDuplicate(t *testing.T) {
	if runtime.GOOS=="windows" { t.Skip("symlink test") }
	base:=t.TempDir(); primary:=filepath.Join(base,"main"); target:=filepath.Join(base,"target"); link:=filepath.Join(base,"link")
	if err:=os.MkdirAll(primary,0o755); err!=nil { t.Fatal(err) }
	if err:=os.MkdirAll(target,0o755); err!=nil { t.Fatal(err) }
	if err:=os.Symlink(target,link); err!=nil { t.Fatal(err) }
	s:=&Server{Workspace:primary}
	first:=callWorkspaceRoots(t,s,http.MethodPost,"/api/workspace-roots",`{"path":"../link"}`)
	if first.Code!=http.StatusCreated { t.Fatalf("first status=%d body=%s",first.Code,first.Body.String()) }
	second:=callWorkspaceRoots(t,s,http.MethodPost,"/api/workspace-roots",`{"path":"../target"}`)
	if second.Code!=http.StatusConflict { t.Fatalf("symlink duplicate status=%d body=%s",second.Code,second.Body.String()) }
}

func TestWorkspaceRootsReportUnavailableAttachedRoot(t *testing.T) {
	base:=t.TempDir(); primary:=filepath.Join(base,"main"); attached:=filepath.Join(base,"gone")
	if err:=os.MkdirAll(primary,0o755); err!=nil { t.Fatal(err) }
	if err:=os.MkdirAll(attached,0o755); err!=nil { t.Fatal(err) }
	s:=&Server{Workspace:primary}
	create:=callWorkspaceRoots(t,s,http.MethodPost,"/api/workspace-roots",`{"path":"../gone","name":"Gone"}`)
	if create.Code!=http.StatusCreated { t.Fatalf("create status=%d body=%s",create.Code,create.Body.String()) }
	var item workspaceRootView; if err:=json.Unmarshal(create.Body.Bytes(),&item); err!=nil { t.Fatal(err) }
	if err:=os.RemoveAll(attached); err!=nil { t.Fatal(err) }
	list:=callWorkspaceRoots(t,s,http.MethodGet,"/api/workspace-roots","")
	if list.Code!=http.StatusOK { t.Fatalf("list status=%d body=%s",list.Code,list.Body.String()) }
	if !strings.Contains(list.Body.String(),`"name":"Gone"`) || !strings.Contains(list.Body.String(),`"available":false`) { t.Fatalf("list=%s",list.Body.String()) }
	if _,err:=s.resolveWorkspaceRoot(item.ID); err==nil { t.Fatal("unavailable root resolved") }
}

func TestWorkspaceRootsSharedModeIsPrimaryOnlyAndReadOnly(t *testing.T) {
	base:=t.TempDir(); primary:=filepath.Join(base,"main"); other:=filepath.Join(base,"other")
	if err:=os.MkdirAll(primary,0o755); err!=nil { t.Fatal(err) }
	if err:=os.MkdirAll(other,0o755); err!=nil { t.Fatal(err) }
	local:=&Server{Workspace:primary}
	created:=callWorkspaceRoots(t,local,http.MethodPost,"/api/workspace-roots",`{"path":"../other"}`)
	if created.Code!=http.StatusCreated { t.Fatalf("seed status=%d body=%s",created.Code,created.Body.String()) }

	shared:=&Server{Workspace:primary,Config:config.Config{SharedServerEnabled:true}}
	list:=callWorkspaceRoots(t,shared,http.MethodGet,"/api/workspace-roots","")
	if list.Code!=http.StatusOK { t.Fatalf("shared list status=%d body=%s",list.Code,list.Body.String()) }
	var payload struct{ Roots []workspaceRootView `json:"roots"`; AttachedEnabled bool `json:"attached_enabled"` }
	if err:=json.Unmarshal(list.Body.Bytes(),&payload); err!=nil { t.Fatal(err) }
	if payload.AttachedEnabled || len(payload.Roots)!=1 || !payload.Roots[0].Primary { t.Fatalf("shared payload=%+v",payload) }
	mutate:=callWorkspaceRoots(t,shared,http.MethodPost,"/api/workspace-roots",`{"path":"../other"}`)
	if mutate.Code!=http.StatusForbidden { t.Fatalf("shared mutation status=%d body=%s",mutate.Code,mutate.Body.String()) }
}

func TestWorkspaceRootsRejectPrimaryAndInvalidName(t *testing.T) {
	workspace:=t.TempDir(); s:=&Server{Workspace:workspace}
	primary:=callWorkspaceRoots(t,s,http.MethodPost,"/api/workspace-roots",`{"path":"."}`)
	if primary.Code!=http.StatusConflict { t.Fatalf("primary status=%d body=%s",primary.Code,primary.Body.String()) }
	child:=filepath.Join(workspace,"child"); if err:=os.MkdirAll(child,0o755); err!=nil { t.Fatal(err) }
	bad:=callWorkspaceRoots(t,s,http.MethodPost,"/api/workspace-roots",`{"path":"child","name":"bad\nname"}`)
	if bad.Code!=http.StatusBadRequest { t.Fatalf("bad name status=%d body=%s",bad.Code,bad.Body.String()) }
}

func TestWorkspaceVirtualPathResolution(t *testing.T) {
	base := t.TempDir()
	primary := filepath.Join(base, "main")
	attached := filepath.Join(base, "client")
	if err := os.MkdirAll(primary, 0o755); err != nil { t.Fatal(err) }
	if err := os.MkdirAll(attached, 0o755); err != nil { t.Fatal(err) }
	s := &Server{Workspace: primary}
	create := callWorkspaceRoots(t, s, http.MethodPost, "/api/workspace-roots", `{"path":"../client","name":"Client"}`)
	if create.Code != http.StatusCreated { t.Fatalf("create status=%d body=%s", create.Code, create.Body.String()) }
	var item workspaceRootView
	if err := json.Unmarshal(create.Body.Bytes(), &item); err != nil { t.Fatal(err) }

	virtual := workspaceVirtualPath(item.ID, "src/main.go")
	root, rel, err := s.projectRootForVirtualPath(virtual)
	if err != nil { t.Fatal(err) }
	if root.ID != item.ID || root.Path != item.Path || rel != "src/main.go" {
		t.Fatalf("root=%+v rel=%q virtual=%q", root, rel, virtual)
	}
	primaryRoot, primaryRel, err := s.projectRootForVirtualPath("src/main.go")
	if err != nil { t.Fatal(err) }
	if !primaryRoot.Primary || primaryRel != "src/main.go" {
		t.Fatalf("primary root=%+v rel=%q", primaryRoot, primaryRel)
	}
	if _, _, err := s.projectRootForVirtualPath("@root/unknown/file.txt"); err == nil {
		t.Fatal("unknown attached root resolved")
	}
}
