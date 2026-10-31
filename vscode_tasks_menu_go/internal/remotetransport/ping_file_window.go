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
 frame,err:=SealRemoteAction(channel,RemoteAction{
  RequestID:id,Operation:RemoteActionProjectFileWindow,
  Path:path,Offset:offset,
 })
 if err!=nil||deadlineCtx.Err()!=nil||len(frame)>MaxEncryptedPingFrame{
  p.Close();return empty,denied
 }
 reply,err:=exchange.ExchangePing(deadlineCtx,frame)
 if err!=nil||deadlineCtx.Err()!=nil||len(reply)<25||
  len(reply)>MaxEncryptedPingFrame{
  p.Close();return empty,denied
 }
 window,err:=OpenRemoteFileWindowResult(channel,reply,id,offset)
 if err!=nil{p.Close();return empty,denied}
 p.mu.Lock()
 valid:=!p.closed&&p.active&&deadlineCtx.Err()==nil&&channel.Established()
 p.mu.Unlock()
 if !valid{p.Close();return empty,denied}
 return window,nil
}
