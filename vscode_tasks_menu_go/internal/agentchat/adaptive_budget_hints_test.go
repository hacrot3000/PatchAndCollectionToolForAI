package agentchat

import (
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "strings"
 "sync"
 "testing"
 "time"
)

func TestAdaptiveBudgetHintDurableScopedExpiryAndCorruption(t *testing.T){
 dir:=filepath.Join(t.TempDir(),"private")
 s,e:=NewStore(dir);if e!=nil{t.Fatal(e)}
 x,e:=NewSession("local","/workspace","project","p","m",Stateless);if e!=nil{t.Fatal(e)}
 x,e=s.Save(x,0);if e!=nil{t.Fatal(e)}
 version,credential:=strings.Repeat("a",64),strings.Repeat("b",64)
 if e=s.RememberAdaptiveBudgetHint(x,version,credential,20000,8192);e!=nil{t.Fatal(e)}
 reopened,e:=NewStore(dir);if e!=nil{t.Fatal(e)}
 got,ok,e:=reopened.AdaptiveBudgetHint(x,version,credential)
 if e!=nil||!ok||got.Input!=20000||got.Output!=8192{t.Fatalf("hint lost: %+v %v",got,e)}
 if _,ok,e=reopened.AdaptiveBudgetHint(x,version,strings.Repeat("c",64));e!=nil||ok{t.Fatal("credential rotation reused stale hint")}
 for _,other:=range []Session{func()Session{a:=x;a.Owner="foreign";return a}(),func()Session{a:=x;a.Workspace="/other";return a}(),func()Session{a:=x;a.ProjectID="other";return a}()}{
  if _,ok,e=reopened.AdaptiveBudgetHint(other,version,credential);ok||!errors.Is(e,ErrNotFound){t.Fatal("foreign scope hint leaked")}
 }
 var wg sync.WaitGroup
 for i:=0;i<8;i++{wg.Add(1);go func(){defer wg.Done();if err:=reopened.RememberAdaptiveBudgetHint(x,version,credential,30000,16000);err!=nil{t.Error(err)}}()}
 wg.Wait()
 got,ok,e=reopened.AdaptiveBudgetHint(x,version,credential)
 if e!=nil||!ok||got.Input!=20000||got.Output!=8192{t.Fatal("conservative bound increased")}
 raw,e:=os.ReadFile(s.adaptiveBudgetHintPath());if e!=nil{t.Fatal(e)}
 if strings.Contains(string(raw),"local")||strings.Contains(string(raw),"/workspace")||strings.Contains(string(raw),credential){t.Fatal("raw identity or credential hash leaked")}
 e=s.withLock(func()error{
  f,e:=s.readAdaptiveHintsLocked();if e!=nil{return e}
  key:=adaptiveHintKey(x,version,credential)
  h:=f.Entries[key];h.ExpiresAt=time.Now().Add(-time.Minute);f.Entries[key]=h
  return s.writeAdaptiveHintsLocked(f)
 });if e!=nil{t.Fatal(e)}
 if _,ok,e=reopened.AdaptiveBudgetHint(x,version,credential);e!=nil||ok{t.Fatal("expired hint was used")}
 if e=os.WriteFile(s.adaptiveBudgetHintPath(),[]byte("{}"),0600);e!=nil{t.Fatal(e)}
 if _,ok,e=reopened.AdaptiveBudgetHint(x,version,credential);e==nil||ok{t.Fatal("corrupt hint accepted")}
 if e=reopened.RememberAdaptiveBudgetHint(x,version,credential,20000,8192);e==nil{t.Fatal("corrupt hint silently overwritten")}
 if _,e=reopened.Get(x.ID,x.Owner);e!=nil{t.Fatal("corrupt advisory blocked session journal")}
}

func TestAdaptiveBudgetHintCapacity(t *testing.T){
 s,x,_:=sendingAttemptFixture(t)
 credential:=strings.Repeat("b",64)
 for i:=0;i<=adaptiveBudgetHintCapacity;i++{
  if e:=s.RememberAdaptiveBudgetHint(x,fmt.Sprintf("%064x",i),credential,20000,8192);e!=nil{t.Fatal(e)}
 }
 if e:=s.withLock(func()error{
  f,e:=s.readAdaptiveHintsLocked();if e!=nil{return e}
  if len(f.Entries)!=adaptiveBudgetHintCapacity{t.Fatalf("size %d",len(f.Entries))}
  if _,ok:=f.Entries[adaptiveHintKey(x,fmt.Sprintf("%064x",adaptiveBudgetHintCapacity),credential)];!ok{t.Fatal("new hint missing")}
  return nil
 });e!=nil{t.Fatal(e)}
}
