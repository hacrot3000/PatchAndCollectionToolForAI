package server

import (
 "encoding/json"
 "strings"
 "testing"
 "bletonfc/vscode_tasks_menu/internal/agentrun"
 cp "bletonfc/vscode_tasks_menu/internal/agentchat/collectionprogress"
)
func TestAgentCLP20UnifiedRetryDebugRedactsUnknownFields(t *testing.T){
 e:=agentrun.Event{Sequence:9,AttemptID:"attempt-1",Payload:[]byte(`{"attempt":2,"outcome":"header_timeout","retry_mode":"unlimited","payload_sha256":"`+strings.Repeat("a",64)+`","canonical_bytes":12345,"error_code":"PRIVATE_PROVIDER_TEXT","next_payload_sha256":"PRIVATE_DIGEST","nested":{"prompt":"PRIVATE_PROMPT"},"same_digest_failures":3}`)}
 row:=agentSafeUnifiedRetry(e)
 raw,err:=json.Marshal(row)
 if err!=nil||strings.Contains(string(raw),"PRIVATE")||!strings.Contains(string(raw),"header_timeout")||!strings.Contains(string(raw),"12345")||!strings.Contains(string(raw),strings.Repeat("a",64)){t.Fatal("retry metadata not safely projected")}
 e.Payload=[]byte(strings.Repeat("x",17<<10))
 if agentSafeUnifiedRetry(e)!=nil{t.Fatal("oversized event accepted")}
}


func TestAgentCLP20UnifiedProgressDebugRejectsUnknownDecisionStrings(t *testing.T){
 g:=runCollectionProgress{Decision:cp.Decision{Kind:"PRIVATE_DECISION",Reason:"PRIVATE_REASON"},LastHistorySize:9}
 g.Counters.NativeReads=4
 raw,err:=json.Marshal(agentSafeUnifiedProgress(g))
 if err!=nil||strings.Contains(string(raw),"PRIVATE")||!strings.Contains(string(raw),`"native_reads":4`)||!strings.Contains(string(raw),`"decision":"unavailable"`){t.Fatal("unknown decision was exported or counters lost")}
 g.Decision=cp.Decision{Kind:cp.Advise,Reason:cp.RepeatedEvidence}
 raw,err=json.Marshal(agentSafeUnifiedProgress(g))
 if err!=nil||!strings.Contains(string(raw),"repeated_verified_evidence"){t.Fatal("known decision reason lost")}
}
