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
 first,err:=store.Charge("user1",path,1,[]string{"line1\n","line2\n"},10,p)
 if err!=nil{t.Fatal(err)}
 if first.File!=3{t.Fatalf("remaining after 2 unique lines = %+v",first)}
 again,err:=store.Charge("user1",path,1,[]string{"line1\n"},10,p)
 if err!=nil||again.File!=3{t.Fatalf("duplicate line consumed quota: %+v %v",again,err)}
 reopened,err:=Open(filepath.Dir(path));if err!=nil{t.Fatal(err)}
 after,err:=reopened.Charge("user1",path,3,[]string{"line3\n"},10,p)
 if err!=nil||after.File!=2{t.Fatalf("reopened store reset quota: %+v %v",after,err)}
}
func TestQuotaBlocksSequentialFullFileAndChangedContent(t *testing.T){
 store,path,p:=testLedger(t)
 for _,chunk:=range [][]string{
  {"one\n","two\n"}, {"three\n","four\n","five\n"},
 }{
  if _,err:=store.Charge("u",path,func() int {if chunk[0]=="one\n"{return 1};return 3}(),chunk,10,p);err!=nil{t.Fatal(err)}
 }
 for _,excess:=range []string{"six\n","new-edited-content-at-one\n"}{
  if _,err:=store.Charge("u",path,6,[]string{excess},10,p);!errors.Is(err,ErrQuota){
   t.Fatalf("extra or edited source line bypassed lifetime quota: %v",err)
  }
 }
 // Each user has independent exposure accounting.
 if _,err:=store.Charge("different-user",path,6,[]string{"six\n"},10,p);err!=nil{t.Fatal(err)}
}
func TestQuotaPerDayAcrossFilesAndNoInflationFromGrowingFiles(t *testing.T){
 store,path,p:=testLedger(t)
 p.MaxUniqueLinesPerDay=3
 if _,err:=store.Charge("u",path,1,[]string{"one","two"},10,p);err!=nil{t.Fatal(err)}
 other:=filepath.Join(filepath.Dir(path),"two.go")
 if err:=os.WriteFile(other,[]byte("a\n"),0600);err!=nil{t.Fatal(err)}
 if _,err:=store.Charge("u",other,1,[]string{"other"},10,p);err!=nil{t.Fatal(err)}
 if _,err:=store.Charge("u",path,3,[]string{"three"},10,p);!errors.Is(err,ErrQuota){
  t.Fatalf("daily per-user limit ignored: %v",err)
 }
 // Quota percent uses a minimum observed baseline. Padding a file with
 // extra lines later must not expand the initial allowance.
 p.MaxUniqueLinesPerDay=20
 if _,err:=store.Charge("u",path,3,[]string{"three"},100,p);err!=nil{t.Fatal(err)}
 if _,err:=store.Charge("u",path,4,[]string{"four","five","six"},100,p);!errors.Is(err,ErrQuota){
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
   _,err:=store.Charge("same-user",path,i+1,[]string{strings.Repeat("X",i+1)},10,p)
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


func TestIdenticalLinesAtDifferentPositionsStillConsumeQuota(t *testing.T){
 store,path,p:=testLedger(t)
 p.MaxVisibleLines=3;p.MaxUniqueLinesPerFile=3;p.MaxCopyLines=2;p.MaxRevealPercent=80
 for line:=1;line<=3;line++{
  if _,err:=store.Charge("same-reader",path,line,[]string{"identical\\n"},10,p);err!=nil{
   t.Fatalf("duplicate text at position %d must count: %v",line,err)
  }
 }
 if _,err:=store.Charge("same-reader",path,4,[]string{"identical\\n"},10,p);!errors.Is(err,ErrQuota){
  t.Fatalf("duplicate content at different line escaped quota: %v",err)
 }
 if _,err:=store.Charge("same-reader",path,1,[]string{"identical\\n"},10,p);err!=nil{
  t.Fatalf("re-reading same location should not consume quota: %v",err)
 }
}
func TestInvalidWindowIndicesNeverCharge(t *testing.T){
 store,path,p:=testLedger(t)
 for _,tc:=range []struct{start,total int;lines []string}{
  {0,10,[]string{"x"}},{11,10,[]string{"x"}},{10,10,[]string{"x","y"}},
 }{
  if _,err:=store.Charge("user",path,tc.start,tc.lines,tc.total,p);!errors.Is(err,ErrQuota){
   t.Fatalf("unsafe source window accepted: %+v, %v",tc,err)
  }
 }
}


func TestEditAuthorizationRequiresExactChargedSourceLines(t *testing.T){
 store,path,p:=testLedger(t)
 if _,err:=store.Charge("userA",path,2,[]string{"same\n","other\n"},10,p);err!=nil{t.Fatal(err)}
 cases:=[]struct{user string;start int;lines []string;allowed bool}{
  {"userA",2,[]string{"same\n"},true},
  {"userA",2,[]string{"same\n","other\n"},true},
  {"userA",1,[]string{"same\n"},false},
  {"userA",2,[]string{"edited\n"},false},
  {"userA",3,[]string{"same\n"},false},
  {"userB",2,[]string{"same\n"},false},
 }
 for _,tc:=range cases{
  ok,err:=store.HasViewed(tc.user,path,tc.start,tc.lines)
  if err!=nil||ok!=tc.allowed{t.Fatalf("HasViewed %+v = %v, %v",tc,ok,err)}
 }
 reopened,err:=Open(filepath.Dir(path));if err!=nil{t.Fatal(err)}
 ok,err:=reopened.HasViewed("userA",path,2,[]string{"same\n"})
 if err!=nil||!ok{t.Fatalf("view authorization was lost on restart: %v, %v",ok,err)}
}

func TestMalformedLedgerFailsClosedWithoutResettingQuota(t *testing.T){
 store,path,p:=testLedger(t)
 if _,err:=store.Charge("account",path,1,[]string{"line01\n"},10,p);err!=nil{t.Fatal(err)}
 location:=filepath.Join(store.dir,"ledger.json")
 for _,body:=range []string{
  "null",
  "{}",
  "{\"version\":1,\"accounts\":null}",
  "{\"version\":1,\"accounts\":{\"account\":null}}",
  "{\"version\":1,\"accounts\":{\"account\":{\"files\":null,\"days\":{}}}}",
  "{\"version\":1,\"accounts\":{\"account\":{\"files\":{\"file\":null},\"days\":{}}}}",
  "{\"version\":1,\"accounts\":{\"account\":{\"files\":{},\"days\":{\"today\":null}}}}",
  "[]",
 }{
  if err:=os.WriteFile(location,[]byte(body),0600);err!=nil{t.Fatal(err)}
  if _,err:=store.Charge("account",path,2,[]string{"line02\n"},10,p);!errors.Is(err,ErrUnsafeState){
   t.Fatalf("unsafe quota state %q accepted: %v",body,err)
  }
  if ok,err:=store.HasViewed("account",path,1,[]string{"line01\n"});ok||!errors.Is(err,ErrUnsafeState){
   t.Fatalf("unsafe edit proof %q accepted: ok=%v err=%v",body,ok,err)
  }
 }
}

func TestLedgerLockRejectsSymlinkAndUnsafeFileTypes(t *testing.T){
 for _,kind:=range []string{"symlink","directory","world-readable"}{
  t.Run(kind,func(t *testing.T){
   store,path,p:=testLedger(t)
   lockPath:=filepath.Join(store.dir,"ledger.lock")
   switch kind {
   case "symlink":
    target:=filepath.Join(t.TempDir(),"target")
    if err:=os.WriteFile(target,[]byte("secret"),0600);err!=nil{t.Fatal(err)}
    if err:=os.Symlink(target,lockPath);err!=nil{t.Fatal(err)}
   case "directory":
    if err:=os.Mkdir(lockPath,0700);err!=nil{t.Fatal(err)}
   case "world-readable":
    if err:=os.WriteFile(lockPath,[]byte(""),0644);err!=nil{t.Fatal(err)}
   }
   if _,err:=store.Charge("u",path,1,[]string{"one\n"},10,p);!errors.Is(err,ErrUnsafeState){
    t.Fatalf("%s lock allowed quota write: %v",kind,err)
   }
   if ok,err:=store.HasViewed("u",path,1,[]string{"one\n"});ok||!errors.Is(err,ErrUnsafeState){
    t.Fatalf("%s lock allowed edit proof: %v %v",kind,ok,err)
   }
  })
 }
}
func TestLedgerLockAcceptsPrivateRegularFile(t *testing.T){
 store,path,p:=testLedger(t)
 if _,err:=store.Charge("u",path,1,[]string{"one\n"},10,p);err!=nil{t.Fatal(err)}
 if _,err:=store.Charge("u",path,1,[]string{"one\n"},10,p);err!=nil{t.Fatal(err)}
}
