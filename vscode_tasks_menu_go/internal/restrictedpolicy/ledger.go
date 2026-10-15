package restrictedpolicy

import (
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "os"
 "path/filepath"
 "sort"
 "time"
)

var ErrRevealBudget=errors.New("protected source exposure budget exhausted")
var ErrLedgerLocked=errors.New("protected source ledger is locked; denying access")
const maxLedgerBytes=8<<20

// Ledger is a durable admission counter. Reserve BEFORE a code window is
// returned. Its storage path must be server-owned, outside project worktrees;
// never use a file path supplied by the browser.
type Ledger struct{ Path string }

type subjectExposure struct {
 Files map[string][]int `json:"files"` // Lifetime unique line numbers per canonical file.
 Days map[string]map[string][]int `json:"days"` // Unique per-file lines viewed each UTC day.
}
type exposureStore struct {
 Subjects map[string]*subjectExposure `json:"subjects"`
}
func hashedID(s string) string {b:=sha256.Sum256([]byte(s));return hex.EncodeToString(b[:])}

func (l Ledger) ReserveWindow(now time.Time,principalID,canonicalFileID string,
 totalLines,startLine,count int,p Policy) error {
 if err:=p.Validate();err!=nil{return err}
 if principalID==""||canonicalFileID=="" {return errors.New("missing stable subject or file identity")}
 if totalLines<=0||totalLines>10000000||startLine<1||count<1||
  count>p.MaxVisibleLines||startLine>totalLines||count>totalLines-startLine+1 {
  return fmt.Errorf("%w: invalid line window",ErrRevealBudget)
 }
 if !filepath.IsAbs(l.Path){return errors.New("source exposure ledger requires absolute private path")}
 dir:=filepath.Dir(l.Path)
 info,err:=os.Lstat(dir)
 if err!=nil{return err}
 if !info.IsDir()||info.Mode()&os.ModeSymlink!=0 {
  return errors.New("ledger directory must be a real directory")
 }
 lock,err:=os.OpenFile(l.Path+".lock",os.O_WRONLY|os.O_CREATE|os.O_EXCL,0600)
 if err!=nil{return fmt.Errorf("%w: %v",ErrLedgerLocked,err)}
 defer func(){lock.Close();os.Remove(l.Path+".lock")}()
 return l.reserveLocked(now,principalID,canonicalFileID,totalLines,startLine,count,p)
}
func (l Ledger) reserveLocked(now time.Time,user,file string,total,start,count int,p Policy) error {
 store:=exposureStore{Subjects:map[string]*subjectExposure{}}
 stat,err:=os.Lstat(l.Path)
 if err==nil {
  if !stat.Mode().IsRegular()||stat.Mode().Perm()&0022!=0 {
   return errors.New("unsafe source ledger permissions or symlink")
  }
  if stat.Size()>maxLedgerBytes{return errors.New("ledger capacity reached; denied")}
  in,err:=os.Open(l.Path);if err!=nil{return err}
  payload,readErr:=io.ReadAll(io.LimitReader(in,maxLedgerBytes+1))
  closeErr:=in.Close()
  if readErr!=nil{return readErr}
  if closeErr!=nil{return closeErr}
  if len(payload)>maxLedgerBytes{return errors.New("ledger too large")}
  if err:=json.Unmarshal(payload,&store);err!=nil{return fmt.Errorf("corrupt exposure ledger; denied: %w",err)}
  if store.Subjects==nil{return errors.New("corrupt exposure ledger; missing subjects")}
 }else if !errors.Is(err,os.ErrNotExist){return err}
 userID,fileID:=hashedID(user),hashedID(file)
 subject:=store.Subjects[userID]
 if subject==nil{subject=&subjectExposure{Files:map[string][]int{},Days:map[string]map[string][]int{}};store.Subjects[userID]=subject}
 if subject.Files==nil||subject.Days==nil{return errors.New("corrupt ledger subject; denied")}
 day:=now.UTC().Format("2006-01-02")
 if subject.Days[day]==nil{subject.Days[day]=map[string][]int{}}
 life:=make(map[int]bool,len(subject.Files[fileID])+count)
 for _,v:=range subject.Files[fileID] {if v<1{return errors.New("corrupt lifetime ledger")};life[v]=true}
 daily:=make(map[int]bool,len(subject.Days[day][fileID])+count)
 for _,v:=range subject.Days[day][fileID]{if v<1{return errors.New("corrupt daily ledger")};daily[v]=true}
 allDaily:=0
 for _,list:=range subject.Days[day] {allDaily+=len(list)}
 prevDaily:=len(daily)
 for line:=start;line<start+count;line++{life[line]=true;daily[line]=true}
 allowedPercent:=int((int64(total)*int64(p.MaxRevealPercent))/100)
 if len(life)>p.MaxUniqueLinesPerFile||len(life)>allowedPercent||
  allDaily+len(daily)-prevDaily>p.MaxUniqueLinesPerDay {
  return ErrRevealBudget
 }
 subject.Files[fileID]=sortedLines(life)
 subject.Days[day][fileID]=sortedLines(daily)
 // Retain lifetime file exposure, but prune stale daily accounting to avoid
 // unbounded growth while remaining conservative across process restarts.
 for date:=range subject.Days{if date!=day {delete(subject.Days,date)}}
 raw,err:=json.Marshal(store);if err!=nil{return err}
 if len(raw)>maxLedgerBytes{return errors.New("ledger size limit reached; denied")}
 tmp,err:=os.CreateTemp(filepath.Dir(l.Path),".exposure-*.tmp")
 if err!=nil{return err}
 tempPath:=tmp.Name()
 defer os.Remove(tempPath)
 if err=tmp.Chmod(0600);err==nil{_,err=tmp.Write(raw)}
 if err==nil{err=tmp.Sync()}
 if closeErr:=tmp.Close();err==nil{err=closeErr}
 if err!=nil{return err}
 if err:=os.Rename(tempPath,l.Path);err!=nil{return err}
 parent,err:=os.Open(filepath.Dir(l.Path));if err!=nil{return err}
 err=parent.Sync();parent.Close()
 // Failing to persist the directory update must fail the read request.
 return err
}
func sortedLines(set map[int]bool) []int {
 lines:=make([]int,0,len(set))
 for n:=range set{lines=append(lines,n)}
 sort.Ints(lines)
 return lines
}
