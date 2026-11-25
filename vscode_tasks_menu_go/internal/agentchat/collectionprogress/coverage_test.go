package collectionprogress

import (
 "fmt"
 "strings"
 "testing"
)

func TestCoverageOverlapAlternatingAnd55Pages(t *testing.T){
 b:=counterFixture(t).Objective
 a,ok:=NewCoverageKey(b,"src/a.go",strings.Repeat("a",64));if !ok{t.Fatal("a")}
 z,ok:=NewCoverageKey(b,"src/b.go",strings.Repeat("a",64));if !ok{t.Fatal("b")}
 index:=NewCoverageIndex(b)
 var c CoverageComparison
 index,c=ObserveCoverage(index,a,LineRange{1,100})
 if c.Novelty!=VerifiedNew||c.NewLines!=100{t.Fatalf("first: %+v",c)}
 index,c=ObserveCoverage(index,a,LineRange{1,110})
 if c.Novelty!=VerifiedNew||c.NewLines!=10||c.OverlapLines!=100{t.Fatalf("overlap: %+v",c)}
 index,c=ObserveCoverage(index,z,LineRange{1,100})
 if c.Novelty!=VerifiedNew{t.Fatal("different path collapsed")}
 index,c=ObserveCoverage(index,a,LineRange{10,50})
 if c.Novelty!=VerifiedDuplicate||c.NewLines!=0||c.OverlapLines!=41{t.Fatalf("subset: %+v",c)}
 for i:=uint64(0);i<55;i++{
  index,c=ObserveCoverage(index,a,LineRange{111+i*10,120+i*10})
  if c.Novelty!=VerifiedNew||c.NewLines!=10{t.Fatalf("page %d: %+v",i,c)}
 }
 if _,ok:=NewCoverageKey(b,"src/a.go","");ok{t.Fatal("missing revision")}
 changed,_:=NewCoverageKey(b,"src/a.go",strings.Repeat("b",64))
 _,c=ObserveCoverage(index,changed,LineRange{1,100})
 if c.Novelty!=VerifiedNew{t.Fatal("changed revision collapsed")}
}

func TestCoverageInvalidationAndEvictionFailConservative(t *testing.T){
 b:=counterFixture(t).Objective
 a,_:=NewCoverageKey(b,"src/a.go",strings.Repeat("a",64))
 z,_:=NewCoverageKey(b,"src/b.go",strings.Repeat("a",64))
 index:=NewCoverageIndex(b)
 index,_=ObserveCoverage(index,a,LineRange{1,100})
 index,_=ObserveCoverage(index,z,LineRange{1,100})
 changed:=InvalidateCoverage(index,[]string{"src/a.go"},true)
 if len(index.Entries)!=2||len(changed.Entries)!=1{t.Fatal("wrong invalidation")}
 _,c:=ObserveCoverage(changed,a,LineRange{1,100})
 if c.Novelty!=UnknownNovelty{t.Fatal("stale path reusable")}
 _,c=ObserveCoverage(changed,z,LineRange{1,100})
 if c.Novelty!=VerifiedDuplicate{t.Fatal("unrelated proof lost")}
 if got:=InvalidateCoverage(index,nil,false);len(got.Entries)!=0||!got.Evicted{t.Fatal("unknown mutation retained coverage")}
 if len(InvalidateCoverage(index,nil,true).Entries)!=2{t.Fatal("proven no-op invalidated")}
 for i:=0;i<MaxCoverageScopes+2;i++{
  key,_:=NewCoverageKey(b,fmt.Sprintf("src/file-%d.go",i),strings.Repeat("a",64))
  index,_=ObserveCoverage(index,key,LineRange{1,100})
 }
 if len(index.Entries)!=MaxCoverageScopes||!index.Evicted{t.Fatal("unbounded index")}
 _,c=ObserveCoverage(index,a,LineRange{1,100})
 if c.Novelty!=UnknownNovelty{t.Fatal("evicted scope guessed")}
}

func TestCoverageIntervalBoundNotReadQuota(t *testing.T){
 b:=counterFixture(t).Objective
 key,_:=NewCoverageKey(b,"src/a.go",strings.Repeat("a",64))
 index:=NewCoverageIndex(b)
 for i:=uint64(0);i<MaxCoverageIntervals;i++{
  var c CoverageComparison
  index,c=ObserveCoverage(index,key,LineRange{1+2*i,1+2*i})
  if c.Novelty!=VerifiedNew{t.Fatal("valid interval lost")}
 }
 unchanged,c:=ObserveCoverage(index,key,LineRange{10001,10001})
 if c.Novelty!=UnknownNovelty||len(unchanged.Entries[0].Ranges)!=MaxCoverageIntervals{t.Fatal("metadata cap not conservative")}
}
