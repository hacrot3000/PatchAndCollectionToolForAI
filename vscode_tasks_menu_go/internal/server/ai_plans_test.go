package server

import (
 "bytes"
 "context"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/session"
)
type aiTestSessions struct {
 session.Service
 metadata map[string]session.Metadata
}
func (f *aiTestSessions) Metadata(id string)(session.Metadata,bool){m,ok:=f.metadata[id];return m,ok}
func (f *aiTestSessions) SupportsSessionOwnership()bool{return true}
func newAIAttestedTestServer(t *testing.T)*Server{
 s:=newAIServerForTest(t)
 s.Sessions=&aiTestSessions{metadata:map[string]session.Metadata{
  "session-A":{ID:"session-A",Kind:"terminal",Status:"running",OwnerUserID:"alice",ProjectID:"project",TargetType:"ssh",TargetProfileID:"production.example",Cwd:"/srv/app"},
  "a":{ID:"a",Kind:"terminal",Status:"running",OwnerUserID:"alice",ProjectID:"project",Cwd:"/test/local"},
 }}
 return s
}
func requestAIWithUser(t *testing.T,handler http.HandlerFunc,method,path string,payload any,user string)*httptest.ResponseRecorder{
 t.Helper();var body []byte
 if payload!=nil{body,_=json.Marshal(payload)}
 req:=httptest.NewRequest(method,path,bytes.NewReader(body))
 if user!=""{req=req.WithContext(context.WithValue(req.Context(),sharedPrincipalContextKey{},identity.Principal{UserID:identity.ID(user),ProjectID:identity.ID("project"),Permissions:map[string]bool{identity.PermissionTerminalViewOwn:true}}))}
 w:=httptest.NewRecorder();handler(w,req);return w
}
func TestAIGuidedPlansJournalApprovesWithoutExecution(t *testing.T){
 s:=newAIAttestedTestServer(t)
 if e:=s.saveAISettings(aiSettings{Enabled:true,Consent:true,Model:"gpt-4.1-mini"});e!=nil{t.Fatal(e)}
 input:=aiPlanRequest{Target:aiPlanTarget{SessionID:"session-A",Host:"production.example",CWD:"/srv/app",Remote:true},
  Steps:[]aiPlanStep{{Command:"pwd",Reason:"Check the directory",Risk:"read-only"},{Command:"systemctl restart game",Reason:"Needs explicit external approval",Risk:"critical"}}}
 created:=requestAIWithUser(t,s.aiPlansAPI,"POST","/api/ai/plans",input,"alice")
 if created.Code!=201{t.Fatalf("create %d %s",created.Code,created.Body.String())}
 var plan aiGuidedPlan;json.Unmarshal(created.Body.Bytes(),&plan)
 if plan.ID==""||plan.ExecutionEnabled||len(plan.Steps)!=2||plan.Steps[1].Decision!="pending"{t.Fatalf("invalid plan %+v",plan)}
 decision:=requestAIWithUser(t,s.aiPlanDecisionAPI,"POST","/api/ai/plans/decision",aiPlanDecision{ID:plan.ID,Step:1,Decision:"approved",Digest:plan.Digest},"alice")
 if decision.Code!=200{t.Fatalf("decision %d %s",decision.Code,decision.Body.String())}
 if !strings.Contains(decision.Body.String(),"\"execution_enabled\":false"){t.Fatal("approved plan must remain non executable")}
 duplicate:=requestAIWithUser(t,s.aiPlanDecisionAPI,"POST","/api/ai/plans/decision",aiPlanDecision{ID:plan.ID,Step:1,Decision:"approved",Digest:plan.Digest},"alice")
 if duplicate.Code!=409{t.Fatalf("duplicate approval should conflict, got %d",duplicate.Code)}
 listed:=requestAIWithUser(t,s.aiPlansAPI,"GET","/api/ai/plans",nil,"alice")
 if listed.Code!=200||!strings.Contains(listed.Body.String(),plan.ID){t.Fatal("plan journal did not persist")}
}
func TestAIGuidedPlansTenantIsolation(t *testing.T){
 s:=newAIAttestedTestServer(t)
 if e:=s.saveAISettings(aiSettings{Enabled:true,Consent:true,Model:"gpt-4.1-mini"});e!=nil{t.Fatal(e)}
 create:=requestAIWithUser(t,s.aiPlansAPI,"POST","/api/ai/plans",aiPlanRequest{
  Target:aiPlanTarget{SessionID:"a"},Steps:[]aiPlanStep{{Command:"pwd",Risk:"read-only"}}},"alice")
 if create.Code!=201{t.Fatalf("create %d",create.Code)}
 var plan aiGuidedPlan;json.Unmarshal(create.Body.Bytes(),&plan)
 bob:=requestAIWithUser(t,s.aiPlansAPI,"GET","/api/ai/plans",nil,"bob")
 if strings.Contains(bob.Body.String(),plan.ID){t.Fatal("cross-user plan leaked")}
 denied:=requestAIWithUser(t,s.aiPlanDecisionAPI,"POST","/api/ai/plans/decision",aiPlanDecision{ID:plan.ID,Step:0,Decision:"approved",Digest:plan.Digest},"bob")
 if denied.Code!=404{t.Fatalf("cross-user approval should be denied: %d",denied.Code)}
}
func TestAIGuidedPlansRejectInvalidStepAndRedact(t *testing.T){
 s:=newAIAttestedTestServer(t)
 if e:=s.saveAISettings(aiSettings{Enabled:true,Consent:true,Model:"gpt-4.1-mini"});e!=nil{t.Fatal(e)}
 invalid:=requestAIWithUser(t,s.aiPlansAPI,"POST","/api/ai/plans",aiPlanRequest{Steps:[]aiPlanStep{{Command:""}}},"alice")
 if invalid.Code!=400{t.Fatalf("empty command accepted %d",invalid.Code)}
 valid:=requestAIWithUser(t,s.aiPlansAPI,"POST","/api/ai/plans",aiPlanRequest{Target:aiPlanTarget{SessionID:"a"},Steps:[]aiPlanStep{{Command:"echo token=topsecret",Risk:"unlikely"}}},"alice")
 if valid.Code!=201{t.Fatalf("valid command rejected %d",valid.Code)}
 if strings.Contains(valid.Body.String(),"topsecret"){t.Fatal("sensitive command persisted or echoed")}
 if !strings.Contains(valid.Body.String(),"\"risk\":\"high\""){t.Fatal("unrecognized risk should default high")}
}
