package server

import (
 "bytes"
 "encoding/json"
 "errors"
 "io"
 "net/http"
 "unicode/utf8"

 "bletonfc/vscode_tasks_menu/internal/restrictedview"
)

// restrictedCompareAPI compares two explicitly bounded physical line windows.
// It never invokes git, emits file contents outside either window, or permits
// a legacy full-file compare/export path.
func (s *Server) restrictedCompareAPI(w http.ResponseWriter,r *http.Request){
 if s.Config.Restricted==nil{http.NotFound(w,r);return}
 if r.Method!=http.MethodPost{http.Error(w,"method not allowed",http.StatusMethodNotAllowed);return}
 w.Header().Set("Cache-Control","no-store")
 r.Body=http.MaxBytesReader(w,r.Body,4096)
 defer r.Body.Close()
 var req struct {
  Left string `json:"left"`
  Right string `json:"right"`
  LeftStart int `json:"left_start"`
  RightStart int `json:"right_start"`
  Count int `json:"count"`
 }
 dec:=json.NewDecoder(r.Body);dec.DisallowUnknownFields()
 if err:=dec.Decode(&req);err!=nil{http.Error(w,"invalid compare request",http.StatusBadRequest);return}
 if err:=dec.Decode(new(any));err!=io.EOF{http.Error(w,"exactly one JSON request required",http.StatusBadRequest);return}
 if req.Count<1||req.Count>s.Config.Restricted.MaxVisibleLines||req.LeftStart<1||req.RightStart<1{
  http.Error(w,"invalid compare window",http.StatusBadRequest);return
 }
 user:="single-instance"
 if s.Config.SharedServerEnabled{
  principal,ok:=PrincipalFromContext(r.Context())
  if !ok{http.Error(w,"authentication required",http.StatusUnauthorized);return}
  user=string(principal.ProjectID)+":"+string(principal.UserID)
 }
 type window struct{path string; start int; total int; lines []string}
 load:=func(value string,start int)(window,error){
  _,path,root,err:=s.restrictedSourcePath(value)
  if err!=nil{return window{},err}
  pinned,err:=openProjectPinnedFile(root,path)
  if err!=nil{return window{},err}
  defer pinned.close()
  data,info,err:=pinned.readCurrent(projectReadableLimit)
  if err!=nil||info==nil||!info.Mode().IsRegular()||!utf8.Valid(data)||!projectTextBytesValid(data){
   return window{},errors.New("invalid protected source")
  }
  parts:=bytes.SplitAfter(data,[]byte("\n"))
  if len(parts)>0&&len(parts[len(parts)-1])==0{parts=parts[:len(parts)-1]}
  if start>len(parts){return window{},errors.New("range exceeds source")}
  end:=start-1+req.Count
  if end>len(parts){end=len(parts)}
  lines:=make([]string,0,end-start+1)
  totalBytes:=0
  for _,part:=range parts[start-1:end]{
   totalBytes+=len(part)
   if totalBytes>64<<10{return window{},errors.New("window exceeds byte limit")}
   lines=append(lines,string(part))
  }
  return window{path:path,start:start,total:len(parts),lines:lines},nil
 }
 left,err:=load(req.Left,req.LeftStart)
 if err!=nil{http.Error(w,"left source unavailable",http.StatusForbidden);return}
 right,err:=load(req.Right,req.RightStart)
 if err!=nil{http.Error(w,"right source unavailable",http.StatusForbidden);return}
 store,err:=restrictedview.Open(s.Workspace)
 if err!=nil{http.Error(w,"quota store unavailable",http.StatusServiceUnavailable);return}
 // Debits are deliberately committed before disclosing either source. If
 // the second debit fails, the first stays charged but NO content is sent.
 for _,part:=range []window{left,right}{
  _,err=store.Charge(user,part.path,part.start,part.lines,part.total,*s.Config.Restricted)
  if errors.Is(err,restrictedview.ErrQuota){http.Error(w,"source viewing budget exhausted",http.StatusTooManyRequests);return}
  if err!=nil{http.Error(w,"quota unavailable",http.StatusServiceUnavailable);return}
 }
 type comparison struct{
  LeftLine int `json:"left_line"`
  RightLine int `json:"right_line"`
  Left string `json:"left"`
  Right string `json:"right"`
  Equal bool `json:"equal"`
 }
 n:=len(left.lines)
 if len(right.lines)>n{n=len(right.lines)}
 rows:=make([]comparison,0,n)
 for i:=0;i<n;i++{
  row:=comparison{}
  if i<len(left.lines){row.LeftLine=left.start+i;row.Left=left.lines[i]}
  if i<len(right.lines){row.RightLine=right.start+i;row.Right=right.lines[i]}
  row.Equal=row.LeftLine!=0&&row.RightLine!=0&&row.Left==row.Right
  rows=append(rows,row)
 }
 writeJSON(w,http.StatusOK,map[string]any{"rows":rows,"window_only":true,"left_count":len(left.lines),"right_count":len(right.lines)})
}
