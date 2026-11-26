package server

import (
 "time"
 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

// Measure the serialized request, not the mutable conversation selection.
// RequestAt is the local admission/start timestamp, not provider completion.
func agentRequestDisplay(p agentchat.ProviderConfig,payload []byte,at time.Time) agentchat.RequestDisplay{
 meta:=agentRequestDiagnosticMetadata(payload)
 model:=modelFromAgentPayload(payload)
 if model==""||p.ID==""{return agentchat.RequestDisplay{}}
 effort:=meta.Reasoning
 if effort==""{effort="default"}
 mode:=meta.ContextMode
 if p.APIType=="chat_completions"{mode="stateless"}
 d:=agentchat.RequestDisplay{ProviderID:p.ID,ModelID:model,RequestEffort:effort,RequestContext:mode,RequestAt:at.UTC().Format(time.RFC3339Nano)}
 if measurement,e:=agentchat.MeasureProviderPayload(p.APIType,payload);e==nil{d.RequestInputTokens=measurement.EstimatedInputTokens}
 return d
}
