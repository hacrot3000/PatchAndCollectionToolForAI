package collectionprogress

import(
 "encoding/json"
 "fmt"
 "strings"
 "testing"
)

func TestNativeRangeCoverageBoundToReceipt(t *testing.T){
 b:=counterFixture(t).Objective
 exact:=NewEvidenceIndex(b)
 var coverage *CoverageIndex
 observe:=func(path,rev string,first,last uint64,mutate func(map[string]any))Novelty{
  t.Helper()
  req:=[]byte(fmt.Sprintf(`{"project":{"key":"key","id":"id"},"actions":[{"type":"range","path":%q,"start_line":%d,"end_line":%d}]}`,path,first,last))
  scope,err:=BindCollectionScope(b,"key","id",req,"");if err!=nil{t.Fatal(err)}
  proof:=map[string]any{"path":path,"type":"range","action_index":1,"total_lines":2000,"start_line":first,"end_line":last,"source_revision_sha256":strings.Repeat(rev,64),"report_sha256":strings.Repeat("f",64)}
  o:=map[string]any{"source":"native_patch_tool_collect","project":map[string]string{"key":"key","id":"id"},"native_session_id":"native","run_id":"read","status":"PASS","native_status":"PASS","rc":0,"evidence_verified":true,"text_excerpt_truncated":false,"native_line_reads":[]any{proof}}
  if mutate!=nil{mutate(o)}
  raw,_:=json.Marshal(o)
  var n Novelty
  exact,coverage,n=ObserveNativeRead(exact,coverage,b,scope,"key","id",req,string(raw));return n
 }
 for i,tc:=range []struct{path,rev string;first,last uint64;want Novelty}{
  {"a.go","a",1,100,VerifiedNew},{"a.go","a",1,110,VerifiedNew},{"b.go","b",1,100,VerifiedNew},
  {"a.go","a",20,60,VerifiedDuplicate},{"b.go","b",20,60,VerifiedDuplicate},
  {"a.go","c",1,100,VerifiedNew},{"a.go","a",1,100,VerifiedNew},
 }{if n:=observe(tc.path,tc.rev,tc.first,tc.last,nil);n!=tc.want{t.Fatalf("step %d: %s want %s",i,n,tc.want)}}
 for i:=uint64(0);i<55;i++{if n:=observe("a.go","a",111+10*i,120+10*i,nil);n!=VerifiedNew{t.Fatalf("page %d: %s",i,n)}}
 before,_:=json.Marshal(coverage)
 for _,mutate:=range []func(map[string]any){
  func(o map[string]any){o["text_excerpt_truncated"]=true},
  func(o map[string]any){o["project"]=map[string]string{"key":"foreign","id":"id"}},
  func(o map[string]any){o["native_line_reads"]=[]any{}},
  func(o map[string]any){o["native_line_reads"].([]any)[0].(map[string]any)["start_line"]=2},
  func(o map[string]any){o["native_line_reads"].([]any)[0].(map[string]any)["source_revision_sha256"]=""},
  func(o map[string]any){o["native_line_reads"].([]any)[0].(map[string]any)["path"]="other.go"},
 }{if n:=observe("a.go","a",1,100,mutate);n!=UnknownNovelty{t.Fatalf("unverified receipt: %s",n)}}
 after,_:=json.Marshal(coverage)
 if string(before)!=string(after){t.Fatal("invalid receipt changed coverage")}
 if !ValidateCoverage(*coverage,b){t.Fatal("invalid durable index")}
 invalidated:=InvalidateCoverage(*coverage,nil,false);coverage=&invalidated
 if n:=observe("a.go","a",1,100,nil);n!=UnknownNovelty{t.Fatal("post-mutation absence incorrectly classified")}
 if n:=observe("a.go","a",1,100,nil);n!=VerifiedDuplicate{t.Fatal("fresh proof was not retained")}
}

 // Build only metadata: 150 MiB native source inspection is exercised by
 // the server integration test. The guard must not open or hash source files.
func boundedCoverageFixture(t testing.TB, scopes int)(ObjectiveBinding,EvidenceIndex,*CoverageIndex,ScopeDescriptor,[]byte,string){
 t.Helper()
 b,err:=BindObjective(ObjectiveScope{Owner:"local",ProjectID:"p",OwnerWorkspace:"w",ExecutionWorkspace:"w",ConversationID:"c",Goal:"Audit source read-only",Generation:1});if err!=nil{t.Fatal(err)}
 index:=NewCoverageIndex(b)
 for i:=0;i<scopes;i++{
  key,ok:=NewCoverageKey(b,fmt.Sprintf("src/f%d.go",i),strings.Repeat("a",64));if !ok{t.Fatal("key")}
  for j:=uint64(0);j<16;j++{index,_=ObserveCoverage(index,key,LineRange{1+j*2,1+j*2})}
 }
 if !ValidateCoverage(index,b){t.Fatal("invalid fixture")}
 req:=[]byte(`{"project":{"key":"key","id":"id"},"actions":[{"type":"range","path":"src/f0.go","start_line":1,"end_line":1}]}`)
 scope,err:=BindCollectionScope(b,"key","id",req,"");if err!=nil{t.Fatal(err)}
 raw,err:=json.Marshal(map[string]any{"source":"native_patch_tool_collect","project":map[string]string{"key":"key","id":"id"},"native_session_id":"native","run_id":"read","status":"PASS","native_status":"PASS","rc":0,"evidence_verified":true,"text_excerpt_truncated":false,"native_line_reads":[]any{map[string]any{"path":"src/f0.go","type":"range","action_index":1,"total_lines":1228800,"start_line":1,"end_line":1,"source_revision_sha256":strings.Repeat("a",64),"report_sha256":strings.Repeat("f",64)}}});if err!=nil{t.Fatal(err)}
 return b,NewEvidenceIndex(b),&index,scope,req,string(raw)
}

func TestNativeCoverageMetadataAggregateBoundAndRenameDelete(t *testing.T){
 b,exact,index,scope,req,receipt:=boundedCoverageFixture(t,MaxCoverageScopes)
 // Add a 1025th interval on an existing scope, which evicts only metadata.
 req=[]byte(strings.Replace(string(req),`"start_line":1,"end_line":1`,`"start_line":100,"end_line":100`,1))
 receipt=strings.Replace(receipt,`"start_line":1`,`"start_line":100`,1)
 receipt=strings.Replace(receipt,`"end_line":1`,`"end_line":100`,1)
 var err error
 scope,err=BindCollectionScope(b,"key","id",req,"");if err!=nil{t.Fatal(err)}
 _,next,n:=ObserveNativeRead(exact,index,b,scope,"key","id",req,receipt)
 if n!=UnknownNovelty||next==nil||!next.Evicted||!ValidateCoverage(*next,b)||len(next.Entries)>=MaxCoverageScopes{t.Fatal("aggregate eviction must be conservative")}
 raw,err:=json.Marshal(next);if err!=nil||len(raw)>64<<10{t.Fatalf("coverage metadata bytes=%d err=%v",len(raw),err)}
 // Complete rename/delete paths invalidate both old and new names. The
 // caller is responsible for proving completeness; unknown paths drop all.
 renamed:=InvalidateCoverage(*index,[]string{"src/f0.go","src/f1.go","src/f2.go"},true)
 if len(renamed.Entries)!=MaxCoverageScopes-3||!renamed.Evicted||!ValidateCoverage(renamed,b){t.Fatal("rename/delete path invalidation")}
 for _,path:=range []string{"src/f0.go","src/f1.go","src/f2.go"}{
  key,_:=NewCoverageKey(b,path,strings.Repeat("a",64))
  if _,c:=ObserveCoverage(renamed,key,LineRange{1,1});c.Novelty!=UnknownNovelty{t.Fatal("stale rename/delete path proof reused")}
 }
 key,_:=NewCoverageKey(b,"src/f3.go",strings.Repeat("a",64))
 if _,c:=ObserveCoverage(renamed,key,LineRange{1,1});c.Novelty!=VerifiedDuplicate{t.Fatal("unrelated complete proof discarded")}
 t.Logf("bounded coverage projection: %d bytes, %d retained scopes",len(raw),len(next.Entries))
}

func BenchmarkNativeCoverageMetadata(b *testing.B){
 for _,scopes:=range []int{1,MaxCoverageScopes}{
  b.Run(fmt.Sprintf("scopes_%d",scopes),func(b *testing.B){
   objective,exact,index,scope,req,receipt:=boundedCoverageFixture(b,scopes)
   b.ReportAllocs()
   b.ResetTimer()
   for i:=0;i<b.N;i++{
    _,next,n:=ObserveNativeRead(exact,index,objective,scope,"key","id",req,receipt)
    if n!=VerifiedDuplicate||next==nil{b.Fatal("unexpected novelty")}
   }
  })
 }
}
