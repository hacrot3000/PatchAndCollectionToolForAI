package server

import "bletonfc/vscode_tasks_menu/internal/remotetransport"

// The Relay can be fully compromised and is NEVER a TaskDeck authorizer.
// An Agent-signed file-transfer readiness assertion can only raise the
// RELAY's opaque quota after the host has reduced its local principal to
// a single immutable, read-only file and a strict sequential 64-chunk budget.
//
// All TaskDeck password/key/RBAC + uncached Portal grant checks still run
// for EACH 4096-byte action in executeRemoteAuthenticatedAction.
func (session *remoteTaskDeckSession) fileTransferActive() bool {
 if session==nil{return false}
 session.transferMu.Lock()
 defer session.transferMu.Unlock()
 return session.transferActive&&!session.closed.Load()&&
  session.transferRounds>0&&session.transferRounds<=remotetransport.MaxScopedFileTransferRounds
}

// Before decoding permission-specific work, refuse ANY other operation
// once an authenticated read has entered file-transfer mode. This includes
// project.file.replace, task.run, session.ping and other read-only operations.
func (session *remoteTaskDeckSession) checkScopedFileTransfer(operation,path string,offset int64) error {
 denied:=remotetransport.ErrRemoteActionDenied
 if session==nil||session.closed.Load(){return denied}
 session.transferMu.Lock()
 defer session.transferMu.Unlock()
 if !session.transferActive{return nil}
 if operation!=remotetransport.RemoteActionProjectFileChunk||
  path!=session.transferPath||offset!=session.transferNext||
  session.transferNext>=session.transferSize||
  session.transferRounds>=remotetransport.MaxScopedFileTransferRounds{
  return denied
 }
 return nil
}

// Called ONLY after BOTH local/Portal authorization passes and the host's
// independent local audit succeeds, while the response is being sealed.
// The first valid result permanently binds filename, entire-file SHA,
// size, next monotonic offset and the maximum number of chunks.
func (session *remoteTaskDeckSession) commitScopedFileTransfer(
 operation,path string,offset int64,w remotetransport.FileWindow,
) error {
 denied:=remotetransport.ErrRemoteActionDenied
 if session==nil||session.closed.Load(){return denied}
 session.transferMu.Lock()
 defer session.transferMu.Unlock()
 if operation!=remotetransport.RemoteActionProjectFileChunk {
  if session.transferActive{return denied}
  return nil
 }
 if w.Offset!=offset||w.Size<0||w.Size>remotetransport.RemoteFileWindowMaxSize||
  len(w.Data)>remotetransport.RemoteFileChunkMaxBytes||
  int64(len(w.Data))>w.Size-offset||offset<0||
  offset>w.Size||len(w.FileSHA256)!=64 {
  return denied
 }
 if session.transferActive {
  if path!=session.transferPath||offset!=session.transferNext||
   w.Size!=session.transferSize||w.FileSHA256!=session.transferSHA||
   session.transferNext>=session.transferSize||
   session.transferRounds>=remotetransport.MaxScopedFileTransferRounds{
   return denied
  }
 }else{
  session.transferPath=path
  session.transferSize=w.Size
  session.transferSHA=w.FileSHA256
  session.transferActive=true
 }
 session.transferNext=offset+int64(len(w.Data))
 session.transferRounds++
 return nil
}
