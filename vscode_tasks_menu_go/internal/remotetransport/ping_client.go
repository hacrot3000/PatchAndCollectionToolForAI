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
  action.Operation==RemoteActionGitLog||action.Operation==RemoteActionGitBranches||action.Operation==RemoteActionGitRemotes||action.Operation==RemoteActionGitTrackingRefs||action.Operation==RemoteActionGitPullReview||action.Operation==RemoteActionGitPullChangesReview||action.Operation==RemoteActionGitPullCachedReview||action.Operation==RemoteActionGitPullApplyCached||action.Operation==RemoteActionGitPullApplyFiles||
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
