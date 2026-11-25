package collectionprogress

import (
	"fmt"
	"strings"
	"testing"
)

func TestNativeEvidenceProofRequiresExactCompleteReceipt(t *testing.T) {
	b:=counterFixture(t).Objective
	request:=[]byte(`{"project":{"key":"key","id":"p"},"actions":[{"type":"file","path":"src/a.go"}]}`)
	scope,err:=BindCollectionScope(b,"key","p",request,"")
	if err!=nil{t.Fatal(err)}
	receipt:=fmt.Sprintf(`{"source":"native_patch_tool_collect","project":{"key":"key","id":"p"},"native_session_id":"s1","run_id":"run1","request_name":"archive-1","status":"PASS","native_status":"PASS","rc":0,"evidence_verified":true,"text_excerpt_truncated":false,"native_full_file_reads":[{"path":"src/a.go","action_index":1,"total_lines":15,"report_sha256":"%s"}]}`, strings.Repeat("a",64))
	e:=InspectNativeEvidence(scope,"key","p",request,receipt)
	if e.Status!=EvidenceComplete||!validDigest(e.ProofDigest){t.Fatalf("valid proof refused: %+v",e)}
	next:=strings.ReplaceAll(strings.ReplaceAll(receipt,"run1","run2"),"archive-1","archive-2")
	if got:=InspectNativeEvidence(scope,"key","p",request,next);got!=e{t.Fatal("volatile native IDs changed proof identity")}
	for _,tc:=range []struct{receipt string;status EvidenceStatus}{
		{strings.Replace(receipt,`"text_excerpt_truncated":false`,`"text_excerpt_truncated":true`,1),EvidencePartial},
		{strings.Replace(receipt,`"status":"PASS"`,`"status":"INCOMPLETE"`,1),EvidencePartial},
		{strings.Replace(receipt,`"native_status":"PASS"`,`"native_status":"FAIL"`,1),EvidenceUnknown},
		{strings.Replace(receipt,`"rc":0`,`"rc":3`,1),EvidenceUnknown},
		{strings.Replace(receipt,`"evidence_verified":true`,`"evidence_verified":false`,1),EvidenceUnknown},
		{strings.Replace(receipt,`"action_index":1`,`"action_index":2`,1),EvidenceUnknown},
		{strings.Replace(receipt,"src/a.go","src/b.go",1),EvidenceUnknown},
		{`{"error":{"code":"COLLECT_SCHEMA_INVALID"}}`,EvidenceError},
	}{
		if got:=InspectNativeEvidence(scope,"key","p",request,tc.receipt);got.Status!=tc.status||got.ProofDigest!=""{t.Fatalf("unsafe evidence: %+v want %s",got,tc.status)}
	}
	other:=[]byte(strings.Replace(string(request),"src/a.go","src/b.go",1))
	if got:=InspectNativeEvidence(scope,"key","p",other,receipt);got.Status!=EvidenceUnknown{t.Fatal("request mismatch accepted")}
}

func TestEvidenceIndexBoundedAndObjectiveIsolated(t *testing.T) {
	b:=counterFixture(t).Objective
	s:=ScopeDescriptor{ObjectiveKey:b.Key,RequestDigest:strings.Repeat("b",64)}
	proof:=Evidence{Status:EvidenceComplete,ProofDigest:strings.Repeat("c",64)}
	index:=NewEvidenceIndex(b)
	next,novelty:=ObserveEvidence(index,s,proof)
	if novelty!=VerifiedNew||len(index.Entries)!=0||len(next.Entries)!=1{t.Fatal("new evidence or immutable index wrong")}
	next,novelty=ObserveEvidence(next,s,proof)
	if novelty!=VerifiedDuplicate||s.SameVerifiedScope(s){t.Fatal("post-read duplicate became pre-read reuse")}
	proof.ProofDigest=strings.Repeat("d",64)
	next,novelty=ObserveEvidence(next,s,proof)
	if novelty!=VerifiedNew{t.Fatal("changed content was not new")}
	foreign:=s;foreign.ObjectiveKey=strings.Repeat("e",64)
	if _,got:=ObserveEvidence(next,foreign,proof);got!=UnknownNovelty{t.Fatal("cross-objective comparison")}
	for i:=0;i<MaxEvidenceEntries+10;i++{
		scope:=s;scope.RequestDigest=fmt.Sprintf("%064x",i+1)
		next,_=ObserveEvidence(next,scope,proof)
	}
	if len(next.Entries)!=MaxEvidenceEntries||!next.Evicted{t.Fatal("unbounded evidence index")}
	if _,got:=ObserveEvidence(next,s,proof);got!=UnknownNovelty{t.Fatal("evicted coverage guessed as new")}
	if _,got:=ObserveEvidence(next,s,Evidence{Status:EvidencePartial});got!=UnknownNovelty{t.Fatal("partial coverage guessed")}
}
