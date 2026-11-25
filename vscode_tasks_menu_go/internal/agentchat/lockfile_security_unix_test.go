//go:build linux || darwin

package agentchat

import (
 "bytes"
 "errors"
 "os"
 "path/filepath"
 "testing"
)

func TestAgentJournalLockRejectsSymlinkHardlinkAndPublicFiles(t *testing.T){
 for _,kind:=range []string{"symlink","hardlink","public-file","directory"}{
  t.Run(kind,func(t *testing.T){
   dir:=filepath.Join(t.TempDir(),"private")
   if err:=os.MkdirAll(dir,0700);err!=nil{t.Fatal(err)}
   path:=filepath.Join(dir,".agent-store.lock")
   target:=filepath.Join(t.TempDir(),"unrelated-private")
   canary:=[]byte("unrelated private canary — do not touch")
   if err:=os.WriteFile(target,canary,0600);err!=nil{t.Fatal(err)}
   switch kind{
   case "symlink":
    if err:=os.Symlink(target,path);err!=nil{t.Skipf("symlink unavailable: %v",err)}
   case "hardlink":
    if err:=os.Link(target,path);err!=nil{t.Skipf("hardlink unavailable: %v",err)}
   case "public-file":
    if err:=os.WriteFile(path,[]byte("public"),0644);err!=nil{t.Fatal(err)}
    if err:=os.Chmod(path,0644);err!=nil{t.Fatal(err)}
   case "directory":
    if err:=os.Mkdir(path,0700);err!=nil{t.Fatal(err)}
   }
   unlock,err:=lockStoreDir(dir)
   if unlock!=nil||!errors.Is(err,ErrUnsafeAgentJournalLock){
    if unlock!=nil{unlock()}
    t.Fatalf("%s lock path admitted: %v",kind,err)
   }
   contents,err:=os.ReadFile(target)
   if err!=nil||!bytes.Equal(contents,canary){
    t.Fatalf("%s lock mutated unrelated private file: %v",kind,err)
   }
  })
 }
}

func TestAgentJournalLockRefusesStaleOpenInode(t *testing.T){
 dir:=t.TempDir()
 path:=filepath.Join(dir,".agent-store.lock")
 if err:=os.WriteFile(path,nil,0600);err!=nil{t.Fatal(err)}
 f,err:=os.OpenFile(path,os.O_RDWR,0600)
 if err!=nil{t.Fatal(err)}
 defer f.Close()
 if err=validateAgentJournalLockFile(f,path);err!=nil{t.Fatal(err)}
 moved:=filepath.Join(dir,".old-journal-lock")
 if err=os.Rename(path,moved);err!=nil{t.Fatal(err)}
 if err=os.WriteFile(path,nil,0600);err!=nil{t.Fatal(err)}
 // A waiter which opened the old inode MUST NOT treat its own successful
 // flock as exclusion of a second process opening the replacement inode.
 if err=validateAgentJournalLockFile(f,path);!errors.Is(err,ErrUnsafeAgentJournalLock){
  t.Fatalf("detached/open journal lock inode was admitted: %v",err)
 }
 // The newly named file remains a valid private lock for a fresh process.
 unlock,err:=lockStoreDir(dir)
 if err!=nil{t.Fatalf("fresh canonical lock refused: %v",err)}
 unlock()
}

func TestAgentSessionCASFailsClosedOnReplacedOrUnsafeJournalLock(t *testing.T){
 dir:=filepath.Join(t.TempDir(),"private")
 store,err:=NewStore(dir);if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/workspace","project","provider","model",Stateless)
 if err!=nil{t.Fatal(err)}
 x,err=store.Save(x,0);if err!=nil{t.Fatal(err)}
 original,err:=os.ReadFile(store.file(x.ID));if err!=nil{t.Fatal(err)}
 path:=filepath.Join(dir,".agent-store.lock")
 if err=os.Remove(path);err!=nil{t.Fatal(err)}
 // A link into a separate sensitive file must never be followed even for
 // a read-only Store.Get or a stale CAS attempt.
 target:=filepath.Join(t.TempDir(),"private-evidence")
 canary:=[]byte("do not follow this symlink")
 if err=os.WriteFile(target,canary,0600);err!=nil{t.Fatal(err)}
 if err=os.Symlink(target,path);err!=nil{t.Skipf("symlink unsupported: %v",err)}
 if _,err=store.Get(x.ID,"local");!errors.Is(err,ErrUnsafeAgentJournalLock){
  t.Fatalf("unsafe lock allowed an owner read: %v",err)
 }
 x.Title="This update is not committed"
 if _,err=store.Save(x,x.Revision);!errors.Is(err,ErrUnsafeAgentJournalLock){
  t.Fatalf("unsafe lock allowed CAS commit: %v",err)
 }
 unchanged,err:=os.ReadFile(store.file(x.ID));if err!=nil{t.Fatal(err)}
 if !bytes.Equal(original,unchanged){t.Fatal("unsafe lock mutated canonical journal")}
 afterCanary,err:=os.ReadFile(target)
 if err!=nil||!bytes.Equal(afterCanary,canary){t.Fatal("unsafe lock touched unrelated file")}
 if err=os.Remove(path);err!=nil{t.Fatal(err)}
 // After repairing local lock path, owner-scoped CAS must work again.
 restored,err:=store.Save(x,x.Revision)
 if err!=nil||restored.Revision!=x.Revision+1||restored.Title!=x.Title{
  t.Fatalf("journal failed to recover after lock path repair: %+v %v",restored,err)
 }
}
