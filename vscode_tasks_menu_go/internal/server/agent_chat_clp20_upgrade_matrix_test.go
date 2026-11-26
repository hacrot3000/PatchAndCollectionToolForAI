package server

import (
 "context"
 "encoding/json"
 "fmt"
 "os"
 "path/filepath"
 "reflect"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/patchtool"
)

func TestAgentCLP20ArchivedLegacyPackUnknownResumeAcrossUpgrade(t *testing.T){
 s,id:=newAgentConversationFixture(t,"previous_response")
 root:=t.TempDir()
 docs:=filepath.Join(root,"_patch_lib","docs")
 if err:=os.MkdirAll(docs,0700);err!=nil{t.Fatal(err)}
 entry:=filepath.Join(root,"python_patch_entry.py")
 if err:=os.WriteFile(entry,[]byte("# fixture only\n"),0700);err!=nil{t.Fatal(err)}
 t.Setenv("TASKDECK_PATCH_RUNTIME",entry)
 old:=`{"tool_version":"old","actions":{"grep":{"required":["query"]}}}`
 current:=`{"tool_version":"new","actions":{"search":{"required":["query"]}}}`
 if err:=os.WriteFile(filepath.Join(docs,"COLLECT_ACTION_SCHEMA.json"),[]byte(current),0600);err!=nil{t.Fatal(err)}
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 pack:=historicalPackPrefix+"Retain workspace policy\n"+historicalCollectionBegin+old+historicalCollectionEnd+"Other native Patch contract\n"+historicalPackEnd+"Original owner objective"
 x.History=[]agentchat.HistoryItem{{Type:"user",Content:pack}}
 x.AIPackFingerprint=agentchat.BodyDigest([]byte(pack))
 x.PreviousResponseID="old-response"
 x.TaskPlan,err=agentchat.NewTaskPlan("Original owner objective",[]agentchat.TaskPlanStep{{ID:"inspect",Title:"Inspect",Acceptance:"Verified native evidence"},{ID:"finish",Title:"Finish",Acceptance:"Human acceptance"}})
 if err!=nil{t.Fatal(err)}
 x.AutoPatch=false;x.AutoFeedback=true;x.AutoFeedbackPersistent=true;x.AutoContinueText=false;x.RetryMode=agentchat.RetryLimited
 for i:=0;i<130;i++{
  callID:=fmt.Sprintf("historic-%d",i)
  x.History=append(x.History,agentchat.HistoryItem{Type:"function_call",CallID:callID,Name:"taskdeck_collect_workspace",Arguments:`{"actions":[{"type":"grep","query":"old"}]}`},agentchat.HistoryItem{Type:"function_call_output",CallID:callID,Content:fmt.Sprintf("historical observation %d",i)})
 }
 patch:=agentchat.ToolCallRecord{CallID:"old-patch",Name:"taskdeck_submit_patch",Status:"done",RunID:"native-old",Result:`{"status":"PASS","run_id":"native-old"}`,FeedbackSent:true,FeedbackStatus:"acknowledged"}
 x.History=append(x.History,agentchat.HistoryItem{Type:"function_call",CallID:patch.CallID,Name:patch.Name,Arguments:`{}`},agentchat.HistoryItem{Type:"function_call_output",CallID:patch.CallID,Content:patch.Result})
 x.ToolCalls=[]agentchat.ToolCallRecord{patch}
 receipt:=`{"status":"PASS","run_id":"native-read","text":"`+strings.Repeat("exact current evidence ",3000)+`"}`
 x.History=append(x.History,agentchat.HistoryItem{Type:"function_call",CallID:"read",Name:"taskdeck_collect_workspace",Arguments:`{"actions":[{"type":"file","path":"a"}]}`},agentchat.HistoryItem{Type:"function_call_output",CallID:"read",Content:receipt})
 now:=time.Now().UTC()
 x.ProviderRun=&agentchat.ProviderRun{ID:"old-unknown",Kind:"collection",Phase:"unknown",CallID:"read",PayloadSHA256:agentchat.ProviderPayloadDigest([]byte("old ambiguous bytes")),ProviderFingerprint:"old-provider",CreatedAt:now,UpdatedAt:now}
 x.Status="failed"
 x,err=store.Save(x,x.Revision);if err!=nil{t.Fatal(err)}
 if x.HistoryArchiveParts==0||!agentHistoricalCollectionPackStale(x,current){t.Fatal("upgrade fixture did not contain archived stale context")}
 archives:=make(map[uint64][]agentchat.HistoryItem)
 for i:=uint64(1);i<=x.HistoryArchiveParts;i++{rows,e:=store.ArchivedCollectionHistory(id,"local",i);if e!=nil{t.Fatal(e)};archives[i]=rows}
 calls:=0
 restarted:=&Server{Workspace:s.Workspace,Config:s.Config,ConnectionSecrets:s.ConnectionSecrets,AgentChatPack:s.AgentChatPack}
 restarted.AgentCollectionCall=func(string,patchtool.ProjectIdentity,[]byte)string{t.Fatal("historical native read replayed");return ""}
 restarted.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,payload []byte)(agentchat.ResponsesResult,error){
  calls++
  var wire struct{Instructions string;Previous string `json:"previous_response_id"`;Conversation string `json:"conversation"`;Input []map[string]any}
  if err:=json.Unmarshal(payload,&wire);err!=nil{t.Fatal(err)}
  if wire.Previous!=""||wire.Conversation!=""||!strings.Contains(wire.Instructions,"search: required=query")||strings.Contains(wire.Instructions,"grep: required=query"){t.Fatal("stale schema or provider lineage reused")}
  goal,count:=false,0
  for _,it:=range wire.Input{
   if c,_:=it["content"].(string);strings.Contains(c,"Original owner objective"){goal=true;if strings.Contains(c,old){t.Fatal("old schema still projected")}}
   if it["type"]=="function_call_output"&&it["call_id"]=="read"{count++;if it["output"]!=receipt{t.Fatal("receipt changed")}}
  }
  if !goal||count!=1||strings.Contains(string(payload),"old ambiguous bytes"){t.Fatal("goal/receipt missing or old payload replayed")}
  return agentchat.ResponsesResult{Text:"New continuation inspected the existing receipt.",ResponseID:"new-response"},nil
 }
 reopened,err:=restarted.agentChatStore();if err!=nil{t.Fatal(err)}
 restarted.agentReconcilePendingOnStartup(context.Background())
 restarted.processAgentAutoFeedbackSession(context.Background(),reopened,id)
 if calls!=0{t.Fatal("startup replayed UNKNOWN without grant")}
 path:="/api/agent/sessions/"+id+"/messages"
 req:=map[string]any{"message":"Resume original task","confirm_external":true,"expected_revision":x.Revision,"request_id":"upgrade-resume","resume_collection_run_id":"old-unknown"}
 w:=invokeAI(t,restarted.agentChatSessionItemAPI,"POST",path,req)
 if w.Code!=200||calls!=1{t.Fatalf("upgrade resume HTTP %d calls %d %s",w.Code,calls,w.Body)}
 got,err:=reopened.Get(id,"local");if err!=nil{t.Fatal(err)}
 if got.ID!=x.ID||got.Mode!=x.Mode||got.AIPackFingerprint!=x.AIPackFingerprint||got.PreviousResponseID!=""||got.ConversationID!=""||got.RetryMode!=x.RetryMode||got.AutoFeedback!=x.AutoFeedback||got.AutoFeedbackPersistent!=x.AutoFeedbackPersistent||got.AutoPatch!=x.AutoPatch{t.Fatal("upgrade changed owner settings or retained stale references")}
 if got.TaskPlan==nil||got.TaskPlan.Goal!=x.TaskPlan.Goal||!reflect.DeepEqual(got.TaskPlan.Steps,x.TaskPlan.Steps)||got.TaskPlan.AutoContinue!=x.TaskPlan.AutoContinue{t.Fatal("upgrade altered task plan")}
 if !reflect.DeepEqual(got.ToolCalls,x.ToolCalls)||got.History[0]!=x.History[0]{t.Fatal("old Patch ACK or pack changed")}
 count:=0
 for _,it:=range got.History{if it.Type=="function_call_output"&&it.CallID=="read"{count++;if it.Content!=receipt{t.Fatal("canonical receipt changed")}}}
 if count!=1||len(got.CollectionResumeHistory)!=1||got.CollectionResumeHistory[0].Previous.Phase!="unknown"||got.CollectionResumeHistory[0].ReceiptSHA256!=agentchat.ProviderPayloadDigest([]byte(receipt)){t.Fatal("old UNKNOWN audit or receipt lost")}
 for part,want:=range archives{rows,e:=reopened.ArchivedCollectionHistory(id,"local",part);if e!=nil||!reflect.DeepEqual(rows,want){t.Fatal("immutable archive changed")}}
 restarted.agentReconcilePendingOnStartup(context.Background())
 w=invokeAI(t,restarted.agentChatSessionItemAPI,"POST",path,req)
 if w.Code!=200||calls!=1{t.Fatal("duplicate resume after restart caused provider egress")}
}
