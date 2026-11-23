package agentchat

import (
 "fmt"
 "os"
 "path/filepath"
 "strconv"
 "strings"
 "sync"
)

// Once per daemon and private directory, run conservative orphan cleanup
// before the Agent session store is used. Explicit recovery can also be
// invoked again for a directory after a later interrupted write.
var planOrphanRecoveryMu sync.Mutex
var planOrphanRecoveryDone = make(map[string]bool)

// EnsureTaskPlanOrphanRecovery is used at Agent store startup. A failed scan
// is not cached: it must never silently declare successful recovery when a
// session journal is corrupt or unreadable.
func (s *Store) EnsureTaskPlanOrphanRecovery() error {
 planOrphanRecoveryMu.Lock()
 defer planOrphanRecoveryMu.Unlock()
 if planOrphanRecoveryDone[s.dir] {return nil}
 if _,err:=s.RecoverTaskPlanOrphans();err!=nil{return err}
 planOrphanRecoveryDone[s.dir]=true
 return nil
}

// parsePlanSegmentCandidate only recognizes exact versioned filenames minted
// by stageTaskPlanSegments: <32-hex-id>.plan.<revision>.<index>.json.
// Unknown, noncanonical or future-format files are NEVER garbage-collected.
func parsePlanSegmentCandidate(name string)(string,bool){
 pieces:=strings.Split(name,".")
 if len(pieces)!=5||pieces[1]!="plan"||pieces[4]!="json"||
  !validID(pieces[0]){return "",false}
 revision,err:=strconv.ParseUint(pieces[2],10,64)
 if err!=nil||revision==0||strconv.FormatUint(revision,10)!=pieces[2]{
  return "",false
 }
 index,err:=strconv.Atoi(pieces[3])
 if err!=nil||index<1||index>(MaxTaskPlanSteps+taskPlanSegmentSize-1)/taskPlanSegmentSize||
  strconv.Itoa(index)!=pieces[3]{return "",false}
 return pieces[0],true
}

// RecoverTaskPlanOrphans removes unreferenced immutable Task plan pages left
// by an interrupted Save/Delete. It holds the SAME process and OS directory
// lock as Save, Get and Delete throughout discovery and deletion, so an
// in-progress writer cannot publish a referencing journal between the two.
//
// FIRST validate ALL canonical journals and their referenced segments.
// If any journal is unreadable, damaged or missing a page, fail closed and
// delete NOTHING. Scan ALL owners and projects in the private directory,
// not merely the current Store scope: cleanup must never erase another
// owner's live pages.
//
// Only non-symlink, owner-private regular files with an EXACT known page
// filename may be removed. Unknown formats, temporary files and suspicious
// paths are deliberately left untouched for manual inspection.
func (s *Store) RecoverTaskPlanOrphans()(int,error){
 removed:=0
 err:=s.withLock(func()error{
  entries,err:=os.ReadDir(s.dir)
  if err!=nil{return err}
  retained:=make(map[string]bool)
  // Phase 1: build the complete live-page set from canonical journals.
  // No deletion is performed unless every journal is fully recoverable.
  for _,entry:=range entries{
   name:=entry.Name()
   if !strings.HasSuffix(name,".json"){continue}
   id:=strings.TrimSuffix(name,".json")
   if !validID(id){continue}
   journal,err:=s.readLocked(id)
   if err!=nil{
    return fmt.Errorf("Task plan orphan recovery aborted by unreadable canonical session: %w",err)
   }
   for _,ref:=range journal.TaskPlanSegments{
    retained[filepath.Base(s.taskPlanSegmentPath(id,ref))]=true
   }
  }
  // Phase 2: collect only recognizable, private, regular orphan pages.
  candidates:=make([]string,0)
  for _,entry:=range entries{
   name:=entry.Name()
   if _,ok:=parsePlanSegmentCandidate(name);!ok||retained[name]{continue}
   path:=filepath.Join(s.dir,name)
   info,err:=os.Lstat(path)
   if os.IsNotExist(err){continue}
   if err!=nil{return err}
   if !info.Mode().IsRegular()||info.Mode().Perm()&0077!=0{
    continue // never follow links or delete alien/publicly writable files
   }
   candidates=append(candidates,path)
  }
  // Phase 3: journal snapshots cannot change while the directory lock is
  // held. No referenced segment can become a newly published orphan target.
  for _,path:=range candidates{
   if err:=os.Remove(path);err!=nil{
    if os.IsNotExist(err){continue}
    return err
   }
   removed++
  }
  // An unlink is a directory-entry mutation: fsync it before declaring
  // startup cleanup complete. A failure AFTER unlink is not evidence that
  // the old page still exists; do not cache it as successful recovery.
  // A rerun remains idempotent and uses the canonical live-page set.
  if removed>0{
   if err:=s.syncDirectory();err!=nil{
    return fmt.Errorf("%w: Task Plan orphan unlink: %v",ErrCleanupDurabilityUnknown,err)
   }
  }
  return nil
 })
 return removed,err
}
