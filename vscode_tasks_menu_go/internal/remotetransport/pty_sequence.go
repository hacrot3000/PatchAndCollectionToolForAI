package remotetransport

import (
 "sync"
)

// BoundedRemotePTYSessionGate is a local-only frame-ordering gate. The
// regular E2EE cipher authenticates individual records but is not sufficient
// to detect a skipped logical PTY input sequence after a reconnect. This
// adds exact session-handle and monotonic sequence checks WITHOUT opening a
// remote shell, registering a new RemoteAction, or modifying Relay quotas.
const RemotePTYMaxFramesPerDirection=1024

type BoundedRemotePTYSessionGate struct{
 mu sync.Mutex
 handle string
 nextClient uint64
 nextHost uint64
 closed bool
}

func NewBoundedRemotePTYSessionGate(handle string)(*BoundedRemotePTYSessionGate,error){
 if !ValidManagedTaskHandle(handle){return nil,ErrInvalidRemoteAction}
 return &BoundedRemotePTYSessionGate{
  handle:handle,nextClient:1,nextHost:1,
 },nil
}

// A terminal's client→host direction contains input and resize. Its
// host→client direction contains output. The *first* frame of each
// direction must have Sequence=1, thereafter every number must increase
// by exactly one. An invalid/out-of-order/duplicate/over-quota frame
// permanently retires the local PTY logical session to avoid ambiguity.
func (g *BoundedRemotePTYSessionGate)AcceptFromClient(v RemotePTYFrame)error{
 return g.accept(v,true)
}
func (g *BoundedRemotePTYSessionGate)AcceptFromHost(v RemotePTYFrame)error{
 return g.accept(v,false)
}
func (g *BoundedRemotePTYSessionGate)accept(v RemotePTYFrame,client bool)error{
 if g==nil{return ErrInvalidRemoteAction}
 g.mu.Lock()
 defer g.mu.Unlock()
 if g.closed{return ErrInvalidRemoteAction}
 if !ValidRemotePTYFrame(v)||v.Handle!=g.handle{
  g.closed=true
  return ErrInvalidRemoteAction
 }
 var next *uint64
 if client{
  if v.Kind!="input"&&v.Kind!="resize"{
   g.closed=true
   return ErrInvalidRemoteAction
  }
  next=&g.nextClient
 }else{
  if v.Kind!="output"{
   g.closed=true
   return ErrInvalidRemoteAction
  }
  next=&g.nextHost
 }
 if *next>RemotePTYMaxFramesPerDirection||v.Sequence!=*next{
  g.closed=true
  return ErrInvalidRemoteAction
 }
 *next++
 return nil
}
func (g *BoundedRemotePTYSessionGate)Close(){
 if g==nil{return}
 g.mu.Lock()
 g.closed=true
 g.handle=""
 g.mu.Unlock()
}
func (g *BoundedRemotePTYSessionGate)Closed()bool{
 if g==nil{return true}
 g.mu.Lock()
 defer g.mu.Unlock()
 return g.closed
}
