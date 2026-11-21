package agentchat

import (
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "strconv"
 "strings"
)

const (
 activePatchRecordsTarget=16
 activePatchRecordsTrigger=24
 patchArchiveBatchLimit=4
)

type PatchArchiveEntry struct {
 Call ToolCallRecord `json:"call"`
 History []HistoryItem `json:"history"`
}
type patchArchiveSegment struct{
 Format string `json:"format"`
 SessionID string `json:"session_id"`
 Sequence uint64 `json:"sequence"`
 SHA256 string `json:"sha256"`
 Entries []PatchArchiveEntry `json:"entries"`
}
func (s *Store) patchArchivePath(id string,n uint64)string {
 return filepath.Join(s.dir,id+".patch."+strconv.FormatUint(n,10)+".json")
}
func archivedPatchSummary(call ToolCallRecord,history []HistoryItem)string {
 name:="unnamed"
 for _,it:=range history{
  if it.Type=="function_call"&&it.CallID==call.CallID{
   var v struct{Name string `json:"name"`}
   if json.Unmarshal([]byte(it.Arguments),&v)==nil&&v.Name!=""{
    name=strings.TrimSpace(v.Name)
   }
  }
 }
 if len(name)>85{name=name[:85]}
 return fmt.Sprintf("Patch %s | %s | call=%s | run=%s",name,call.Status,call.CallID,call.RunID)
}
func patchArchiveEligible(call ToolCallRecord,history []HistoryItem)bool {
 if (call.Status!="done"&&call.Status!="error"&&call.Status!="rejected")||
  !call.FeedbackSent||call.FeedbackStatus!="acknowledged" {return false}
 foundCall,foundOutput:=false,false
 for _,it:=range history{
  if it.CallID!=call.CallID{continue}
  if it.Type=="function_call"&&it.Name==call.Name{foundCall=true}
  if it.Type=="function_call_output"{foundOutput=true}
 }
 return foundCall&&(foundOutput||call.TextFallback)
}
// archivePatchHistory is a persistence operation, not a provider context
// operation. It only moves already-final, acknowledged Patch commitments to
// immutable private segments. An unresolved/ambiguous native side effect is
// never touched. The canonical session pointer is committed last by Save().
func (s *Store) archivePatchHistory(x *Session)error{
 if len(x.ToolCalls)<activePatchRecordsTrigger {return nil}
 for len(x.ToolCalls)>activePatchRecordsTarget {
  indices:=make([]int,0,patchArchiveBatchLimit)
  for i,call:=range x.ToolCalls{
   if patchArchiveEligible(call,x.History){
    indices=append(indices,i)
    if len(indices)>=patchArchiveBatchLimit{break}
   }
  }
  if len(indices)==0 {break}
  toRemove:=map[string]bool{}
  entries:=make([]PatchArchiveEntry,0,len(indices))
  removeCall:=map[int]bool{}
  for _,index:=range indices {
   call:=x.ToolCalls[index]
   removeCall[index]=true
   toRemove[call.CallID]=true
   h:=make([]HistoryItem,0,2)
   for _,it:=range x.History {
    if it.CallID==call.CallID{h=append(h,it)}
   }
   entries=append(entries,PatchArchiveEntry{Call:call,History:h})
  }
  payload,err:=json.Marshal(entries)
  if err!=nil{return err}
  digest:=sha256.Sum256(payload)
  seq:=x.PatchArchiveParts+1
  segment:=patchArchiveSegment{Format:"taskdeck-agent-patch-history-v1",
   SessionID:x.ID,Sequence:seq,SHA256:hex.EncodeToString(digest[:]),Entries:entries}
  encoded,err:=json.Marshal(segment)
  if err!=nil{return err}
  if len(encoded)>4<<20{return errors.New("Patch archive segment exceeds 4 MiB")}
  temp,err:=os.CreateTemp(s.dir,".agent-patch-archive-*")
  if err!=nil{return err}
  name:=temp.Name()
  defer os.Remove(name)
  if err=temp.Chmod(0600);err!=nil{temp.Close();return err}
  if _,err=temp.Write(encoded);err!=nil{temp.Close();return err}
  if err=temp.Sync();err!=nil{temp.Close();return err}
  if err=temp.Close();err!=nil{return err}
  if err=os.Rename(name,s.patchArchivePath(x.ID,seq));err!=nil{return err}
  oldLen:=len(x.History)
  prefix:=make([]int,oldLen+1)
  keep:=make([]HistoryItem,0,oldLen)
  for i,it:=range x.History{
   prefix[i+1]=prefix[i]
   if toRemove[it.CallID]{prefix[i+1]++;continue}
   keep=append(keep,it)
  }
  for i:=range x.Events{
   idx:=x.Events[i].HistoryIndex
   if idx<0{idx=0};if idx>oldLen{idx=oldLen}
   x.Events[i].HistoryIndex=idx-prefix[idx]
  }
  x.History=keep
  calls:=make([]ToolCallRecord,0,len(x.ToolCalls)-len(indices))
  for i,call:=range x.ToolCalls{if !removeCall[i]{calls=append(calls,call)}}
  x.ToolCalls=calls
  x.PatchArchiveParts=seq
  x.ArchivedPatchCalls+=uint64(len(entries))
  for _,entry:=range entries{
   summary:=archivedPatchSummary(entry.Call,entry.History)
   if len(summary)>220 {summary=summary[:220]}
   x.PatchArchiveSummaries=append(x.PatchArchiveSummaries,summary)
  }
  if len(x.PatchArchiveSummaries)>24{
   x.PatchArchiveSummaries=append([]string(nil),x.PatchArchiveSummaries[len(x.PatchArchiveSummaries)-24:]...)
  }
 }
 return nil
}
// ArchivedPatchHistory returns exact prior approvals and native receipts for
// an authenticated owner. This is NOT a re-execution or provider egress path.
func (s *Store) ArchivedPatchHistory(id,owner string,part uint64)([]PatchArchiveEntry,error){
 if !validID(id)||part==0{return nil,ErrNotFound}
 var entries []PatchArchiveEntry
 err:=s.withLock(func()error{
  x,e:=s.readLocked(id);if e!=nil{return e}
  if x.Owner!=owner||!s.matchesProject(x)||part>x.PatchArchiveParts{return ErrNotFound}
  path:=s.patchArchivePath(id,part)
  fi,e:=os.Lstat(path);if e!=nil{return e}
  if !fi.Mode().IsRegular()||fi.Mode().Perm()&0077!=0||fi.Size()>4<<20{return ErrInvalidSession}
  raw,e:=os.ReadFile(path);if e!=nil{return e}
  var rec patchArchiveSegment
  if json.Unmarshal(raw,&rec)!=nil||rec.Format!="taskdeck-agent-patch-history-v1"||
    rec.SessionID!=id||rec.Sequence!=part{return ErrInvalidSession}
  payload,e:=json.Marshal(rec.Entries);if e!=nil{return e}
  d:=sha256.Sum256(payload)
  if hex.EncodeToString(d[:])!=rec.SHA256{return ErrInvalidSession}
  entries=rec.Entries
  return nil
 })
 return entries,err
}
