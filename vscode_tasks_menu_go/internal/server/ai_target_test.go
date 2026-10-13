package server

import (
 "encoding/json"
 "net/http"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/session"
)

func prepareAITargetTest(t *testing.T)*Server{
 t.Helper()
 s:=newAIAttestedTestServer(t)
 if err:=s.saveAISettings(aiSettings{Enabled:true,Consent:true,Model:"gpt-4.1-mini"});err!=nil{t.Fatal(err)}
 return s
}

func TestAIAttestedPlanTargetIgnoresClientHostCWDAndRemote(t *testing.T){
 s:=prepareAITargetTest(t)
 submitted:=aiPlanRequest{Target:aiPlanTarget{SessionID:"session-A",Host:"forged-server",CWD:"/wrong",Remote:false},
  Steps:[]aiPlanStep{{Command:"pwd",Risk:"read-only"}}}
 res:=requestAIWithUser(t,s.aiPlansAPI,"POST","/api/ai/plans",submitted,"alice")
 if res.Code!=http.StatusCreated {t.Fatalf("create: %d %s",res.Code,res.Body.String())}
 var plan aiGuidedPlan
 if err:=json.Unmarshal(res.Body.Bytes(),&plan);err!=nil{t.Fatal(err)}
 if plan.Target.Host!="production.example"||plan.Target.CWD!="/srv/app"||!plan.Target.Remote{
  t.Fatalf("target was not derived from server: %+v",plan.Target)
 }
}

func TestAIAttestedTargetRejectsMissingForeignAndStoppedSessions(t *testing.T){
 s:=prepareAITargetTest(t)
 for _,tt:=range []struct{sessionID,user string;want int}{
  {"missing","alice",http.StatusNotFound},
  {"a","bob",http.StatusForbidden},
  {"","alice",http.StatusBadRequest},
 }{
  res:=requestAIWithUser(t,s.aiPlansAPI,"POST","/api/ai/plans",
   aiPlanRequest{Target:aiPlanTarget{SessionID:tt.sessionID},Steps:[]aiPlanStep{{Command:"pwd"}}},tt.user)
  if res.Code!=tt.want{t.Errorf("session=%q user=%q got %d want %d: %s",tt.sessionID,tt.user,res.Code,tt.want,res.Body.String())}
 }
 store:=s.Sessions.(*aiTestSessions)
 stopped:=store.metadata["a"];stopped.Status="stopped";store.metadata["a"]=stopped
 res:=requestAIWithUser(t,s.aiPlansAPI,"POST","/api/ai/plans",
  aiPlanRequest{Target:aiPlanTarget{SessionID:"a"},Steps:[]aiPlanStep{{Command:"pwd"}}},"alice")
 if res.Code!=http.StatusConflict{t.Fatalf("stopped session accepted: %d",res.Code)}
}

func TestAIGuidedPlanApprovalRejectsStaleTarget(t *testing.T){
 s:=prepareAITargetTest(t)
 create:=requestAIWithUser(t,s.aiPlansAPI,"POST","/api/ai/plans",
  aiPlanRequest{Target:aiPlanTarget{SessionID:"a"},Steps:[]aiPlanStep{{Command:"pwd",Risk:"read-only"}}},"alice")
 if create.Code!=http.StatusCreated{t.Fatalf("create: %d %s",create.Code,create.Body.String())}
 var plan aiGuidedPlan
 json.Unmarshal(create.Body.Bytes(),&plan)
 store:=s.Sessions.(*aiTestSessions)
 changed:=store.metadata["a"];changed.Cwd="/unexpected";store.metadata["a"]=changed
 res:=requestAIWithUser(t,s.aiPlanDecisionAPI,"POST","/api/ai/plans/decision",
  aiPlanDecision{ID:plan.ID,Step:0,Decision:"approved",Digest:plan.Digest},"alice")
 if res.Code!=http.StatusConflict{t.Fatalf("stale target approval accepted: %d %s",res.Code,res.Body.String())}
}

func TestAIAssistRequiresAttestedLiveSessionBeforeProvider(t *testing.T){
 s:=prepareAITargetTest(t)
 missing:=requestAIWithUser(t,s.aiAssistAPI,"POST","/api/ai/assist",
  aiAssistRequest{Mode:"explain",ConfirmExternal:true,Question:"explain",SessionID:"no-such-session"},"alice")
 if missing.Code!=http.StatusNotFound{t.Fatalf("missing session: %d",missing.Code)}
 foreign:=requestAIWithUser(t,s.aiAssistAPI,"POST","/api/ai/assist",
  aiAssistRequest{Mode:"suggest",ConfirmExternal:true,SessionID:"a"},"bob")
 if foreign.Code!=http.StatusForbidden{t.Fatalf("foreign session: %d",foreign.Code)}
 // Valid session reaches the missing-credential gate, not the external API.
 valid:=requestAIWithUser(t,s.aiAssistAPI,"POST","/api/ai/assist",
  aiAssistRequest{Mode:"suggest",ConfirmExternal:true,SessionID:"session-A"},"alice")
 if valid.Code!=http.StatusServiceUnavailable || !strings.Contains(valid.Body.String(),"key"){
  t.Fatalf("expected missing key without contacting provider: %d %s",valid.Code,valid.Body.String())
 }
}

func TestAIAttestationRejectsWrongKindAndProject(t *testing.T){
 s:=prepareAITargetTest(t)
 st:=s.Sessions.(*aiTestSessions)
 wrongKind:=session.Metadata{ID:"task-1",Kind:"task",Status:"running",OwnerUserID:"alice",ProjectID:"project"}
 st.metadata["task-1"]=wrongKind
 wrongProject:=session.Metadata{ID:"other-project",Kind:"terminal",Status:"running",OwnerUserID:"alice",ProjectID:"project-2"}
 st.metadata["other-project"]=wrongProject
 for _,tt:=range []struct{id string;want int}{{"task-1",http.StatusConflict},{"other-project",http.StatusForbidden}}{
  res:=requestAIWithUser(t,s.aiPlansAPI,"POST","/api/ai/plans",
   aiPlanRequest{Target:aiPlanTarget{SessionID:tt.id},Steps:[]aiPlanStep{{Command:"pwd"}}},"alice")
  if res.Code!=tt.want{t.Errorf("%s expected %d got %d",tt.id,tt.want,res.Code)}
 }
}

func TestAILegacySingleUserTerminalCanBeAttested(t *testing.T){
 s:=prepareAITargetTest(t)
 mock:=s.Sessions.(*aiTestSessions)
 mock.metadata["legacy-local"]=session.Metadata{ID:"legacy-local",Kind:"",TaskID:0,Status:"running",TargetType:"local",Cwd:"/workspace"}
 mock.metadata["legacy-ssh"]=session.Metadata{ID:"legacy-ssh",Kind:"",TaskID:0,Status:"running",TargetType:"ssh",TargetProfileID:"ssh-profile",Cwd:"/remote"}
 for _,tc:=range []struct{id,host string;remote bool}{{"legacy-local","local",false},{"legacy-ssh","ssh-profile",true}} {
  resp:=requestAIWithUser(t,s.aiSessionContextAPI,http.MethodGet,
   "/api/ai/session-context?session_id="+tc.id,nil,"")
  if resp.Code!=http.StatusOK{t.Errorf("legacy %s rejected: %d %s",tc.id,resp.Code,resp.Body.String());continue}
  var result struct{Host string `json:"host"`;Remote bool `json:"remote"`}
  if err:=json.Unmarshal(resp.Body.Bytes(),&result);err!=nil{t.Fatal(err)}
  if result.Host!=tc.host||result.Remote!=tc.remote{t.Fatalf("incorrect legacy target: %+v",result)}
 }
}
func TestAILegacyKindMustNeverAuthorizeNonterminalsOrSharedMode(t *testing.T){
 s:=prepareAITargetTest(t)
 mock:=s.Sessions.(*aiTestSessions)
 mock.metadata["legacy-patch"]=session.Metadata{ID:"legacy-patch",Status:"running",TaskID:-1,Kind:"",TargetType:"local"}
 mock.metadata["legacy-task"]=session.Metadata{ID:"legacy-task",Status:"running",TaskID:1,Kind:"",TargetType:"local"}
 mock.metadata["legacy-unknown"]=session.Metadata{ID:"legacy-unknown",Status:"running",Kind:"",TargetType:"unexpected"}
 for _,id:=range []string{"legacy-patch","legacy-task","legacy-unknown"}{
  resp:=requestAIWithUser(t,s.aiSessionContextAPI,http.MethodGet,
   "/api/ai/session-context?session_id="+id,nil,"")
  if resp.Code!=http.StatusConflict{t.Errorf("nonterminal %s accepted: %d",id,resp.Code)}
 }
 // Shared-server requires explicit kind AND ownership, even for old sessions.
 s.Config.SharedServerEnabled=true
 mock.metadata["legacy-local"]=session.Metadata{ID:"legacy-local",Kind:"",TaskID:0,Status:"running",TargetType:"local",OwnerUserID:"alice",ProjectID:"project"}
 resp:=requestAIWithUser(t,s.aiSessionContextAPI,http.MethodGet,
  "/api/ai/session-context?session_id=legacy-local",nil,"alice")
 if resp.Code==http.StatusOK{t.Fatalf("shared legacy terminal bypassed explicit metadata classification")}
}
