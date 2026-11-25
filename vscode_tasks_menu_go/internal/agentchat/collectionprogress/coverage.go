package collectionprogress

import (
 "crypto/sha256"
 "encoding/hex"
 "sort"
)

const MaxCoverageScopes=64
const MaxCoverageIntervals=128
const maxCoverageLine=uint64(1)<<48

// Range comparisons require an authoritative source revision, not a request
// hash, artifact filename, model assertion or HEAD alone in a dirty tree.
type CoverageKey struct {
 ObjectiveKey string `json:"objective_key"`
 PathDigest string `json:"path_digest"`
 SourceRevisionDigest string `json:"source_revision_digest"`
}

func NewCoverageKey(b ObjectiveBinding,p,revision string)(CoverageKey,bool){
 if !b.Valid()||!validEvidencePath(p)||!validDigest(revision){return CoverageKey{},false}
 hash:=sha256.Sum256([]byte(p))
 return CoverageKey{b.Key,hex.EncodeToString(hash[:]),revision},true
}
func(k CoverageKey)Valid()bool{return validDigest(k.ObjectiveKey)&&validDigest(k.PathDigest)&&validDigest(k.SourceRevisionDigest)}

type LineRange struct {
 First uint64 `json:"first"`
 Last uint64 `json:"last"`
}
func(r LineRange)Valid()bool{return r.First>0&&r.First<=r.Last&&r.Last<=maxCoverageLine}
func(r LineRange)length()uint64{return r.Last-r.First+1}

type CoverageEntry struct{
 Key CoverageKey `json:"key"`
 Ranges []LineRange `json:"ranges"`
}
type CoverageIndex struct{
 ObjectiveKey string `json:"objective_key"`
 Evicted bool `json:"evicted,omitempty"`
 Entries []CoverageEntry `json:"entries,omitempty"`
}
type CoverageComparison struct{
 Novelty Novelty
 NewLines uint64
 OverlapLines uint64
}
func NewCoverageIndex(b ObjectiveBinding)CoverageIndex{
 if !b.Valid(){return CoverageIndex{}}
 return CoverageIndex{ObjectiveKey:b.Key}
}

func compareRanges(old []LineRange,observed LineRange)(uint64,bool){
 if !observed.Valid()||len(old)>MaxCoverageIntervals{return 0,false}
 var overlap uint64
 for i,r:=range old{
  if !r.Valid()||(i>0&&r.First<=old[i-1].Last+1){return 0,false}
  first,last:=r.First,r.Last
  if observed.First>first{first=observed.First}
  if observed.Last<last{last=observed.Last}
  if first<=last{overlap+=last-first+1}
 }
 return overlap,true
}
func mergeRanges(old []LineRange,observed LineRange)([]LineRange,bool){
 all:=append(append([]LineRange(nil),old...),observed)
 sort.Slice(all,func(i,j int)bool{return all[i].First<all[j].First})
 out:=make([]LineRange,0,len(all))
 for _,r:=range all{
  if len(out)>0&&r.First<=out[len(out)-1].Last+1{
   if r.Last>out[len(out)-1].Last{out[len(out)-1].Last=r.Last}
  }else{out=append(out,r)}
 }
 if len(out)>MaxCoverageIntervals{return nil,false}
 return out,true
}

// ObserveCoverage consumes verified complete ranges from an authoritative
// native report. Requested offsets are not proof. It never permits a skip.
func ObserveCoverage(index CoverageIndex,key CoverageKey,observed LineRange)(CoverageIndex,CoverageComparison){
 unknown:=CoverageComparison{Novelty:UnknownNovelty}
 if !key.Valid()||key.ObjectiveKey!=index.ObjectiveKey||!observed.Valid()||len(index.Entries)>MaxCoverageScopes{return index,unknown}
 next:=index
 next.Entries=append([]CoverageEntry(nil),index.Entries...)
 for i,entry:=range next.Entries{
  if entry.Key!=key{continue}
  overlap,ok:=compareRanges(entry.Ranges,observed)
  if !ok{return index,unknown}
  ranges,ok:=mergeRanges(entry.Ranges,observed)
  if !ok{return index,unknown}
  novelty:=VerifiedNew
  if overlap==observed.length(){novelty=VerifiedDuplicate}
  next.Entries=append(next.Entries[:i],next.Entries[i+1:]...)
  next.Entries=append(next.Entries,CoverageEntry{key,ranges})
  return next,CoverageComparison{novelty,observed.length()-overlap,overlap}
 }
 if len(next.Entries)==MaxCoverageScopes{next.Entries=next.Entries[1:];next.Evicted=true}
 next.Entries=append(next.Entries,CoverageEntry{key,[]LineRange{observed}})
 if next.Evicted{return next,unknown}
 return next,CoverageComparison{Novelty:VerifiedNew,NewLines:observed.length()}
}

// Only complete, verified mutation paths may invalidate selectively.
// Unknown/partial mutation invalidates all metadata, never canonical history.
func InvalidateCoverage(index CoverageIndex,changedPaths []string,complete bool)CoverageIndex{
 if !complete||len(changedPaths)>1024{return CoverageIndex{ObjectiveKey:index.ObjectiveKey,Evicted:true}}
 changed:=map[string]bool{}
 for _,p:=range changedPaths{
  if !validEvidencePath(p){return CoverageIndex{ObjectiveKey:index.ObjectiveKey,Evicted:true}}
  hash:=sha256.Sum256([]byte(p))
  changed[hex.EncodeToString(hash[:])]=true
 }
 if len(changed)==0{return index}
 next:=CoverageIndex{ObjectiveKey:index.ObjectiveKey,Evicted:index.Evicted}
 for _,entry:=range index.Entries{
  if changed[entry.Key.PathDigest]{next.Evicted=true;continue}
  next.Entries=append(next.Entries,entry)
 }
 return next
}

// Exact-request proof index has no verified per-entry mutation-path coverage.
// Invalidate broadly unless source-unchanged is authoritatively established.
func InvalidateEvidence(index EvidenceIndex,sourceUnchangedProven bool)EvidenceIndex{
 if sourceUnchangedProven{return index}
 return EvidenceIndex{ObjectiveKey:index.ObjectiveKey,Evicted:true}
}
