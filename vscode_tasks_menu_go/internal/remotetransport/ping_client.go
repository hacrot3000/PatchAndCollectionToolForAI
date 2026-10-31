package remotetransport

import (
 "context"
 "crypto/rand"
 "encoding/hex"
 "sync"
 "time"
)

// PingExchange transports ONE opaque, encrypted request and ONE encrypted
// reply. An implementation must honor context cancellation and must never
// decrypt bytes or run TaskDeck actions. This interface is internal only:
// no HTTP, PHP, Relay, WebSocket or TaskDeck command route uses it yet.
type PingExchange interface {
 ExchangePing(ctx context.Context, encrypted []byte) ([]byte,error)
}

const MaxEncryptedPingFrame = maxRemoteActionPlaintext + 9 + 16
const pingExchangeDeadline = 15 * time.Second

// PingClient implements a small, stop-and-wait set of explicitly typed,
// authenticated E2EE operations. The host decides each permission and rejects
// any arbitrary HTTP request, command line, cookie or transferable token.
// The caller must have independently pinned the Agent Ed25519 Device ID and
// completed the native password/client-key E2EE login; Agent is still
// responsible for binding an in-process TaskDeck-local principal to the
// corresponding Channel and for per-action authorization.
type PingClient struct {
 mu sync.Mutex
 channel *Channel
 exchange PingExchange
 closed bool
 active bool
 cancel context.CancelFunc
}

func NewPingClient(channel *Channel,exchange PingExchange) (*PingClient,error) {
 if channel==nil || !channel.Established() || exchange==nil {
  return nil,ErrRemoteActionDenied
 }
 return &PingClient{channel:channel,exchange:exchange},nil
}

// Close is safe concurrently with ExchangePing and never waits for network
// activity. It invalidates the AEAD channel (and therefore this principal's
// session) even if a late forged reply eventually arrives.
func (p *PingClient) Close() {
 if p==nil{return}
 p.mu.Lock()
 if !p.closed {
  p.closed=true
  if p.cancel!=nil {p.cancel()}
  if p.channel!=nil {p.channel.Close()}
 }
 p.mu.Unlock()
}

// Ping performs one stop-and-wait E2EE roundtrip. Concurrent operations,
// canceled contexts, unknown records and replies with a wrong request ID
// permanently close this client. The exchange implementation MUST honor ctx
// cancellation; the client refuses late results even if it does not.
func (p *PingClient) Ping(ctx context.Context) error {
 if p==nil || ctx==nil || ctx.Err()!=nil {return ErrRemoteActionDenied}
 deadlineCtx,cancel:=context.WithTimeout(ctx,pingExchangeDeadline)
 defer cancel()
 p.mu.Lock()
 if p.closed || p.active || p.channel==nil || !p.channel.Established() ||
  p.exchange==nil || deadlineCtx.Err()!=nil {
  p.mu.Unlock()
  return ErrRemoteActionDenied
 }
 p.active=true
 p.cancel=cancel
 channel:=p.channel
 exchange:=p.exchange
 p.mu.Unlock()
 defer func(){
  p.mu.Lock()
  p.active=false
  p.cancel=nil
  p.mu.Unlock()
 }()
 // Peer closure, revocation or unauthenticated ciphertext must wake a
 // context-aware transport even when the HTTP/WebSocket side is stalled.
 stopWatch:=make(chan struct{})
 go func(){
  select{
  case <-channel.Done():cancel()
  case <-deadlineCtx.Done():
  case <-stopWatch:
  }
 }()
 defer close(stopWatch)
 var requestID [16]byte
 if _,err:=rand.Read(requestID[:]);err!=nil {p.Close();return ErrRemoteActionDenied}
 id:=hex.EncodeToString(requestID[:])
 frame,err:=SealRemoteAction(channel,RemoteAction{
  RequestID:id,Operation:RemoteActionSessionPing,
 })
 if err!=nil || deadlineCtx.Err()!=nil {p.Close();return ErrRemoteActionDenied}
 if len(frame)>MaxEncryptedPingFrame {p.Close();return ErrRemoteActionDenied}
 reply,err:=exchange.ExchangePing(deadlineCtx,frame)
 if err!=nil || deadlineCtx.Err()!=nil || len(reply)<25 ||
  len(reply)>MaxEncryptedPingFrame {
  p.Close()
  return ErrRemoteActionDenied
 }
 if err:=OpenRemoteActionPingResult(channel,reply,id);err!=nil {
  p.Close()
  return ErrRemoteActionDenied
 }
 // A locally revoked/closed session cannot be revived by a positive result
 // which arrived concurrently with cancellation or a second exchange.
 p.mu.Lock()
 valid:=!p.closed && p.active && deadlineCtx.Err()==nil && channel.Established()
 p.mu.Unlock()
 if !valid {p.Close();return ErrRemoteActionDenied}
 return nil
}



// ReadFile uses the SAME authenticated native E2EE channel and stop-and-wait
// quota as session.ping. Only a small regular UTF-8 workspace file can be
// read; the host checks FilesRead and local client-key approval on every call.
// This is never a generic file/command/HTTP proxy.
func (p *PingClient) ReadFile(ctx context.Context,path string)(string,error){
 return p.readTextOperation(ctx,RemoteActionProjectFileRead,path)
}

// ListDirectory returns a bounded UTF-8 set of *visible* relative names.
// The host independently enforces FilesRead, pinned directory access and
// rejects symlinks, hidden files, overlarge directories and unsafe paths.
// "." is the primary workspace root; there is no generic file-system browse.
func (p *PingClient) ListDirectory(ctx context.Context,path string)(string,error){
 return p.readTextOperation(ctx,RemoteActionProjectDirectoryList,path)
}

// GitStatus performs a fixed read-only Git operation at the already verified
// workspace root: no shell string, URL, user-selected command or file write.
func (p *PingClient) GitStatus(ctx context.Context)(string,error){
 return p.readTextOperation(ctx,RemoteActionGitStatus,"")
}
// GitDiff shows a fixed, read-only worktree diff for one visible file.
// The host checks GitStatus RBAC and independently rejects external helpers.
func (p *PingClient) GitDiff(ctx context.Context,path string)(string,error){
 return p.readTextOperation(ctx,RemoteActionGitDiff,path)
}
// ReplaceFile writes at most 96 UTF-8 bytes to an EXISTING small text file,
// only after owner-approved E2EE login and an exact optimistic SHA-256 check.
// The caller must have the current file hash; a wrong hash never writes.
func (p *PingClient) ReplaceFile(ctx context.Context,path,content,expectedSHA string)(string,error){
 return p.exchangeTextAction(ctx,RemoteAction{
  Operation:RemoteActionProjectFileReplace,Path:path,
  Content:content,ExpectedSHA256:expectedSHA,
 })
}
// RunTask executes only an existing host-authored task whose local
// taskdeckRemoteAllowed flag is true, with a hard TaskDeck-side timeout.
// It does not grant a shell, PTY handle, Git/SSH session, or cookies.
func (p *PingClient) RunTask(ctx context.Context,taskID int)(string,error){
 return p.exchangeTextAction(ctx,RemoteAction{
  Operation:RemoteActionTaskRun,TaskID:taskID,
 })
}
func (p *PingClient) readTextOperation(ctx context.Context,operation,path string)(string,error){
 return p.exchangeTextAction(ctx,RemoteAction{Operation:operation,Path:path})
}
func (p *PingClient) exchangeTextAction(ctx context.Context,action RemoteAction)(string,error){
 denied:=ErrRemoteActionDenied
 if p==nil||ctx==nil||ctx.Err()!=nil ||
  (action.Operation!=RemoteActionProjectFileRead &&
   action.Operation!=RemoteActionProjectDirectoryList &&
   action.Operation!=RemoteActionGitStatus &&
   action.Operation!=RemoteActionGitDiff &&
   action.Operation!=RemoteActionProjectFileReplace &&
   action.Operation!=RemoteActionTaskRun){
   return "",denied
 }
 deadlineCtx,cancel:=context.WithTimeout(ctx,pingExchangeDeadline)
 defer cancel()
 p.mu.Lock()
 if p.closed||p.active||p.channel==nil||!p.channel.Established()||
  p.exchange==nil||deadlineCtx.Err()!=nil{
  p.mu.Unlock();return "",denied
 }
 p.active=true
 p.cancel=cancel
 channel:=p.channel
 exchange:=p.exchange
 p.mu.Unlock()
 defer func(){p.mu.Lock();p.active=false;p.cancel=nil;p.mu.Unlock()}()
 stopWatch:=make(chan struct{})
 go func(){
  select{
  case <-channel.Done():cancel()
  case <-deadlineCtx.Done():
  case <-stopWatch:
  }
 }()
 defer close(stopWatch)
 var requestID [16]byte
 if _,err:=rand.Read(requestID[:]);err!=nil{p.Close();return "",denied}
 id:=hex.EncodeToString(requestID[:])
 action.RequestID=id
 frame,err:=SealRemoteAction(channel,action)
 if err!=nil||deadlineCtx.Err()!=nil||len(frame)>MaxEncryptedPingFrame{
  p.Close();return "",denied
 }
 reply,err:=exchange.ExchangePing(deadlineCtx,frame)
 maxReply:=MaxEncryptedPingFrame
 if action.Operation==RemoteActionGitDiff{maxReply=MaxEncryptedGitDiffFrame}
 if err!=nil||deadlineCtx.Err()!=nil||len(reply)<25||
  len(reply)>maxReply{
  p.Close();return "",denied
 }
 var content string
 switch action.Operation{
 case RemoteActionProjectFileRead:
  content,err=OpenRemoteFileReadResult(channel,reply,id)
 case RemoteActionProjectDirectoryList:
  content,err=OpenRemoteDirectoryListResult(channel,reply,id)
 case RemoteActionGitStatus:
  content,err=OpenRemoteGitStatusResult(channel,reply,id)
 case RemoteActionGitDiff:
  content,err=OpenRemoteGitDiffResult(channel,reply,id)
 case RemoteActionProjectFileReplace:
  content,err=OpenRemoteFileReplaceResult(channel,reply,id)
 case RemoteActionTaskRun:
  content,err=OpenRemoteTaskRunResult(channel,reply,id)
 default: err=ErrRemoteActionDenied
 }
 if err!=nil{p.Close();return "",denied}
 p.mu.Lock()
 valid:=!p.closed&&p.active&&deadlineCtx.Err()==nil&&channel.Established()
 p.mu.Unlock()
 if !valid{p.Close();return "",denied}
 return content,nil
}
