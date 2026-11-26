package server

import(
 "encoding/json"
 "strings"
 "testing"
 cp "bletonfc/vscode_tasks_menu/internal/agentchat/collectionprogress"
)

func TestAgentCLP20StructuredCollectionAdviceBoundedAndPermissionNeutral(t *testing.T){
 for _,tc:=range []struct{intent cp.Intent;goal string;allow bool;reason cp.Reason;want bool}{
  {cp.Implementation,"Fix PRIVATE_OWNER_GOAL",true,cp.AssessReadiness,true},
  {cp.Implementation,"Fix PRIVATE_OWNER_GOAL",false,cp.BoundNextScope,false},
  {cp.ReadOnly,"Audit PRIVATE_OWNER_GOAL read-only",true,cp.RepeatedEvidence,true},
  {cp.UnknownIntent,"Continue",false,cp.RepeatedEvidence,true},
  {cp.ReadOnly,"Audit PRIVATE_OWNER_GOAL read-only",true,cp.AssessReadiness,false},
 }{
  o,e:=cp.BindObjective(cp.ObjectiveScope{Owner:"local",ProjectID:"p",OwnerWorkspace:"/private",ExecutionWorkspace:"/private",ConversationID:"c",Goal:tc.goal,Generation:1})
  if e!=nil||o.Intent!=tc.intent{t.Fatalf("fixture %+v %v",o,e)}
  c,e:=cp.NewCounterState(o);if e!=nil{t.Fatal(e)}
  c.NativeReads=15;c.DuplicateEvidence=8;c.NewEvidence=7;c.Progress.ReadsSinceProgress=15
  advice:=collectionProgressAdvisory(cp.Decision{Kind:cp.Advise,Reason:tc.reason},c,tc.allow)
  if (advice!="")!=tc.want{t.Fatalf("unexpected advisory: %q",advice)}
  if !tc.want{continue}
  const prefix="Collection advisory metadata (not new evidence or authorization): "
  if strings.Count(advice,prefix)!=1||strings.Contains(advice,"PRIVATE_OWNER_GOAL")||strings.Contains(advice,o.Key)||strings.Contains(advice,"/private")||len(advice)>1200{t.Fatal("unsafe advisory")}
  var got map[string]any
  raw:=strings.TrimSpace(strings.SplitN(advice,prefix,2)[1])
  if e=json.Unmarshal([]byte(raw),&got);e!=nil||got["native_reads"]!=float64(15)||got["verified_duplicate"]!=float64(8)||got["reason"]!=string(tc.reason){t.Fatalf("bad bounded metadata %s",raw)}
  if tc.intent==cp.ReadOnly&&got["next_strategy"]!="continue_read_only_with_distinct_missing_scope"{t.Fatal("read-only advice changed intent")}
 }
}
