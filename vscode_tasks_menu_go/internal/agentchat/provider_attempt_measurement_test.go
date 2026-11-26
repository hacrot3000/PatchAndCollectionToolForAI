package agentchat

import(
 "context"
 "testing"
 "time"
)
type clp20Measurement struct{
 attempt int
 measurement ProviderPayloadMeasurement
 digest string
 outcome ProviderAttemptOutcome
}
type clp20MeasurementResolver struct{
 fakeProviderResolver
 records []clp20Measurement
}
func(r *clp20MeasurementResolver) ObserveProviderAttempt(_ RunSpec,attempt int,m ProviderPayloadMeasurement,digest string,outcome ProviderAttemptOutcome){
 r.records=append(r.records,clp20Measurement{attempt,m,digest,outcome})
}
func TestConfiguredAdapterCLP20MeasuresEveryAttempt(t *testing.T){
 for _,api:=range []string{"responses","chat_completions"}{
  for _,tc:=range []struct{name string;mode RetryMode;seq []*ProviderError;want []ProviderAttemptOutcome}{
   {"success",RetryOff,nil,[]ProviderAttemptOutcome{AttemptResponse}},
   {"permanent",RetryUnlimited,[]*ProviderError{{HTTPStatus:401}},[]ProviderAttemptOutcome{AttemptHTTPRejected}},
   {"retry",RetryLimited,[]*ProviderError{{Code:"RESPONSE_HEADER_TIMEOUT",Retryable:true}},[]ProviderAttemptOutcome{AttemptHeaderTimeout,AttemptResponse}},
  }{
   t.Run(api+"/"+tc.name,func(t *testing.T){
    p:=ProviderConfig{ID:"gpt",APIType:api,Models:[]ModelConfig{{ID:"gpt-model"}}}
    body,e:=BuildResponsesRequest(RequestConfig{Model:"gpt-model",Mode:Stateless,Instructions:"trusted policy",MaxOutputTokens:8192,Next:[]HistoryItem{{Type:"user",Content:"private goal"}}});if e!=nil{t.Fatal(e)}
    r:=&clp20MeasurementResolver{fakeProviderResolver:fakeProviderResolver{resolved:ResolvedProvider{Config:p,ConfigVersion:"cfg-v1",Credential:"owner-key"}}}
    inv:=&clp20RecoveryInvoker{t:t,original:body,sequence:tc.seq}
    a:=ConfiguredProviderAdapter{ProviderID:"gpt",Resolver:r,Builder:clp20StagnationBuilder{body},Invoker:inv,Wait:func(context.Context,time.Duration)error{return nil}}
    spec:=runSpecFor("gpt","gpt-model","measurement-run");spec.ProviderConfigVersion,spec.RetryMode="cfg-v1",tc.mode
    _=a.Execute(context.Background(),spec,nil,&fixedProviderSink{})
    expected,e:=MeasureProviderPayload(api,body);if e!=nil{t.Fatal(e)}
    if len(r.records)!=len(tc.want){t.Fatalf("observed %d want %d",len(r.records),len(tc.want))}
    for i,rec:=range r.records{if rec.attempt!=i+1||rec.measurement!=expected||rec.digest!=BodyDigest(body)||rec.outcome!=tc.want[i]{t.Fatalf("wrong measurement %+v",rec)}}
   })
  }
 }
}

func TestConfiguredAdapterCLP20MeasurementStopDoesNotAcknowledge(t *testing.T){
 p:=ProviderConfig{ID:"gpt",APIType:"responses",Models:[]ModelConfig{{ID:"gpt-model"}}}
 body,e:=BuildResponsesRequest(RequestConfig{Model:"gpt-model",Mode:Stateless,Instructions:"policy",MaxOutputTokens:8192,Next:[]HistoryItem{{Type:"user",Content:"objective"}}});if e!=nil{t.Fatal(e)}
 r:=&clp20MeasurementResolver{fakeProviderResolver:fakeProviderResolver{resolved:ResolvedProvider{Config:p,ConfigVersion:"cfg-v1",Credential:"owner-key"}}}
 ctx,cancel:=context.WithCancel(context.Background());defer cancel()
 inv:=&clp20CancelAfterReturnInvoker{cancel:cancel}
 a:=ConfiguredProviderAdapter{ProviderID:"gpt",Resolver:r,Builder:clp20StagnationBuilder{body},Invoker:inv}
 spec:=runSpecFor("gpt","gpt-model","measurement-stop");spec.ProviderConfigVersion,spec.RetryMode="cfg-v1",RetryUnlimited
 sink:=&fixedProviderSink{}
 err:=a.Execute(ctx,spec,nil,sink)
 if err==nil||len(sink.events)!=0||len(r.records)!=1||r.records[0].outcome!=AttemptCancelled||r.records[0].digest!=BodyDigest(body){t.Fatalf("cancelled observation became ACK or was lost: %v %+v %+v",err,r.records,sink.events)}
}
