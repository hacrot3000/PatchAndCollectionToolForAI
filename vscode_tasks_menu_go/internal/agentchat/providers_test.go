package agentchat

import (
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func sampleProvider()ProviderConfig{
 return ProviderConfig{ID:"custom-r2d",Name:"Custom Endpoint",APIType:"responses",BaseURL:"https://api.r2d.dev/v1",
  Models:[]ModelConfig{{ID:"composer-2.5",Name:"Composer",ToolCalling:true,Vision:true,MaxInputTokens:1280000,MaxOutputTokens:160000,ReasoningEfforts:[]string{"low","medium","high"},DefaultReasoningEffort:"medium"}}}
}
func TestProviderValidationAndResponsePath(t *testing.T){
 p:=sampleProvider()
 if err:=ValidateProvider(p);err!=nil{t.Fatal(err)}
 u,err:=p.ResponsesURL();if err!=nil||u!="https://api.r2d.dev/v1/responses"{t.Fatalf("%s %v",u,err)}
 p.BaseURL="https://api.r2d.dev/v1/responses"
 u,err=p.ResponsesURL();if err!=nil||strings.Contains(u,"responses/responses"){t.Fatalf("duplicate endpoint: %q %v",u,err)}
}
func TestProviderRejectsUnsafeDestinations(t *testing.T){
 for _,endpoint:=range []string{
 "http://api.r2d.dev/v1","https://localhost/v1","https://foo.local/v1",
 "https://127.0.0.1/v1","https://192.168.1.1/v1","https://169.254.169.254/latest",
 "https://100.64.0.2/v1","https://198.19.0.1/v1","https://[::1]/v1",
 "https://user:pass@api.r2d.dev/v1","https://api.r2d.dev/v1?token=x",
 }{
  p:=sampleProvider();p.BaseURL=endpoint
  if err:=ValidateProvider(p);err==nil{t.Errorf("accepted unsafe URL: %s",endpoint)}
 }
}
func TestProviderModelAndReasoningValidation(t *testing.T){
 p:=sampleProvider()
 p.Models[0].DefaultReasoningEffort="max"
 if ValidateProvider(p)==nil{t.Fatal("accepted undeclared reasoning effort")}
 p=sampleProvider();p.Models=append(p.Models,p.Models[0])
 if ValidateProvider(p)==nil{t.Fatal("accepted duplicate model")}
 p=sampleProvider();p.APIType="chat_completions"
 if ValidateProvider(p)==nil{t.Fatal("accepted unsupported adapter")}
}
func TestProviderStorePersistsWithoutSecretOrOwnerLeak(t *testing.T){
 dir:=filepath.Join(t.TempDir(),"providers")
 s,err:=NewProviderStore(dir);if err!=nil{t.Fatal(err)}
 p:=sampleProvider()
 if err=s.Put("alice",p);err!=nil{t.Fatal(err)}
 ps,err:=s.List("alice");if err!=nil||len(ps)!=1{t.Fatalf("missing provider: %v %+v",err,ps)}
 ps,err=s.List("bob");if err!=nil||len(ps)!=0{t.Fatalf("provider leaked to other owner: %v %+v",err,ps)}
 again,err:=NewProviderStore(dir);if err!=nil{t.Fatal(err)}
 got,err:=again.Get("alice","custom-r2d");if err!=nil||got.BaseURL!=p.BaseURL{t.Fatalf("provider not persisted: %v %+v",err,got)}
 path,err:=s.file("alice");if err!=nil{t.Fatal(err)}
 stat,err:=os.Stat(path);if err!=nil{t.Fatal(err)}
 if stat.Mode().Perm()!=0600{t.Fatalf("private config file mode %o",stat.Mode().Perm())}
 if strings.Contains(path,"alice"){t.Fatal("owner should not appear in filename")}
 raw,err:=os.ReadFile(path);if err!=nil{t.Fatal(err)}
 if strings.Contains(strings.ToLower(string(raw)),"api_key"){t.Fatal("key field leaked into config")}
 p.Name="Changed"
 if err=s.Put("alice",p);err!=nil{t.Fatal(err)}
 ps,err=s.List("alice");if err!=nil||len(ps)!=1||ps[0].Name!="Changed"{t.Fatalf("upsert failed %v %+v",err,ps)}
}
