package agentchat

import(
 "strings"
 "testing"
 "time"
)

func TestCacheObservationInstructionBlockPrefix(t *testing.T){
 for _,api:=range []string{"responses","chat_completions"}{
  c:=RequestConfig{Model:"model",Mode:Stateless,Instructions:strings.Repeat("a",3500)+" old plan",Next:[]HistoryItem{{Type:"user",Content:"goal"}}}
  raw,e:=BuildResponsesRequest(c);if e!=nil{t.Fatal(e)}
  a,e:=cacheSnapshot(api,raw);if e!=nil{t.Fatal(e)}
  c.Instructions=strings.Repeat("a",3500)+" updated plan"
  raw,e=BuildResponsesRequest(c);if e!=nil{t.Fatal(e)}
  b,e:=cacheSnapshot(api,raw);if e!=nil{t.Fatal(e)}
  if _,_,why:=compareCacheSnapshots(a,b);why!="instructions_changed"{t.Fatal(why)}
  if n:=commonInstructionBlockBytes(a,b);n!=3072{t.Fatalf("%s: %d",api,n)}
  if n:=len(instructionBlockDigests(strings.Repeat("x",1<<20)));n!=maxCacheInstructionBlocks{t.Fatal("unbounded instruction hashes")}
 }
}

func TestCacheObservationSuccessBaselineSurvivesFailedRetryAndRestart(t *testing.T){
 s,x,original:=sendingAttemptFixture(t)
 p:=ProviderConfig{ID:"provider",APIType:"responses",BaseURL:"https://gateway.example/v1",Models:[]ModelConfig{{ID:"model"}}}
 cfg:=RequestConfig{Model:"model",Mode:Stateless,Instructions:"stable policy",Next:[]HistoryItem{{Type:"user",Content:"original task"}}}
 a,e:=BuildResponsesRequest(cfg);if e!=nil{t.Fatal(e)}
 cfg.Next=[]HistoryItem{{Type:"user",Content:"unrelated reduced retry"}}
 b,e:=BuildResponsesRequest(cfg);if e!=nil{t.Fatal(e)}
 cfg.History=[]HistoryItem{{Type:"user",Content:"original task"}}
 cfg.Next=[]HistoryItem{{Type:"user",Content:"continue"}}
 c,e:=BuildResponsesRequest(cfg);if e!=nil{t.Fatal(e)}
 if _,e=s.RecordProviderAttempt(x.ID,x.Owner,x.ProviderRun.ID,1,original,false);e!=nil{t.Fatal(e)}
 now:=time.Now().UTC()
 read:=100;u:=ProviderCacheUsage{ReadTokens:&read,ComparableTotal:true}
 for i,v:=range []struct{body []byte;success bool}{{a,true},{b,false},{c,true}}{
  if _,e=s.RecordProviderAttempt(x.ID,x.Owner,x.ProviderRun.ID,uint64(i+2),v.body,false);e!=nil{t.Fatal(e)}
  detail,_,e:=s.RecordCacheObservation(x.ID,x.Owner,x.ProviderRun.ID,uint64(i+2),p,"credential",v.body,now.Add(time.Duration(i)*time.Minute),200,u,v.success)
  if e!=nil{t.Fatal(e)}
  if i==2&&(!strings.Contains(detail,"reason=history_prefix_changed")||!strings.Contains(detail,"last_success_reason=append_only")||!strings.Contains(detail,"last_success_common_items=1")){t.Fatalf("lost success baseline: %s",detail)}
  s,e=NewStore(s.dir);if e!=nil{t.Fatal(e)}
 }
}


func TestCacheObservationNearZeroUsesComparableSamplesOnly(t *testing.T){
 s,x,original:=sendingAttemptFixture(t)
 p:=ProviderConfig{ID:"provider",APIType:"responses",BaseURL:"https://gateway.example/v1",Models:[]ModelConfig{{ID:"model"}}}
 body,e:=BuildResponsesRequest(RequestConfig{Model:"model",Mode:Stateless,Instructions:"policy",Next:[]HistoryItem{{Type:"user",Content:"goal"}}});if e!=nil{t.Fatal(e)}
 if _,e=s.RecordProviderAttempt(x.ID,x.Owner,x.ProviderRun.ID,1,original,false);e!=nil{t.Fatal(e)}
 now:=time.Now().UTC()
 zero,below,boundary,above:=0,9,10,800
 usages:=[]ProviderCacheUsage{{},{ReadTokens:&zero,ComparableTotal:true},{ReadTokens:&below,ComparableTotal:true},{ReadTokens:&boundary,ComparableTotal:true},{ReadTokens:&above,ComparableTotal:true},{ReadTokens:&zero}}
 for i,u:=range usages{
  attempt:=uint64(i+2)
  if _,e=s.RecordProviderAttempt(x.ID,x.Owner,x.ProviderRun.ID,attempt,body,false);e!=nil{t.Fatal(e)}
  _,stats,e:=s.RecordCacheObservation(x.ID,x.Owner,x.ProviderRun.ID,attempt,p,"credential",body,now.Add(time.Duration(i)*time.Second),1000,u,true)
  if e!=nil{t.Fatal(e)}
  if i==0&&!strings.Contains(stats,"reported_cache_read_total=unreported"){t.Fatal(stats)}
  if i==len(usages)-1&&(!strings.Contains(stats,"near_zero_samples=4; near_zero_hits=2; near_zero_rate=50.0%")||!strings.Contains(stats,"zero_read=2")){t.Fatal(stats)}
 }
}
