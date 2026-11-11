package server

import (
 "context"
 "errors"
 "os"
 "os/exec"
 "path/filepath"
 "runtime"
 "strconv"
 "strings"
 "sync"
 "time"
 "unicode"
 "unicode/utf8"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

const remoteLongTaskMaxDuration=90*time.Second
const remoteLongTaskMaxCaptureBytes=16*1024
const remoteLongTaskMaxChunkBytes=128
const remoteLongTaskPollWait=7*time.Second

// One job per authenticated TaskDeck E2EE session. No background work is
// transferred to PHP/Relay; all execution state, output and cancellation
// remain on the Agent. Start/poll/cancel are staged host primitives until
// individually permission-gated E2EE actions and native GUI land.
type remoteLongTaskJob struct {
 taskID int
 started time.Time
 cancel context.CancelFunc
 done chan struct{}
 // Held until the last host process has exited, including after session
 // logout. Never released by an unauthenticated Poll/Cancel/Clear action.
 workspaceRelease func()
 capture *remoteLongTaskCapture
 mu sync.Mutex
 status string
 // A successful cancellation request is one-shot for this host job.
 // A duplicate or a request after completion must not re-arm the
 // context or report a fake "cancel_requested" E2EE acknowledgement.
 cancelRequested bool
 // Wait has reaped the process, but the exclusive workspace lease may
 // still be retiring. Do not accept Cancel during this transition even
 // though Poll must continue reporting running until release completes.
 finalizing bool
 exitCode int
 outputCursor int
 workflowStep int
 workflowTotal int
 workflowAttempt int
 workflowAttemptTotal int
 workflowStepStatus string
 workflowStepHistory string
 workflowStepDetails []string
 workflowParallel bool
 workflowRunID string // Random per execution; never sent as a command token.
}

type remoteLongTaskCapture struct {
 mu sync.Mutex
 data []byte
 truncated bool
 notify chan struct{}
 // Terminal-only rolling output. Long-running Tasks retain their
 // original first-16KiB capture semantics and cursor behavior.
 rolling bool
 rollingBase int // Absolute UTF-8 byte offset of data[0].
 rollingPending []byte // Incomplete UTF-8 sequence across PTY reads.
 finalized bool // Prevent a tardy PTY reader from mutating a final Poll.
}
func newRemoteLongTaskCapture()*remoteLongTaskCapture{
 return &remoteLongTaskCapture{data:make([]byte,0,512),notify:make(chan struct{},1)}
}
// A remote PTY must keep yielding new output after its first 16KiB have
// been displayed. This bounded sliding window stores only sanitized
// UTF-8, never shell bytes or credentials in a persistent audit.
func newRemoteTerminalCapture()*remoteLongTaskCapture{
 c:=newRemoteLongTaskCapture()
 c.rolling=true
 return c
}
func (c *remoteLongTaskCapture)Write(p []byte)(int,error){
 n:=len(p)
 c.mu.Lock()
 if !c.finalized{
  if c.rolling{
   c.writeTerminalRollingLocked(p)
  }else{
   space:=remoteLongTaskMaxCaptureBytes-len(c.data)
   if space>0{
    if space>len(p){space=len(p)}
    c.data=append(c.data,p[:space]...)
   }
   if space<n{c.truncated=true}
  }
 }
 c.mu.Unlock()
 select{case c.notify<-struct{}{}:default:}
 return n,nil // always drain the command's stdout/stderr
}

// Sanitize every complete UTF-8 rune incrementally; io.Copy may split a
// rune between Write calls. Prune in bounded batches at rune boundaries.
// Retain a monotonic absolute cursor so a deferred, authenticated E2EE
// Seal cannot accidentally replay or skip newer output after compaction.
func(c *remoteLongTaskCapture)writeTerminalRollingLocked(p []byte){
 for len(p)>0{
  count:=len(p)
  if count>4096{count=4096}
  block:=make([]byte,0,len(c.rollingPending)+count)
  block=append(block,c.rollingPending...)
  block=append(block,p[:count]...)
  c.rollingPending=c.rollingPending[:0]
  for len(block)>0{
   if !utf8.FullRune(block){
    c.rollingPending=append(c.rollingPending,block...)
    break
   }
   r,width:=utf8.DecodeRune(block)
   if (r!=utf8.RuneError||width>1)&&
    (r=='\n'||r=='\t'||(!unicode.IsControl(r)&&
     !unicode.In(r,unicode.Cf,unicode.Zl,unicode.Zp))){
    c.data=append(c.data,block[:width]...)
   }
   block=block[width:]
  }
  if len(c.data)>remoteLongTaskMaxCaptureBytes{
   cut:=len(c.data)-remoteLongTaskMaxCaptureBytes
   for cut<len(c.data)&&!utf8.RuneStart(c.data[cut]){cut++}
   c.rollingBase+=cut
   copy(c.data,c.data[cut:])
   c.data=c.data[:len(c.data)-cut]
  }
  p=p[count:]
 }
}

// Once the PTY has been closed and its output reader drained (or timed
// out), freeze the capture before exposing 'exited'. A late io.Copy
// Write must never append data after the client saw a final state.
func(c *remoteLongTaskCapture)finishRemoteTerminal(incomplete bool){
 c.mu.Lock()
 if incomplete||len(c.rollingPending)>0{c.truncated=true}
 c.rollingPending=nil
 c.finalized=true
 c.mu.Unlock()
}

func sanitizeLongTaskOutput(raw []byte)string{
 clear:=strings.ToValidUTF8(string(raw),"")
 return strings.Map(func(r rune)rune{
  if r=='\n'||r=='\t'{return r}
  if unicode.IsControl(r)||unicode.In(r,unicode.Cf,unicode.Zl,unicode.Zp){return -1}
  return r
 },clear)
}
func validLongTaskStatus(status string)bool{
 return status=="running"||status=="completed"||status=="failed"||
  status=="cancelled"||status=="timed_out"
}

// Output chunks are each bounded, text-only, and start at a Unicode
// boundary. Each poll consumes the next chunk; a full capture drops new
// bytes rather than allocating unbounded memory. Nothing is logged in DB.
func (c *remoteLongTaskCapture)next(cursor int)(string,int,bool,bool){
 return c.nextBounded(cursor,remoteLongTaskMaxChunkBytes)
}
func (c *remoteLongTaskCapture)nextBounded(cursor,limit int)(string,int,bool,bool){
 c.mu.Lock()
 defer c.mu.Unlock()
 if c.rolling{
  lost:=cursor<c.rollingBase
  if lost{cursor=c.rollingBase}
  last:=c.rollingBase+len(c.data)
  if cursor>last{cursor=last}
  text:=string(c.data[cursor-c.rollingBase:])
  end:=cursor
  for index,r:=range text{
   width:=utf8.RuneLen(r)
   if width<1||index+width>limit{break}
   end=cursor+index+width
  }
  return text[:end-cursor],end,lost||c.truncated,end<last
 }
 clean:=sanitizeLongTaskOutput(c.data)
 if cursor<0||cursor>len(clean){cursor=len(clean)}
 end:=cursor
 for index,r:=range clean[cursor:]{
  width:=utf8.RuneLen(r)
  if width<1||index+width>limit{break}
  end=cursor+index+width
 }
 return clean[cursor:end],end,c.truncated,end<len(clean)
}

type remoteLongTaskSnapshot struct {
 TaskID int
 Status string
 ExitCode int
 Output string
 Truncated bool
 More bool // Captured output still unread in the host-owned cursor.
 Step int
 StepTotal int
 Attempt int
 AttemptTotal int
 StepStatus string
 StepHistory string
 StepDetails string
 Parallel bool
}

// Must be entered only from TaskDeck's verified tasks.run role, live
// uncached Portal authorization, approved native key and E2EE dispatcher.
// It is not a generic HTTP command runner. Even then, the host re-verifies
// the previously published Task.ID and exact .vscode/tasks.json bytes.
func (s *Server) startRemoteLongTask(
 ctx context.Context,session *remoteTaskDeckSession,
 taskID,choice int,textInput string,
)error{
 denied:=remotetransport.ErrRemoteActionDenied
 if s==nil||ctx==nil||ctx.Err()!=nil||session==nil||
  runtime.GOOS=="windows"||
  os.Getenv("TASKDECK_REMOTE_TASKS_PREVIEW")!="1"||
  os.Getenv("TASKDECK_REMOTE_TASKS_RUN")!="1"||
  os.Getenv("TASKDECK_REMOTE_TASKS_LONG")!="1"||
  session.closed.Load()||session.channel==nil||!session.channel.Established()||
  !session.taskPageReady||!session.taskIssuedIDs[taskID]{
  return denied
 }
 // Match the PTY lock order (terminalMu -> longTaskMu). A terminal
 // opened in this authenticated session must exclude task execution in
 // both directions, including a concurrent terminal-open request.
 session.terminalMu.Lock()
 defer session.terminalMu.Unlock()
 if session.terminal!=nil{return denied}
 session.longTaskMu.Lock()
 defer session.longTaskMu.Unlock()
 if session.longTask!=nil{return denied} // one job, never auto-overwrite
 releaseWorkspace,err:=reserveRemoteWorkspaceExecution(ctx,s.Workspace)
 if err!=nil{return denied}
 launched:=false
 defer func(){if !launched{releaseWorkspace()}}()
 spec,_,err:=prepareRemoteTaskExecutionInput(
  ctx,s.Workspace,session.taskPageHash,taskID,choice,false,textInput)
 if err!=nil||spec.TaskID!=taskID{return denied}
 root,err:=filepath.EvalSymlinks(s.Workspace)
 if err!=nil{return denied}
 root,err=filepath.Abs(root)
 if err!=nil{return denied}
 cwd,err:=filepath.EvalSymlinks(spec.Cwd)
 if err!=nil{return denied}
 rel,err:=filepath.Rel(root,cwd)
 if err!=nil||filepath.IsAbs(rel)||rel==".."||
  strings.HasPrefix(rel,".."+string(filepath.Separator)){return denied}
 if ctx.Err()!=nil||session.closed.Load(){return denied}
 // The irreversible attempted audit is always BEFORE cmd.Start.
 if err:=s.auditRemoteTaskRunAttempt(ctx,session,taskID);err!=nil{return denied}
 jobCtx,cancel:=context.WithTimeout(context.Background(),remoteLongTaskMaxDuration)
 capture:=newRemoteLongTaskCapture()
 command:=exec.CommandContext(jobCtx,spec.Command,spec.Args...)
 command.Dir=cwd
 command.Env=spec.Env
 command.Stdout=capture
 command.Stderr=capture
 prepareGitJobProcess(command)
 command.Cancel=func()error{killGitJobProcess(command);return nil}
 command.WaitDelay=time.Second
 if ctx.Err()!=nil||session.closed.Load()||session.portalGrant==nil||
  !session.portalGrant.ActiveForSession(session.channel){
  cancel();return denied
 }
 if err:=command.Start();err!=nil{
  cancel()
  s.auditRemoteTaskRunFinish(session,taskID,"failed",-1,0)
  return denied
 }
 job:=&remoteLongTaskJob{
  taskID:taskID,started:time.Now(),cancel:cancel,
  done:make(chan struct{}),capture:capture,status:"running",exitCode:-1,
  workspaceRelease:releaseWorkspace,
 }
 session.longTask=job
 launched=true
 go func(){
  select{
  case <-session.lifetimeDone:cancel()
  case <-session.portalGrant.Done():cancel()
  case <-session.channel.Done():cancel()
  case <-job.done:
  }
 }()
 go func(){
  processErr:=command.Wait()
  killGitJobProcess(command)
  // The subprocess is already reaped; deny any further Cancel even while
  // result classification and the mandatory finish audit are pending.
  acceptedCancel:=job.beginFinalizing()
  exitCode:=0
  if processErr!=nil {
   var exit *exec.ExitError
   if errors.As(processErr,&exit)&&exit.ExitCode()>=0&&exit.ExitCode()<=255{
    exitCode=exit.ExitCode()
   }else{exitCode=-1}
  }
  status:="completed"
  if errors.Is(jobCtx.Err(),context.DeadlineExceeded){
   status="timed_out"
  }else if acceptedCancel||jobCtx.Err()!=nil||session.closed.Load(){status="cancelled"
  }else if processErr!=nil{status="failed"}
  // A revocation cannot prevent a host-local final audit attempt.
  s.auditRemoteTaskRunFinish(session,taskID,status,exitCode,time.Since(job.started))
  job.publishCompletion(status,exitCode)
  cancel()
 }()
 return nil
}

// Release the canonical workspace lease BEFORE making a final status
// observable to Poll. The previous order published completed/failed/
// cancelled and only then released the OS lock, so a GUI could receive
// a terminal reply yet its next authenticated Task/Git action would be
// spuriously denied as busy. Publication, done and capture notification
// are ordered AFTER release; a running snapshot may still be returned
// while the host is in the final cleanup phase.
// beginFinalizing closes the cancellation admission window BEFORE final
// result classification/audit. Cancel arriving first may still win; once
// this lock is released a dead process can never acknowledge new Cancel.
// The snapshot is taken under the SAME lock as cancelRemoteLongTask:
// if Cancel won admission immediately before process exit, retain its
// acknowledged intent even if context cancellation is still in flight.
func (job *remoteLongTaskJob)beginFinalizing()bool{
 job.mu.Lock()
 job.finalizing=true
 acceptedCancel:=job.cancelRequested
 job.mu.Unlock()
 return acceptedCancel
}

func (job *remoteLongTaskJob)publishCompletion(status string,exitCode int){
 job.beginFinalizing()
 job.workspaceRelease()
 job.mu.Lock()
 job.status=status
 job.exitCode=exitCode
 job.mu.Unlock()
 close(job.done)
 select{case job.capture.notify<-struct{}{}:default:}
}

// Polling never starts a process or accepts an offset/filename from client.
// One bounded output chunk is delivered per read, and a subsequent poll may
// retrieve the next. A done job remains available until explicit cleanup.
// These fields are Agent-owned, not parsed from arbitrary child stdout.
// Job and cursor mutations share the same mutex, so no poll can see a
// mixed step index/attempt from different attempts.
func (job *remoteLongTaskJob) updateWorkflowStep(index,total,attempt,attemptTotal int,
 state string,exitCode int,elapsed time.Duration){
 job.mu.Lock()
 job.workflowStep=index
 job.workflowTotal=total
 job.workflowAttempt=attempt
 job.workflowAttemptTotal=attemptTotal
 job.workflowStepStatus=state
 if index>=1&&index<=total&&len(job.workflowStepHistory)==total{
  code:=byte('?')
  switch state{
  case "running":code='R'
  case "completed":code='C'
  case "skipped":code='S'
  case "failed":code='F'
  case "cancelled":code='X'
  case "timed_out":code='T'
  }
  if code!='?'{
   history:=[]byte(job.workflowStepHistory)
   history[index-1]=code
   job.workflowStepHistory=string(history)
   if len(job.workflowStepDetails)==total{
    if state=="running"||state=="skipped"{
     job.workflowStepDetails[index-1]="-"
    }else{
     millis:=elapsed.Milliseconds()
     if millis<0{millis=0}
     if millis>90000{millis=90000}
     if exitCode< -1||exitCode>255{exitCode=-1}
     job.workflowStepDetails[index-1]=strconv.Itoa(exitCode)+":"+
      strconv.FormatInt(millis,10)
    }
   }
  }
 }
 job.mu.Unlock()
 select{case job.capture.notify<-struct{}{}:default:}
}

func (session *remoteTaskDeckSession) pollRemoteLongTask(
 ctx context.Context,taskID int,
)(remoteLongTaskSnapshot,error){
 return session.pollRemoteLongTaskWithWait(ctx,taskID,remoteLongTaskPollWait)
}

// E2EE Polls must not consume output before the authenticated response
// has been sealed; a failed seal must leave the same bytes for next Poll.
func (session *remoteTaskDeckSession) pollRemoteLongTaskForSeal(
 ctx context.Context,taskID int,
)(remoteLongTaskSnapshot,func(),error){
 return session.pollRemoteLongTaskWithWaitMode(
  ctx,taskID,remoteLongTaskPollWait,true)
}

// A bounded wait is injected only by host-side regression tests; the E2EE
// action always uses remoteLongTaskPollWait. A missed/coalesced capture
// notification must not turn an already buffered chunk into a false empty
// timeout response.
func (session *remoteTaskDeckSession)pollRemoteLongTaskWithWait(
 ctx context.Context,taskID int,wait time.Duration,
)(remoteLongTaskSnapshot,error){
 snapshot,_,err:=session.pollRemoteLongTaskWithWaitMode(
  ctx,taskID,wait,false)
 return snapshot,err
}

// Both normal and deadline-returned Polls must use the SAME payload
// limit. Five-step sequential workflows carry extra signed history and
// detail fields; reduce only their output chunk to preserve the 512-byte
// authenticated envelope. Parallel joins remain capped at four steps.
func remoteLongTaskWorkflowChunkLimit(step,total int,parallel bool)int{
 if step==0{return remoteLongTaskMaxChunkBytes}
 if total==5{return 32}
 if parallel&&total==4{return 48}
 return 64
}

// Host Poll consumes immediately, while E2EE returns a callback which
// advances the cursor only after successful AEAD Seal. E2EE taskPageMu
// serializes authenticated task operations through sealing.
func (session *remoteTaskDeckSession)pollRemoteLongTaskWithWaitMode(
 ctx context.Context,taskID int,wait time.Duration,deferCursor bool,
)(remoteLongTaskSnapshot,func(),error){
 denied:=remoteLongTaskSnapshot{}
 if session==nil||ctx==nil||ctx.Err()!=nil||session.closed.Load(){
  return denied,nil,remotetransport.ErrRemoteActionDenied
 }
 session.longTaskMu.Lock()
 job:=session.longTask
 session.longTaskMu.Unlock()
 if job==nil||job.taskID!=taskID{return denied,nil,remotetransport.ErrRemoteActionDenied}
 deadlineCtx,cancel:=context.WithTimeout(ctx,wait)
 defer cancel()
 // Called with job.mu held. A stale deferred commit cannot overwrite
 // a cursor already advanced by another host reader.
 consume:=func(from,to int)func(){
  if !deferCursor{
   job.outputCursor=to
   return nil
  }
  return func(){
   job.mu.Lock()
   if job.outputCursor==from{job.outputCursor=to}
   job.mu.Unlock()
  }
 }
 for{
  // Serialize snapshot and cursor consumption. Two concurrent native GUI
  // polls must not both send the same chunk or rewind an advanced cursor.
  job.mu.Lock()
  status,exit,cursor:=job.status,job.exitCode,job.outputCursor
  state:=remoteLongTaskSnapshot{
   TaskID:taskID,Status:status,ExitCode:exit,
   Step:job.workflowStep,StepTotal:job.workflowTotal,
   Attempt:job.workflowAttempt,AttemptTotal:job.workflowAttemptTotal,
   StepStatus:job.workflowStepStatus,
   StepHistory:job.workflowStepHistory,
   StepDetails:strings.Join(job.workflowStepDetails,"|"),
   Parallel:job.workflowParallel&&job.workflowStep>0,
  }
  if state.Step==0{state.StepHistory="";state.StepDetails=""}
  limit:=remoteLongTaskWorkflowChunkLimit(
   state.Step,state.StepTotal,state.Parallel)
  chunk,next,truncated,more:=job.capture.nextBounded(cursor,limit)
  if chunk!=""||status!="running"{
   if ctx.Err()!=nil||session.closed.Load(){
    job.mu.Unlock()
    return denied,nil,remotetransport.ErrRemoteActionDenied
   }
   commit:=consume(cursor,next)
   job.mu.Unlock()
   state.Output=chunk
   state.Truncated=truncated
   state.More=more
   return state,commit,nil
  }
  job.mu.Unlock()
  select{
  case <-deadlineCtx.Done():
   if ctx.Err()!=nil||session.closed.Load(){return denied,nil,remotetransport.ErrRemoteActionDenied}
   job.mu.Lock()
   state=remoteLongTaskSnapshot{
    TaskID:taskID,Status:job.status,ExitCode:job.exitCode,
    Step:job.workflowStep,StepTotal:job.workflowTotal,
    Attempt:job.workflowAttempt,AttemptTotal:job.workflowAttemptTotal,
    StepStatus:job.workflowStepStatus,
    StepHistory:job.workflowStepHistory,
    StepDetails:strings.Join(job.workflowStepDetails,"|"),
   Parallel:job.workflowParallel&&job.workflowStep>0,
   }
   if state.Step==0{state.StepHistory="";state.StepDetails=""}
   // A notification is coalesced (one-slot channel) and can be consumed
   // by another concurrent Poll. Recheck the capture under the same
   // cursor lock before returning an empty 7-second timeout snapshot.
   limit:=remoteLongTaskWorkflowChunkLimit(
    state.Step,state.StepTotal,state.Parallel)
   cursor:=job.outputCursor
   chunk,next,truncated,more:=job.capture.nextBounded(cursor,limit)
   if ctx.Err()!=nil||session.closed.Load(){
    job.mu.Unlock()
    return denied,nil,remotetransport.ErrRemoteActionDenied
   }
   commit:=consume(cursor,next)
   job.mu.Unlock()
   state.Output=chunk
   state.Truncated=truncated
   state.More=more
   return state,commit,nil
  case <-job.capture.notify:
  case <-job.done:
  }
 }
}

func (session *remoteTaskDeckSession) cancelRemoteLongTask(taskID int)error{
 if session==nil||session.closed.Load(){return remotetransport.ErrRemoteActionDenied}
 session.longTaskMu.Lock()
 defer session.longTaskMu.Unlock()
 job:=session.longTask
 if job==nil||job.taskID!=taskID{return remotetransport.ErrRemoteActionDenied}
 job.mu.Lock()
 // The waiter marks finalizing immediately after process exit and
 // before releasing the workspace lock. An authenticated Cancel in
 // that gap must not acknowledge cancellation of a dead process.
 if job.status!="running"||job.finalizing||job.cancelRequested {
  job.mu.Unlock()
  return remotetransport.ErrRemoteActionDenied
 }
 job.cancelRequested=true
 job.mu.Unlock()
 job.cancel()
 return nil
}

func (session *remoteTaskDeckSession) clearRemoteLongTask(taskID int)error{
 if session==nil||session.closed.Load(){return remotetransport.ErrRemoteActionDenied}
 session.longTaskMu.Lock()
 defer session.longTaskMu.Unlock()
 job:=session.longTask
 if job==nil||job.taskID!=taskID{return remotetransport.ErrRemoteActionDenied}
 select{
 case <-job.done:
  job.cancel()
  session.longTask=nil
  return nil
 default:
  return remotetransport.ErrRemoteActionDenied
 }
}
