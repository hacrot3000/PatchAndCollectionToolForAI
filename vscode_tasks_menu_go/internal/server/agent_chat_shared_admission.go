package server

import (
 "context"
 "sync"
 "time"
)

// acquireAgentProviderSlot preserves the existing per-daemon bounded, fair
// admission queue and adds a cross-daemon capacity lease before provider I/O.
// Both leases stay held until the operation ends. This is NOT a network
// filesystem/distributed broker, and cannot acknowledge/replay a ProviderRun.
func acquireAgentProviderSlot(ctx context.Context, owner string)(func(),bool){
 if ctx==nil{ctx=context.Background()}
 if owner==""||ctx.Err()!=nil{return nil,false}
 timeout:=agentQueueTimeout
 if timeout<=0{return nil,false}
 waitCtx,cancel:=context.WithTimeout(ctx,timeout)
 localRelease,ok:=acquireAgentSlot(waitCtx,owner)
 if !ok{cancel();return nil,false}
 crossRelease,ok:=acquireAgentCrossProcessCapacity(waitCtx,owner)
 if !ok{
  localRelease()
  cancel()
  return nil,false
 }
 if waitCtx.Err()!=nil{
  crossRelease()
  localRelease()
  cancel()
  return nil,false
 }
 var once sync.Once
 return func(){once.Do(func(){
  crossRelease()
  localRelease()
  cancel()
 })},true
}
