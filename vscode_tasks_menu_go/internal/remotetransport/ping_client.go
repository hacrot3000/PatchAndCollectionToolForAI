package remotetransport

import (
 "context"
 "crypto/rand"
 "encoding/hex"
 "fmt"
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
func (p *PingClient) GitBranches(ctx context.Context)(string,error){
 reply,err:=p.exchangeAction(ctx,RemoteActionGitBranches)
 return reply.gitBranches,err
}
func (p *PingClient) GitBranchCreate(ctx context.Context)(RemoteGitBranchCreateResult,error){
 reply,err:=p.exchangeAction(ctx,RemoteActionGitBranchCreate)
 return reply.gitBranchCreate,err
}
func (p *PingClient) GitDiff(ctx context.Context)(string,error){
 reply,err:=p.exchangeAction(ctx,RemoteActionGitDiff)
 return reply.gitDiff,err
}
func (p *PingClient) GitPatchStart(ctx context.Context)(RemoteGitPatchPage,error){
 reply,err:=p.exchangeAction(ctx,RemoteActionGitPatchStart)
 return reply.gitPatch,err
}
func (p *PingClient) GitPatchNext(ctx context.Context)(RemoteGitPatchPage,error){
 reply,err:=p.exchangeAction(ctx,RemoteActionGitPatchNext)
 return reply.gitPatch,err
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

func (p *PingClient) GitFetch(ctx context.Context)(RemoteGitFetchResult,error){
 reply,err:=p.exchangeAction(ctx,RemoteActionGitFetch)
 return reply.gitFetch,err
}

func (p *PingClient) GitStageTracked(ctx context.Context)(RemoteGitIndexResult,error){
 reply,err:=p.exchangeAction(ctx,RemoteActionGitStageTracked)
 return reply.gitIndex,err
}
func (p *PingClient) GitUnstageAll(ctx context.Context)(RemoteGitIndexResult,error){
 reply,err:=p.exchangeAction(ctx,RemoteActionGitUnstageAll)
 return reply.gitIndex,err
}
func (p *PingClient) GitStageFile(ctx context.Context,index int,snapshot string)(RemoteGitIndexResult,error){
 reply,err:=p.exchangeFileAction(ctx,RemoteActionGitStageFile,index,snapshot)
 return reply.gitIndex,err
}
func (p *PingClient) GitUnstageFile(ctx context.Context,index int,snapshot string)(RemoteGitIndexResult,error){
 reply,err:=p.exchangeFileAction(ctx,RemoteActionGitUnstageFile,index,snapshot)
 return reply.gitIndex,err
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
 gitBranches string
 gitBranchCreate RemoteGitBranchCreateResult
 gitDiff string
 gitPatch RemoteGitPatchPage
 gitPull RemoteGitPullResult
 gitCommit RemoteGitCommitResult
 gitPush RemoteGitPushResult
 gitFetch RemoteGitFetchResult
 gitIndex RemoteGitIndexResult
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
 return p.exchangePayloadAction(ctx,operation,taskID,choice,value,message,"",0,0,0,"")
}
func (p *PingClient) exchangeTerminalAction(ctx context.Context,operation,input string,cols,rows int)(remoteActionReply,error){
 return p.exchangePayloadAction(ctx,operation,0,0,"","",input,cols,rows,0,"")
}
func (p *PingClient) exchangeFileAction(ctx context.Context,operation string,index int,snapshot string)(remoteActionReply,error){
 return p.exchangePayloadAction(ctx,operation,0,0,"","","",0,0,index,snapshot)
}
func (p *PingClient) exchangePayloadAction(ctx context.Context,operation string,
 taskID,choice int,value,message,terminalInput string,cols,rows,index int,snapshot string)(remoteActionReply,error){
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
  TerminalInput:terminalInput,Cols:cols,Rows:rows,FileIndex:index,Snapshot:snapshot,
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
 case RemoteActionGitBranches:
  result.gitBranches,err=OpenRemoteGitBranchesResult(channel,reply,id)
 case RemoteActionGitBranchCreate:
  result.gitBranchCreate,err=OpenRemoteGitBranchCreateResult(channel,reply,id)
 case RemoteActionGitDiff:
  result.gitDiff,err=OpenRemoteGitDiffResult(channel,reply,id)
 case RemoteActionGitPatchStart,RemoteActionGitPatchNext:
  result.gitPatch,err=OpenRemoteGitPatchPage(channel,reply,id)
 case RemoteActionGitPull:
  result.gitPull,err=OpenRemoteGitPullResult(channel,reply,id)
 case RemoteActionGitCommit:
  result.gitCommit,err=OpenRemoteGitCommitResult(channel,reply,id)
 case RemoteActionGitPush:
  result.gitPush,err=OpenRemoteGitPushResult(channel,reply,id)
 case RemoteActionGitFetch:
  result.gitFetch,err=OpenRemoteGitFetchResult(channel,reply,id)
 case RemoteActionGitStageTracked,RemoteActionGitUnstageAll,
  RemoteActionGitStageFile,RemoteActionGitUnstageFile:
  result.gitIndex,err=OpenRemoteGitIndexResult(channel,reply,id,operation)
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



// ReadFile uses the SAME authenticated native E2EE channel and stop-and-wait
// quota as session.ping. Only a small regular UTF-8 workspace file can be
// read; the host checks FilesRead and local client-key approval on every call.
// This is never a generic file/command/HTTP proxy.
func (p *PingClient) ReadFile(ctx context.Context,path string)(string,error){
 return p.readTextOperation(ctx,RemoteActionProjectFileRead,path)
}
// ReadFilePreview returns the complete <=4096-byte visible UTF-8 file.
// It stays within the normal application session; no binary download mode.
func (p *PingClient) ReadFilePreview(ctx context.Context,path string)(string,error){
 return p.readTextOperation(ctx,RemoteActionProjectFilePreview,path)
}
func (p *PingClient) ReadFileLines(ctx context.Context,path string,startLine int64)(string,error){
 return p.exchangeTextAction(ctx,RemoteAction{
  Operation:RemoteActionProjectFileLines,Path:path,Offset:startLine,
 })
}
func (p *PingClient) EditLargeFileLine(ctx context.Context,path string,line int64,text,expectedSHA string)(string,error){
 return p.exchangeTextAction(ctx,RemoteAction{Operation:RemoteActionProjectFileEditLine,
  Path:path,Offset:line,Content:text,ExpectedSHA256:expectedSHA})
}
func (p *PingClient) InsertLargeFileLine(ctx context.Context,path string,line int64,text,expectedSHA string)(string,error){
 return p.exchangeTextAction(ctx,RemoteAction{Operation:RemoteActionProjectFileInsertLine,
  Path:path,Offset:line,Content:text,ExpectedSHA256:expectedSHA})
}
func (p *PingClient) DeleteLargeFileLine(ctx context.Context,path string,line int64,expectedSHA string)(string,error){
 return p.exchangeTextAction(ctx,RemoteAction{Operation:RemoteActionProjectFileDeleteLine,
  Path:path,Offset:line,Content:"",ExpectedSHA256:expectedSHA})
}
func (p *PingClient) FindFileLines(ctx context.Context,path,literal string)(string,error){
 return p.exchangeTextAction(ctx,RemoteAction{
  Operation:RemoteActionProjectFileFindLines,Path:path,Content:literal,
 })
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
func (p *PingClient) GitStatusFull(ctx context.Context)(string,error){
 return p.readTextOperation(ctx,RemoteActionGitStatusFull,"")
}
func (p *PingClient) GitCommitReview(ctx context.Context)(string,error){
 return p.readTextOperation(ctx,RemoteActionGitCommitReview,"")
}
func (p *PingClient) GitCommit(ctx context.Context,message,reviewSHA string)(string,error){
 return p.exchangeTextAction(ctx,RemoteAction{
  Operation:RemoteActionGitCommit,Content:message,ExpectedSHA256:reviewSHA,
 })
}
func (p *PingClient) GitBranchReview(ctx context.Context)(string,error){
 return p.readTextOperation(ctx,RemoteActionGitBranchReview,"")
}
func (p *PingClient) GitBranchCreate(ctx context.Context,slug,reviewSHA string)(string,error){
 return p.exchangeTextAction(ctx,RemoteAction{
  Operation:RemoteActionGitBranchCreate,Path:slug,ExpectedSHA256:reviewSHA,
 })
}
func (p *PingClient) GitSwitchReview(ctx context.Context,slug string)(string,error){
 return p.exchangeTextAction(ctx,RemoteAction{Operation:RemoteActionGitSwitchReview,Path:slug})
}
func (p *PingClient) GitSwitchSameHead(ctx context.Context,slug,reviewSHA string)(string,error){
 return p.exchangeTextAction(ctx,RemoteAction{Operation:RemoteActionGitSwitchSameHead,
  Path:slug,ExpectedSHA256:reviewSHA})
}
func (p *PingClient) GitBranchDeleteReview(ctx context.Context,slug string)(string,error){
 return p.exchangeTextAction(ctx,RemoteAction{Operation:RemoteActionGitBranchDeleteReview,Path:slug})
}
func (p *PingClient) GitBranchDelete(ctx context.Context,slug,reviewSHA string)(string,error){
 return p.exchangeTextAction(ctx,RemoteAction{Operation:RemoteActionGitBranchDelete,
  Path:slug,ExpectedSHA256:reviewSHA})
}
// GitDiff shows a fixed, read-only worktree diff for one visible file.
// The host checks GitStatus RBAC and independently rejects external helpers.
func (p *PingClient) GitDiff(ctx context.Context,path string)(string,error){
 return p.readTextOperation(ctx,RemoteActionGitDiff,path)
}
func (p *PingClient) GitLog(ctx context.Context)(string,error){
 return p.readTextOperation(ctx,RemoteActionGitLog,"")
}
func (p *PingClient) GitBranches(ctx context.Context)(string,error){
 return p.readTextOperation(ctx,RemoteActionGitBranches,"")
}
func (p *PingClient) GitRemotes(ctx context.Context)(string,error){
 return p.readTextOperation(ctx,RemoteActionGitRemotes,"")
}
func (p *PingClient) GitTrackingRefs(ctx context.Context)(string,error){
 return p.readTextOperation(ctx,RemoteActionGitTrackingRefs,"")
}
func (p *PingClient) GitPullReview(ctx context.Context)(string,error){
 return p.readTextOperation(ctx,RemoteActionGitPullReview,"")
}
func (p *PingClient) GitPullChangesReview(ctx context.Context)(string,error){
 return p.readTextOperation(ctx,RemoteActionGitPullChangesReview,"")
}
func (p *PingClient) GitPullCachedReview(ctx context.Context)(string,error){
 return p.readTextOperation(ctx,RemoteActionGitPullCachedReview,"")
}
func (p *PingClient) GitPullApplyCached(ctx context.Context,sha string)(string,error){
 return p.exchangeTextAction(ctx,RemoteAction{Operation:RemoteActionGitPullApplyCached,ExpectedSHA256:sha})
}
func (p *PingClient) GitPullApplyFiles(ctx context.Context,sha string)(string,error){
 return p.exchangeTextAction(ctx,RemoteAction{Operation:RemoteActionGitPullApplyFiles,ExpectedSHA256:sha})
}
func (p *PingClient) GitFetchReview(ctx context.Context)(string,error){
 return p.readTextOperation(ctx,RemoteActionGitFetchReview,"")
}
func (p *PingClient) GitFetchApply(ctx context.Context,digest string)(string,error){
 return p.exchangeTextAction(ctx,RemoteAction{
  Operation:RemoteActionGitFetchApply,ExpectedSHA256:digest})
}
func (p *PingClient) GitPushReview(ctx context.Context)(string,error){
 return p.readTextOperation(ctx,RemoteActionGitPushReview,"")
}
func (p *PingClient) GitPushApply(ctx context.Context,digest string)(string,error){
 return p.exchangeTextAction(ctx,RemoteAction{
  Operation:RemoteActionGitPushApply,ExpectedSHA256:digest})
}
func (p *PingClient) GitStagedDiff(ctx context.Context,path string)(string,error){
 return p.readTextOperation(ctx,RemoteActionGitStagedDiff,path)
}
func (p *PingClient) GitHeadDiff(ctx context.Context,path string)(string,error){
 return p.readTextOperation(ctx,RemoteActionGitHeadDiff,path)
}
func (p *PingClient) GitStage(ctx context.Context,path,expectedSHA string)(string,error){
 return p.exchangeTextAction(ctx,RemoteAction{
  Operation:RemoteActionGitStage,Path:path,ExpectedSHA256:expectedSHA,
 })
}
func (p *PingClient) GitUnstage(ctx context.Context,path,expectedDiffSHA string)(string,error){
 return p.exchangeTextAction(ctx,RemoteAction{
  Operation:RemoteActionGitUnstage,Path:path,ExpectedSHA256:expectedDiffSHA,
 })
}
func (p *PingClient) ListApprovedTasks(ctx context.Context)(string,error){
 return p.readTextOperation(ctx,RemoteActionTaskList,"")
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
// ReplaceFileText is an independently host-opt-in 4KiB UTF-8 CAS edit.
func (p *PingClient) ReplaceFileText(ctx context.Context,path,content,expectedSHA string)(string,error){
 return p.exchangeTextAction(ctx,RemoteAction{
  Operation:RemoteActionProjectFileReplaceText,Path:path,
  Content:content,ExpectedSHA256:expectedSHA,
 })
}
// Display-only task exit report; response is a typed authenticated E2EE
// object, not text parsed from the program's stdout.
func (p *PingClient) StartManagedTask(ctx context.Context,taskID int)(string,error){
 return p.exchangeTextAction(ctx,RemoteAction{Operation:RemoteActionTaskManagedStart,TaskID:taskID})
}
func (p *PingClient) ReadManagedOutput(ctx context.Context,handle string,offset int64)(string,error){
 return p.exchangeTextAction(ctx,RemoteAction{
  Operation:RemoteActionTaskManagedOutput,Path:handle,Offset:offset,
 })
}
func (p *PingClient) StatusManagedTask(ctx context.Context,handle string)(string,error){
 return p.exchangeTextAction(ctx,RemoteAction{Operation:RemoteActionTaskManagedStatus,Path:handle})
}
func (p *PingClient) CancelManagedTask(ctx context.Context,handle string)(string,error){
 return p.exchangeTextAction(ctx,RemoteAction{Operation:RemoteActionTaskManagedCancel,Path:handle})
}
func (p *PingClient) RunTaskReport(ctx context.Context,taskID int)(string,error){
 return p.exchangeTextAction(ctx,RemoteAction{Operation:RemoteActionTaskRunReport,TaskID:taskID})
}
func (p *PingClient) RunTaskText(ctx context.Context,taskID int)(string,error){
 return p.exchangeTextAction(ctx,RemoteAction{
  Operation:RemoteActionTaskRunText,TaskID:taskID,
 })
}
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
   action.Operation!=RemoteActionProjectFilePreview &&
   action.Operation!=RemoteActionProjectFileLines &&
   action.Operation!=RemoteActionProjectFileEditLine &&
   action.Operation!=RemoteActionProjectFileDeleteLine &&
   action.Operation!=RemoteActionProjectFileInsertLine &&
   action.Operation!=RemoteActionProjectFileFindLines &&
   action.Operation!=RemoteActionProjectDirectoryList &&
   action.Operation!=RemoteActionGitStatus &&
   action.Operation!=RemoteActionGitStatusFull &&
   action.Operation!=RemoteActionGitCommitReview &&
   action.Operation!=RemoteActionGitCommit &&
   action.Operation!=RemoteActionGitBranchReview &&
   action.Operation!=RemoteActionGitBranchCreate &&
   action.Operation!=RemoteActionGitSwitchReview &&
   action.Operation!=RemoteActionGitSwitchSameHead &&
    action.Operation!=RemoteActionGitBranchDeleteReview &&
    action.Operation!=RemoteActionGitBranchDelete &&
   action.Operation!=RemoteActionGitLog &&
   action.Operation!=RemoteActionGitBranches &&
   action.Operation!=RemoteActionGitRemotes &&
   action.Operation!=RemoteActionGitTrackingRefs &&
    action.Operation!=RemoteActionGitPullReview &&
    action.Operation!=RemoteActionGitPullChangesReview &&
    action.Operation!=RemoteActionGitPullCachedReview &&
    action.Operation!=RemoteActionGitPullApplyCached &&
    action.Operation!=RemoteActionGitPullApplyFiles &&
    action.Operation!=RemoteActionGitFetchReview &&
    action.Operation!=RemoteActionGitFetchApply &&
    action.Operation!=RemoteActionGitPushReview &&
    action.Operation!=RemoteActionGitPushApply &&
   action.Operation!=RemoteActionGitDiff &&
   action.Operation!=RemoteActionGitStagedDiff &&
   action.Operation!=RemoteActionGitHeadDiff &&
   action.Operation!=RemoteActionGitStage &&
   action.Operation!=RemoteActionGitUnstage &&
   action.Operation!=RemoteActionTaskList &&
   action.Operation!=RemoteActionProjectFileReplace &&
   action.Operation!=RemoteActionProjectFileReplaceText &&
   action.Operation!=RemoteActionTaskRun &&
   action.Operation!=RemoteActionTaskRunText &&
   action.Operation!=RemoteActionTaskRunReport &&
   action.Operation!=RemoteActionTaskManagedStart &&
   action.Operation!=RemoteActionTaskManagedStatus &&
   action.Operation!=RemoteActionTaskManagedCancel &&
   action.Operation!=RemoteActionTaskManagedOutput){
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
 maxRequest:=MaxEncryptedPingFrame
 if action.Operation==RemoteActionProjectFileReplaceText||action.Operation==RemoteActionProjectFileEditLine||action.Operation==RemoteActionProjectFileDeleteLine||action.Operation==RemoteActionProjectFileInsertLine{
  maxRequest=MaxEncryptedTextEditFrame
 }
 if err!=nil||deadlineCtx.Err()!=nil||len(frame)>maxRequest{
  p.Close();return "",denied
 }
 reply,err:=exchange.ExchangePing(deadlineCtx,frame)
 maxReply:=MaxEncryptedPingFrame
 if action.Operation==RemoteActionGitDiff||action.Operation==RemoteActionProjectFilePreview||action.Operation==RemoteActionProjectFileLines||action.Operation==RemoteActionProjectFileFindLines||
  action.Operation==RemoteActionGitLog||action.Operation==RemoteActionGitBranches||action.Operation==RemoteActionGitRemotes||action.Operation==RemoteActionGitTrackingRefs||action.Operation==RemoteActionGitPullReview||action.Operation==RemoteActionGitPullChangesReview||action.Operation==RemoteActionGitPullCachedReview||action.Operation==RemoteActionGitPullApplyCached||action.Operation==RemoteActionGitPullApplyFiles||action.Operation==RemoteActionGitFetchReview||action.Operation==RemoteActionGitFetchApply||action.Operation==RemoteActionGitPushReview||action.Operation==RemoteActionGitPushApply||
  action.Operation==RemoteActionGitStagedDiff||action.Operation==RemoteActionGitHeadDiff||action.Operation==RemoteActionGitStage||action.Operation==RemoteActionGitUnstage||action.Operation==RemoteActionTaskList||
  action.Operation==RemoteActionTaskRunText||action.Operation==RemoteActionTaskRunReport||
  action.Operation==RemoteActionTaskManagedStart||action.Operation==RemoteActionTaskManagedStatus||action.Operation==RemoteActionTaskManagedCancel||action.Operation==RemoteActionTaskManagedOutput||
  action.Operation==RemoteActionGitStatusFull||action.Operation==RemoteActionGitCommitReview||action.Operation==RemoteActionGitCommit||action.Operation==RemoteActionGitBranchReview||action.Operation==RemoteActionGitSwitchReview||action.Operation==RemoteActionGitSwitchSameHead||action.Operation==RemoteActionGitBranchDeleteReview||action.Operation==RemoteActionGitBranchDelete||action.Operation==RemoteActionGitBranchCreate{
  maxReply=MaxEncryptedGitDiffFrame
 }
 if err!=nil||deadlineCtx.Err()!=nil||len(reply)<25||
  len(reply)>maxReply{
  p.Close();return "",denied
 }
 var content string
 switch action.Operation{
 case RemoteActionProjectFileRead:
  content,err=OpenRemoteFileReadResult(channel,reply,id)
 case RemoteActionProjectFilePreview:
  content,err=OpenRemoteFilePreviewResult(channel,reply,id)
 case RemoteActionProjectFileLines:
  content,err=OpenRemoteFileLinesResult(channel,reply,id)
 case RemoteActionProjectFileEditLine:
  content,err=OpenRemoteLargeLineEditResult(channel,reply,id)
 case RemoteActionProjectFileDeleteLine:
  content,err=OpenRemoteLargeLineDeleteResult(channel,reply,id)
 case RemoteActionProjectFileInsertLine:
  content,err=OpenRemoteLargeLineInsertResult(channel,reply,id)
 case RemoteActionProjectFileFindLines:
  content,err=OpenRemoteFindLinesResult(channel,reply,id)
 case RemoteActionProjectDirectoryList:
  content,err=OpenRemoteDirectoryListResult(channel,reply,id)
 case RemoteActionGitStatus:
  content,err=OpenRemoteGitStatusResult(channel,reply,id)
 case RemoteActionGitStatusFull:
  content,err=OpenRemoteGitStatusFullResult(channel,reply,id)
 case RemoteActionGitCommitReview:
  content,err=OpenRemoteGitCommitReviewResult(channel,reply,id)
 case RemoteActionGitCommit:
  content,err=OpenRemoteGitCommitResult(channel,reply,id)
 case RemoteActionGitBranchReview:
  content,err=OpenRemoteGitBranchReviewResult(channel,reply,id)
 case RemoteActionGitBranchCreate:
  content,err=OpenRemoteGitBranchCreateResult(channel,reply,id)
 case RemoteActionGitSwitchReview:
  content,err=OpenRemoteGitSwitchReviewResult(channel,reply,id)
 case RemoteActionGitSwitchSameHead:
  content,err=OpenRemoteGitSwitchResult(channel,reply,id)
 case RemoteActionGitBranchDeleteReview:
  content,err=OpenRemoteGitBranchDeleteReviewResult(channel,reply,id)
 case RemoteActionGitBranchDelete:
  content,err=OpenRemoteGitBranchDeleteResult(channel,reply,id)
 case RemoteActionGitDiff:
  content,err=OpenRemoteGitDiffResult(channel,reply,id)
 case RemoteActionGitLog:
  content,err=OpenRemoteGitLogResult(channel,reply,id)
 case RemoteActionGitBranches:
  content,err=OpenRemoteGitBranchesResult(channel,reply,id)
 case RemoteActionGitRemotes:
  content,err=OpenRemoteGitRemotesResult(channel,reply,id)
 case RemoteActionGitTrackingRefs:
  content,err=OpenRemoteGitTrackingRefsResult(channel,reply,id)
 case RemoteActionGitPullReview:
  content,err=OpenRemoteGitPullReviewResult(channel,reply,id)
 case RemoteActionGitPullChangesReview:
  content,err=OpenRemoteGitPullChangesResult(channel,reply,id)
 case RemoteActionGitPullCachedReview:
  content,err=OpenRemoteGitPullCachedReviewResult(channel,reply,id)
 case RemoteActionGitPullApplyCached:
  content,err=OpenRemoteGitPullApplyCachedResult(channel,reply,id)
 case RemoteActionGitPullApplyFiles:
  content,err=OpenRemoteGitPullApplyFilesResult(channel,reply,id)
 case RemoteActionGitFetchReview:
  content,err=OpenRemoteGitFetchReviewResult(channel,reply,id)
 case RemoteActionGitFetchApply:
  content,err=OpenRemoteGitFetchApplyResult(channel,reply,id)
 case RemoteActionGitPushReview:
  content,err=OpenRemoteGitPushReviewResult(channel,reply,id)
 case RemoteActionGitPushApply:
  content,err=OpenRemoteGitPushApplyResult(channel,reply,id)
 case RemoteActionGitStagedDiff:
  content,err=OpenRemoteGitStagedDiffResult(channel,reply,id)
 case RemoteActionGitHeadDiff:
  content,err=OpenRemoteGitHeadDiffResult(channel,reply,id)
 case RemoteActionGitStage:
  content,err=OpenRemoteGitStageResult(channel,reply,id)
 case RemoteActionGitUnstage:
  content,err=OpenRemoteGitUnstageResult(channel,reply,id)
 case RemoteActionTaskList:
  content,err=OpenRemoteTaskListResult(channel,reply,id)
 case RemoteActionProjectFileReplace:
  content,err=OpenRemoteFileReplaceResult(channel,reply,id)
 case RemoteActionProjectFileReplaceText:
  content,err=OpenRemoteTextEditResult(channel,reply,id)
 case RemoteActionTaskRun:
  content,err=OpenRemoteTaskRunResult(channel,reply,id)
 case RemoteActionTaskRunText:
  content,err=OpenRemoteTaskRunTextResult(channel,reply,id)
 case RemoteActionTaskManagedOutput:
  var w RemoteManagedOutput
  w,err=OpenRemoteManagedOutput(channel,reply,id)
  if err==nil{
   eof:="no"
   if w.EOF{eof="yes"}
   content=fmt.Sprintf("TASK_HANDLE %s\nOUTPUT_OFFSET %d\nOUTPUT_NEXT %d\nOUTPUT_TOTAL %d\nTASK_STATE %s\nOUTPUT_EOF %s\n-- TASK OUTPUT WINDOW --\n%s",
    w.Handle,w.Offset,w.Next,w.Total,w.State,eof,w.Data)
  }
 case RemoteActionTaskManagedStart,RemoteActionTaskManagedStatus,RemoteActionTaskManagedCancel:
  var result RemoteManagedTaskResult
  result,err=OpenRemoteManagedTaskResult(channel,reply,id)
  if err==nil {
   // A typed omission marker is present only for the host-confirmed
   // terminal status whose complete stdout cannot fit the E2EE result.
   // Do not render synthetic "complete" output or guess from stdout.
   omitted:=""
   if result.OutputOmitted{omitted="TASK_OUTPUT_OMITTED yes\n"}
   content=fmt.Sprintf("TASK_HANDLE %s\nTASK_STATE %s\nTASK_EXIT_CODE %d\n%s-- TASK OUTPUT --\n%s",
    result.Handle,result.State,result.ExitCode,omitted,result.Output)
  }
 case RemoteActionTaskRunReport:
  var result RemoteTaskReport
  result,err=OpenRemoteTaskReportResult(channel,reply,id)
  if err==nil{
   omitted:=""
   if result.OutputOmitted{omitted="TASK_OUTPUT_OMITTED yes\n"}
   content=fmt.Sprintf("TASK_STATUS %s\nTASK_EXIT_CODE %d\n%s-- COMPLETE TASK OUTPUT --\n%s",
    result.Status,result.ExitCode,omitted,result.Output)
  }
 default: err=ErrRemoteActionDenied
 }
 if err!=nil{p.Close();return "",denied}
 p.mu.Lock()
 valid:=!p.closed&&p.active&&deadlineCtx.Err()==nil&&channel.Established()
 p.mu.Unlock()
 if !valid{p.Close();return "",denied}
 return content,nil
}
