package server

import (
 "os"
 "path/filepath"
 "runtime"
 "testing"
)

func TestSemanticWritableWorkerUsesExistingWriteableCheckout(t *testing.T){
 t.Setenv("XDG_CONFIG_HOME",t.TempDir())
 dir:=filepath.Join(t.TempDir(),"browser-worker")
 makeTestSemanticWorker(t,dir)
 got,err:=semanticWritableWorker(dir)
 if err!=nil{t.Fatal(err)}
 if got!=dir{t.Fatalf("writable checkout should not be duplicated: %s",got)}
}
func TestSemanticWritableWorkerCopiesReadOnlyReleasePrivately(t *testing.T){
 if runtime.GOOS=="windows"||os.Geteuid()==0{t.Skip("needs POSIX non-root permissions")}
 t.Setenv("XDG_CONFIG_HOME",t.TempDir())
 dir:=filepath.Join(t.TempDir(),"browser-worker")
 makeTestSemanticWorker(t,dir)
 if err:=os.Chmod(dir,0555);err!=nil{t.Fatal(err)}
 t.Cleanup(func(){_ =os.Chmod(dir,0700)})
 got,err:=semanticWritableWorker(dir)
 if err!=nil{t.Fatal(err)}
 if got==dir{t.Fatal("read-only release was used for npm installation")}
 if !semanticWorkerDirectoryValid(got){t.Fatal("copied worker is invalid")}
 if got!=filepath.Join(os.Getenv("XDG_CONFIG_HOME"),"taskdeck-semantic-browser","managed-worker","browser-worker"){
  t.Fatalf("unexpected private copy path %q",got)
 }
 again,err:=semanticWritableWorker(dir)
 if err!=nil||again!=got{t.Fatalf("staged worker not reused: %q %v",again,err)}
}
func TestSemanticWritableWorkerRejectsFakePackages(t *testing.T){
 t.Setenv("XDG_CONFIG_HOME",t.TempDir())
 dir:=filepath.Join(t.TempDir(),"browser-worker")
 if err:=os.MkdirAll(dir,0700);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(dir,"package.json"),[]byte(`{"name":"not-taskdeck"}`),0600);err!=nil{t.Fatal(err)}
 if _,err:=semanticWritableWorker(dir);err==nil{t.Fatal("untrusted package accepted")}
}

func TestSemanticWritableWorkerUpgradesPrivateRuntimeWithoutRemovingDependencies(t *testing.T){
 if runtime.GOOS=="windows"||os.Geteuid()==0{t.Skip("requires non-root POSIX permissions")}
 t.Setenv("XDG_CONFIG_HOME",t.TempDir())
 dir:=filepath.Join(t.TempDir(),"immutable-release","browser-worker")
 makeTestSemanticWorker(t,dir)
 if err:=os.Chmod(dir,0555);err!=nil{t.Fatal(err)}
 t.Cleanup(func(){_ =os.Chmod(dir,0700)})
 managedRoot:=filepath.Join(os.Getenv("XDG_CONFIG_HOME"),"taskdeck-semantic-browser","managed-worker")
 managed:=filepath.Join(managedRoot,"browser-worker")
 if err:=os.MkdirAll(filepath.Join(managed,"node_modules","playwright"),0700);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(managed,"node_modules","playwright","package.json"),
  []byte("already-installed"),0600);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(managed,"package.json"),
  []byte(`{"name":"taskdeck-semantic-browser-worker"}`),0600);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(managed,"worker.mjs"),[]byte("old"),0600);err!=nil{t.Fatal(err)}
 // Old runtime deliberately misses chrome_identity.mjs and other modules.
 updated,err:=semanticWritableWorker(dir)
 if err!=nil{t.Fatal(err)}
 if updated!=managed{t.Fatalf("managed worker not reused: %s",updated)}
 if !semanticWorkerDirectoryValid(updated){t.Fatal("upgraded worker incomplete")}
 contents,err:=os.ReadFile(filepath.Join(managed,"node_modules","playwright","package.json"))
 if err!=nil||string(contents)!="already-installed"{
  t.Fatalf("upgrade must preserve installed Playwright: %s %v",contents,err)
 }
}
