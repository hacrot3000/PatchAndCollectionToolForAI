package server

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"bletonfc/vscode_tasks_menu/internal/agentchat"
)

// Seed the actual old quota implementation, not a fake HTTP 429.
func exhaustLegacyAgentQuota(t *testing.T, s *Server) {
	t.Helper()
	key := aiWorkspaceHash(s.Workspace)+":local"
	for i:=0;i<20;i++ {
		release,ok:=s.reserveAIRequest(key,20)
		if !ok {t.Fatalf("legacy quota seed failed at %d",i)}
		release()
	}
	if release,ok:=s.reserveAIRequest(key,20);ok {
		release()
		t.Fatal("legacy quota not exhausted")
	}
}

func TestAgentLocalMessagesExceedTwentyAdmissionsPerHour(t *testing.T) {
	s,id:=newAgentConversationFixture(t,"stateless")
	store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
	calls:=0
	s.AgentChatCall=func(_ context.Context,_ agentchat.ProviderConfig,_ string,_ []byte)(agentchat.ResponsesResult,error){
		calls++
		return agentchat.ResponsesResult{ResponseID:fmt.Sprintf("quota-turn-%d",calls),Text:"Answer"},nil
	}
	for i:=0;i<25;i++ {
		x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
		w:=invokeAI(t,s.agentChatSessionItemAPI,http.MethodPost,"/api/agent/sessions/"+id+"/messages",map[string]any{
			"message":fmt.Sprintf("Question %d",i),"confirm_external":true,"expected_revision":x.Revision,
			"request_id":fmt.Sprintf("quota-request-%d",i),
		})
		if w.Code!=200{t.Fatalf("turn %d rejected: %d %s",i,w.Code,w.Body)}
	}
	if calls!=25{t.Fatalf("expected 25 real provider admissions, got %d",calls)}
	// Local Agent turns do not consume the separate Smart Terminal budget.
	exhaustLegacyAgentQuota(t,s)
}

func TestAgentChatQuotaRetainsSharedAndNonlocalProtection(t *testing.T) {
	s:=newAIServerForTest(t)
	exhaustLegacyAgentQuota(t,s)
	release,ok:=s.reserveAgentChatQuota(s.Workspace,"local")
	if !ok{t.Fatal("local agent still rate limited")}
	defer release()
	// The old owner-wide latch must not serialize independent local Agent turns.
	second,ok:=s.reserveAgentChatQuota(s.Workspace,"local")
	if !ok{t.Fatal("second local reservation refused")}
	second()
	s.Config.SharedServerEnabled=true
	if release,ok:=s.reserveAgentChatQuota(s.Workspace,"local");ok{
		release()
		t.Fatal("shared server bypassed old quota")
	}
	s.Config.SharedServerEnabled=false
	owner:="user:other"
	for i:=0;i<agentSharedRequestsPerHour;i++{
		r,ok:=s.reserveAgentChatQuota(s.Workspace,owner)
		if !ok{t.Fatalf("nonlocal quota exhausted too early at %d",i)}
		r()
	}
	if r,ok:=s.reserveAgentChatQuota(s.Workspace,owner);ok{
		r()
		t.Fatal("authenticated nonlocal owner bypassed quota")
	}
}
