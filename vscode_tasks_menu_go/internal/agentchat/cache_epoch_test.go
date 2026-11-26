package agentchat

import (
 "os"
 "path/filepath"
 "testing"
)

func TestCacheEpochStorePrivateRestartAndOwnerFence(t *testing.T){
 dir:=filepath.Join(t.TempDir(),"private")
 s,err:=NewStore(dir);if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/project","project-id","gateway","model",Stateless);if err!=nil{t.Fatal(err)}
 x,err=s.Save(x,0);if err!=nil{t.Fatal(err)}
 digest:=BodyDigest([]byte("test"))
 e:=&CacheEpoch{ID:digest,Scope:digest,SourceDigest:digest,SourceCount:1,Visible:[]HistoryItem{{Type:"user",Content:"goal"}}}
 if err=s.WriteCacheEpoch(x.ID,x.Owner,x.Revision,e);err!=nil{t.Fatal(err)}
 s,err=NewStore(dir);if err!=nil{t.Fatal(err)}
 got,err:=s.ReadCacheEpoch(x.ID,x.Owner,x.Revision)
 if err!=nil||got==nil||got.ID!=e.ID||got.Visible[0].Content!="goal"{t.Fatalf("restart: %+v %v",got,err)}
 if _,err=s.ReadCacheEpoch(x.ID,"other",x.Revision);err==nil{t.Fatal("cross-owner read")}
 if err=s.WriteCacheEpoch(x.ID,x.Owner,x.Revision+1,e);err==nil{t.Fatal("stale revision write")}
 if err=os.WriteFile(s.cacheEpochPath(x.ID),[]byte("{}"),0600);err!=nil{t.Fatal(err)}
 if _,err=s.ReadCacheEpoch(x.ID,x.Owner,x.Revision);err==nil{t.Fatal("corrupt sidecar accepted")}
 if err=s.WriteCacheEpoch(x.ID,x.Owner,x.Revision,nil);err!=nil{t.Fatal(err)}
 if _,err=os.Stat(s.cacheEpochPath(x.ID));!os.IsNotExist(err){t.Fatal("invalidated cache retained")}
}
func TestCachePrefixDiagnosticsAndLifetime(t *testing.T){
 c:=RequestConfig{Model:"model",Mode:Stateless,Instructions:"stable policy",Next:[]HistoryItem{{Type:"user",Content:"goal"}}}
 first,err:=BuildResponsesRequest(c);if err!=nil{t.Fatal(err)}
 c.History=c.Next;c.Next=[]HistoryItem{{Type:"user",Content:"more"}}
 second,err:=BuildResponsesRequest(c);if err!=nil{t.Fatal(err)}
 for _,api:=range []string{"responses","chat_completions"}{
  a,err:=cacheSnapshot(api,first);if err!=nil{t.Fatal(err)}
  b,err:=cacheSnapshot(api,second);if err!=nil{t.Fatal(err)}
  bytes,n,reason:=compareCacheSnapshots(a,b);if bytes<=0||n!=1||reason!="append_only"{t.Fatalf("%s: %d %d %s",api,bytes,n,reason)}
  b.Instructions=cachePart([]byte("changed"))
  if _,_,reason=compareCacheSnapshots(a,b);reason!="instructions_changed"{t.Fatal(reason)}
 }
 c.PromptCacheRetention="24h"
 body,err:=BuildResponsesRequest(c);if err!=nil{t.Fatal(err)}
 if _,err=BuildChatCompletionsRequest(body);err!=nil{t.Fatal(err)}
 c.PromptCacheTTL="12h"
 if _,err=BuildResponsesRequest(c);err==nil{t.Fatal("mixed lifetime accepted")}
 c.PromptCacheRetention=""
 body,err=BuildResponsesRequest(c);if err!=nil{t.Fatal(err)}
 if _,err=BuildChatCompletionsRequest(body);err==nil{t.Fatal("Responses-only TTL forwarded to Chat Completions")}
}


func TestCacheEpochInvalidationExactSendingRun(t *testing.T){
 s,x,_:=sendingAttemptFixture(t)
 d:=BodyDigest([]byte("test"))
 e:=&CacheEpoch{ID:d,Scope:d,SourceDigest:d,SourceCount:1,Visible:[]HistoryItem{{Type:"user",Content:"goal"}}}
 if err:=s.WriteCacheEpoch(x.ID,x.Owner,x.Revision,e);err!=nil{t.Fatal(err)}
 if err:=s.InvalidateCacheEpochForRun(x.ID,"other",x.ProviderRun.ID);err==nil{t.Fatal("cross-owner invalidation")}
 if err:=s.InvalidateCacheEpochForRun(x.ID,x.Owner,"older-run");err==nil{t.Fatal("stale run invalidation")}
 if got,err:=s.ReadCacheEpoch(x.ID,x.Owner,x.Revision);err!=nil||got==nil{t.Fatal("checkpoint unexpectedly removed")}
 if err:=s.InvalidateCacheEpochForRun(x.ID,x.Owner,x.ProviderRun.ID);err!=nil{t.Fatal(err)}
 if got,err:=s.ReadCacheEpoch(x.ID,x.Owner,x.Revision);err!=nil||got!=nil{t.Fatal("rewritten retry kept old checkpoint")}
 current,err:=s.Get(x.ID,x.Owner);if err!=nil||current.Revision!=x.Revision||current.ProviderRun.PayloadSHA256!=x.ProviderRun.PayloadSHA256||current.ProviderRun.Phase!="sending"{t.Fatal("canonical journal changed")}
}


func TestFullHistoryMetadataEpochStoreNeverContainsSource(t *testing.T){
 s,err:=NewStore(filepath.Join(t.TempDir(),"private"));if err!=nil{t.Fatal(err)}
 x,err:=NewSession("local","/project","project-id","gateway","m",Stateless);if err!=nil{t.Fatal(err)}
 x,err=s.Save(x,0);if err!=nil{t.Fatal(err)}
 d:=BodyDigest([]byte("private source"))
 e:=&CacheEpoch{ID:d,Scope:d,SourceCount:1,SourceDigest:d,VisibleCount:1,VisibleDigest:d}
 if err=s.WriteCacheEpoch(x.ID,x.Owner,x.Revision,e);err!=nil{t.Fatal(err)}
 raw,err:=os.ReadFile(s.cacheEpochPath(x.ID));if err!=nil||len(raw)>1024{t.Fatalf("metadata size: %d %v",len(raw),err)}
 s,err=NewStore(s.dir);if err!=nil{t.Fatal(err)}
 got,err:=s.ReadCacheEpoch(x.ID,x.Owner,x.Revision);if err!=nil||got==nil||got.VisibleDigest!=d||len(got.Visible)!=0{t.Fatal("metadata restart failed")}
 e.Visible=[]HistoryItem{{Type:"user",Content:"private source"}}
 if err=s.WriteCacheEpoch(x.ID,x.Owner,x.Revision,e);err==nil{t.Fatal("mixed raw+metadata checkpoint accepted")}
}
