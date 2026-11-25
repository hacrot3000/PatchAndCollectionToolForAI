package collectionprogress

import (
 "strings"
 "testing"
)

func TestCanonicalFingerprintRetainsExecutionSemantics(t *testing.T){
 a:=`{"project":{"key":"key","id":"p"},"id":"x","actions":[{"type":"file","path":"src/a","start_line":1,"end_line":100,"title":"first"}],"limits":{"max_bytes":9007199254740993},"ai_context":{"mode":"a"}}`
 b:=`{"ai_context":{"mode":"a"},"limits":{"max_bytes":9007199254740993},"title":"changed","actions":[{"id":"new","end_line":100,"start_line":1,"path":"src/a","type":"file"}],"project":{"id":"p","key":"key"}}`
 first,err:=CanonicalRequestFingerprint([]byte(a));if err!=nil{t.Fatal(err)}
 second,err:=CanonicalRequestFingerprint([]byte(b));if err!=nil||first!=second{t.Fatalf("display metadata or key order changed fingerprint: %v",err)}
 for _,pair:=range [][2]string{{"src/a","src/b"},{"100","101"},{"9007199254740993","9007199254740992"},{`"mode":"a"`,`"mode":"b"`},{`"id":"p"`,`"id":"other"`}}{
  got,err:=CanonicalRequestFingerprint([]byte(strings.Replace(a,pair[0],pair[1],1)))
  if err!=nil||got==first{t.Fatalf("execution semantics collapsed: %v %v",pair,err)}
 }
 for _,raw:=range []string{`null`,`{"actions":[],"actions":[]}`,`{"actions":[{"path":"a","path":"b"}]}`,`{} {}`}{
  if _,err:=CanonicalRequestFingerprint([]byte(raw));err==nil{t.Fatalf("ambiguous fingerprint: %s",raw)}
 }
}

func TestCollectionScopeRequiresKnownRevisionForReuse(t *testing.T){
 objective:=counterFixture(t).Objective
 raw:=[]byte(`{"project":{"key":"key","id":"p"},"actions":[{"type":"file","path":"a"}]}`)
 revision:=strings.Repeat("a",64)
 s,err:=BindCollectionScope(objective,"key","p",raw,revision)
 if err!=nil||!s.SameVerifiedScope(s){t.Fatalf("scope binding failed: %+v %v",s,err)}
 unknown,err:=BindCollectionScope(objective,"key","p",raw,"")
 if err!=nil||unknown.SameVerifiedScope(unknown)||s.SameVerifiedScope(unknown){t.Fatal("missing source revision permitted reuse")}
 other,err:=BindCollectionScope(objective,"key","p",raw,strings.Repeat("b",64))
 if err!=nil||s.SameVerifiedScope(other){t.Fatal("changed revision permitted reuse")}
 if _,err:=BindCollectionScope(objective,"key","foreign",raw,revision);err==nil{t.Fatal("foreign project accepted")}
 changed:=objective;changed.Key=strings.Repeat("c",64)
 other,err=BindCollectionScope(changed,"key","p",raw,revision)
 if err!=nil||s.SameVerifiedScope(other){t.Fatal("cross-objective evidence reuse")}
}
