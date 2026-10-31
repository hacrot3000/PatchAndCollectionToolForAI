package remotetransport

import (
 "context"
 "crypto/rand"
 "encoding/hex"
)

// ReadFileWindow is an independently authorized, read-only FILES_READ action,
// one stop-and-wait binary window per AEAD round. No arbitrary HTTP or shell.
// Six rounds per WSS login are still the Relay's existing hard quota.
func (p *PingClient) ReadFileWindow(ctx context.Context,path string,offset int64)(FileWindow,error){
 return p.readAuthenticatedWindow(ctx,path,offset,false)
}
// ReadFileChunk uses the same password/key/RBAC-protected, stop-and-wait
// E2EE channel but returns up to 4096 bytes in one bounded response.
// It is read-only and does not increase the six-action Relay quota.
func (p *PingClient) ReadFileChunk(ctx context.Context,path string,offset int64)(FileWindow,error){
 return p.readAuthenticatedWindow(ctx,path,offset,true)
}
func (p *PingClient) readAuthenticatedWindow(ctx context.Context,path string,offset int64,bulk bool)(FileWindow,error){
 var empty FileWindow
 denied:=ErrRemoteActionDenied
 if p==nil||ctx==nil||ctx.Err()!=nil||
  offset<0||offset>RemoteFileWindowMaxSize{return empty,denied}
 deadlineCtx,cancel:=context.WithTimeout(ctx,pingExchangeDeadline)
 defer cancel()
 p.mu.Lock()
 if p.closed||p.active||p.channel==nil||!p.channel.Established()||
  p.exchange==nil||deadlineCtx.Err()!=nil{
  p.mu.Unlock();return empty,denied
 }
 p.active=true
 p.cancel=cancel
 channel:=p.channel
 exchange:=p.exchange
 p.mu.Unlock()
 defer func(){p.mu.Lock();p.active=false;p.cancel=nil;p.mu.Unlock()}()
 stopWatch:=make(chan struct{})
 go func(){
  select {
  case <-channel.Done():cancel()
  case <-deadlineCtx.Done():
  case <-stopWatch:
  }
 }()
 defer close(stopWatch)
 var requestID [16]byte
 if _,err:=rand.Read(requestID[:]);err!=nil{p.Close();return empty,denied}
 id:=hex.EncodeToString(requestID[:])
 operation:=RemoteActionProjectFileWindow
 if bulk{operation=RemoteActionProjectFileChunk}
 frame,err:=SealRemoteAction(channel,RemoteAction{
  RequestID:id,Operation:operation,Path:path,Offset:offset,
 })
 if err!=nil||deadlineCtx.Err()!=nil||len(frame)>MaxEncryptedPingFrame{
  p.Close();return empty,denied
 }
 reply,err:=exchange.ExchangePing(deadlineCtx,frame)
 maxReply:=MaxEncryptedPingFrame
 if bulk{maxReply=MaxEncryptedAppFrame}
 if err!=nil||deadlineCtx.Err()!=nil||len(reply)<25||
  len(reply)>maxReply{
  p.Close();return empty,denied
 }
 var window FileWindow
 if bulk{
  window,err=OpenRemoteFileChunkResult(channel,reply,id,offset)
 }else{
  window,err=OpenRemoteFileWindowResult(channel,reply,id,offset)
 }
 if err!=nil{p.Close();return empty,denied}
 p.mu.Lock()
 valid:=!p.closed&&p.active&&deadlineCtx.Err()==nil&&channel.Established()
 p.mu.Unlock()
 if !valid{p.Close();return empty,denied}
 return window,nil
}
