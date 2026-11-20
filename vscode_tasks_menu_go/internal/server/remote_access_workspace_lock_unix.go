//go:build linux || darwin

package server

import (
 "errors"
 "os"
 "sync"
 "syscall"
)

// Lock the canonical workspace DIRECTORY inode, not a project-owned .lock
// file. A separate TaskDeck process opening the same directory obtains
// an independent descriptor and must fail LOCK_NB while a task/Git/PTY
// action holds it. No path is created and no stale lock can survive a
// daemon crash: the kernel automatically closes process descriptors.
// A directory swap between Stat and Open fails closed by inode identity.
func acquireRemoteWorkspaceOSLock(root string,expected os.FileInfo)(func(),error){
 if root==""||expected==nil||!expected.IsDir(){
  return nil,errors.New("invalid workspace for advisory lock")
 }
 dir,err:=os.Open(root)
 if err!=nil{return nil,err}
 opened,err:=dir.Stat()
 if err!=nil||!opened.IsDir()||!os.SameFile(expected,opened){
  _=dir.Close()
  return nil,errors.New("workspace changed during lock admission")
 }
 if err:=syscall.Flock(int(dir.Fd()),syscall.LOCK_EX|syscall.LOCK_NB);err!=nil{
  _=dir.Close()
  return nil,err
 }
 var once sync.Once
 return func(){
  once.Do(func(){
   _=syscall.Flock(int(dir.Fd()),syscall.LOCK_UN)
   _=dir.Close()
  })
 },nil
}
