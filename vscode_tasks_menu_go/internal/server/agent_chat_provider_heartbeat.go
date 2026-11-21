package server

import (
 "context"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

// Production interval is intentionally small compared with multi-day runs
// without causing full session revision writes or excessive disk syncs.
var agentProviderHeartbeatInterval=30*time.Second

// The sidecar is only diagnostic. In particular, missing/stale heartbeats
// must never be interpreted as proof that a provider did not receive bytes.
// The main session turn lock and persisted egress phase own execution safety.
func startAgentProviderHeartbeat(ctx context.Context,store *agentchat.Store,owner,id string)func(){
 if ctx==nil||store==nil{return func(){}}
 stop:=make(chan struct{})
 done:=make(chan struct{})
 go func(){
  defer close(done)
  _=store.TouchActiveProviderHeartbeat(id,owner)
  ticker:=time.NewTicker(agentProviderHeartbeatInterval)
  defer ticker.Stop()
  for{
   select{
   case <-stop:return
   case <-ctx.Done():return
   case <-ticker.C:
    _=store.TouchActiveProviderHeartbeat(id,owner)
   }
  }
 }()
 return func(){close(stop);<-done}
}
