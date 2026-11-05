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

// PingClient carries only the fixed permitted read actions and bounded
// tasks.run selected by a previously published stable Task.ID. No generic
// RPC selector, command/path argument, or transferable session token.
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
func (p *PingClient) GitLog(ctx context.Context)(string,error){
 reply,err:=p.exchangeAction(ctx,RemoteActionGitLog)
 return reply.gitLog,err
}

// GitPull requests a fixed host-only fast-forward operation. There is no
// remote-selected branch, remote name, path, command or arbitrary argument.
func (p *PingClient) GitPull(ctx context.Context)(RemoteGitPullResult,error){
 reply,err:=p.exchangeAction(ctx,RemoteActionGitPull)
 return reply.gitPull,err
}

// GitCommit accepts only a bounded message for the already-staged host
// index. It never chooses paths, auto-stages files or invokes a shell.
func (p *PingClient) GitCommit(ctx context.Context,message string)(RemoteGitCommitResult,error){
 reply,err:=p.exchangeMessageAction(ctx,RemoteActionGitCommit,message)
 return reply.gitCommit,err
}

// GitPush pushes only the exact committed host HEAD to its configured
// upstream, with no client-selected remote, ref, path or force options.
func (p *PingClient) GitPush(ctx context.Context)(RemoteGitPushResult,error){
 reply,err:=p.exchangeAction(ctx,RemoteActionGitPush)
 return reply.gitPush,err
}

// TerminalOpen starts one short-lived host PTY bound to this authenticated
// E2EE session. No shell selector, remote URL, cwd or PID may be supplied.
func (p *PingClient) TerminalOpen(ctx context.Context)(RemoteTerminalProgress,error){
 reply,err:=p.exchangeTerminalAction(ctx,RemoteActionTerminalOpen,"",0,0)
 return reply.terminal,err
}
func (p *PingClient) TerminalPoll(ctx context.Context)(RemoteTerminalProgress,error){
 reply,err:=p.exchangeTerminalAction(ctx,RemoteActionTerminalPoll,"",0,0)
 return reply.terminal,err
}
func (p *PingClient) TerminalWrite(ctx context.Context,input string)(RemoteTerminalProgress,error){
 reply,err:=p.exchangeTerminalAction(ctx,RemoteActionTerminalWrite,input,0,0)
 return reply.terminal,err
}
func (p *PingClient) TerminalResize(ctx context.Context,cols,rows int)(RemoteTerminalProgress,error){
 reply,err:=p.exchangeTerminalAction(ctx,RemoteActionTerminalResize,"",cols,rows)
 return reply.terminal,err
}
func (p *PingClient) TerminalClose(ctx context.Context)(RemoteTerminalProgress,error){
 reply,err:=p.exchangeTerminalAction(ctx,RemoteActionTerminalClose,"",0,0)
 return reply.terminal,err
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

// InspectTask verifies a previously published, host-owned task snapshot.
// It never requests task content, commands, environment or execution.
func (p *PingClient) InspectTask(ctx context.Context,taskID int)(RemoteTaskInspect,error){
 reply,err:=p.exchangeSelectedAction(ctx,RemoteActionTasksInspect,taskID)
 return reply.inspect,err
}

func (p *PingClient) RunTask(ctx context.Context,taskID int)(RemoteTaskRun,error){
 return p.RunTaskChoice(ctx,taskID,0)
}

func (p *PingClient) RunTaskChoice(ctx context.Context,taskID,choice int)(RemoteTaskRun,error){
 reply,err:=p.exchangeChoiceAction(ctx,RemoteActionTasksRun,taskID,choice)
 return reply.run,err
}

func (p *PingClient) StartLongTask(ctx context.Context,taskID,choice int,value string)(RemoteLongTaskProgress,error){
 reply,err:=p.exchangeInputAction(ctx,RemoteActionTasksStart,taskID,choice,value)
 return reply.long,err
}
func (p *PingClient) PollLongTask(ctx context.Context,taskID int)(RemoteLongTaskProgress,error){
 reply,err:=p.exchangeSelectedAction(ctx,RemoteActionTasksPoll,taskID)
 return reply.long,err
}
func (p *PingClient) CancelLongTask(ctx context.Context,taskID int)(RemoteLongTaskProgress,error){
 reply,err:=p.exchangeSelectedAction(ctx,RemoteActionTasksCancel,taskID)
 return reply.long,err
}
func (p *PingClient) ClearLongTask(ctx context.Context,taskID int)(RemoteLongTaskProgress,error){
 reply,err:=p.exchangeSelectedAction(ctx,RemoteActionTasksClear,taskID)
 return reply.long,err
}

func (p *PingClient) RunTaskText(ctx context.Context,taskID int,value string)(RemoteTaskRun,error){
 reply,err:=p.exchangeInputAction(ctx,RemoteActionTasksRun,taskID,0,value)
 return reply.run,err
}

type remoteActionReply struct {
 status string
 gitLog string
 gitPull RemoteGitPullResult
 gitCommit RemoteGitCommitResult
 gitPush RemoteGitPushResult
 terminal RemoteTerminalProgress
 info RemoteProjectInfo
 preview RemoteTasksPreview
 inspect RemoteTaskInspect
 run RemoteTaskRun
 long RemoteLongTaskProgress
 features RemoteFeatures
}

func (p *PingClient) exchangeAction(ctx context.Context,operation string) (remoteActionReply,error){
 return p.exchangeSelectedAction(ctx,operation,0)
}

func (p *PingClient) exchangeSelectedAction(ctx context.Context,operation string,taskID int) (remoteActionReply,error) {
 return p.exchangeChoiceAction(ctx,operation,taskID,0)
}

func (p *PingClient) exchangeChoiceAction(ctx context.Context,operation string,taskID,choice int) (remoteActionReply,error) {
 return p.exchangeInputAction(ctx,operation,taskID,choice,"")
}
func (p *PingClient) exchangeInputAction(ctx context.Context,operation string,taskID,choice int,value string) (remoteActionReply,error) {
 return p.exchangeInputOrMessageAction(ctx,operation,taskID,choice,value,"")
}
func (p *PingClient) exchangeMessageAction(ctx context.Context,operation,message string) (remoteActionReply,error) {
 return p.exchangeInputOrMessageAction(ctx,operation,0,0,"",message)
}
func (p *PingClient) exchangeInputOrMessageAction(ctx context.Context,operation string,taskID,choice int,value,message string) (remoteActionReply,error) {
 return p.exchangePayloadAction(ctx,operation,taskID,choice,value,message,"",0,0)
}
func (p *PingClient) exchangeTerminalAction(ctx context.Context,operation,input string,cols,rows int)(remoteActionReply,error){
 return p.exchangePayloadAction(ctx,operation,0,0,"","",input,cols,rows)
}
func (p *PingClient) exchangePayloadAction(ctx context.Context,operation string,
 taskID,choice int,value,message,terminalInput string,cols,rows int)(remoteActionReply,error){
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
  RequestID:id,Operation:operation,TaskID:taskID,Choice:choice,Text:value,Message:message,
  TerminalInput:terminalInput,Cols:cols,Rows:rows,
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
 case RemoteActionTerminalOpen,RemoteActionTerminalPoll,RemoteActionTerminalWrite,
  RemoteActionTerminalResize,RemoteActionTerminalClose:
  result.terminal,err=OpenRemoteTerminalProgress(channel,reply,id)
 case RemoteActionGitStatus:
  result.status,err=OpenRemoteActionGitStatusResult(channel,reply,id)
 case RemoteActionGitLog:
  result.gitLog,err=OpenRemoteGitLogResult(channel,reply,id)
 case RemoteActionGitPull:
  result.gitPull,err=OpenRemoteGitPullResult(channel,reply,id)
 case RemoteActionGitCommit:
  result.gitCommit,err=OpenRemoteGitCommitResult(channel,reply,id)
 case RemoteActionGitPush:
  result.gitPush,err=OpenRemoteGitPushResult(channel,reply,id)
 case RemoteActionProjectInfo:
  result.info,err=OpenRemoteActionProjectInfoResult(channel,reply,id)
 case RemoteActionTasksPreview,RemoteActionTasksPreviewNext:
  result.preview,err=OpenRemoteActionTasksPreviewResult(channel,reply,id)
 case RemoteActionTasksInspect:
  result.inspect,err=OpenRemoteActionTasksInspectResult(channel,reply,id,taskID)
 case RemoteActionTasksRun:
  result.run,err=OpenRemoteActionTaskRunResult(channel,reply,id,taskID)
 case RemoteActionTasksStart,RemoteActionTasksPoll,RemoteActionTasksCancel,RemoteActionTasksClear:
  result.long,err=OpenRemoteLongTaskProgress(channel,reply,id,taskID)
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

