package server

import (
 "context"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "testing"
 "time"
)

func TestGitReadOnlyAvoidOptionalLocks(t *testing.T) {
 for _, item := range []struct {
  args []string
  want bool
 }{
  {[]string{"status","--porcelain=v1"},true},
  {[]string{"diff","--cached"},true},
  {[]string{"add","-A"},false},
  {[]string{"commit","-m","test"},false},
  {[]string{"reset","--hard"},false},
  {[]string{"rebase","--continue"},false},
  {[]string{"gc"},false},
 } {
  if got:=gitReadOnlyAvoidOptionalLocks(item.args);got!=item.want {
   t.Errorf("%v: got=%v want=%v",item.args,got,item.want)
  }
 }
}

func TestGitStatusDoesNotAcquireExistingIndexLock(t *testing.T) {
 if _,err:=exec.LookPath("git");err!=nil {t.Skip("git not installed")}
 root:=t.TempDir()
 init:=exec.Command("git","init",root)
 if out,err:=init.CombinedOutput();err!=nil{t.Fatalf("git init: %v: %v",err,string(out))}
 if err:=os.WriteFile(filepath.Join(root,"README.md"),[]byte("hi\n"),0600);err!=nil{t.Fatal(err)}
 lock:=filepath.Join(root,".git","index.lock")
 if err:=os.WriteFile(lock,[]byte("other process"),0600);err!=nil{t.Fatal(err)}
 defer os.Remove(lock)
 out,_,_,err:=runGitInDirectory(context.Background(),3*time.Second,root,"status","--porcelain=v1")
 if err!=nil {
  t.Fatalf("read-only git status with index.lock must succeed: %v",err)
 }
 if !strings.Contains(out,"README.md") {t.Fatalf("expected untracked file in status: %q",out)}
 if b,err:=os.ReadFile(lock);err!=nil||string(b)!="other process" {
  t.Fatalf("background status must not touch another Git process's index.lock: %q / %v",b,err)
 }
}
