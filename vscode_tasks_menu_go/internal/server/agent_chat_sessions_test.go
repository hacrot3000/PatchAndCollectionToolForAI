package server

import (
	"encoding/json"
	"net/http"
	"os"
	"net/url"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/identity"
)

func TestAgentChatSessionsCreateListAndGet(t *testing.T){
	s:=newAIServerForTest(t)
	if err:=os.MkdirAll(s.Workspace,0700);err!=nil{t.Fatal(err)}
	if err:=s.saveAISettings(aiSettings{Enabled:true,Consent:true,Provider:"openai",Model:"gpt-4.1-mini"});err!=nil{t.Fatal(err)}
	created:=invokeAI(t,s.agentChatSessionsAPI,http.MethodPost,"/api/agent/sessions",map[string]string{"mode":"stateless"})
	if created.Code!=http.StatusCreated{t.Fatalf("create %d %s",created.Code,created.Body)}
	var c struct{
		Session struct{ID string `json:"id"`;ProjectID string `json:"project_id"`; Revision uint64 `json:"revision"`} `json:"session"`
		ExecutionEnabled bool `json:"execution_enabled"`
	}
	if err:=json.Unmarshal(created.Body.Bytes(),&c);err!=nil{t.Fatal(err)}
	if c.Session.ID==""||c.Session.ProjectID==""||c.Session.Revision!=1||c.ExecutionEnabled{t.Fatalf("bad session: %+v",c)}
	list:=invokeAI(t,s.agentChatSessionsAPI,http.MethodGet,"/api/agent/sessions",nil)
	if list.Code!=http.StatusOK||!strings.Contains(list.Body.String(),c.Session.ID){t.Fatalf("list %d %s",list.Code,list.Body)}
	get:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodGet,"/api/agent/sessions/"+c.Session.ID,nil)
	if get.Code!=http.StatusOK||!strings.Contains(get.Body.String(),c.Session.ProjectID){t.Fatalf("get %d %s",get.Code,get.Body)}
	other:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodGet,"/api/agent/sessions/../../etc/passwd",nil)
	if other.Code!=http.StatusBadRequest{t.Fatalf("path traversal accepted: %d",other.Code)}
}

func TestAgentChatSessionsDisabledAndUnknownFields(t *testing.T){
	s:=newAIServerForTest(t)
	w:=invokeAI(t,s.agentChatSessionsAPI,http.MethodPost,"/api/agent/sessions",map[string]string{"mode":"stateless"})
	if w.Code!=http.StatusForbidden{t.Fatalf("expected opt-in rejection, got %d",w.Code)}
	if err:=s.saveAISettings(aiSettings{Enabled:true,Consent:true,Provider:"openai"});err!=nil{t.Fatal(err)}
	w=invokeAI(t,s.agentChatSessionsAPI,http.MethodPost,"/api/agent/sessions",map[string]string{"mode":"unknown"})
	if w.Code!=http.StatusBadRequest && w.Code!=http.StatusConflict{t.Fatalf("invalid mode accepted: %d",w.Code)}
	w=invokeAI(t,s.agentChatSessionsAPI,http.MethodPost,"/api/agent/sessions",map[string]string{"unexpected":"value"})
	if w.Code!=http.StatusBadRequest{t.Fatalf("unknown fields accepted: %d",w.Code)}
}

func TestAgentChatRoutesExplicitSharedPermission(t *testing.T){
	routes:=[]struct{method,path string;perms []string}{
		{"GET","/api/agent/sessions",[]string{identity.PermissionSettingsRead}},
		{"POST","/api/agent/sessions",[]string{identity.PermissionSettingsWrite}},
		{"GET","/api/agent/sessions/123",[]string{identity.PermissionSettingsRead}},
	}
	for _,tc:=range routes{
		r:=&http.Request{Method:tc.method,URL:mustAgentTestURL(t,tc.path)}
		p:=sharedRoutePermissions(r)
		if len(p)!=len(tc.perms)||len(p)!=1||p[0]!=tc.perms[0]{
			t.Fatalf("missing shared permission %s %s: %v",tc.method,tc.path,p)
		}
	}
}

func mustAgentTestURL(t *testing.T,path string)*url.URL{
	t.Helper()
	u,err:=url.Parse(path);if err!=nil{t.Fatal(err)}
	return u
}
