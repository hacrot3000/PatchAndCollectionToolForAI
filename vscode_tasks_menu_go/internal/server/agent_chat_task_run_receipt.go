package server

import (
 "crypto/hmac"
 "crypto/rand"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "os"
 "path/filepath"
 "strings"

 "bletonfc/vscode_tasks_menu/internal/patchtool"
 "bletonfc/vscode_tasks_menu/internal/session"
)

// Durable task-run receipts live in TaskDeck's private host config tree,
// NOT in project-owned .vscode/task_run_history or browser-provided log text.
// Only native SessionService metadata may be signed; the long-term receipt
// cannot be minted merely by posting arbitrary metadata to a public API.
const taskRunReceiptVersion=1

type signedTaskRunPayload struct{
 Version int `json:"version"`
 ProjectID string `json:"project_id"`
 Metadata session.Metadata `json:"metadata"`
}
type signedTaskRunReceipt struct{
 Payload signedTaskRunPayload `json:"payload"`
 HMAC string `json:"hmac_sha256"`
}

func (s *Server) taskRunReceiptDirectory(create bool)(string,error){
 base,err:=aiPrivatePath(s.Workspace);if err!=nil{return "",err}
 dir:=filepath.Join(filepath.Dir(base),"agent-task-run-receipts-"+aiWorkspaceHash(s.Workspace))
 if create{
  if err:=os.MkdirAll(dir,0700);err!=nil{return "",err}
 }
 info,err:=os.Lstat(dir)
 if err!=nil{return "",err}
 if !info.IsDir()||info.Mode()&os.ModeSymlink!=0||info.Mode().Perm()&0077!=0{
  return "",errors.New("private Task receipt directory is unsafe")
 }
 return dir,nil
}

func nativeTaskRunReceiptFile(dir,sessionID string)(string,error){
 if strings.TrimSpace(sessionID)==""||len(sessionID)>256{return "",errors.New("invalid native Task session ID")}
 sum:=sha256.Sum256([]byte(sessionID))
 return filepath.Join(dir,"run-"+hex.EncodeToString(sum[:])+".json"),nil
}

func nativeTaskRunReceiptKey(dir string,create bool)([]byte,error){
 path:=filepath.Join(dir,"receipt.key")
 info,err:=os.Lstat(path)
 if os.IsNotExist(err)&&create{
  seed:=make([]byte,32)
  if _,err:=rand.Read(seed);err!=nil{return nil,err}
  fd,e:=os.OpenFile(path,os.O_WRONLY|os.O_CREATE|os.O_EXCL,0600)
  if e==nil{
   _,err=fd.Write(seed)
   if err==nil{err=fd.Sync()}
   closeErr:=fd.Close()
   if err==nil{err=closeErr}
   if err!=nil{return nil,err}
   return seed,nil
  }
  if !os.IsExist(e){return nil,e}
  info,err=os.Lstat(path)
 }
 if err!=nil{return nil,err}
 if !info.Mode().IsRegular()||info.Mode().Perm()&0077!=0{
  return nil,errors.New("native Task receipt key is unsafe")
 }
 seed,err:=os.ReadFile(path)
 if err!=nil{return nil,err}
 if len(seed)!=32{return nil,errors.New("invalid or incomplete Task receipt key")}
 return seed,nil
}

func nativeTaskReceiptMAC(key []byte,payload signedTaskRunPayload)(string,error){
 raw,err:=json.Marshal(payload);if err!=nil{return "",err}
 mac:=hmac.New(sha256.New,key)
 _,_=mac.Write(raw)
 return hex.EncodeToString(mac.Sum(nil)),nil
}

// Record only a finished real native Task; this is called by the existing
// /api/task-runs POST after SessionService.Metadata has verified the session.
// Failed/stopped tasks, legacy history, and browser logs never earn PASS proof.
func (s *Server) recordNativeTaskRunReceipt(meta session.Metadata)error{
 if meta.ID==""||meta.TaskID<1||meta.Status!="exited"||meta.ExitCode==nil||*meta.ExitCode!=0||
  len(meta.CommandSHA256)!=64||strings.TrimSpace(meta.CommandPreview)==""{
  return errors.New("native Task has no finished successful command evidence")
 }
 if meta.Kind!=""&&meta.Kind!="task"{return errors.New("not a native Task")}
 identity,err:=patchtool.ReadProjectIdentity(s.Workspace)
 if err!=nil||identity.ID==""{return errors.New("Task project identity unavailable")}
 dir,err:=s.taskRunReceiptDirectory(true);if err!=nil{return err}
 key,err:=nativeTaskRunReceiptKey(dir,true);if err!=nil{return err}
 target,err:=nativeTaskRunReceiptFile(dir,meta.ID);if err!=nil{return err}
 if _,err:=os.Lstat(target);err==nil{
  return errors.New("Task receipt already exists (immutable)")
 }else if !os.IsNotExist(err){return err}
 payload:=signedTaskRunPayload{Version:taskRunReceiptVersion,ProjectID:identity.ID,Metadata:meta}
 mac,err:=nativeTaskReceiptMAC(key,payload);if err!=nil{return err}
 b,err:=json.Marshal(signedTaskRunReceipt{Payload:payload,HMAC:mac});if err!=nil{return err}
 if len(b)>16<<10{return errors.New("Task metadata receipt exceeds size limit")}
 tmp,err:=os.CreateTemp(dir,".receipt-*.tmp");if err!=nil{return err}
 tmpPath:=tmp.Name();defer os.Remove(tmpPath)
 if err:=tmp.Chmod(0600);err!=nil{tmp.Close();return err}
 if _,err:=tmp.Write(b);err!=nil{tmp.Close();return err}
 if err:=tmp.Sync();err!=nil{tmp.Close();return err}
 if err:=tmp.Close();err!=nil{return err}
 // An existing committed receipt is immutable; no attempt to forge old
 // proof from a repeat browser history submission.
 if _,err:=os.Lstat(target);err==nil{return errors.New("Task receipt already exists")}
 // Create the durable name exclusively even across another TaskDeck process:
 // rename alone could silently replace an existing authenticated receipt.
 if err:=os.Link(tmpPath,target);err!=nil{return err}
 return nil
}

// This is the only persistent fallback after SessionService eviction.
// Stored Task history rows and client-provided console logs are never used.
// A missing key, corrupt receipt, changed project or modified metadata fails
// closed without re-running the Task.
func (s *Server) verifiedNativeTaskRunReceipt(ref,projectID string)(session.Metadata,error){
 dir,err:=s.taskRunReceiptDirectory(false);if err!=nil{return session.Metadata{},err}
 path,err:=nativeTaskRunReceiptFile(dir,ref);if err!=nil{return session.Metadata{},err}
 fi,err:=os.Lstat(path)
 if err!=nil{return session.Metadata{},err}
 if !fi.Mode().IsRegular()||fi.Mode().Perm()&0077!=0||fi.Size()<1||fi.Size()>16<<10{
  return session.Metadata{},errors.New("Task receipt is missing or unsafe")
 }
 key,err:=nativeTaskRunReceiptKey(dir,false);if err!=nil{return session.Metadata{},err}
 raw,err:=os.ReadFile(path);if err!=nil{return session.Metadata{},err}
 var receipt signedTaskRunReceipt
 if err:=json.Unmarshal(raw,&receipt);err!=nil{return session.Metadata{},err}
 if receipt.Payload.Version!=taskRunReceiptVersion||
  receipt.Payload.ProjectID!=projectID||receipt.Payload.Metadata.ID!=ref{
  return session.Metadata{},errors.New("native Task receipt identity mismatch")
 }
 want,err:=nativeTaskReceiptMAC(key,receipt.Payload);if err!=nil{return session.Metadata{},err}
 got,err:=hex.DecodeString(receipt.HMAC)
 if err!=nil||!hmac.Equal(got,mustDecodeReceiptTag(want)){
  return session.Metadata{},errors.New("Task receipt integrity check failed")
 }
 return receipt.Payload.Metadata,nil
}

func mustDecodeReceiptTag(s string)[]byte{
 b,_:=hex.DecodeString(s)
 return b
}

func (s *Server) recordTaskReceiptBestEffort(meta session.Metadata)bool{
 if err:=s.recordNativeTaskRunReceipt(meta);err!=nil{
  // Task History continues to work without a signed receipt. The plan API
  // must fail closed if an old native session disappears without one.
  if s.Log!=nil{s.Log.Printf("Task receipt unavailable for finished native Task: %v",err)}
  return false
 }
 return true
}
