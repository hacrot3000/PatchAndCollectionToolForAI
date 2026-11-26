package agentchat

import (
 "context"
 "encoding/json"
 "reflect"
 "strings"
 "testing"
 "time"
)

type clp20RecoveryInvoker struct{
 t *testing.T
 original []byte
 sequence []*ProviderError
 payloads [][]byte
}
func(i *clp20RecoveryInvoker) Invoke(_ context.Context,p ProviderConfig,key string,payload []byte)(ResponsesResult,error){
 i.payloads=append(i.payloads,append([]byte(nil),payload...))
 if p.ID!="gpt"||key!="owner-key"{i.t.Fatal("provider binding changed")}
 var before,after map[string]json.RawMessage
 if json.Unmarshal(i.original,&before)!=nil||json.Unmarshal(payload,&after)!=nil{i.t.Fatal("invalid canonical payload")}
 for _,field:=range []string{"input","tools","model","store"}{
  if !reflect.DeepEqual(before[field],after[field]){i.t.Fatalf("mandatory %s changed",field)}
 }
 if len(i.payloads)<=len(i.sequence){return ResponsesResult{},i.sequence[len(i.payloads)-1]}
 return ResponsesResult{Text:"done"},nil
}

func TestConfiguredAdapterCLP20BoundedRecovery(t *testing.T){
 contextErr:=&ProviderError{Code:"PROVIDER_HTTP_400",HTTPStatus:400,ProviderErrorCode:"context_length_exceeded"}
 incomplete:=&ProviderError{Code:"RESPONSE_INCOMPLETE",ProviderErrorCode:"max_output_tokens",Retryable:true}
 timeout:=&ProviderError{Code:"RESPONSE_HEADER_TIMEOUT",Retryable:true}
 generic:=&ProviderError{Code:"PROVIDER_HTTP_400",HTTPStatus:400,ProviderErrorCode:"invalid_request_error"}
 ambiguous:=&ProviderError{Code:"PROVIDER_HTTP_400",HTTPStatus:400,ProviderErrorCode:"context_length_exceeded",Ambiguous:true}
 for _,api:=range []string{"responses","chat_completions"}{
  for _,tc:=range []struct{name string; mode RetryMode;initial int;seq []*ProviderError;want []int;success bool}{
   {"progressive",RetryUnlimited,65536,[]*ProviderError{contextErr,contextErr,contextErr},[]int{65536,32768,16384,8192},true},
   {"bounded",RetryUnlimited,65536,[]*ProviderError{contextErr,contextErr,contextErr,contextErr},[]int{65536,32768,16384,8192},false},
   {"off",RetryOff,65536,[]*ProviderError{contextErr},[]int{65536},false},
   {"unset_mode","",65536,[]*ProviderError{contextErr},[]int{65536},false},
   {"minimum",RetryUnlimited,8192,[]*ProviderError{contextErr},[]int{8192},false},
   {"generic",RetryUnlimited,65536,[]*ProviderError{generic},[]int{65536},false},
   {"ambiguous",RetryUnlimited,65536,[]*ProviderError{ambiguous},[]int{65536},false},
   {"incomplete",RetryLimited,8192,[]*ProviderError{incomplete,timeout},[]int{8192,16384,16384},true},
   {"incomplete_twice",RetryUnlimited,8192,[]*ProviderError{incomplete,incomplete},[]int{8192,16384},false},
  }{
   t.Run(api+"/"+tc.name,func(t *testing.T){
    provider:=ProviderConfig{ID:"gpt",APIType:api,Models:[]ModelConfig{{ID:"gpt-model",MaxOutputTokens:65536}}}
    history:=[]HistoryItem{
     {Type:"user",Content:"original private goal"},
     {Type:"function_call",CallID:"patch",Name:"taskdeck_submit_patch",Arguments:`{"name":"p"}`},
     {Type:"function_call_output",CallID:"patch",Content:`{"status":"PASS","run_id":"prior"}`},
     {Type:"user",Content:"pinned new task"},
     {Type:"function_call",CallID:"collect",Name:"taskdeck_collect_workspace",Arguments:`{"actions":[{"type":"file","path":"a"}]}`},
     {Type:"function_call_output",CallID:"collect",Content:strings.Repeat("exact private evidence ",100)},
    }
    body,err:=BuildResponsesRequest(RequestConfig{Model:"gpt-model",Mode:Stateless,Instructions:"trusted policy",History:history[:5],Next:history[5:],EnableCollectionTool:true,MaxOutputTokens:tc.initial})
    if err!=nil{t.Fatal(err)}
    inv:=&clp20RecoveryInvoker{t:t,original:body,sequence:tc.seq}
    resolver:=&fakeProviderResolver{resolved:ResolvedProvider{Config:provider,ConfigVersion:"cfg-v1",Credential:"owner-key"}}
    adapter:=ConfiguredProviderAdapter{ProviderID:"gpt",Resolver:resolver,Builder:clp20StagnationBuilder{body},Invoker:inv,Wait:func(context.Context,time.Duration)error{return nil}}
    spec:=runSpecFor("gpt","gpt-model","run-clp20-recovery");spec.ProviderConfigVersion,spec.RetryMode="cfg-v1",tc.mode
    sink:=&fixedProviderSink{}
    err=adapter.Execute(context.Background(),spec,history,sink)
    if (err==nil)!=tc.success||len(inv.payloads)!=len(tc.want){t.Fatalf("attempts=%d want=%d error=%v",len(inv.payloads),len(tc.want),err)}
    for n,payload:=range inv.payloads{
     m,e:=MeasureProviderPayload(api,payload);if e!=nil||m.OutputBudget!=tc.want[n]{t.Fatalf("wrong reservation %v %v",m,e)}
     if n>0&&tc.want[n]==tc.want[n-1]&&!reflect.DeepEqual(payload,inv.payloads[n-1]){t.Fatal("transport retry changed effective bytes")}
    }
    retries:=0
    for _,e:=range sink.events{
     if e.Type==RunEventToolCall{t.Fatal("recovery published invented tool")}
     if e.Type!=RunEventRetry{continue}
     var record struct{Digest string `json:"payload_sha256"`;Next string `json:"next_payload_sha256"`}
     if json.Unmarshal(e.Payload,&record)!=nil||record.Digest!=BodyDigest(inv.payloads[retries])||record.Next!=BodyDigest(inv.payloads[retries+1]){t.Fatal("retry digest mismatch")}
     if strings.Contains(string(e.Payload),"private evidence")||strings.Contains(string(e.Payload),"owner-key"){t.Fatal("retry leaked data")}
     retries++
    }
    if retries!=len(tc.want)-1{t.Fatal("wrong retry count")}
   })
  }
 }
}
