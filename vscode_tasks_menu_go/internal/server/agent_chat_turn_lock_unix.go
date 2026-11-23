//go:build linux || darwin

package server

import (
 "errors"
 "os"
 "path/filepath"
 "syscall"
)

var errAgentTurnLockUnavailable = errors.New("agent session OS lease unavailable")

// acquireAgentTurnOSLock is a nonblocking per-session lease. It spans provider
// network I/O, Auto-continue and native feedback decisions, unlike the short
// Store.withLock journal CAS. A second daemon sharing this private state
// directory cannot start the same Agent turn. The kernel drops flock on
// process death; it does NOT resolve a durable SENDING/UNKNOWN egress.
func acquireAgentTurnOSLock(workspace,id string)(func(),error){
 if len(id)!=32{return nil,errAgentTurnLockUnavailable}
 for _,c:=range id{
  if c<'0'||c>'9'&&c<'a'||c>'f'{return nil,errAgentTurnLockUnavailable}
 }
 settings,err:=aiPrivatePath(workspace)
 if err!=nil{return nil,errAgentTurnLockUnavailable}
 dir:=filepath.Join(filepath.Dir(settings),"agent-sessions-"+aiWorkspaceHash(workspace))
 parent,err:=os.Lstat(dir)
 if err!=nil||!parent.IsDir()||parent.Mode().Perm()&0077!=0{
  return nil,errAgentTurnLockUnavailable
 }
 path:=filepath.Join(dir,".agent-turn-"+id+".lock")
 f,err:=os.OpenFile(path,os.O_RDWR|os.O_CREATE|syscall.O_NOFOLLOW,0600)
 if err!=nil{return nil,errAgentTurnLockUnavailable}
 closeOnFail:=func()(func(),error){_ = f.Close();return nil,errAgentTurnLockUnavailable}
 info,err:=f.Stat()
 if err!=nil||!info.Mode().IsRegular()||info.Mode().Perm()&0077!=0{
  return closeOnFail()
 }
 current,err:=os.Lstat(path)
 if err!=nil||!current.Mode().IsRegular()||!os.SameFile(info,current){
  return closeOnFail()
 }
 if err=syscall.Flock(int(f.Fd()),syscall.LOCK_EX|syscall.LOCK_NB);err!=nil{
  return closeOnFail()
 }
 return func(){
  _=syscall.Flock(int(f.Fd()),syscall.LOCK_UN)
  _=f.Close()
 },nil
}
