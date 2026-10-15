// Package restrictedview records cumulative source-code exposure outside
// the workspace so repeated requests/reloads cannot reset the viewing quota.
package restrictedview

import (
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "os"
 "path/filepath"
 "strings"
 "time"

 "bletonfc/vscode_tasks_menu/internal/restrictedpolicy"
)

const MaxLedgerBytes=16<<20
var ErrQuota=errors.New("restricted source viewing quota exhausted")
var ErrUnsafeState=errors.New("restricted ledger storage is unsafe")

type Remaining struct {
 File int `json:"file"`
 Today int `json:"today"`
}

type fileUsage struct{
 Baseline int `json:"baseline"`
 Seen map[string]bool `json:"seen"`
}
type accountUsage struct{
 Files map[string]*fileUsage `json:"files"`
 Days map[string]map[string]bool `json:"days"`
}
type ledgerData struct{
 Version int `json:"version"`
 Accounts map[string]*accountUsage `json:"accounts"`
}

type Store struct{
 dir string
}

func hashed(value string)string{
 sum:=sha256.Sum256([]byte(value))
 return hex.EncodeToString(sum[:])
}
func within(root,path string)bool{
 rel,err:=filepath.Rel(root,path)
 return err==nil&&(rel=="."||(!strings.HasPrefix(rel,".."+string(os.PathSeparator))&&rel!=".."))
}

// Open requires storage outside the project. Never use .vscode or writable
// workspace paths for security counters.
func Open(workspace string)(*Store,error){
 project,err:=filepath.EvalSymlinks(workspace);if err!=nil{return nil,err}
 configDir,err:=os.UserConfigDir();if err!=nil{return nil,err}
 configDir,err=filepath.Abs(configDir);if err!=nil{return nil,err}
 dir:=filepath.Join(configDir,"taskdeck-restricted-ledger",hashed(project)[:24])
 if err:=os.MkdirAll(dir,0700);err!=nil{return nil,err}
 info,err:=os.Lstat(dir)
 if err!=nil||!info.IsDir()||info.Mode().Perm()&0077!=0{
  return nil,ErrUnsafeState
 }
 realDir,err:=filepath.EvalSymlinks(dir);if err!=nil{return nil,ErrUnsafeState}
 if within(project,realDir){return nil,ErrUnsafeState}
 return &Store{dir:realDir},nil
}

func readLedger(path string)(ledgerData,error){
 out:=ledgerData{Version:1,Accounts:map[string]*accountUsage{}}
 info,err:=os.Lstat(path)
 if errors.Is(err,os.ErrNotExist){return out,nil}
 if err!=nil{return out,err}
 if !info.Mode().IsRegular()||info.Mode().Perm()&0077!=0||info.Size()>MaxLedgerBytes {
  return out,ErrUnsafeState
 }
 f,err:=os.Open(path);if err!=nil{return out,err};defer f.Close()
 data,err:=io.ReadAll(io.LimitReader(f,MaxLedgerBytes+1))
 if err!=nil||len(data)>MaxLedgerBytes{return out,ErrUnsafeState}
 if err=json.Unmarshal(data,&out);err!=nil||out.Version!=1||out.Accounts==nil{return out,ErrUnsafeState}
 return out,nil
}

func saveLedger(path string,data ledgerData)error{
 body,err:=json.Marshal(data);if err!=nil{return err}
 if len(body)>MaxLedgerBytes{return ErrQuota}
 dir:=filepath.Dir(path)
 temp,err:=os.CreateTemp(dir,".ledger-*");if err!=nil{return err}
 success:=false
 defer func(){temp.Close();if !success{os.Remove(temp.Name())}}()
 if _,err=temp.Write(body);err!=nil{return err}
 if err=temp.Sync();err!=nil{return err}
 if err=temp.Close();err!=nil{return err}
 if err=os.Rename(temp.Name(),path);err!=nil{return err}
 success=true
 d,err:=os.Open(dir);if err==nil{_ = d.Sync();_ = d.Close()}
 return nil
}

// Charge reserves the exact line content hashes that will be returned.
// Duplicate reads in a day do not charge again, but viewing different
// content at the same line number DOES. Limits are checked before writing.
func (s *Store) Charge(user,projectFile string,startLine int,lines []string,totalLines int,p restrictedpolicy.Policy)(Remaining,error){
 if s==nil||user==""||projectFile==""||startLine<1||totalLines<1||len(lines)<1||len(lines)>p.MaxVisibleLines||startLine>totalLines||len(lines)>totalLines-startLine+1{
  return Remaining{},ErrQuota
 }
 stat,err:=os.Stat(projectFile)
 if err!=nil||!stat.Mode().IsRegular(){return Remaining{},ErrQuota}
 canonical,err:=filepath.EvalSymlinks(projectFile);if err!=nil{return Remaining{},err}
 lock,err:=os.OpenFile(filepath.Join(s.dir,"ledger.lock"),os.O_CREATE|os.O_RDWR,0600)
 if err!=nil{return Remaining{},err}
 defer lock.Close()
 if err=lockExclusive(lock);err!=nil{return Remaining{},err}
 defer unlockExclusive(lock)
 location:=filepath.Join(s.dir,"ledger.json")
 ledger,err:=readLedger(location);if err!=nil{return Remaining{},err}
 accountKey:=hashed(user)
 ac:=ledger.Accounts[accountKey]
 if ac==nil{ac=&accountUsage{Files:map[string]*fileUsage{},Days:map[string]map[string]bool{}};ledger.Accounts[accountKey]=ac}
 fileKey:=hashed(canonical)
 usage:=ac.Files[fileKey]
 if usage==nil{usage=&fileUsage{Baseline:totalLines,Seen:map[string]bool{}};ac.Files[fileKey]=usage}
 if usage.Baseline==0||totalLines<usage.Baseline{usage.Baseline=totalLines}
 date:=time.Now().UTC().Format("2006-01-02")
 daily:=ac.Days[date]
 if daily==nil{daily=map[string]bool{};ac.Days[date]=daily}
 // Old daily counters can be removed only after they are no longer part of
 // the active daily window; lifetime file counters are never removed.
 for day:=range ac.Days{if day!=date{delete(ac.Days,day)}}
 // Construct prospective sets without mutating the ledger until approved.
 seen:=make(map[string]bool,len(usage.Seen))
 for hash,ok:=range usage.Seen{if ok{seen[hash]=true}}
 today:=make(map[string]bool,len(daily))
 for hash,ok:=range daily{if ok{today[hash]=true}}
 for i,line:=range lines{
  // Equal text at different source positions is a different disclosure.
  // Include content digest to ensure an edited line also counts as a new
  // exposure; a request for the same unchanged line is free.
  digest:=hashed(fmt.Sprintf("%d:%s",startLine+i,line))
  seen[digest]=true
  today[fileKey+":"+digest]=true
 }
 maxByPercent:=usage.Baseline*p.MaxRevealPercent/100
 limit:=p.MaxUniqueLinesPerFile
 if maxByPercent<limit{limit=maxByPercent}
 if len(seen)>limit||len(today)>p.MaxUniqueLinesPerDay{
  return Remaining{},ErrQuota
 }
 usage.Seen=seen
 ac.Days[date]=today
 if err=saveLedger(location,ledger);err!=nil{return Remaining{},fmt.Errorf("persist source exposure quota: %w",err)}
 return Remaining{File:limit-len(seen),Today:p.MaxUniqueLinesPerDay-len(today)},nil
}
