package server

import (
 "os"
 "os/exec"
 "sync"
 "time"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// One PTY per in-process authenticated E2EE principal. Never persisted, and
// never indexed by a token or ID supplied by PHP, Relay or native browser.
type remoteTerminalJob struct{
 ptyFile *os.File
 process *exec.Cmd
 cancel func()
 done chan struct{}
 workspaceRelease func()
 workspaceReleased chan struct{}
 capture *remoteLongTaskCapture
 started time.Time
 mu sync.Mutex
 state string
 cursor int
}
func (session *remoteTaskDeckSession) remoteTerminalSnapshot()(remotetransport.RemoteTerminalProgress,error){
 value,_,err:=session.remoteTerminalSnapshotWithMode(false)
 return value,err
}

// Native E2EE Poll must not lose PTY output when validation or AEAD Seal
// fails. Direct host snapshots retain consume-on-read behavior; the
// E2EE dispatcher commits a deferred cursor only after sealing.
func (session *remoteTaskDeckSession) remoteTerminalSnapshotWithMode(
 deferCursor bool,
)(remotetransport.RemoteTerminalProgress,func(),error){
 var zero remotetransport.RemoteTerminalProgress
 if session==nil||session.closed.Load(){
  return zero,nil,remotetransport.ErrRemoteActionDenied
 }
 session.terminalMu.Lock()
 job:=session.terminal
 session.terminalMu.Unlock()
 if job==nil||job.capture==nil{
  return zero,nil,remotetransport.ErrRemoteActionDenied
 }
 job.mu.Lock()
 from:=job.cursor
 output,next,truncated,more:=job.capture.next(from)
 state:=job.state
 if !deferCursor{job.cursor=next}
 job.mu.Unlock()
 result:=remotetransport.RemoteTerminalProgress{
  State:state,Output:output,Truncated:truncated,More:more,
 }
 if !deferCursor{return result,nil,nil}
 // A stale seal completion cannot rewind a cursor advanced by another
 // reader. The closure owns this original job, never a re-opened PTY.
 return result,func(){
  job.mu.Lock()
  if job.cursor==from{job.cursor=next}
  job.mu.Unlock()
 },nil
}
