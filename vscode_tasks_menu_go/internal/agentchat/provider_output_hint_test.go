package agentchat

import (
 "context"
 "reflect"
 "testing"
 "time"
)

type clp20HintResolver struct{
 fakeProviderResolver
 hint int
 writes int
}
func(r *clp20HintResolver) OutputBudgetHint(context.Context,RunSpec,ResolvedProvider)int{return r.hint}
func(r *clp20HintResolver) RememberOutputBudgetHint(_ context.Context,_ RunSpec,_ ResolvedProvider,out int){r.hint=out;r.writes++}

func TestConfiguredAdapterCLP20LearnsOutputOnlyHint(t *testing.T){
 for _,api:=range []string{"responses","chat_completions"}{
  t.Run(api,func(t *testing.T){
   provider:=ProviderConfig{ID:"gpt",APIType:api,Models:[]ModelConfig{{ID:"gpt-model",MaxOutputTokens:65536}}}
   body,err:=BuildResponsesRequest(RequestConfig{Model:"gpt-model",Mode:Stateless,Instructions:"policy",MaxOutputTokens:65536,Next:[]HistoryItem{{Type:"user",Content:"immutable objective"}}})
   if err!=nil{t.Fatal(err)}
   resolver:=&clp20HintResolver{fakeProviderResolver:fakeProviderResolver{resolved:ResolvedProvider{Config:provider,ConfigVersion:"cfg-v1",Credential:"owner-key"}}}
   spec:=runSpecFor("gpt","gpt-model","hint-run");spec.ProviderConfigVersion,spec.RetryMode="cfg-v1",RetryUnlimited
   originalSpec:=spec
   inv:=&clp20RecoveryInvoker{t:t,original:body,sequence:[]*ProviderError{{Code:"PROVIDER_HTTP_400",HTTPStatus:400,ProviderErrorCode:"context_length_exceeded"}}}
   adapter:=ConfiguredProviderAdapter{ProviderID:"gpt",Resolver:resolver,Builder:clp20StagnationBuilder{body},Invoker:inv,Wait:func(context.Context,time.Duration)error{return nil}}
   if err:=adapter.Execute(context.Background(),spec,nil,&fixedProviderSink{});err!=nil||resolver.hint!=32768||resolver.writes!=1{t.Fatalf("hint not learned: %v %+v",err,resolver)}
   for _,mode:=range []RetryMode{RetryUnlimited,RetryOff}{
    next:=spec;next.RetryMode=mode
    inv:=&clp20RecoveryInvoker{t:t,original:body}
    adapter.Invoker=inv
    if err:=adapter.Execute(context.Background(),next,nil,&fixedProviderSink{});err!=nil||len(inv.payloads)!=1{t.Fatalf("next request: %v",err)}
    m,e:=MeasureProviderPayload(api,inv.payloads[0]);want:=32768;if mode==RetryOff{want=65536}
    if e!=nil||m.OutputBudget!=want||resolver.writes!=1{t.Fatalf("incorrect future hint: %v %+v",e,m)}
   }
   if !reflect.DeepEqual(spec,originalSpec)||provider.Models[0].MaxOutputTokens!=65536{t.Fatal("pinned config mutated")}
  })
 }
}
