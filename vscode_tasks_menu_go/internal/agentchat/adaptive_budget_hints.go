package agentchat

import (
 "encoding/json"
 "os"
 "path/filepath"
 "time"
)

const adaptiveBudgetHintCapacity=256
const adaptiveBudgetHintTTL=24*time.Hour
const maxAdaptiveBudgetHintBytes=128<<10

// Advisory limits only. No prompt, response, key, credential fingerprint or
// owner name is stored in plaintext. Scope includes exact project, owner,
// workspace and provider/model/config/credential version.
type AdaptiveBudgetHint struct{
 Input int `json:"input"`
 Output int `json:"output"`
 ExpiresAt time.Time `json:"expires_at"`
}
type adaptiveBudgetHintFile struct{
 Version int `json:"version"`
 Entries map[string]AdaptiveBudgetHint `json:"entries"`
 SHA256 string `json:"sha256"`
}
func(s *Store) adaptiveBudgetHintPath()string{return filepath.Join(s.dir,"adaptive-budget-hints.json")}
func adaptiveHintKey(x Session,version,credential string)string{
 return guardDigest([]string{x.Owner,x.Workspace,x.ProjectID,version,credential})
}
func validAdaptiveHint(h AdaptiveBudgetHint)bool{
 return h.Input>=4096&&h.Input<=4000000&&h.Output>0&&h.Output<=1000000&&!h.ExpiresAt.IsZero()
}
func(s *Store) readAdaptiveHintsLocked()(adaptiveBudgetHintFile,error){
 f:=adaptiveBudgetHintFile{Version:1,Entries:map[string]AdaptiveBudgetHint{}}
 path:=s.adaptiveBudgetHintPath()
 fi,e:=os.Lstat(path)
 if os.IsNotExist(e){return f,nil}
 if e!=nil{return f,e}
 if !fi.Mode().IsRegular()||fi.Mode().Perm()&0077!=0||fi.Size()>maxAdaptiveBudgetHintBytes{return f,ErrInvalidSession}
 raw,e:=os.ReadFile(path);if e!=nil{return f,e}
 if json.Unmarshal(raw,&f)!=nil||f.Version!=1||f.Entries==nil||len(f.Entries)>adaptiveBudgetHintCapacity||
  f.SHA256!=guardDigest(f.Entries){return f,ErrInvalidSession}
 for key,h:=range f.Entries{if !validGuardDigest(key)||!validAdaptiveHint(h){return f,ErrInvalidSession}}
 return f,nil
}
func(s *Store) writeAdaptiveHintsLocked(f adaptiveBudgetHintFile)error{
 f.SHA256=guardDigest(f.Entries)
 raw,e:=json.Marshal(f);if e!=nil{return e}
 if len(raw)>maxAdaptiveBudgetHintBytes{return ErrInvalidSession}
 tmp,e:=os.CreateTemp(s.dir,".adaptive-budget-");if e!=nil{return e}
 defer os.Remove(tmp.Name())
 if e=tmp.Chmod(0600);e!=nil{tmp.Close();return e}
 if _,e=tmp.Write(raw);e!=nil{tmp.Close();return e}
 if e=tmp.Sync();e!=nil{tmp.Close();return e}
 if e=tmp.Close();e!=nil{return e}
 if e=os.Rename(tmp.Name(),s.adaptiveBudgetHintPath());e!=nil{return e}
 return s.syncDirectory()
}
// Validate against authoritative owner/project, not a caller-created scope.
// An advisory hint never authorizes egress or changes the session journal.
func(s *Store) adaptiveHintScopeLocked(x Session,version,credential string)(string,error){
 if !validID(x.ID)||!validGuardDigest(version)||!validGuardDigest(credential){return "",ErrInvalidSession}
 current,e:=s.readLocked(x.ID);if e!=nil{return "",e}
 if current.Owner!=x.Owner||current.Workspace!=x.Workspace||current.ProjectID!=x.ProjectID||!s.matchesProject(current){return "",ErrNotFound}
 return adaptiveHintKey(current,version,credential),nil
}
func(s *Store) AdaptiveBudgetHint(x Session,version,credential string)(AdaptiveBudgetHint,bool,error){
 var out AdaptiveBudgetHint
 found:=false
 e:=s.withLock(func()error{
  key,e:=s.adaptiveHintScopeLocked(x,version,credential);if e!=nil{return e}
  f,e:=s.readAdaptiveHintsLocked();if e!=nil{return e}
  out,found=f.Entries[key]
  now:=time.Now().UTC()
  if found&&(!now.Before(out.ExpiresAt)||out.ExpiresAt.After(now.Add(adaptiveBudgetHintTTL))){found=false}
  return nil
 })
 return out,found&&e==nil,e
}
func(s *Store) RememberAdaptiveBudgetHint(x Session,version,credential string,input,output int)error{
 return s.withLock(func()error{
  key,e:=s.adaptiveHintScopeLocked(x,version,credential);if e!=nil{return e}
  f,e:=s.readAdaptiveHintsLocked();if e!=nil{return e} // preserve corrupt/future version
  // Use the clock inside the lock: a waiting writer may have started
  // before a concurrent writer's newer valid TTL was committed.
  now:=time.Now().UTC()
  hint:=AdaptiveBudgetHint{Input:input,Output:output,ExpiresAt:now.Add(adaptiveBudgetHintTTL)}
  if !validAdaptiveHint(hint){return ErrInvalidSession}
  for k,old:=range f.Entries{if !now.Before(old.ExpiresAt)||old.ExpiresAt.After(now.Add(adaptiveBudgetHintTTL)){delete(f.Entries,k)}}
  if old,ok:=f.Entries[key];ok{
   if old.Input<hint.Input{hint.Input=old.Input}
   if old.Output<hint.Output{hint.Output=old.Output}
  }else if len(f.Entries)>=adaptiveBudgetHintCapacity{
   oldest:=""
   var expiry time.Time
   for k,v:=range f.Entries{
    if oldest==""||v.ExpiresAt.Before(expiry)||(v.ExpiresAt.Equal(expiry)&&k<oldest){oldest=k;expiry=v.ExpiresAt}
   }
   delete(f.Entries,oldest)
  }
  f.Entries[key]=hint
  return s.writeAdaptiveHintsLocked(f)
 })
}
