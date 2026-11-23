//go:build linux || darwin

package server

import (
 "context"
 "crypto/sha256"
 "encoding/hex"
 "errors"
 "os"
 "path/filepath"
 "syscall"
 "time"

 "bletonfc/vscode_tasks_menu/internal/secretstore"
)

var errAgentSharedAdmissionUnsafe=errors.New("unsafe private Agent capacity lease")
var errAgentSharedAdmissionBusy=errors.New("Agent capacity slot busy")

// Slots are private, permanently named flock inodes. Unlinking a released
// lease would allow two daemons to lock different inodes for the same slot.
// This constrains provider capacity across local TaskDeck daemons sharing
// one user-owned private configuration directory. Neither the owner nor any
// provider-controlled string is embedded directly into a filename.
func agentSharedCapacityDir()(string,error){
 base,err:=secretstore.DefaultDir()
 if err!=nil||!filepath.IsAbs(base){return "",errAgentSharedAdmissionUnsafe}
 if err=os.MkdirAll(base,0700);err!=nil{return "",err}
 info,err:=os.Lstat(base)
 if err!=nil||!info.IsDir()||info.Mode().Perm()&0077!=0{
  return "",errAgentSharedAdmissionUnsafe
 }
 dir:=filepath.Join(base,"agent-provider-capacity")
 if err=os.Mkdir(dir,0700);err!=nil&&!os.IsExist(err){return "",err}
 info,err=os.Lstat(dir)
 if err!=nil||!info.IsDir()||info.Mode().Perm()&0077!=0{
  return "",errAgentSharedAdmissionUnsafe
 }
 return dir,nil
}

// Return BUSY only for an existing competing flock; unsafe files, symlinks,
// wrong permissions and other filesystem errors fail closed without waiting.
func tryAgentCapacityLease(path string)(func(),error){
 f,err:=os.OpenFile(path,os.O_RDWR|os.O_CREATE|syscall.O_NOFOLLOW,0600)
 if err!=nil{return nil,errAgentSharedAdmissionUnsafe}
 closeBad:=func()(func(),error){_ = f.Close();return nil,errAgentSharedAdmissionUnsafe}
 fi,err:=f.Stat()
 if err!=nil||!fi.Mode().IsRegular()||fi.Mode().Perm()&0077!=0{return closeBad()}
 if err=syscall.Flock(int(f.Fd()),syscall.LOCK_EX|syscall.LOCK_NB);err!=nil{
  _=f.Close()
  if errors.Is(err,syscall.EWOULDBLOCK)||errors.Is(err,syscall.EAGAIN){
   return nil,errAgentSharedAdmissionBusy
  }
  return nil,errAgentSharedAdmissionUnsafe
 }
 observed,err:=os.Lstat(path)
 if err!=nil||!observed.Mode().IsRegular()||!os.SameFile(fi,observed){
  _=syscall.Flock(int(f.Fd()),syscall.LOCK_UN)
  return closeBad()
 }
 return func(){
  _=syscall.Flock(int(f.Fd()),syscall.LOCK_UN)
  _=f.Close()
 },nil
}

// Per-owner slots cap 2 active turns for the SAME workspace+owner; global
// slots cap 4 active provider turns across ALL workspaces of this installation.
// Fixed lock order global->owner prevents deadlock. After a failed owner
// acquisition, the global slot is immediately released before trying again.
func tryAgentSharedCapacity(dir,ownerDigest string)(func(),bool,error){
 for g:=0;g<agentMaxConcurrentGlobal;g++{
  gl,e:=tryAgentCapacityLease(filepath.Join(dir,"global-"+stringSlot(g)+".lock"))
  if errors.Is(e,errAgentSharedAdmissionBusy){continue}
  if e!=nil{return nil,false,e}
  for i:=0;i<agentMaxConcurrentPerOwner;i++{
   ol,e:=tryAgentCapacityLease(filepath.Join(dir,"owner-"+ownerDigest+"-"+stringSlot(i)+".lock"))
   if errors.Is(e,errAgentSharedAdmissionBusy){continue}
   if e!=nil{gl();return nil,false,e}
   return func(){ol();gl()},true,nil
  }
  gl()
  // No owner capacity is available; do not cycle through the remaining
  // global slots while another owner could use them.
  return nil,false,nil
 }
 return nil,false,nil
}

func stringSlot(i int)string{
 return string(rune('0'+i))
}

func acquireAgentCrossProcessCapacity(ctx context.Context,owner string)(func(),bool){
 if ctx==nil||ctx.Err()!=nil||owner==""{return nil,false}
 dir,err:=agentSharedCapacityDir()
 if err!=nil{return nil,false}
 digest:=sha256.Sum256([]byte(owner))
 name:=hex.EncodeToString(digest[:])
 ticker:=time.NewTicker(70*time.Millisecond)
 defer ticker.Stop()
 for {
  if ctx.Err()!=nil{return nil,false}
  release,ok,err:=tryAgentSharedCapacity(dir,name)
  if err!=nil{return nil,false}
  if ok{
   if ctx.Err()!=nil{release();return nil,false}
   return release,true
  }
  select{
  case <-ctx.Done():return nil,false
  case <-ticker.C:
  }
 }
}
