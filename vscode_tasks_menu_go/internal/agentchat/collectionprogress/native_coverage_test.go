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
