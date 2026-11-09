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
 capture *remoteLongTaskCapture
 started time.Time
 mu sync.Mutex
 state string
 cursor int
}
func (session *remoteTaskDeckSession) remoteTerminalSnapshot()(remotetransport.RemoteTerminalProgress,error){
 var zero remotetransport.RemoteTerminalProgress
 if session==nil||session.closed.Load(){return zero,remotetransport.ErrRemoteActionDenied}
 session.terminalMu.Lock()
 job:=session.terminal
 session.terminalMu.Unlock()
 if job==nil{return zero,remotetransport.ErrRemoteActionDenied}
 job.mu.Lock()
 defer job.mu.Unlock()
 output,next,truncated,more:=job.capture.next(job.cursor)
 job.cursor=next
 return remotetransport.RemoteTerminalProgress{
  State:job.state,Output:output,Truncated:truncated,More:more,
 },nil
}
