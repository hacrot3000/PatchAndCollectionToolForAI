package server

import (
 "fmt"
 "reflect"
 "sync"
 "testing"
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
 if got:=p.acquireFairBatch(scope,[]string{"b","d","f"});!reflect.DeepEqual(got,[]string{"b","d","f"}){
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
