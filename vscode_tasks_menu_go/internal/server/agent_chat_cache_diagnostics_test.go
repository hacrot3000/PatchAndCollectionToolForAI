package server

import (
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

type cacheDiagnosticCapture map[string]string
func(c cacheDiagnosticCapture)RecordProviderDiagnostic(e agentchat.DiagnosticEvent){c[e.Event]=agentchat.SafeProviderText(e.SafeMessage)}
func TestAgentCacheDiagnosticsRemainVisibleAfterRedactionCap(t *testing.T){
 prefix:="reason=history_prefix_changed; instructions_digest="+strings.Repeat("a",64)+"; tools_digest="+strings.Repeat("b",64)+"; common_prefix_segment_bytes=123456; common_input_items=128; previous_attempt_gap=12345.0s; provider_input_tokens=200000; measured_wire_bytes_per_input_token=4.00"
 success:="last_success_reason=append_only; last_success_gap=12345.0s; last_success_common_segment_bytes=123456; last_success_common_items=128; last_success_common_instruction_block_bytes=65536"
 learning:="token_samples=16; learned_input_estimate=112004"
 stats:="scope attempts=100; success=95; read_reported=94; zero_read=3; reported_cache_read_total=5000000; write_reported=0; reported_cache_write_total=unreported; comparable_weighted_hit=88.0%"
 nearZero:="near_zero_samples=94; near_zero_hits=4; near_zero_rate=4.3%"
 note:=prefix+"; "+success+"; "+learning
 if len(note)<=512{t.Fatal("fixture does not exceed redaction cap")}
 c:=cacheDiagnosticCapture{}
 recordAgentCacheObservationDiagnostics(c,note,stats+"; "+nearZero)
 for k,want:=range map[string]string{"cache.prefix":prefix,"cache.last_success_prefix":success,"cache.token_learning":learning,"cache.session_stats":stats,"cache.near_zero":nearZero}{if c[k]!=want{t.Fatalf("%s truncated or missing: %q",k,c[k])}}
}
