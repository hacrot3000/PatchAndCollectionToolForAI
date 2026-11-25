//go:build linux || darwin

package server

import (
 "errors"
 "os"
 "path/filepath"
 "testing"
)

func TestAgentTurnLockRejectsLinkedAndWorldReadableInodes(t *testing.T){
 for _,kind:=range []string{"symlink","hardlink","public-file","directory"}{
  t.Run(kind,func(t *testing.T){
   s,id:=newAgentConversationFixture(t,"stateless")
   settings,err:=aiPrivatePath(s.Workspace);if err!=nil{t.Fatal(err)}
   dir:=filepath.Join(filepath.Dir(settings),"agent-sessions-"+aiWorkspaceHash(s.Workspace))
   path:=filepath.Join(dir,".agent-turn-"+id+".lock")
   target:=filepath.Join(t.TempDir(),"canary")
   if err=os.WriteFile(target,[]byte("do not touch"),0600);err!=nil{t.Fatal(err)}
   switch kind{
   case "symlink":
    if err=os.Symlink(target,path);err!=nil{t.Skipf("no symlinks: %v",err)}
   case "hardlink":
    if err=os.Link(target,path);err!=nil{t.Skipf("no hardlinks: %v",err)}
   case "public-file":
    if err=os.WriteFile(path,nil,0644);err!=nil{t.Fatal(err)}
    if err=os.Chmod(path,0644);err!=nil{t.Fatal(err)}
   case "directory":
    if err=os.Mkdir(path,0700);err!=nil{t.Fatal(err)}
   }
   release,err:=acquireAgentTurnOSLock(s.Workspace,id)
   if release!=nil||!errors.Is(err,errAgentTurnLockUnavailable){
    if release!=nil{release()}
    t.Fatalf("%s turn lock admitted unsafe file: %v",kind,err)
   }
   // Same high-level in-process+OS admission used by production must also
   // fail closed, never falling back to an in-memory lock.
   release,ok:=lockAgentTurn(s.Workspace,"local",id)
   if release!=nil||ok{
    if release!=nil{release()}
    t.Fatal("unsafe per-turn OS inode bypassed through lockAgentTurn")
   }
  })
 }
}

func TestAgentTurnLockRejectsDetachedOpenInode(t *testing.T){
 s,id:=newAgentConversationFixture(t,"stateless")
 settings,err:=aiPrivatePath(s.Workspace);if err!=nil{t.Fatal(err)}
 dir:=filepath.Join(filepath.Dir(settings),"agent-sessions-"+aiWorkspaceHash(s.Workspace))
 path:=filepath.Join(dir,".agent-turn-"+id+".lock")
 if err=os.WriteFile(path,nil,0600);err!=nil{t.Fatal(err)}
 f,err:=os.OpenFile(path,os.O_RDWR,0600);if err!=nil{t.Fatal(err)}
 defer f.Close()
 if !validateAgentTurnLockFile(f,path){t.Fatal("valid private lock refused")}
 old:=filepath.Join(dir,".old-turn-lock")
 if err=os.Rename(path,old);err!=nil{t.Fatal(err)}
 if err=os.WriteFile(path,nil,0600);err!=nil{t.Fatal(err)}
 if validateAgentTurnLockFile(f,path){
  t.Fatal("replaced turn lock inode was accepted")
 }
 release,err:=acquireAgentTurnOSLock(s.Workspace,id)
 if err!=nil{t.Fatalf("fresh lock file wrongly rejected: %v",err)}
 release()
}
