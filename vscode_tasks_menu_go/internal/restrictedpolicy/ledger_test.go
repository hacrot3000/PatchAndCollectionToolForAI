package restrictedpolicy

import (
 "errors"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"
)
func budgetPolicy() Policy {
 p:=Default()
 p.MaxVisibleLines=10
 p.MaxCopyLines=5
 p.MaxUniqueLinesPerFile=20
 p.MaxUniqueLinesPerDay=8
 p.MaxRevealPercent=50
 return p
}
func TestRevealLedgerPersistsAndCountsUniqueLines(t *testing.T) {
 file:=filepath.Join(t.TempDir(),"ledger.json")
 today:=time.Date(2026,10,15,4,0,0,0,time.UTC)
 p:=budgetPolicy()
 ledger:=Ledger{Path:file}
 if err:=ledger.ReserveWindow(today,"user-A","/workspace/file.go",20,1,5,p);err!=nil{t.Fatal(err)}
 // Reload new object as after a browser reconnect/restart: repeats are free.
 restored:=Ledger{Path:file}
 if err:=restored.ReserveWindow(today,"user-A","/workspace/file.go",20,3,4,p);err!=nil{t.Fatal(err)}
 if err:=restored.ReserveWindow(today,"user-A","/workspace/file.go",20,7,2,p);err!=nil{t.Fatal(err)}
 if err:=restored.ReserveWindow(today,"user-A","/workspace/file.go",20,9,1,p);!errors.Is(err,ErrRevealBudget){
  t.Fatalf("daily cap not enforced: %v",err)
 }
 // Daily cap is per-user across all files, not reset by changing filename.
 if err:=restored.ReserveWindow(today,"user-A","/workspace/second.go",100,1,1,p);!errors.Is(err,ErrRevealBudget){
  t.Fatalf("cross-file cap bypass: %v",err)
 }
 // A different authenticated user receives a separate fixed quota.
 if err:=restored.ReserveWindow(today,"user-B","/workspace/file.go",20,1,5,p);err!=nil{t.Fatal(err)}
 // New UTC day refreshes daily allowance but not the lifetime per-file cap.
 tomorrow:=today.Add(24*time.Hour)
 if err:=restored.ReserveWindow(tomorrow,"user-A","/workspace/file.go",20,9,2,p);err!=nil{t.Fatal(err)}
 if err:=restored.ReserveWindow(tomorrow,"user-A","/workspace/file.go",20,11,1,p);!errors.Is(err,ErrRevealBudget){
  t.Fatalf("percentage cap across days bypassed: %v",err)
 }
 body,err:=os.ReadFile(file);if err!=nil{t.Fatal(err)}
 if strings.Contains(string(body),"user-A")||strings.Contains(string(body),"/workspace/file.go"){
  t.Fatal("ledger stores unhashed user or file identifiers")
 }
}
func TestRevealLedgerFailsClosedOnLockOrCorruption(t *testing.T) {
 root:=t.TempDir();path:=filepath.Join(root,"ledger.json")
 p:=budgetPolicy();now:=time.Now()
 lock,err:=os.OpenFile(path+".lock",os.O_WRONLY|os.O_CREATE|os.O_EXCL,0600)
 if err!=nil{t.Fatal(err)}
 if err:= (Ledger{Path:path}).ReserveWindow(now,"user","file",100,1,1,p);!errors.Is(err,ErrLedgerLocked){
  t.Fatalf("concurrent ledger lock not enforced: %v",err)
 }
 lock.Close();os.Remove(path+".lock")
 if err:=os.WriteFile(path,[]byte("{bad-json"),0600);err!=nil{t.Fatal(err)}
 if err:=(Ledger{Path:path}).ReserveWindow(now,"user","file",100,1,1,p);err==nil{
  t.Fatal("corrupt ledger was reset instead of failing closed")
 }
}
func TestRevealLedgerRejectsOversizedOrAnonymousWindows(t *testing.T){
 p:=budgetPolicy();now:=time.Now()
 l:=Ledger{Path:filepath.Join(t.TempDir(),"ledger.json")}
 for _,tc:=range []struct{user,file string;total,start,count int}{
  {"","file",100,1,1},{"user","",100,1,1},
  {"user","file",100,1,11},{"user","file",100,100,2},
  {"user","file",0,1,1},
  {"user","file",100,0,1},
 } {
  if err:=l.ReserveWindow(now,tc.user,tc.file,tc.total,tc.start,tc.count,p);err==nil {
   t.Fatalf("accepted unsafe window: %+v",tc)
  }
 }
}
