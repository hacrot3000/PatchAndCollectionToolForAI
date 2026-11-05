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
 return p.readAuthenticatedWindow(ctx,path,offset,0)
}
// ReadFileChunk uses the same password/key/RBAC-protected, stop-and-wait
// E2EE channel but returns up to 4096 bytes in one bounded response.
// It is read-only and does not increase the six-action Relay quota.
func (p *PingClient) ReadFileChunk(ctx context.Context,path string,offset int64)(FileWindow,error){
 return p.readAuthenticatedWindow(ctx,path,offset,1)
}
// ReadFileBlock opts in to a separately typed 32KiB FilesRead-only E2EE
// response; host local scope and full-file digest remain mandatory.
func (p *PingClient) ReadFileBlock(ctx context.Context,path string,offset int64)(FileWindow,error){
 return p.readAuthenticatedWindow(ctx,path,offset,2)
}
func (p *PingClient) readAuthenticatedWindow(ctx context.Context,path string,offset int64,kind int)(FileWindow,error){
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
 if kind==1{operation=RemoteActionProjectFileChunk}
 if kind==2{operation=RemoteActionProjectFileBlock}
 frame,err:=SealRemoteAction(channel,RemoteAction{
  RequestID:id,Operation:operation,Path:path,Offset:offset,
 })
 if err!=nil||deadlineCtx.Err()!=nil||len(frame)>MaxEncryptedPingFrame{
  p.Close();return empty,denied
 }
 reply,err:=exchange.ExchangePing(deadlineCtx,frame)
 maxReply:=MaxEncryptedPingFrame
 if kind==1{maxReply=MaxEncryptedChunkFrame}
 if kind==2{maxReply=MaxEncryptedAppFrame}
 if err!=nil||deadlineCtx.Err()!=nil||len(reply)<25||
  len(reply)>maxReply{
  p.Close();return empty,denied
 }
 var window FileWindow
 if kind==1{
  window,err=OpenRemoteFileChunkResult(channel,reply,id,offset)
 }else if kind==2{
  window,err=OpenRemoteFileBlockResult(channel,reply,id,offset)
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
