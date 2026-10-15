package restrictedview

import (
 "errors"
 "os"
 "path/filepath"
 "strings"
 "sync"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/restrictedpolicy"
)
func testLedger(t *testing.T)(*Store,string,restrictedpolicy.Policy){
 t.Helper()
 t.Setenv("XDG_CONFIG_HOME",filepath.Join(t.TempDir(),"private-config"))
 root:=t.TempDir()
 path:=filepath.Join(root,"code.go")
 if err:=os.WriteFile(path,[]byte(strings.Repeat("dummy\n",10)),0600);err!=nil{t.Fatal(err)}
 store,err:=Open(root);if err!=nil{t.Fatal(err)}
 p:=restrictedpolicy.Default()
 p.MaxVisibleLines=10;p.MaxCopyLines=2;p.MaxUniqueLinesPerFile=10
 p.MaxUniqueLinesPerDay=20;p.MaxRevealPercent=50
 return store,path,p
}
func TestQuotaSurvivesStoreReopenAndRepeatedReads(t *testing.T){
 store,path,p:=testLedger(t)
 first,err:=store.Charge("user1",path,[]string{"line1\n","line2\n"},10,p)
 if err!=nil{t.Fatal(err)}
 if first.File!=3{t.Fatalf("remaining after 2 unique lines = %+v",first)}
 again,err:=store.Charge("user1",path,[]string{"line1\n"},10,p)
 if err!=nil||again.File!=3{t.Fatalf("duplicate line consumed quota: %+v %v",again,err)}
 reopened,err:=Open(filepath.Dir(path));if err!=nil{t.Fatal(err)}
 after,err:=reopened.Charge("user1",path,[]string{"line3\n"},10,p)
 if err!=nil||after.File!=2{t.Fatalf("reopened store reset quota: %+v %v",after,err)}
}
func TestQuotaBlocksSequentialFullFileAndChangedContent(t *testing.T){
 store,path,p:=testLedger(t)
 for _,chunk:=range [][]string{
  {"one\n","two\n"}, {"three\n","four\n","five\n"},
 }{
  if _,err:=store.Charge("u",path,chunk,10,p);err!=nil{t.Fatal(err)}
 }
 for _,excess:=range []string{"six\n","new-edited-content-at-one\n"}{
  if _,err:=store.Charge("u",path,[]string{excess},10,p);!errors.Is(err,ErrQuota){
   t.Fatalf("extra or edited source line bypassed lifetime quota: %v",err)
  }
 }
 // Each user has independent exposure accounting.
 if _,err:=store.Charge("different-user",path,[]string{"six\n"},10,p);err!=nil{t.Fatal(err)}
}
func TestQuotaPerDayAcrossFilesAndNoInflationFromGrowingFiles(t *testing.T){
 store,path,p:=testLedger(t)
 p.MaxUniqueLinesPerDay=3
 if _,err:=store.Charge("u",path,[]string{"one","two"},10,p);err!=nil{t.Fatal(err)}
 other:=filepath.Join(filepath.Dir(path),"two.go")
 if err:=os.WriteFile(other,[]byte("a\n"),0600);err!=nil{t.Fatal(err)}
 if _,err:=store.Charge("u",other,[]string{"other"},10,p);err!=nil{t.Fatal(err)}
 if _,err:=store.Charge("u",path,[]string{"three"},10,p);!errors.Is(err,ErrQuota){
  t.Fatalf("daily per-user limit ignored: %v",err)
 }
 // Quota percent uses a minimum observed baseline. Padding a file with
 // extra lines later must not expand the initial allowance.
 p.MaxUniqueLinesPerDay=20
 if _,err:=store.Charge("u",path,[]string{"three"},100,p);err!=nil{t.Fatal(err)}
 if _,err:=store.Charge("u",path,[]string{"four","five","six"},100,p);!errors.Is(err,ErrQuota){
  t.Fatalf("growing file defeated original 50-percent cap: %v",err)
 }
}
func TestQuotaConcurrentRequestsCannotOversubscribe(t *testing.T){
 store,path,p:=testLedger(t)
 var wg sync.WaitGroup
 successes:=make(chan struct{},20)
 for i:=0;i<12;i++{
  wg.Add(1)
  go func(i int){
   defer wg.Done()
   // Distinct hashes from independently concurrent browser requests.
   _,err:=store.Charge("same-user",path,[]string{strings.Repeat("X",i+1)},10,p)
   if err==nil{successes<-struct{}{}}
  }(i)
 }
 wg.Wait();close(successes)
 count:=0;for range successes{count++}
 if count!=5{t.Fatalf("expected lifetime cap of 5, accepted %d",count)}
}
func TestStateMustLiveOutsideWorkspace(t *testing.T){
 root:=t.TempDir()
 t.Setenv("XDG_CONFIG_HOME",filepath.Join(root,".private-config"))
 if _,err:=Open(root);!errors.Is(err,ErrUnsafeState){
  t.Fatalf("quota state under editable workspace accepted: %v",err)
 }
}
