package server

import (
	"encoding/json"
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
