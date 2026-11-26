package agentchat

import(
 "encoding/json"
 "strings"
 "testing"
 "time"
)
func TestCacheTokenEstimateConservativeSampleWindow(t *testing.T){
 now:=time.Now().UTC()
 samples:=make([]cacheTokenSample,0,16)
 for i:=0;i<8;i++{samples=append(samples,cacheTokenSample{Wire:280000,Input:70000,At:now.Add(-time.Duration(i)*time.Minute)})}
 if _,ok:=learnedCacheEstimate(samples[:7],280000,now);ok{t.Fatal("insufficient evidence")}
 if n,ok:=learnedCacheEstimate(samples,280000,now);!ok||n!=112004{t.Fatalf("unexpected conservative estimate %d %v",n,ok)}
 for _,wire:=range []int{100000,400000}{if _,ok:=learnedCacheEstimate(samples,wire,now);ok{t.Fatal("extrapolated beyond sampled sizes")}}
 if _,ok:=learnedCacheEstimate(samples,280000,now.Add(25*time.Hour));ok{t.Fatal("expired evidence accepted")}
 samples[0].Input=140000
 if _,ok:=learnedCacheEstimate(samples,280000,now);ok{t.Fatal("dense sample did not revoke relaxation")}
}
func TestCacheTokenSamplesResetOnUntrustedOrChangingContext(t *testing.T){
 now:=time.Now().UTC()
 prefix:=cachePrefixSnapshot{Instructions:cachePart([]byte("stable")),Tools:cachePart([]byte("tools"))}
 read:=10000
 usage:=ProviderCacheUsage{ReadTokens:&read,ComparableTotal:true}
 var samples []cacheTokenSample
 for i:=0;i<30;i++{samples=updateCacheTokenSamples(samples,prefix,prefix,280000,70000,usage,true,now)}
 if len(samples)!=16{t.Fatal("sample bound")}
 changed:=prefix;changed.Instructions=cachePart([]byte("new authority"))
 if n:=len(updateCacheTokenSamples(samples,prefix,changed,280000,70000,usage,true,now));n!=1{t.Fatal("policy change kept old samples")}
 if updateCacheTokenSamples(samples,prefix,prefix,280000,70000,usage,false,now)!=nil{t.Fatal("failed request retained confidence")}
 if updateCacheTokenSamples(samples,prefix,prefix,280000,70000,ProviderCacheUsage{},true,now)!=nil{t.Fatal("missing usage learned")}
 changed=prefix;changed.Stateful=true
 if updateCacheTokenSamples(samples,prefix,changed,280000,70000,usage,true,now)!=nil{t.Fatal("stateful usage learned")}
}
func TestCacheObservationExactAttemptAndWriteReporting(t *testing.T){
 s,x,_:=sendingAttemptFixture(t)
 p:=ProviderConfig{ID:"provider",APIType:"responses",BaseURL:"https://gateway.example/v1",Models:[]ModelConfig{{ID:"model"}}}
 body,err:=BuildResponsesRequest(RequestConfig{Model:"model",Mode:Stateless,Instructions:"policy",Next:[]HistoryItem{{Type:"user",Content:strings.Repeat("evidence ",32000)}}})
 if err!=nil{t.Fatal(err)}
 // Bind the fixture's first attempt to the exact actual wire bytes.
 x.ProviderRun.PayloadSHA256=ProviderPayloadDigest(body)
 // Saving a changed SENDING payload is prohibited in some journal versions;
 // build a separate fresh fixture using the normal Store instead if needed.
 // The test uses the original SENDING binding and verifies the same digest
 // through a recorded recovery attempt, never changes staged bytes.
 current,err:=s.Get(x.ID,x.Owner);if err!=nil{t.Fatal(err)}
 original:=[]byte(`{"model":"m","input":"original immutable prompt"}`)
 if _,err=s.RecordProviderAttempt(x.ID,x.Owner,current.ProviderRun.ID,1,original,false);err!=nil{t.Fatal(err)}
 if _,err=s.RecordProviderAttempt(x.ID,x.Owner,current.ProviderRun.ID,2,body,false);err!=nil{t.Fatal(err)}
 read:=20000
 now:=time.Now().UTC()
 usage:=ProviderCacheUsage{ReadTokens:&read,ComparableTotal:true}
 _,stats,err:=s.RecordCacheObservation(x.ID,x.Owner,current.ProviderRun.ID,2,p,"credential",body,now,70000,usage,true)
 if err!=nil||!strings.Contains(stats,"write_reported=0")||!strings.Contains(stats,"reported_cache_write_total=unreported"){t.Fatalf("missing write semantics: %s %v",stats,err)}
 if _,_,err=s.RecordCacheObservation(x.ID,x.Owner,current.ProviderRun.ID,2,p,"credential",body,now,70000,usage,true);err==nil{t.Fatal("duplicate attempt counted")}
 if _,err=s.RecordProviderAttempt(x.ID,x.Owner,current.ProviderRun.ID,3,body,false);err!=nil{t.Fatal(err)}
 if _,_,err=s.RecordCacheObservation(x.ID,x.Owner,current.ProviderRun.ID,2,p,"credential",body,now.Add(time.Second),70000,usage,true);err==nil{t.Fatal("late same-digest attempt accepted")}
 if _,_,err=s.RecordCacheObservation(x.ID,x.Owner,"older-run",3,p,"credential",body,now.Add(time.Second),70000,usage,true);err==nil{t.Fatal("late same-digest run accepted")}
 zero:=0;usage.WriteTokens=&zero
 _,stats,err=s.RecordCacheObservation(x.ID,x.Owner,current.ProviderRun.ID,3,p,"credential",body,now.Add(time.Second),70000,usage,true)
 if err!=nil||!strings.Contains(stats,"write_reported=1")||!strings.Contains(stats,"reported_cache_write_total=0"){t.Fatalf("explicit zero semantics: %s %v",stats,err)}
 var request map[string]any
 _=json.Unmarshal(body,&request);request["instructions"]="changed";bad,_:=json.Marshal(request)
 if _,_,err=s.RecordCacheObservation(x.ID,x.Owner,current.ProviderRun.ID,3,p,"credential",bad,now.Add(2*time.Second),70000,usage,true);err==nil{t.Fatal("wrong payload accepted")}
}
