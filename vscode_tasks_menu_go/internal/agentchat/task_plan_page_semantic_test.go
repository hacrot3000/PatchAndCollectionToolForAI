package agentchat

import (
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "os"
 "path/filepath"
 "testing"
)

func TestPagedGetRejectsInvalidOffscreenMilestoneDespiteMatchingSHA(t *testing.T){
 cases:=[]struct{
  name string
  change func(*TaskPlanStep)
 }{
  {"empty-title",func(s *TaskPlanStep){s.Title=""}},
  {"duplicate-id-across-pages",func(s *TaskPlanStep){s.ID="step-0000"}},
  {"unverified-manual-proof",func(s *TaskPlanStep){
   s.Status="done"
   s.Evidence=[]TaskPlanEvidence{{Kind:"manual",Ref:"forged-result",Note:""}}
  }},
  {"invalid-owner-go-binding",func(s *TaskPlanStep){
   s.GoTestRequirements=&TaskGoTestRequirements{Package:"bad!",Tests:[]string{"TestOne"}}
  }},
 }
 for _,tc:=range cases{
  t.Run(tc.name,func(t *testing.T){
   store,err:=NewStore(filepath.Join(t.TempDir(),"private","agent"))
   if err!=nil{t.Fatal(err)}
   x,err:=NewSession("local","/workspace","project","provider","model",Stateless)
   if err!=nil{t.Fatal(err)}
   x.TaskPlan,err=NewTaskPlan("All page acceptance checks matter",planStepFixtures(160))
   if err!=nil{t.Fatal(err)}
   x,err=store.Save(x,0);if err!=nil{t.Fatal(err)}
   // Select an off-screen part (page index 4), mutate a milestone, then
   // update its own SHA-256 in the canonical journal. This is not a simple
   // digest mismatch: only full semantic checks can detect it.
   partIndex:=4
   ref:=x.TaskPlanSegments[partIndex]
   file:=store.taskPlanSegmentPath(x.ID,ref)
   raw,err:=os.ReadFile(file);if err!=nil{t.Fatal(err)}
   var steps []TaskPlanStep
   if err=json.Unmarshal(raw,&steps);err!=nil{t.Fatal(err)}
   tc.change(&steps[0])
   tampered,err:=json.Marshal(steps);if err!=nil{t.Fatal(err)}
   if err=os.WriteFile(file,tampered,0600);err!=nil{t.Fatal(err)}
   sum:=sha256.Sum256(tampered)
   journalRaw,err:=os.ReadFile(store.file(x.ID));if err!=nil{t.Fatal(err)}
   var journal Session
   if err=json.Unmarshal(journalRaw,&journal);err!=nil{t.Fatal(err)}
   journal.TaskPlanSegments[partIndex].SHA256=hex.EncodeToString(sum[:])
   manifest,err:=json.Marshal(journal);if err!=nil{t.Fatal(err)}
   if err=os.WriteFile(store.file(x.ID),manifest,0600);err!=nil{t.Fatal(err)}

   if _,err=store.Get(x.ID,"local");!errors.Is(err,ErrInvalidSession){
    t.Fatalf("fully hydrated reader should reject semantically invalid part: %v",err)
   }
   if _,err=store.TaskPlanPage(x.ID,"local",0,24);!errors.Is(err,ErrInvalidSession){
    t.Fatalf("paged GET accepted offscreen invalid milestone despite matching SHA: %v",err)
   }
   if _,err=store.SetSegmentTaskPlanStepStatus(x.ID,"local",x.Revision,
    "step-0001","blocked",0,24);!errors.Is(err,ErrInvalidSession){
    t.Fatalf("direct status write accepted offscreen invalid milestone: %v",err)
   }
   // A failed paginated read/write may not silently replace the journal or
   // change the revision, even when a corrupted page was encountered.
   final,err:=os.ReadFile(store.file(x.ID));if err!=nil{t.Fatal(err)}
   if string(final)!=string(manifest){
    t.Fatal("failed offscreen semantic validation rewrote canonical session")
   }
  })
 }
}
