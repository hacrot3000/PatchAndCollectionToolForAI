package agentchat

import (
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func TestHandoffRegeneratedEverySessionRevision(t *testing.T){
 store,err:=NewStore(filepath.Join(t.TempDir(),"private","agent"))
 if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/workspace/test","project-uuid","provider","model",Stateless)
 if err!=nil{t.Fatal(err)}
 x.History=append(x.History,HistoryItem{Type:"user",Content:"Build all required modules and verify the results"})
 x,err=store.Save(x,0)
 if err!=nil{t.Fatal(err)}
 x.History=append(x.History,HistoryItem{Type:"function_call",Name:"taskdeck_collect_workspace",CallID:"collect-1",
  Arguments:"{\"actions\":[{\"type\":\"read\",\"path\":\"source/main.go\"}]}"})
 x.History=append(x.History,HistoryItem{Type:"function_call_output",CallID:"collect-1",
  Content:"{\"results\":[{\"files\":[{\"path\":\"source/main.go\",\"content\":\"secret source text\"}]}]}"})
 x.Status="requesting_ai"
 x.AppendEvent("collect_observed","requesting_ai","collect-1","")
 x,err=store.Save(x,x.Revision)
 if err!=nil{t.Fatal(err)}
 handoff,rev,err:=store.Handoff(x.ID,"local")
 if err!=nil{t.Fatal(err)}
 if rev!=x.Revision||!strings.Contains(handoff,"Build all required modules")||
  !strings.Contains(handoff,"source/main.go")||!strings.Contains(handoff,"collect-1")||
  !strings.Contains(handoff,"requesting_ai"){
  t.Fatalf("handoff omitted verified task state:\n%s",handoff)
 }
 if strings.Contains(handoff,"secret source text"){t.Fatal("handoff includes unbounded file contents")}
 recorded,err:=os.ReadFile(store.handoffFile(x.ID))
 if err!=nil||string(recorded)!=handoff{t.Fatalf("materialized checkpoint is stale: %v",err)}
 x.Status="completed"
 x.AppendEvent("turn_finished","completed","","")
 x,err=store.Save(x,x.Revision)
 if err!=nil{t.Fatal(err)}
 updated,rev,err:=store.Handoff(x.ID,"local")
 if err!=nil||rev!=x.Revision||!strings.Contains(updated,"turn_finished")||
  !strings.Contains(updated,"does NOT prove the user's whole plan") {
  t.Fatalf("completed turn should not assert task completion: %v\n%s",err,updated)
 }
 if _,_,err:=store.Handoff(x.ID,"different-owner");err!=ErrNotFound{t.Fatal("handoff leaked to another owner")}
 if err:=store.Delete(x.ID,"local",x.Revision);err!=nil{t.Fatal(err)}
 if _,err:=os.Stat(store.handoffFile(x.ID));!os.IsNotExist(err){t.Fatal("deleted session left handoff on disk")}
}
func TestHandoffInterruptDoesNotAuthorizeDuplicatePatch(t *testing.T){
 x:=Session{ID:"id",ProjectID:"project",Status:"failed",
  History:[]HistoryItem{{Type:"user",Content:"Fix failing code"}},
  ToolCalls:[]ToolCallRecord{{CallID:"patch-1",Status:"done",FeedbackStatus:"unknown",FeedbackSent:true,RunID:"run-1",Result:"PASS"}},
 }
 out:=HandoffMarkdown(x)
 if !strings.Contains(out,"Needs manual/provider reconciliation")||
  !strings.Contains(out,"Never automatically resend ambiguous Patch feedback")||
  !strings.Contains(out,"run-1"){t.Fatalf("unsafe or incomplete recovery instructions: %s",out)}
}
