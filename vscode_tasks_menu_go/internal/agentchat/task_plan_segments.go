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
)

const (
 taskPlanSegmentSize = 32
 taskPlanSegmentBytesLimit = 512 << 10
)

// TaskPlanSegmentRef is an immutable, owner-scoped, content-checked page.
// Its filename is derived only from the validated session ID, revision,
// and sequential index. The canonical session journal holds the manifest.
// An incomplete pre-commit segment is never referenced by the journal.
type TaskPlanSegmentRef struct {
 Index int `json:"index"`
 Revision uint64 `json:"revision"`
 Count int `json:"count"`
 SHA256 string `json:"sha256"`
}

func (s *Store) taskPlanSegmentPath(id string,ref TaskPlanSegmentRef)string{
 return filepath.Join(s.dir,id+".plan."+strconv.FormatUint(ref.Revision,10)+"."+strconv.Itoa(ref.Index)+".json")
}

func validTaskPlanSegmentRef(ref TaskPlanSegmentRef,sessionRevision uint64)bool{
 if ref.Index<1||ref.Index>(MaxTaskPlanSteps+taskPlanSegmentSize-1)/taskPlanSegmentSize||
  ref.Revision==0||ref.Revision>sessionRevision||ref.Count<1||ref.Count>taskPlanSegmentSize||
  len(ref.SHA256)!=64{return false}
 _,err:=hex.DecodeString(ref.SHA256)
 return err==nil&&ref.SHA256==stringLower(ref.SHA256)
}

func stringLower(s string)string{
 b:=[]byte(s)
 for i:=range b{if b[i]>='A'&&b[i]<='Z'{b[i]+='a'-'A'}}
 return string(b)
}

// loadTaskPlanSegments runs under the journal lock, AFTER decoding the
// versioned session and BEFORE validating the hydrated canonical plan.
func (s *Store) loadTaskPlanSegments(x *Session)error{
 if x==nil{return ErrInvalidSession}
 if len(x.TaskPlanSegments)==0{return nil} // legacy inline journal
 if x.TaskPlan==nil||len(x.TaskPlan.Steps)!=0||
  len(x.TaskPlanSegments)>(MaxTaskPlanSteps+taskPlanSegmentSize-1)/taskPlanSegmentSize{
  return ErrInvalidSession
 }
 steps:=make([]TaskPlanStep,0,len(x.TaskPlanSegments)*taskPlanSegmentSize)
 for i,ref:=range x.TaskPlanSegments{
  if ref.Index!=i+1||!validTaskPlanSegmentRef(ref,x.Revision){
   return ErrInvalidSession
  }
  // Each segment except the final must be a full page. Prevent splicing.
  if i<len(x.TaskPlanSegments)-1&&ref.Count!=taskPlanSegmentSize{return ErrInvalidSession}
  part,err:=s.readVerifiedTaskPlanSegment(x.ID,ref)
  if err!=nil{return err}
  steps=append(steps,part...)
 }
 if len(steps)<=MaxInlineTaskPlanSteps||len(steps)>MaxTaskPlanSteps{return ErrInvalidSession}
 x.TaskPlan.Steps=steps
 return nil
}

func (s *Store) stageTaskPlanSegments(x *Session,previous *Session,newRevision uint64)error{
 if x==nil{return ErrInvalidSession}
 if x.TaskPlan==nil||len(x.TaskPlan.Steps)<=MaxInlineTaskPlanSteps{
  x.TaskPlanSegments=nil
  return nil
 }
 if len(x.TaskPlan.Steps)>MaxTaskPlanSteps{return ErrInvalidSession}
 oldByIndex:=map[int]TaskPlanSegmentRef{}
 if previous!=nil{
  for _,ref:=range previous.TaskPlanSegments{oldByIndex[ref.Index]=ref}
 }
 count:=(len(x.TaskPlan.Steps)+taskPlanSegmentSize-1)/taskPlanSegmentSize
 refs:=make([]TaskPlanSegmentRef,0,count)
 for i:=0;i<count;i++{
  end:=(i+1)*taskPlanSegmentSize
  if end>len(x.TaskPlan.Steps){end=len(x.TaskPlan.Steps)}
  batch:=x.TaskPlan.Steps[i*taskPlanSegmentSize:end]
  raw,err:=json.Marshal(batch)
  if err!=nil{return err}
  if len(raw)==0||len(raw)>taskPlanSegmentBytesLimit{
   return errors.New("Task plan segment exceeds size limit")
  }
  sum:=sha256.Sum256(raw)
  digest:=hex.EncodeToString(sum[:])
  if previous!=nil{
   if old,ok:=oldByIndex[i+1];ok&&old.SHA256==digest&&old.Count==len(batch){
    refs=append(refs,old)
    continue
   }
  }
  ref:=TaskPlanSegmentRef{Index:i+1,Revision:newRevision,Count:len(batch),SHA256:digest}
  tmp,err:=os.CreateTemp(s.dir,".agent-plan-segment-*")
  if err!=nil{return err}
  defer os.Remove(tmp.Name())
  if err=tmp.Chmod(0600);err!=nil{tmp.Close();return err}
  if _,err=tmp.Write(raw);err!=nil{tmp.Close();return err}
  if err=tmp.Sync();err!=nil{tmp.Close();return err}
  if err=tmp.Close();err!=nil{return err}
  if err=os.Rename(tmp.Name(),s.taskPlanSegmentPath(x.ID,ref));err!=nil{return err}
  refs=append(refs,ref)
 }
 x.TaskPlanSegments=refs
 return nil
}

// Cleanup is best-effort and occurs only AFTER the journal's atomic commit.
// Any crash-orphan file can be overwritten on a retry of that uncommitted
// revision. Never delete a segment still referenced by the current journal.
func (s *Store) cleanupTaskPlanSegments(old,current []TaskPlanSegmentRef,id string){
 retained:=make(map[string]bool,len(current))
 for _,ref:=range current{retained[s.taskPlanSegmentPath(id,ref)]=true}
 for _,ref:=range old{
  p:=s.taskPlanSegmentPath(id,ref)
  if !retained[p]{_ =os.Remove(p)}
 }
}

func (s *Store) readVerifiedTaskPlanSegment(id string,ref TaskPlanSegmentRef)([]TaskPlanStep,error){
  path:=s.taskPlanSegmentPath(id,ref)
  info,err:=os.Lstat(path)
  if err!=nil{return nil,fmt.Errorf("missing Task plan segment: %w",err)}
  if !info.Mode().IsRegular()||info.Mode().Perm()&0077!=0||
   info.Size()==0||info.Size()>taskPlanSegmentBytesLimit{return nil,ErrInvalidSession}
  raw,err:=os.ReadFile(path);if err!=nil{return nil,err}
  if int64(len(raw))!=info.Size(){return nil,ErrInvalidSession}
  sum:=sha256.Sum256(raw)
  if hex.EncodeToString(sum[:])!=ref.SHA256{return nil,errors.New("Task plan segment digest mismatch")}
  var part []TaskPlanStep
  if json.Unmarshal(raw,&part)!=nil||len(part)!=ref.Count{return nil,ErrInvalidSession}
  return part,nil
}
