package server

import (
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestAgentLivePreviewIsOwnerScopedBoundedAndAttemptLocal(t *testing.T) {
 x:=agentchat.Session{ID:"preview-test",Owner:"local",Workspace:"preview-workspace",Status:"requesting_ai"}
 cb,finish:=beginAgentLivePreview(x.Workspace,x.Owner,x.ID,"attempt-1",true,time.Now())
 defer finish()
 _=cb(agentchat.StreamEvent{Type:"response.output_text.delta",TextDelta:"<think>Inspecting"})
 _=cb(agentchat.StreamEvent{Type:"response.reasoning_summary_text.delta",SummaryDelta:"Checking files"})
 _=cb(agentchat.StreamEvent{Type:"response.function_call_arguments.delta",Arguments:"secret tool arguments"})
 p:=agentLivePreviewForSession(x.Workspace,x)
 if p==nil||p.Text!="<think>Inspecting"||p.Summary!="Checking files"{t.Fatalf("unexpected preview: %+v",p)}
 other:=x;other.Owner="different"
 if agentLivePreviewForSession(x.Workspace,other)!=nil||agentLivePreviewForSession("other-workspace",x)!=nil{t.Fatal("preview escaped owner/workspace")}
 _=cb(agentchat.StreamEvent{Type:"response.output_text.delta",TextDelta:strings.Repeat("x",agentLivePreviewLimit)})
 p=agentLivePreviewForSession(x.Workspace,x)
 if !p.Truncated||len(p.Text)!=agentLivePreviewLimit{t.Fatal("preview exceeded bound")}
 next,end:=beginAgentLivePreview(x.Workspace,x.Owner,x.ID,"attempt-2",false,time.Now())
 defer end()
 finish()
 _=cb(agentchat.StreamEvent{Type:"response.output_text.delta",TextDelta:"late"})
 _=next(agentchat.StreamEvent{Type:"response.output_text.delta",TextDelta:"not streaming"})
 p=agentLivePreviewForSession(x.Workspace,x)
 if p==nil||p.RequestID!="attempt-2"||p.Text!=""||p.Streaming{t.Fatal("stale attempt leaked into current preview")}
 end()
 if agentLivePreviewForSession(x.Workspace,x)!=nil{t.Fatal("preview survived completion")}
}
