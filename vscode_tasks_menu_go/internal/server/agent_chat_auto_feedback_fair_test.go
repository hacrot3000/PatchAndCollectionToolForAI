package server

import (
 "fmt"
 "reflect"
 "sync"
 "testing"
 "time"
)

func TestAgentAutoFeedbackFairAdmissionRotatesAfterSaturation(t *testing.T){
 p:=&agentAutoFeedbackWorkerPool{active:map[string]struct{}{}}
 scope:="workspace:local"
 // Deliberately imitate Store.List's changed, newest-first ordering.
 candidates:=[]string{"d","c","b","a","g","f","e"}
 next:=p.acquireFairBatch(scope,candidates)
 if want:=[]string{"a","b","c","d"};!reflect.DeepEqual(next,want){
  t.Fatalf("first bounded round-robin batch=%v want=%v",next,want)
 }
 if got:=p.acquireFairBatch(scope,[]string{"g","f","e","d","c","b","a"});len(got)!=0{
  t.Fatalf("saturated worker pool admitted duplicate work: %v",got)
 }
 for _,tc:=range []struct{released,expect string}{
  {"a","e"},{"b","f"},{"c","g"},{"d","a"},{"e","b"},
 }{
  p.release(scope+":"+tc.released)
  got:=p.acquireFairBatch(scope,[]string{"a","f","b","e","d","g","c"})
  if !reflect.DeepEqual(got,[]string{tc.expect}){
   t.Fatalf("released %s: next=%v want=%s; quiet sessions must be admitted",tc.released,got,tc.expect)
  }
 }
 if len(p.active)!=agentAutoFeedbackWorkerLimit{
  t.Fatalf("global worker cap lost after rotations: %d",len(p.active))
 }
 if p.lastAdmitted[scope]!="b"{
  t.Fatalf("cursor did not advance only on admitted sessions: %q",p.lastAdmitted[scope])
 }
}

func TestAgentAutoFeedbackFairAdmissionHandlesRemovedCandidateAndScopes(t *testing.T){
 p:=&agentAutoFeedbackWorkerPool{}
 scope:="scope-one:local"
 if got:=p.acquireFairBatch(scope,[]string{"a","c","e"});!reflect.DeepEqual(got,[]string{"a","c","e"}){
  t.Fatalf("unexpected initial batch: %v",got)
 }
 for _,id:=range []string{"a","c","e"}{p.release(scope+":"+id)}
 // Cursor points to a removed ID; use upper-bound lookup, not an index
 // into yesterday's list. No endlessly retrying the same high-activity chat.
 if got:=p.acquireFairBatch(scope,[]string{"b","d","f"});!reflect.DeepEqual(got,[]string{"f","b","d"}){
  t.Fatalf("removed cursor/candidate was not reconciled: %v",got)
 }
 for _,id:=range []string{"b","d","f"}{p.release(scope+":"+id)}
 if got:=p.acquireFairBatch("scope-two:local",[]string{"x","y"});!reflect.DeepEqual(got,[]string{"x","y"}){
  t.Fatalf("new workspace inherited another workspace cursor: %v",got)
 }
 if got:=p.acquireFairBatch("",[]string{"z"});len(got)!=0{
  t.Fatalf("missing scope admitted a worker: %v",got)
 }
 if got:=p.acquireFairBatch(scope,nil);len(got)!=0{t.Fatalf("empty work queue admitted a worker: %v",got)}
}

func TestAgentAutoFeedbackFairAdmissionIsAtomicAcrossConcurrentTicks(t *testing.T){
 p:=&agentAutoFeedbackWorkerPool{active:map[string]struct{}{}}
 scope:="workspace-race:local"
 ids:=[]string{"one","two","three","four","five","six","seven","eight"}
 start:=make(chan struct{})
 var wg sync.WaitGroup
 var mu sync.Mutex
 var all []string
 for i:=0;i<20;i++{
  wg.Add(1)
  go func(){
   defer wg.Done()
   <-start
   got:=p.acquireFairBatch(scope,ids)
   mu.Lock();all=append(all,got...);mu.Unlock()
  }()
 }
 close(start);wg.Wait()
 if len(all)!=agentAutoFeedbackWorkerLimit{
  t.Fatalf("racing schedulers oversubscribed worker pool: %v",all)
 }
 seen:=map[string]bool{}
 for _,id:=range all{
  if seen[id]{t.Fatalf("duplicate worker admission for %s",id)}
  seen[id]=true
 }
 if got:=p.acquireFairBatch(scope,ids);len(got)!=0{
  t.Fatalf("double dispatch occurred after concurrent reservations: %v",got)
 }
}

func TestAgentAutoFeedbackFairCursorMemoryIsBounded(t *testing.T){
 p:=&agentAutoFeedbackWorkerPool{}
 for i:=0;i<maxAgentAutoFeedbackCursorScopes+30;i++{
  scope:=fmt.Sprintf("project-%d:local",i)
  admitted:=p.acquireFairBatch(scope,[]string{"one"})
  if !reflect.DeepEqual(admitted,[]string{"one"}){t.Fatalf("scope %s admission failed: %v",scope,admitted)}
  p.release(scope+":one")
 }
 if len(p.lastAdmitted)!=maxAgentAutoFeedbackCursorScopes||
  len(p.cursorOrder)!=maxAgentAutoFeedbackCursorScopes{
  t.Fatalf("workspace fairness cursor leaked: map=%d queue=%d",len(p.lastAdmitted),len(p.cursorOrder))
 }
 if _,ok:=p.lastAdmitted["project-0:local"];ok{
  t.Fatal("oldest idle workspace cursor was never evicted")
 }
 if p.lastAdmitted[fmt.Sprintf("project-%d:local",maxAgentAutoFeedbackCursorScopes+29)]!="one"{
  t.Fatal("most recently admitted workspace cursor was lost")
 }
}

func TestAgentAutoFeedbackCrossWorkspacePendingTurnPreventsSlotTheft(t *testing.T){
 p:=&agentAutoFeedbackWorkerPool{active:map[string]struct{}{}}
 a,b:="workspace-A:local","workspace-B:local"
 candidatesA:=[]string{"a7","a6","a5","a4","a3","a2","a1"}
 candidatesB:=[]string{"b3","b1","b2"}
 if got:=p.acquireFairBatch(a,candidatesA);!reflect.DeepEqual(got,[]string{"a1","a2","a3","a4"}){
  t.Fatalf("initial A reservation: %v",got)
 }
 // B's normal poll registers it while all four workers remain occupied.
 if got:=p.acquireFairBatch(b,candidatesB);len(got)!=0{
  t.Fatalf("B bypassed global capacity: %v",got)
 }
 if _,registered:=p.pendingScopes[b];!registered{
  t.Fatal("workspace B did not retain its pending fair turn")
 }
 p.release(a+":a1")
 if got:=p.acquireFairBatch(a,candidatesA);len(got)!=0{
  t.Fatalf("noisy A stole B's next global slot: %v",got)
 }
 if got:=p.acquireFairBatch(b,candidatesB);!reflect.DeepEqual(got,[]string{"b1"}){
  t.Fatalf("waiting B did not receive its reserved turn: %v",got)
 }
 if len(p.active)!=agentAutoFeedbackWorkerLimit{t.Fatal("worker count grew beyond global limit")}
 p.release(a+":a2")
 if got:=p.acquireFairBatch(a,candidatesA);!reflect.DeepEqual(got,[]string{"a5"}){
  t.Fatalf("A did not get the following turn: %v",got)
 }
 p.release(b+":b1")
 if got:=p.acquireFairBatch(b,candidatesB);!reflect.DeepEqual(got,[]string{"b2"}){
  t.Fatalf("B did not regain its fair turn: %v",got)
 }
 if len(p.active)!=agentAutoFeedbackWorkerLimit{
  t.Fatalf("worker capacity incorrect after alternation: %d",len(p.active))
 }
 if p.lastAdmittedScope!=b{t.Fatalf("global fairness cursor incorrect: %s",p.lastAdmittedScope)}
 if got:=p.acquireFairBatch(b,candidatesB);len(got)!=0{
  t.Fatalf("B stole a second consecutive slot: %v",got)
 }
}

func TestAgentAutoFeedbackCrossWorkspaceStaleWaitingScopeDoesNotBlock(t *testing.T){
 p:=&agentAutoFeedbackWorkerPool{active:map[string]struct{}{}}
 a,b:="workspace-A:local","workspace-B:local"
 ids:=[]string{"a1","a2","a3","a4","a5"}
 if got:=p.acquireFairBatch(a,ids);len(got)!=agentAutoFeedbackWorkerLimit{
  t.Fatalf("initial slots=%v",got)
 }
 p.acquireFairBatch(b,[]string{"b1"})
 p.mu.Lock()
 p.pendingScopes[b]=time.Now().Add(-agentAutoFeedbackPendingScopeTTL-time.Second)
 p.mu.Unlock()
 p.release(a+":a1")
 if got:=p.acquireFairBatch(a,ids);!reflect.DeepEqual(got,[]string{"a5"}){
  t.Fatalf("stale B waiter incorrectly blocked active A: %v",got)
 }
 if _,still:=p.pendingScopes[b];still{t.Fatal("stale workspace waiter was retained")}
 // A cleared an empty/no-ready queue; no abandoned scope should block B.
 if got:=p.acquireFairBatch(a,nil);len(got)!=0{t.Fatal("empty queue unexpectedly allocated worker")}
 if _,still:=p.pendingScopes[a];still{t.Fatal("empty workspace did not unregister")}
}

func TestAgentAutoFeedbackCrossWorkspacePendingRegistryIsBounded(t *testing.T){
 p:=&agentAutoFeedbackWorkerPool{active:map[string]struct{}{}}
 busy:="busy:local"
 p.acquireFairBatch(busy,[]string{"a","b","c","d","e"})
 for i:=0;i<maxAgentAutoFeedbackCursorScopes+40;i++{
  name:=fmt.Sprintf("scope-%03d:local",i)
  if got:=p.acquireFairBatch(name,[]string{"ready"});len(got)!=0{
   t.Fatalf("full pool admitted %q: %v",name,got)
  }
 }
 if len(p.pendingScopes)>maxAgentAutoFeedbackCursorScopes{
  t.Fatalf("pending scope registry grew without bound: %d",len(p.pendingScopes))
 }
 last:=fmt.Sprintf("scope-%03d:local",maxAgentAutoFeedbackCursorScopes+39)
 if _,ok:=p.pendingScopes[last];!ok{t.Fatal("latest waiting workspace was lost")}
 if _,ok:=p.pendingScopes["scope-000:local"];ok{
  t.Fatal("oldest inactive waiter was not evicted")
 }
}

func TestAgentAutoFeedbackCrossWorkspaceConcurrentReservationsKeepGlobalCap(t *testing.T){
 p:=&agentAutoFeedbackWorkerPool{active:map[string]struct{}{}}
 const total=32
 start:=make(chan struct{})
 var wg sync.WaitGroup
 for i:=0;i<total;i++{
  wg.Add(1)
  go func(n int){
   defer wg.Done()
   <-start
   p.acquireFairBatch(fmt.Sprintf("scope-%d:local",n),[]string{"one","two","three"})
  }(i)
 }
 close(start);wg.Wait()
 p.mu.Lock()
 defer p.mu.Unlock()
 if len(p.active)>agentAutoFeedbackWorkerLimit{
  t.Fatalf("concurrent workspace ticks overbooked pool: %d",len(p.active))
 }
 if len(p.pendingScopes)>maxAgentAutoFeedbackCursorScopes{
  t.Fatal("concurrent waiter registry grew without bound")
 }
}
