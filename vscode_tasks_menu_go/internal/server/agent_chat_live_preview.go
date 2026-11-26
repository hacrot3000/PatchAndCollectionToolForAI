package server

import (
 "sync"
 "time"
 "unicode/utf8"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

// Display-only process memory. Never write a partial response into the journal,
// provider feedback, native tool admission or diagnostic export.
const agentLivePreviewLimit = 64 << 10

type agentLivePreview struct {
 RequestID string `json:"request_id"`
 Streaming bool `json:"streaming"`
 StartedAt time.Time `json:"started_at"`
 Text string `json:"text,omitempty"`
 Summary string `json:"summary,omitempty"`
 Truncated bool `json:"truncated,omitempty"`
}

var agentLivePreviewMu sync.Mutex
var agentLivePreviews = map[string]*agentLivePreview{}

func beginAgentLivePreview(workspace, owner, id, requestID string, streaming bool, startedAt time.Time) (func(agentchat.StreamEvent) error, func()) {
 key := agentRetryKey(workspace,owner,id)
 p := &agentLivePreview{RequestID:requestID,Streaming:streaming,StartedAt:startedAt}
 agentLivePreviewMu.Lock()
 agentLivePreviews[key]=p
 agentLivePreviewMu.Unlock()
 return func(e agentchat.StreamEvent) error {
  agentLivePreviewMu.Lock()
  defer agentLivePreviewMu.Unlock()
  if agentLivePreviews[key]!=p || !p.Streaming {return nil}
  var dst *string
  var delta string
  switch e.Type {
  case "response.output_text.delta","assistant.delta":
   dst,delta=&p.Text,e.TextDelta
  case "response.reasoning_summary_text.delta":
   dst,delta=&p.Summary,e.SummaryDelta
  default:
   return nil
  }
  remaining:=agentLivePreviewLimit-len(*dst)
  if len(delta)>remaining {
   delta=delta[:remaining]
   for !utf8.ValidString(delta) {delta=delta[:len(delta)-1]}
   p.Truncated=true
  }
  *dst+=delta
  return nil
 },func(){
  agentLivePreviewMu.Lock()
  defer agentLivePreviewMu.Unlock()
  if agentLivePreviews[key]==p {delete(agentLivePreviews,key)}
 }
}

func agentLivePreviewForSession(workspace string, x agentchat.Session) *agentLivePreview {
 if x.Workspace!=workspace || x.Status!="requesting_ai" || x.Archived {return nil}
 agentLivePreviewMu.Lock()
 defer agentLivePreviewMu.Unlock()
 p:=agentLivePreviews[agentRetryKey(workspace,x.Owner,x.ID)]
 if p==nil {return nil}
 copy:=*p
 return &copy
}
