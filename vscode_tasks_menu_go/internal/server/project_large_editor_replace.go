package server

import (
 "bufio"
 "bytes"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net/http"
 "os"
 "regexp"
 "strings"
 "unicode/utf8"
)

// Large-file replace is literal (not sed/regex syntax) and streams one line
// at a time into a pinned atomic temp. No arbitrary shell processes are run.
type largeEditorReplaceInput struct {
 Path string `json:"path"`
 Find string `json:"find"`
 Replacement string `json:"replacement"`
 MatchCase bool `json:"match_case"`
 All bool `json:"all"`
 Line int `json:"line"`
 Column int `json:"column"` // UTF-8 byte offset, zero based
 Size int64 `json:"expected_size"`
 MtimeNS int64 `json:"expected_mtime_ns"`
 SHA256 string `json:"expected_sha256"`
}

const largeEditorReplaceMaxMatches = 2000000

type largeEditorReplaceOutcome struct {
 Replacements int
 FirstLine int
 SourceHash string
 Error error
}

func buildLargeEditorReplacePattern(needle string, matchCase bool) (*regexp.Regexp,error) {
 if needle==""||len(needle)>256||strings.ContainsAny(needle,"\r\n")||strings.ContainsRune(needle,'\x00')||!utf8.ValidString(needle) {
  return nil,fmt.Errorf("find must be a nonempty single-line UTF-8 literal up to 256 bytes")
 }
 pattern:=regexp.QuoteMeta(needle)
 if !matchCase { pattern="(?i)"+pattern }
 return regexp.Compile(pattern)
}

func writeLargeEditorReplacement(dst io.Writer,src io.Reader,matcher *regexp.Regexp,req largeEditorReplaceInput) largeEditorReplaceOutcome {
 reader:=bufio.NewReaderSize(src,64<<10)
 hash:=sha256.New()
 lineNo:=0
 result:=largeEditorReplaceOutcome{}
 for {
  raw,err:=readBoundedEditorLine(reader)
  if err!=nil&&!errors.Is(err,io.EOF) {result.Error=err;return result}
  if len(raw)==0&&errors.Is(err,io.EOF) {break}
  lineNo++
  if !utf8.Valid(raw)||bytes.IndexByte(raw,0)>=0 {
   result.Error=fmt.Errorf("large-file replace requires valid UTF-8 text")
   return result
  }
  if _,hashErr:=hash.Write(raw);hashErr!=nil {result.Error=hashErr;return result}
  // Avoid substring copies on unrelated lines, and don't touch any byte that
  // doesn't belong to a confirmed literal match.
  lineText:=string(raw)
  editLine:=req.All||lineNo==req.Line
  if editLine {
   matches:=matcher.FindAllStringIndex(lineText,-1)
   if len(matches)>0 {
    if !req.All {
     found:=false
     for _,m:=range matches {
      if m[0]==req.Column {matches=[][]int{m};found=true;break}
     }
     if !found {matches=nil}
    }
    if len(matches)>0 {
     result.Replacements+=len(matches)
     if result.Replacements>largeEditorReplaceMaxMatches {
      result.Error=fmt.Errorf("too many replacements (limit %d)",largeEditorReplaceMaxMatches)
      return result
     }
     if result.FirstLine==0 {result.FirstLine=lineNo}
     // Bound allocations before constructing the output. A 2 MiB line
     // containing many short matches and a large replacement must not
     // expand into gigabytes in memory.
     nextSize:=int64(len(lineText))
     for _,m:=range matches {
      nextSize+=int64(len(req.Replacement)-(m[1]-m[0]))
      if nextSize>8<<20 {
       result.Error=fmt.Errorf("one replaced line exceeds 8 MiB safety limit")
       return result
      }
     }
     var output strings.Builder
     output.Grow(int(nextSize))
     pos:=0
     for _,m:=range matches {
      output.WriteString(lineText[pos:m[0]])
      output.WriteString(req.Replacement)
      pos=m[1]
     }
     output.WriteString(lineText[pos:])
     lineText=output.String()
    }
   }
  }
  if _,writeErr:=io.WriteString(dst,lineText);writeErr!=nil {result.Error=writeErr;return result}
  if err!=nil {break}
 }
 result.SourceHash=hex.EncodeToString(hash.Sum(nil))
 return result
}

func (s *Server) projectLargeFileReplace(w http.ResponseWriter,r *http.Request) {
 if r.Method!=http.MethodPost {http.Error(w,"method not allowed",http.StatusMethodNotAllowed);return}
 w.Header().Set("Cache-Control","no-store")
 var req largeEditorReplaceInput
 dec:=json.NewDecoder(http.MaxBytesReader(w,r.Body,16<<10))
 dec.DisallowUnknownFields()
 if err:=dec.Decode(&req);err!=nil {http.Error(w,"invalid replace payload",http.StatusBadRequest);return}
 if err:=dec.Decode(new(any));err!=io.EOF {http.Error(w,"exactly one JSON object required",http.StatusBadRequest);return}
 rel,err:=cleanProjectRelativePath(req.Path,false)
 if err!=nil {http.Error(w,"invalid file path",http.StatusBadRequest);return}
 matcher,err:=buildLargeEditorReplacePattern(req.Find,req.MatchCase)
 if err!=nil||!utf8.ValidString(req.Replacement)||strings.ContainsRune(req.Replacement,'\x00')||len(req.Replacement)>64<<10 {
  http.Error(w,"invalid UTF-8 find/replacement",http.StatusBadRequest);return
 }
 if !req.All&&(req.Line<1||req.Column<0){http.Error(w,"missing absolute match position",http.StatusBadRequest);return}
 if req.Size<0||req.MtimeNS<=0||len(req.SHA256)!=64 {
  http.Error(w,"missing source version",http.StatusBadRequest);return
 }
 pinned,file,path,info,err:=s.openLargeEditorFile(rel)
 if err!=nil {http.Error(w,"file unavailable",http.StatusNotFound);return}
 defer pinned.close()
 defer file.Close()
 if info.Mode().Perm()&0o222==0 {http.Error(w,"file is read-only",http.StatusForbidden);return}
 lease,ok:=s.acquireSharedMutation(w,r,"file.write",rel)
 if !ok{return}
 defer s.releaseSharedMutation(lease)
 if req.Size!=info.Size()||req.MtimeNS!=info.ModTime().UnixNano() {
  http.Error(w,"file changed outside editor",http.StatusConflict);return
 }
 if _,err=file.Seek(0,io.SeekStart);err!=nil {http.Error(w,"cannot read file",http.StatusInternalServerError);return}

 // writeTempStream performs an atomic temporary write, preserving file mode
 // and metadata. The pipe lets us stream without full-file memory allocation.
 pr,pw:=io.Pipe()
 completed:=make(chan largeEditorReplaceOutcome,1)
 go func(){
  outcome:=writeLargeEditorReplacement(pw,file,matcher,req)
  _=pw.CloseWithError(outcome.Error)
  completed<-outcome
 }()
 writeErr:=pinned.writeTempStream(pr,info)
 _=pr.CloseWithError(writeErr)
 outcome:=<-completed
 if outcome.Error!=nil||writeErr!=nil {
  http.Error(w,"replacement could not be prepared",http.StatusInternalServerError);return
 }
 if !strings.EqualFold(outcome.SourceHash,req.SHA256) {
  http.Error(w,"file content changed before replace",http.StatusConflict);return
 }
 if !req.All&&outcome.Replacements!=1 {
  http.Error(w,"selected match is no longer present",http.StatusConflict);return
 }
 if outcome.Replacements==0 {
  writeJSON(w,http.StatusOK,map[string]any{"ok":true,"replacements":0,"first_line":0,"unchanged":true})
  return
 }
 // Recheck the path via pinned parent immediately before committing the temp.
 latest,openErr:=pinned.openCurrent()
 if openErr!=nil {http.Error(w,"file replaced during operation",http.StatusConflict);return}
 current,statErr:=latest.Stat()
 if statErr!=nil||current.Size()!=req.Size||current.ModTime().UnixNano()!=req.MtimeNS||!os.SameFile(info,current) {
  latest.Close()
  http.Error(w,"file changed during replacement",http.StatusConflict);return
 }
 if _,err=latest.Seek(0,io.SeekStart);err!=nil {
  latest.Close();http.Error(w,"file unavailable",http.StatusConflict);return
 }
 verification:=sha256.New()
 _,err=io.Copy(verification,latest)
 latest.Close()
 if err!=nil||!strings.EqualFold(hex.EncodeToString(verification.Sum(nil)),req.SHA256) {
  http.Error(w,"file content changed during replacement",http.StatusConflict);return
 }
 if err=pinned.commitTemp();err!=nil {http.Error(w,"cannot commit replacement",http.StatusInternalServerError);return}
 forgetLargeEditorIndex(path)
 s.invalidateProjectSymbolIndex()
 s.auditSharedSuccess(r,"file.write","file",rel,map[string]any{
  "replace_all":req.All,"replacements":outcome.Replacements,
 })
 writeJSON(w,http.StatusOK,map[string]any{
  "ok":true,"replacements":outcome.Replacements,"first_line":outcome.FirstLine,
  "history_warning":"Large-file replace does not create a local history snapshot; use Git for recovery.",
 })
}
