package agentchat

import (
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "sort"
 "strconv"
)

// An unlimited number of read-only Collection rounds cannot fit inside a
// single 4 MiB session JSON or a provider's 256-item request. Store completed
// older observations in append-only, private segments and keep a bounded
// active window. The initial user goal and all Patch commitments stay live.
const (
 archiveHistoryWindow = 96
 archiveHistoryThreshold = 148
 archiveHistoryBytesThreshold = 1500 << 10
)

type historyArchiveSegment struct {
 Format string `json:"format"`
 SessionID string `json:"session_id"`
 Sequence uint64 `json:"sequence"`
 ItemCount int `json:"item_count"`
 SHA256 string `json:"sha256"`
 Items []HistoryItem `json:"items"`
}

func (s *Store) historyArchivePath(id string, sequence uint64) string {
 return filepath.Join(s.dir,id+".history."+strconv.FormatUint(sequence,10)+".json")
}

// archiveCollectionHistory is called ONLY under Store.withLock, after the
// revision/project fences have passed and before the new session is written.
// Referencing the segment happens in the primary session's atomic rename.
// An interrupted pre-commit archive is unreferenced and can be overwritten
// on a subsequent identical CAS attempt; no verified evidence is lost.
func (s *Store) archiveCollectionHistory(x *Session) error {
 count:=len(x.History)
 contentBytes:=0
 for _,it:=range x.History {contentBytes+=len(it.Content)+len(it.Arguments)}
 if count<=archiveHistoryThreshold && contentBytes<=archiveHistoryBytesThreshold{return nil}
 until:=count-archiveHistoryWindow
 if until<=1{return nil}
 removed:=make([]bool,count)
 archived:=make([]HistoryItem,0,until)
 // Remove only completed read-only tool pairs, generated Collection fallback
 // observations and old assistant narration. Never remove human messages,
 // unresolved tool calls or a Patch proposal/result with provenance.
 for i:=1;i<until;i++{
  item:=x.History[i]
  if item.Type=="function_call" && item.Name=="taskdeck_collect_workspace" &&
   i+1<until && x.History[i+1].Type=="function_call_output" &&
   x.History[i+1].CallID==item.CallID {
   removed[i],removed[i+1]=true,true
   archived=append(archived,item,x.History[i+1]);i++
   continue
  }
  if item.Type=="assistant" {
   removed[i]=true;archived=append(archived,item);continue
  }
  if CollectionTextObservation(x.History,i) {
   removed[i]=true;archived=append(archived,item)
  }
 }
 if len(archived)==0 {return nil}
 for _,it:=range archived{
  if err:=validHistoryItem(it);err!=nil{return fmt.Errorf("invalid archived observation: %w",err)}
 }
 payload,err:=json.Marshal(archived)
 if err!=nil{return err}
 sum:=sha256.Sum256(payload)
 seq:=x.HistoryArchiveParts+1
 record:=historyArchiveSegment{Format:"taskdeck-agent-collection-history-v1",
  SessionID:x.ID,Sequence:seq,ItemCount:len(archived),
  SHA256:hex.EncodeToString(sum[:]),Items:archived}
 encoded,err:=json.Marshal(record)
 if err!=nil{return err}
 if len(encoded)>4<<20{return errors.New("Collection archive batch is unexpectedly oversized")}
 // A private, synced temporary file precedes the archive rename; no
 // symlinks or workspace paths are ever used to write these records.
 tmp,err:=os.CreateTemp(s.dir,".agent-collection-archive-*")
 if err!=nil{return err}
 defer os.Remove(tmp.Name())
 if err=tmp.Chmod(0600);err!=nil{tmp.Close();return err}
 if _,err=tmp.Write(encoded);err!=nil{tmp.Close();return err}
 if err=tmp.Sync();err!=nil{tmp.Close();return err}
 if err=tmp.Close();err!=nil{return err}
 if err=os.Rename(tmp.Name(),s.historyArchivePath(x.ID,seq));err!=nil{return err}

 before:=make([]int,count+1)
 keep:=make([]HistoryItem,0,count-len(archived))
 for i,it:=range x.History {
  before[i+1]=before[i]
  if removed[i] {before[i+1]++;continue}
  keep=append(keep,it)
 }
 for i:=range x.Events{
  n:=x.Events[i].HistoryIndex
  if n<0{n=0};if n>count{n=count}
  x.Events[i].HistoryIndex=n-before[n]
 }
 x.History=keep
 x.HistoryArchiveParts=seq
 x.ArchivedHistoryItems+=uint64(len(archived))
 return nil
}

// readArchivedCollectionHistoryLocked uses the same verified immutable
// archive decoder as the owner-scoped public API, while the caller already
// holds Store.withLock. Never invoke the public API under that lock.
func (s *Store) readArchivedCollectionHistoryLocked(x Session,part uint64)([]HistoryItem,error){
 if !validID(x.ID)||part==0||part>x.HistoryArchiveParts{return nil,ErrNotFound}
 path:=s.historyArchivePath(x.ID,part)
 fi,e:=os.Lstat(path);if e!=nil{return nil,e}
 if !fi.Mode().IsRegular()||fi.Mode().Perm()&0077!=0||fi.Size()>4<<20{return nil,ErrInvalidSession}
 raw,e:=os.ReadFile(path);if e!=nil{return nil,e}
 var rec historyArchiveSegment
 if json.Unmarshal(raw,&rec)!=nil||rec.Format!="taskdeck-agent-collection-history-v1"||
  rec.SessionID!=x.ID||rec.Sequence!=part||len(rec.Items)!=rec.ItemCount{return nil,ErrInvalidSession}
 payload,e:=json.Marshal(rec.Items);if e!=nil{return nil,e}
 digest:=sha256.Sum256(payload)
 if rec.SHA256!=hex.EncodeToString(digest[:]){return nil,ErrInvalidSession}
 return rec.Items,nil
}

// ArchivedCollectionHistory returns historical evidence for audit without
// bringing all older observations into the provider-visible context.
func (s *Store) ArchivedCollectionHistory(id,owner string,part uint64)([]HistoryItem,error){
 if !validID(id)||part==0{return nil,ErrNotFound}
 var result []HistoryItem
 err:=s.withLock(func()error{
  x,e:=s.readLocked(id);if e!=nil{return e}
  if x.Owner!=owner||!s.matchesProject(x){return ErrNotFound}
  result,e=s.readArchivedCollectionHistoryLocked(x,part)
  return e
 })
 return result,err
}

// HistoryArchiveSequence offers simple, stable browsing of completed
// segments. The ordering is append-only and does not depend on mtime.
func (s *Store) historyArchiveSequences(x Session) []uint64{
 out:=make([]uint64,0,x.HistoryArchiveParts)
 for n:=uint64(1);n<=x.HistoryArchiveParts;n++{out=append(out,n)}
 sort.Slice(out,func(i,j int)bool{return out[i]<out[j]})
 return out
}
