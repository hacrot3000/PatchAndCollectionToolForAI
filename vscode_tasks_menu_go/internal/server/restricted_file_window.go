package server

import (
 "bytes"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "io"
 "net/http"
 "strings"
 "unicode/utf8"

 "bletonfc/vscode_tasks_menu/internal/restrictedview"
)

// This POST route is intentionally separate from the legacy full-file editor.
// It only emits a requested, quota-charged source window, never the full file.
func (s *Server) restrictedFileWindowAPI(w http.ResponseWriter,r *http.Request){
 if s.Config.Restricted==nil{http.NotFound(w,r);return}
 if r.Method!=http.MethodPost {
  w.Header().Set("Allow","POST")
  http.Error(w,"method not allowed",http.StatusMethodNotAllowed);return
 }
 w.Header().Set("Cache-Control","no-store")
 r.Body=http.MaxBytesReader(w,r.Body,4096);defer r.Body.Close()
 var input struct{
  Path string `json:"path"`
  Start int `json:"start"`
  Count int `json:"count"`
 }
 dec:=json.NewDecoder(r.Body);dec.DisallowUnknownFields()
 if err:=dec.Decode(&input);err!=nil {
  http.Error(w,"invalid protected view request",http.StatusBadRequest);return
 }
 if err:=dec.Decode(new(any));err!=io.EOF{
  http.Error(w,"exactly one request object is required",http.StatusBadRequest);return
 }
 if input.Start<1||input.Count<1||input.Count>s.Config.Restricted.MaxVisibleLines {
  http.Error(w,"invalid line window",http.StatusBadRequest);return
 }
 rel,err:=cleanProjectRelativePath(input.Path,false)
 if err!=nil{http.Error(w,"invalid source file",http.StatusBadRequest);return}
 for _,part:=range strings.Split(strings.ReplaceAll(rel,"\\","/"),"/"){
  if part==""||strings.HasPrefix(part,".") {
   http.Error(w,"protected path is not accessible",http.StatusForbidden);return
  }
 }
 path,err:=s.resolveProjectPath(rel,false,false)
 if err!=nil{http.Error(w,"source unavailable",http.StatusNotFound);return}
 rootView,_,err:=s.projectRootForVirtualPath(rel)
 if err!=nil{http.Error(w,"source unavailable",http.StatusNotFound);return}
 pinned,err:=openProjectPinnedFile(rootView.Path,path)
 if err!=nil{http.Error(w,"source unavailable",http.StatusNotFound);return}
 defer pinned.close()
 data,info,err:=pinned.readCurrent(projectReadableLimit)
 if err!=nil||info==nil||!info.Mode().IsRegular(){
  http.Error(w,"source unavailable or too large",http.StatusRequestEntityTooLarge);return
 }
 if !utf8.Valid(data)||!projectTextBytesValid(data){
  http.Error(w,"source file is not UTF-8 text",http.StatusUnsupportedMediaType);return
 }
 segments:=bytes.SplitAfter(data,[]byte("\n"))
 if len(segments)>0&&len(segments[len(segments)-1])==0{segments=segments[:len(segments)-1]}
 if input.Start>len(segments){
  http.Error(w,"requested line does not exist",http.StatusRequestedRangeNotSatisfiable);return
 }
 end:=input.Start-1+input.Count
 if end>len(segments){end=len(segments)}
 window:=segments[input.Start-1:end]
 // Guard extremely long physical lines even for a small line count.
 const maxWindowBytes=64<<10
 textLines:=make([]string,0,len(window))
 totalBytes:=0
 for _,line:=range window{
  totalBytes+=len(line)
  if totalBytes>maxWindowBytes{
   http.Error(w,"protected window is too large",http.StatusRequestEntityTooLarge);return
  }
  textLines=append(textLines,string(line))
 }
 user:="single-instance"
 if s.Config.SharedServerEnabled{
  principal,ok:=PrincipalFromContext(r.Context())
  if !ok{http.Error(w,"authentication required",http.StatusUnauthorized);return}
  user=string(principal.ProjectID)+":"+string(principal.UserID)
 }
 store,err:=restrictedview.Open(s.Workspace)
 if err!=nil{
  http.Error(w,"restricted source quota storage is unavailable",http.StatusServiceUnavailable);return
 }
 remaining,err:=store.Charge(user,path,textLines,len(segments),*s.Config.Restricted)
 if errors.Is(err,restrictedview.ErrQuota){
  http.Error(w,"source viewing budget exhausted",http.StatusTooManyRequests);return
 }
 if err!=nil{
  http.Error(w,"protected source unavailable",http.StatusServiceUnavailable);return
 }
 hash:=sha256.Sum256(data)
 writeJSON(w,http.StatusOK,map[string]any{
  "path":rel,"start_line":input.Start,"end_line":end,
  "total_lines":len(segments),"content":string(bytes.Join(window,nil)),
  "sha256":hex.EncodeToString(hash[:]),"remaining":remaining,
  "editable":false,"window_only":true,
 })
}
