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

// PingClient carries only session.ping, session.features, host-root
// git.status, project.info and tasks.preview. No
// generic operation selector, command/path arguments or transferable token.
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

// Both methods perform one stop-and-wait E2EE roundtrip. Concurrent operations,
// canceled contexts, unknown records and replies with a wrong request ID
// permanently close this client. The exchange implementation MUST honor ctx
// cancellation; the client refuses late results even if it does not.
func (p *PingClient) Ping(ctx context.Context) error {
 _,err:=p.exchangeAction(ctx,RemoteActionSessionPing)
 return err
}

// Features returns only an advisory snapshot of host opt-in intersected
// with local TaskDeck RBAC. No future action may rely on this snapshot
// for authorization: the host checks again on every invocation.
func (p *PingClient) Features(ctx context.Context)(RemoteFeatures,error){
 reply,err:=p.exchangeAction(ctx,RemoteActionSessionFeatures)
 return reply.features,err
}

// GitStatus returns a short, authenticated read-only status of the exact
// TaskDeck project Git root. The browser never selects a path or command,
// and the host independently checks local git.status permission on every call.
func (p *PingClient) GitStatus(ctx context.Context)(string,error) {
 reply,err:=p.exchangeAction(ctx,RemoteActionGitStatus)
 return reply.status,err
}

// ProjectInfo retrieves only bounded display metadata for the single
// locally authorized TaskDeck project. No remote-selected project key or path.
func (p *PingClient) ProjectInfo(ctx context.Context)(RemoteProjectInfo,error){
 reply,err:=p.exchangeAction(ctx,RemoteActionProjectInfo)
 return reply.info,err
}

// TasksPreview is a bounded set of display labels only. It cannot
// choose or execute tasks and cannot retrieve their commands/arguments.
func (p *PingClient) TasksPreview(ctx context.Context)(RemoteTasksPreview,error){
 reply,err:=p.exchangeAction(ctx,RemoteActionTasksPreview)
 return reply.preview,err
}

// TasksPreviewNext requests the next host-maintained page. The native client
// has no selected offset, task ID, command, or generic RPC args.
func (p *PingClient) TasksPreviewNext(ctx context.Context)(RemoteTasksPreview,error){
 reply,err:=p.exchangeAction(ctx,RemoteActionTasksPreviewNext)
 return reply.preview,err
}

type remoteActionReply struct {
 status string
 info RemoteProjectInfo
 preview RemoteTasksPreview
 features RemoteFeatures
}

func (p *PingClient) exchangeAction(ctx context.Context,operation string) (remoteActionReply,error) {
 var empty remoteActionReply
 if p==nil || ctx==nil || ctx.Err()!=nil {return empty,ErrRemoteActionDenied}
 deadlineCtx,cancel:=context.WithTimeout(ctx,pingExchangeDeadline)
 defer cancel()
 p.mu.Lock()
 if p.closed || p.active || p.channel==nil || !p.channel.Established() ||
  p.exchange==nil || deadlineCtx.Err()!=nil {
  p.mu.Unlock()
  return empty,ErrRemoteActionDenied
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
 if _,err:=rand.Read(requestID[:]);err!=nil {p.Close();return empty,ErrRemoteActionDenied}
 id:=hex.EncodeToString(requestID[:])
 frame,err:=SealRemoteAction(channel,RemoteAction{
  RequestID:id,Operation:operation,
 })
 if err!=nil || deadlineCtx.Err()!=nil {p.Close();return empty,ErrRemoteActionDenied}
 if len(frame)>MaxEncryptedPingFrame {p.Close();return empty,ErrRemoteActionDenied}
 reply,err:=exchange.ExchangePing(deadlineCtx,frame)
 if err!=nil || deadlineCtx.Err()!=nil || len(reply)<25 ||
  len(reply)>MaxEncryptedPingFrame {
  p.Close()
  return empty,ErrRemoteActionDenied
 }
 var result remoteActionReply
 switch operation{
 case RemoteActionSessionPing:
  err=OpenRemoteActionPingResult(channel,reply,id)
 case RemoteActionSessionFeatures:
  result.features,err=OpenRemoteActionFeaturesResult(channel,reply,id)
 case RemoteActionGitStatus:
  result.status,err=OpenRemoteActionGitStatusResult(channel,reply,id)
 case RemoteActionProjectInfo:
  result.info,err=OpenRemoteActionProjectInfoResult(channel,reply,id)
 case RemoteActionTasksPreview,RemoteActionTasksPreviewNext:
  result.preview,err=OpenRemoteActionTasksPreviewResult(channel,reply,id)
 default:
  err=ErrInvalidRemoteAction
 }
 if err!=nil {
  p.Close()
  return empty,ErrRemoteActionDenied
 }
 // A locally revoked/closed session cannot be revived by a positive result
 // which arrived concurrently with cancellation or a second exchange.
 p.mu.Lock()
 valid:=!p.closed && p.active && deadlineCtx.Err()==nil && channel.Established()
 p.mu.Unlock()
 if !valid {p.Close();return empty,ErrRemoteActionDenied}
 return result,nil
}

