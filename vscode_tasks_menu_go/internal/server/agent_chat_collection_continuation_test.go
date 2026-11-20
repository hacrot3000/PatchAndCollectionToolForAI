package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
	"bletonfc/vscode_tasks_menu/internal/patchtool"
)

func TestAgentCollectionReadPHPThenProposeVietnamesePatch(t *testing.T) {
	s, id := newAgentConversationFixture(t, "stateless")
	original := "<?php echo 'Hello World';\n"
	if err := os.WriteFile(filepath.Join(s.Workspace, "helloworld.php"), []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	identity, err := patchtool.ReadProjectIdentity(s.Workspace)
	if err != nil { t.Fatal(err) }
	py := fmt.Sprintf("# TASKDECK_PROJECT_KEY: %s\n# TASKDECK_PROJECT_ID: %s\nfrom pathlib import Path\np=Path('helloworld.php')\np.write_text(p.read_text().replace('Hello World', 'Xin chào thế giới'))\n", identity.Key, identity.ID)
	rawPatch, err := json.Marshal(map[string]string{"name":"Translate helloworld.php greeting to Vietnamese", "patch_python":py})
	if err != nil { t.Fatal(err) }
	deadlines := make([]time.Time, 0, 2)
	round := 0
	s.AgentChatCall = func(ctx context.Context, _ agentchat.ProviderConfig, _ string, raw []byte) (agentchat.ResponsesResult, error) {
		round++
		deadline, ok := ctx.Deadline()
		if !ok { t.Fatal("provider request has no per-call timeout") }
		deadlines = append(deadlines, deadline)
		if round == 1 {
			time.Sleep(35*time.Millisecond)
			return agentchat.ResponsesResult{ResponseID:"inspect-php", Calls:[]agentchat.FunctionCall{{
				CallID:"read-php", Name:"taskdeck_collect_workspace",
				Arguments:`{"actions":[{"type":"read","path":"helloworld.php"}]}`,
			}}}, nil
		}
		if round != 2 { t.Fatalf("unexpected provider call: %d", round) }
		var payload struct {
			Input []map[string]any `json:"input"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil { t.Fatal(err) }
		found := false
		for _, item := range payload.Input {
			if item["type"]=="function_call_output" && item["call_id"]=="read-php" {
				// The JSON encoder may escape PHP's leading '<' as \\u003c.
				// Compare the decoded file contents, not the raw JSON string.
				var observed struct {
					Results []struct {
						Files []struct{ Content string `json:"content"`; Path string `json:"path"` } `json:"files"`
					} `json:"results"`
				}
				if err:=json.Unmarshal([]byte(fmt.Sprint(item["output"])),&observed);err!=nil { t.Fatal(err) }
				for _, result:=range observed.Results {
					for _, file:=range result.Files {
						if file.Path=="helloworld.php" && file.Content==original { found=true }
					}
				}
			}
		}
		if !found { t.Fatalf("Collection read result missing from provider continuation: %+v", payload.Input) }
		return agentchat.ResponsesResult{ResponseID:"propose-vi", Calls:[]agentchat.FunctionCall{{
			CallID:"patch-vi", Name:"taskdeck_submit_patch", Arguments:string(rawPatch),
		}}}, nil
	}
	w := invokeAI(t,s.agentChatSessionItemAPI,"POST","/api/agent/sessions/"+id+"/messages",
		map[string]any{"message":"trong file helloworld.php, hãy đổi câu chào sang tiếng Việt","confirm_external":true,"expected_revision":1})
	if w.Code!=200 || round!=2 || !strings.Contains(w.Body.String(),"waiting_tool") {
		t.Fatalf("edit failed after Collection: HTTP %d, %d calls: %s",w.Code,round,w.Body)
	}
	if !deadlines[1].After(deadlines[0]) {
		t.Fatal("Collection provider call reused the initial request's deadline")
	}
	content,err:=os.ReadFile(filepath.Join(s.Workspace,"helloworld.php"))
	if err!=nil || string(content)!=original { t.Fatal("unapproved Patch modified the PHP file") }
	store,err:=s.agentChatStore()
	if err!=nil { t.Fatal(err) }
	saved,err:=store.Get(id,"local")
	if err!=nil { t.Fatal(err) }
	if len(saved.ToolCalls)!=1 || saved.ToolCalls[0].Status!="pending" || saved.ToolCalls[0].Name!="taskdeck_submit_patch" {
		t.Fatalf("no native Patch proposal after Collection: %+v",saved.ToolCalls)
	}
}

func TestAgentCollectionLegacyTextFallbackStaysStatelessAcrossRounds(t *testing.T) {
	s, id := newAgentConversationFixture(t, "previous_response")
	identity, err := patchtool.ReadProjectIdentity(s.Workspace)
	if err!=nil { t.Fatal(err) }
	request, err := json.Marshal(agentCollectionArguments{Project: identity, Actions: []agentCollectionAction{{Type:"ls",Path:"."}}})
	if err!=nil { t.Fatal(err) }
	calls := 0
	s.AgentChatCall = func(_ context.Context, _ agentchat.ProviderConfig, _ string, raw []byte) (agentchat.ResponsesResult,error) {
		calls++
		var payload map[string]any
		if err:=json.Unmarshal(raw,&payload);err!=nil {t.Fatal(err)}
		if calls>1 {
			if _,ok:=payload["previous_response_id"];ok {
				t.Fatalf("stateless fallback reused an unstored previous_response_id on round %d",calls)
			}
			if len(payload["input"].([]any))<2 {t.Fatal("full history missing after text-only Collection fallback")}
		}
		if calls<=2 {return agentchat.ResponsesResult{ResponseID:fmt.Sprintf("raw-%d",calls),Text:string(request)},nil}
		return agentchat.ResponsesResult{ResponseID:"done",Text:"Inspected project root"},nil
	}
	w:=invokeAI(t,s.agentChatSessionItemAPI,"POST","/api/agent/sessions/"+id+"/messages",
		map[string]any{"message":"inspect project root","confirm_external":true,"expected_revision":1})
	if w.Code!=200 || calls!=3 { t.Fatalf("stateful fallback failed: %d %s; calls=%d",w.Code,w.Body,calls) }
	store,err:=s.agentChatStore()
	if err!=nil { t.Fatal(err) }
	saved,err:=store.Get(id,"local")
	if err!=nil {t.Fatal(err)}
	if saved.PreviousResponseID!="" {
		t.Fatalf("unstored previous_response_id leaked into next turn: %q",saved.PreviousResponseID)
	}
}

func TestAgentCollectionProviderFailurePreservesObservationAndTrace(t *testing.T) {
	s,id:=newAgentConversationFixture(t,"stateless")
	count:=0
	s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,_ []byte)(agentchat.ResponsesResult,error){
		count++
		if count==1 {return agentchat.ResponsesResult{ResponseID:"read-1",Calls:[]agentchat.FunctionCall{{
			CallID:"collect-1",Name:"taskdeck_collect_workspace",
			Arguments:`{"actions":[{"type":"ls","path":"."}]}`,
		}}},nil}
		return agentchat.ResponsesResult{},&agentchat.ProviderError{
			Code:"PROVIDER_HTTP_400",Stage:"provider_http",Message:"Provider rejected continuation format",
			HTTPStatus:400,Ambiguous:false,
		}
	}
	w:=invokeAI(t,s.agentChatSessionItemAPI,"POST","/api/agent/sessions/"+id+"/messages",
		map[string]any{"message":"inspect project root","confirm_external":true,"expected_revision":1})
	if w.Code!=502 || count!=2 || !strings.Contains(w.Body.String(),"PROVIDER_HTTP_400") ||
		!strings.Contains(w.Body.String(),"provider_http") || strings.Contains(w.Body.String(),`"ambiguous":true`) {
		t.Fatalf("lost precise provider failure: %d %s",w.Code,w.Body)
	}
	store,err:=s.agentChatStore()
	if err!=nil {t.Fatal(err)}
	saved,err:=store.Get(id,"local")
	if err!=nil {t.Fatal(err)}
	if saved.Status!="failed" || len(saved.ToolCalls)!=0 {
		t.Fatalf("Collection failure unexpectedly submitted a Patch: %+v",saved)
	}
	found:=false
	for _,item:=range saved.History {
		if item.Type=="function_call_output" && item.CallID=="collect-1" {found=true}
	}
	if !found {t.Fatal("verified Collection observation was lost after provider HTTP failure")}
}

func TestAgentCollectionMalformedReadRequestIsRepairable(t *testing.T) {
	s,id:=newAgentConversationFixture(t,"stateless")
	if err:=os.WriteFile(filepath.Join(s.Workspace,"helloworld.php"),[]byte("<?php echo 'Hello World';\n"),0600);err!=nil {t.Fatal(err)}
	calls:=0
	s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,body []byte)(agentchat.ResponsesResult,error) {
		calls++
		switch calls {
		case 1:
			return agentchat.ResponsesResult{ResponseID:"bad-read",Calls:[]agentchat.FunctionCall{{
				CallID:"bad-1",Name:"taskdeck_collect_workspace",Arguments:`{"actions":[{"type":"write","path":"helloworld.php"}]}`,
			}}},nil
		case 2:
			var payload struct{ Input []map[string]any `json:"input"` }
			if err:=json.Unmarshal(body,&payload);err!=nil {t.Fatal(err)}
			found:=false
			for _,item:=range payload.Input {
				if item["type"]=="function_call_output" && item["call_id"]=="bad-1" &&
					strings.Contains(fmt.Sprint(item["output"]),"INVALID_COLLECTION_REQUEST") {found=true}
			}
			if !found {t.Fatal("malformed read was not returned as a repairable tool error")}
			return agentchat.ResponsesResult{ResponseID:"fixed-read",Calls:[]agentchat.FunctionCall{{
				CallID:"read-2",Name:"taskdeck_collect_workspace",Arguments:`{"actions":[{"type":"read","path":"helloworld.php"}]}`,
			}}},nil
		case 3:
			return agentchat.ResponsesResult{ResponseID:"answer-read",Text:"Read the PHP greeting; ready to propose a patch."},nil
		}
		t.Fatalf("unexpected Collection round %d",calls)
		return agentchat.ResponsesResult{},nil
	}
	w:=invokeAI(t,s.agentChatSessionItemAPI,"POST","/api/agent/sessions/"+id+"/messages",
		map[string]any{"message":"Read helloworld.php and translate its greeting","confirm_external":true,"expected_revision":1})
	if w.Code!=200 || calls!=3 {t.Fatalf("malformed request terminated the turn: %d %s",w.Code,w.Body)}
	if _,err:=os.Stat(filepath.Join(s.Workspace,"helloworld.php"));err!=nil {t.Fatal("read-only Collection damaged project file")}
}

func TestAgentCollectionLocalFailuresHaveConcreteErrorStage(t *testing.T) {
	err:=&agentCollectionLocalError{Stage:"request_build",Round:2,Cause:fmt.Errorf("too many items")}
	data:=debugErrorJSON(err,"trace-correct","COLLECTION_CONTINUATION_FAILED","tool_execution","generic")
	body,_:=json.Marshal(data)
	for _,value:=range []string{"COLLECTION_REQUEST_BUILD_FAILED","request_build",`"round":2`,`"ambiguous":false`} {
		if !strings.Contains(string(body),value) {t.Fatalf("missing specific diagnostic %q: %s",value,body)}
	}
}

func TestAgentCollectionContinuesBeyondSixReadsToPatchProposal(t *testing.T){
	s,id:=newAgentConversationFixture(t,"stateless")
	identity,err:=patchtool.ReadProjectIdentity(s.Workspace);if err!=nil{t.Fatal(err)}
	original:="<?php echo 'Initial';\n"
	if err:=os.WriteFile(filepath.Join(s.Workspace,"main.php"),[]byte(original),0600);err!=nil{t.Fatal(err)}
	patchCode:=fmt.Sprintf("# TASKDECK_PROJECT_KEY: %s\n# TASKDECK_PROJECT_ID: %s\nprint('noop')\n",identity.Key,identity.ID)
	raw,_:=json.Marshal(map[string]string{"name":"After paged research","patch_python":patchCode})
	count:=0
	s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,payload []byte)(agentchat.ResponsesResult,error){
		count++
		if count<=9 {
			if count>1 {
				var req struct{Input []map[string]any `json:"input"`}
				if err:=json.Unmarshal(payload,&req);err!=nil{t.Fatal(err)}
				var found bool
				for _,row:=range req.Input{
					if row["type"]=="function_call_output"&&row["call_id"]==fmt.Sprintf("read-%d",count-1){
						found=true
					}
				}
				if !found{t.Fatalf("Collection #%d was not persisted into provider input",count-1)}
			}
			return agentchat.ResponsesResult{ResponseID:fmt.Sprintf("inspect-%d",count),Calls:[]agentchat.FunctionCall{{
				CallID:fmt.Sprintf("read-%d",count),Name:"taskdeck_collect_workspace",
				Arguments:fmt.Sprintf(`{"actions":[{"type":"read","path":"main.php","start_line":%d,"max_lines":1}]}`,count),
			}}},nil
		}
		if count==10{
			return agentchat.ResponsesResult{ResponseID:"patch-after-inspection",Calls:[]agentchat.FunctionCall{{
				CallID:"patch-10",Name:"taskdeck_submit_patch",Arguments:string(raw),
			}}},nil
		}
		t.Fatalf("unexpected extra provider call %d",count)
		return agentchat.ResponsesResult{},nil
	}
	w:=invokeAI(t,s.agentChatSessionItemAPI,"POST","/api/agent/sessions/"+id+"/messages",
		map[string]any{"message":"Inspect enough source pages and propose one patch","confirm_external":true,"expected_revision":1})
	if w.Code!=200||count!=10||!strings.Contains(w.Body.String(),"waiting_tool"){
		t.Fatalf("agent stopped before a Patch after six Collection turns: status %d calls %d: %s",w.Code,count,w.Body)
	}
	store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
	saved,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
	observations:=0
	for _,row:=range saved.History{if row.Type=="function_call_output"{observations++}}
	if observations!=9 || len(saved.ToolCalls)!=1 || saved.ToolCalls[0].Status!="pending"{
		t.Fatalf("lost persistent read history or unapproved Patch after 9 Collection rounds: %+v",saved)
	}
	value,err:=os.ReadFile(filepath.Join(s.Workspace,"main.php"))
	if err!=nil||string(value)!=original{t.Fatal("Collection bypassed approval and modified project")}
}

func TestAgentCollectionStopsIdenticalNoProgressReads(t *testing.T){
	s,id:=newAgentConversationFixture(t,"stateless")
	calls:=0
	s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,_ []byte)(agentchat.ResponsesResult,error){
		calls++
		if calls>5 {t.Fatalf("no-progress loop was not fenced: %d",calls)}
		return agentchat.ResponsesResult{ResponseID:fmt.Sprintf("read-loop-%d",calls),
			Calls:[]agentchat.FunctionCall{{CallID:fmt.Sprintf("repeat-%d",calls),
				Name:"taskdeck_collect_workspace",Arguments:`{"actions":[{"type":"ls","path":"."}]}`}}},nil
	}
	w:=invokeAI(t,s.agentChatSessionItemAPI,"POST","/api/agent/sessions/"+id+"/messages",
		map[string]any{"message":"inspect the project and make progress","confirm_external":true,"expected_revision":1})
	if w.Code!=200 || calls!=3 || !strings.Contains(w.Body.String(),"not") && !strings.Contains(w.Body.String(),"non-progressing"){
		t.Fatalf("no-progress loop did not produce a truthful checkpoint: HTTP %d calls=%d body=%s",w.Code,calls,w.Body)
	}
	store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
	saved,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
	if len(saved.ToolCalls)!=0 {t.Fatal("no-progress loop executed a Patch")}
}
