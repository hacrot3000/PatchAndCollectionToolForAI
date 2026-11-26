package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestAgentCLP20HistoricalPackSchemaProjection(t *testing.T) {
	old := `{"tool_version":"old","actions":{"grep":{"required":["query"]}}}`
	current := `{"tool_version":"new","actions":{"search":{"required":["query"]}}}`
	goal := "Original user goal\n\nAttachment: literal " + historicalCollectionBegin + " must remain exactly"
	envelope := historicalPackPrefix + "PROJECT-SPECIFIC RULES: retain these\n\n" +
		historicalCollectionBegin + old + historicalCollectionEnd +
		"Other native Patch contract document\n" + historicalPackEnd + goal
	x := agentchat.Session{
		Mode: agentchat.PreviousResponse, PreviousResponseID: "old-provider-response",
		AIPackFingerprint: "canonical-fingerprint",
		History: []agentchat.HistoryItem{{Type:"user", Content:envelope}},
	}
	if !agentHistoricalCollectionPackStale(x,current) || agentHistoricalCollectionPackStale(x,old+"\n") {
		t.Fatal("stale contract detection or whitespace normalization failed")
	}
	if agentContextRequestMode(x,"responses",true,current) != agentchat.Stateless ||
		agentContextRequestMode(x,"responses",true,old) != agentchat.PreviousResponse {
		t.Fatal("stale provider context was not isolated")
	}
	projected := agentProjectHistoricalCollectionPack(x,current)
	if x.History[0].Content != envelope || projected.AIPackFingerprint != x.AIPackFingerprint ||
		projected.PreviousResponseID != x.PreviousResponseID || projected.Mode != x.Mode {
		t.Fatal("canonical state mutated")
	}
	visible := projected.History[0].Content
	if strings.Contains(visible,old) || !strings.Contains(visible,"(stale)") ||
		!strings.Contains(visible,"PROJECT-SPECIFIC RULES: retain these") ||
		!strings.Contains(visible,"Other native Patch contract document") ||
		!strings.HasSuffix(visible,goal) {
		t.Fatal("schema projection removed owner context or retained stale schema")
	}
	for _, bad := range []string{"", "{}", "{broken"} {
		if !reflect.DeepEqual(agentProjectHistoricalCollectionPack(x,bad),x) {
			t.Fatal("unavailable current contract caused historical loss")
		}
	}
	unbound := x; unbound.AIPackFingerprint = ""
	if !reflect.DeepEqual(agentProjectHistoricalCollectionPack(unbound,current),unbound) {
		t.Fatal("unbound plain user input was interpreted as generated pack")
	}
	ambiguous := x; ambiguous.History = append([]agentchat.HistoryItem(nil),x.History...)
	ambiguous.History[0].Content += historicalPackEnd
	if !reflect.DeepEqual(agentProjectHistoricalCollectionPack(ambiguous,current),ambiguous) {
		t.Fatal("ambiguous pack delimiter was interpreted")
	}
	for _, api := range []string{"responses","chat_completions"} {
		body,err := agentBuildContextRequest(x,agentchat.ModelConfig{MaxInputTokens:65536},agentchat.ProviderConfig{APIType:api},agentchat.RequestConfig{
			Model:"m",Mode:agentchat.Stateless,Instructions:"Current policy",EnableCollectionTool:true,NativeCollectionContract:current,
			Next:[]agentchat.HistoryItem{{Type:"user",Content:"Continue original task"}},
		})
		if err != nil { t.Fatal(err) }
		var wire struct { Instructions string; Input []map[string]any }
		if err=json.Unmarshal(body,&wire);err!=nil {t.Fatal(err)}
		if !strings.Contains(wire.Instructions,"search: required=query") || strings.Contains(wire.Instructions,"grep: required=query") {
			t.Fatal("current native schema not authoritative")
		}
		if len(wire.Input)<2 || wire.Input[0]["content"]!=visible || wire.Input[len(wire.Input)-1]["content"]!="Continue original task" {
			t.Fatal("first owner goal or continuation changed in actual provider request")
		}
	}
}

func TestAgentCLP20RealPackSchemaUpgradeAcrossRestart(t *testing.T) {
 s,id:=newAgentConversationFixture(t,"previous_response")
 s.AgentChatPack=nil // exercise the actual installed pack builder and renderer
 root:=t.TempDir()
 docs:=filepath.Join(root,"_patch_lib","docs")
 if err:=os.MkdirAll(docs,0700);err!=nil{t.Fatal(err)}
 entry:=filepath.Join(root,"python_patch_entry.py")
 if err:=os.WriteFile(entry,[]byte("# fixture only\n"),0700);err!=nil{t.Fatal(err)}
 t.Setenv("TASKDECK_PATCH_RUNTIME",entry)
 write:=func(name,content string){
  t.Helper()
  if err:=os.WriteFile(filepath.Join(docs,name),[]byte(content),0600);err!=nil{t.Fatal(err)}
 }
 for _,name:=range []string{"AI_USAGE_CONTRACT.md","CODE_COLLECTION_GUIDE.md","PATCH_PACKAGE_GUIDE.md","PATCH_PACKAGE_SCHEMA.json","PATCH_PACKAGE_CHECKLIST.json"}{write(name,"retain document "+name+"\n")}
 write("AI_STANDARD_PROMPTS.json",`{"version":1,"vi":"Current fixture instructions","en":"Current fixture instructions","ru":"Current fixture instructions"}`)
 old:=`{"tool_version":"old","actions":{"grep":{"required":["query"]}}}`
 current:=`{"tool_version":"new","actions":{"search":{"required":["query"]}}}`
 write("COLLECT_ACTION_SCHEMA.json",old+"\n")
 calls:=0
 s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,payload []byte)(agentchat.ResponsesResult,error){
  calls++
  if calls!=1||!strings.Contains(string(payload),"Original owner goal"){t.Fatal("unexpected first payload")}
  return agentchat.ResponsesResult{ResponseID:"pre-upgrade-response",Text:"Initial response"},nil
 }
 path:="/api/agent/sessions/"+id+"/messages"
 w:=invokeAI(t,s.agentChatSessionItemAPI,"POST",path,map[string]any{"message":"Original owner goal","confirm_external":true,"expected_revision":1})
 if w.Code!=200||calls!=1{t.Fatalf("initial real pack: %d %s",w.Code,w.Body)}
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 before,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if before.PreviousResponseID!="pre-upgrade-response"||before.AIPackFingerprint==""||!agentHistoricalCollectionPackStale(before,current){t.Fatal("real pack layout not detected or stateful reference missing")}
 canonical:=append([]agentchat.HistoryItem(nil),before.History...)
 write("COLLECT_ACTION_SCHEMA.json",current+"\n")
 restarted:=&Server{Workspace:s.Workspace,Config:s.Config,ConnectionSecrets:s.ConnectionSecrets,AgentCollectionCall:s.AgentCollectionCall}
 restarted.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,payload []byte)(agentchat.ResponsesResult,error){
  calls++
  var wire struct{
   Previous string `json:"previous_response_id"`
   Conversation string `json:"conversation"`
   Instructions string `json:"instructions"`
   Input []map[string]any `json:"input"`
  }
  if err:=json.Unmarshal(payload,&wire);err!=nil{t.Fatal(err)}
  if wire.Previous!=""||wire.Conversation!=""||!strings.Contains(wire.Instructions,"search: required=query")||strings.Contains(wire.Instructions,"grep: required=query"){t.Fatal("stale provider context or schema reused")}
  found:=false
  for _,it:=range wire.Input{
   content,_:=it["content"].(string)
   if strings.Contains(content,"Original owner goal"){
    found=true
    if strings.Contains(content,old)||!strings.Contains(content,"(stale)")||!strings.Contains(content,"retain document PATCH_PACKAGE_GUIDE.md"){t.Fatal("incorrect historical projection")}
   }
  }
  if !found{t.Fatal("original goal lost")}
  return agentchat.ResponsesResult{ResponseID:"post-upgrade-response",Text:"Continued same task"},nil
 }
 w=invokeAI(t,restarted.agentChatSessionItemAPI,"POST",path,map[string]any{"message":"Continue","confirm_external":true,"expected_revision":before.Revision,"request_id":"upgrade-continuation"})
 if w.Code!=200||calls!=2{t.Fatalf("upgrade continuation: %d %s calls=%d",w.Code,w.Body,calls)}
 after,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 if after.ID!=before.ID||after.Mode!=before.Mode||after.AIPackFingerprint!=before.AIPackFingerprint||len(after.History)<len(canonical)||!reflect.DeepEqual(after.History[:len(canonical)],canonical){t.Fatal("upgrade rewrote conversation, mode or canonical pack")}
 if after.PreviousResponseID!=""||after.ConversationID!=""{t.Fatal("stale stateful reference persisted after rebase")}
}
