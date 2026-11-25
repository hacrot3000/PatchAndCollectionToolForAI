//go:build linux || darwin

package agentchat

import (
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "syscall"
)

// ErrUnsafeAgentJournalLock means that the OS lock path cannot safely
// identify one private, stable inode. No journal read/compare/write may
// proceed without an authoritative flock on that inode.
var ErrUnsafeAgentJournalLock = errors.New("unsafe agent journal lock file")

// validateAgentJournalLockFile checks both the open descriptor and the
// currently named file. Run this BEFORE and AFTER acquiring flock: a
// contender may have opened an inode that was replaced while it waited.
// Never silently lock a symlink, hardlink alias or public/nonregular file.
func validateAgentJournalLockFile(f *os.File, path string) error {
 info,err:=f.Stat()
 if err!=nil{return fmt.Errorf("%w: open descriptor unavailable: %v",ErrUnsafeAgentJournalLock,err)}
 if !info.Mode().IsRegular()||info.Mode().Perm()&0077!=0{
  return ErrUnsafeAgentJournalLock
 }
 stat,ok:=info.Sys().(*syscall.Stat_t)
 if !ok||stat.Nlink!=1{return ErrUnsafeAgentJournalLock}
 named,err:=os.Lstat(path)
 if err!=nil{return fmt.Errorf("%w: named lock unavailable: %v",ErrUnsafeAgentJournalLock,err)}
 if !named.Mode().IsRegular()||named.Mode().Perm()&0077!=0||
  !os.SameFile(info,named){
  return ErrUnsafeAgentJournalLock
 }
 return nil
}

// lockStoreDir serializes all journal CAS on a dedicated, owner-private,
// stable local inode. This is distinct from the longer lived session-turn
// lock. It is NOT a distributed consensus protocol or a guarantee against
// hostile same-UID processes replacing a file AFTER these checks.
func lockStoreDir(dir string) (func(), error) {
 if dir==""||!filepath.IsAbs(dir){
  return nil,errors.New("absolute agent state directory required for locking")
 }
 path:=filepath.Join(dir,".agent-store.lock")
 // O_NOFOLLOW prevents following a pre-planted symlink to a different file.
 // The usual O_CREATE still creates a new private lock on first use.
 f,err:=os.OpenFile(path,os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW,0600)
 if err!=nil{return nil,fmt.Errorf("%w: open failed: %v",ErrUnsafeAgentJournalLock,err)}
 closeOnFail:=func(e error)(func(),error){_ =f.Close();return nil,e}
 if err=validateAgentJournalLockFile(f,path);err!=nil{return closeOnFail(err)}
 if err=syscall.Flock(int(f.Fd()),syscall.LOCK_EX);err!=nil{
  return closeOnFail(fmt.Errorf("%w: flock failed: %v",ErrUnsafeAgentJournalLock,err))
 }
 // A queued contender might have opened an earlier, now-unlinked inode.
 // In that case releasing the stale lock is mandatory: otherwise two
 // daemons could mutate the same canonical journal under different locks.
 if err=validateAgentJournalLockFile(f,path);err!=nil{
  _=syscall.Flock(int(f.Fd()),syscall.LOCK_UN)
  return closeOnFail(err)
 }
 return func(){
  _=syscall.Flock(int(f.Fd()),syscall.LOCK_UN)
  _=f.Close()
 },nil
}
