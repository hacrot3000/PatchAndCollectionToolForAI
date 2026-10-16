package server

import (
 "bytes"
 "encoding/json"
 "errors"
 "io"
 "net/http"
 "strings"
 "unicode/utf8"

 "bletonfc/vscode_tasks_menu/internal/restrictedview"
)

// restrictedSearchAPI searches ONLY an explicit physical-line window of one
// source file. Every line inspected is charged before returning results;
// no global scan, directory enumeration, source export or regex execution.
func (s *Server) restrictedSearchAPI(w http.ResponseWriter,r *http.Request){
 if s.Config.Restricted==nil{http.NotFound(w,r);return}
 if r.Method!=http.MethodPost{http.Error(w,"method not allowed",http.StatusMethodNotAllowed);return}
 w.Header().Set("Cache-Control","no-store")
 r.Body=http.MaxBytesReader(w,r.Body,4096);defer r.Body.Close()
 var req struct{
  Path string `json:"path"`
  Start int `json:"start"`
  Count int `json:"count"`
  Query string `json:"query"`
 }
 dec:=json.NewDecoder(r.Body);dec.DisallowUnknownFields()
 if err:=dec.Decode(&req);err!=nil{http.Error(w,"invalid search request",http.StatusBadRequest);return}
 if err:=dec.Decode(new(any));err!=io.EOF{http.Error(w,"exactly one request required",http.StatusBadRequest);return}
 if req.Start<1||req.Count<1||req.Count>s.Config.Restricted.MaxVisibleLines||
  len(req.Query)<2||len(req.Query)>128||!utf8.ValidString(req.Query)||strings.ContainsAny(req.Query,"\r\n\x00"){
  http.Error(w,"invalid bounded search",http.StatusBadRequest);return
 }
 user:="single-instance"
 if s.Config.SharedServerEnabled{
  principal,ok:=PrincipalFromContext(r.Context())
  if !ok{http.Error(w,"authentication required",http.StatusUnauthorized);return}
  user=string(principal.ProjectID)+":"+string(principal.UserID)
 }
 rel,path,root,err:=s.restrictedSourcePath(req.Path)
 if err!=nil{http.Error(w,"source unavailable",http.StatusForbidden);return}
 pinned,err:=openProjectPinnedFile(root,path)
 if err!=nil{http.Error(w,"source unavailable",http.StatusNotFound);return}
 defer pinned.close()
 data,info,err:=pinned.readCurrent(projectReadableLimit)
 if err!=nil||info==nil||!info.Mode().IsRegular()||!utf8.Valid(data)||!projectTextBytesValid(data){
  http.Error(w,"source is not bounded UTF-8 text",http.StatusUnsupportedMediaType);return
 }
 parts:=bytes.SplitAfter(data,[]byte("\n"))
 if len(parts)>0&&len(parts[len(parts)-1])==0{parts=parts[:len(parts)-1]}
 if req.Start>len(parts){http.Error(w,"invalid source range",http.StatusRequestedRangeNotSatisfiable);return}
 end:=len(parts)
 if req.Count<=len(parts)-req.Start+1{end=req.Start-1+req.Count}
 lines:=make([]string,0,end-req.Start+1)
 size:=0
 for _,line:=range parts[req.Start-1:end]{
  size+=len(line)
  if size>64<<10{http.Error(w,"window too large",http.StatusRequestEntityTooLarge);return}
  lines=append(lines,string(line))
 }
 store,err:=restrictedview.Open(s.Workspace)
 if err!=nil{http.Error(w,"quota unavailable",http.StatusServiceUnavailable);return}
 remaining,err:=store.Charge(user,path,req.Start,lines,len(parts),*s.Config.Restricted)
 if errors.Is(err,restrictedview.ErrQuota){http.Error(w,"source viewing budget exhausted",http.StatusTooManyRequests);return}
 if err!=nil{http.Error(w,"source quota unavailable",http.StatusServiceUnavailable);return}
 type match struct{Line int `json:"line"`; Text string `json:"text"`}
 matches:=make([]match,0)
 for i,line:=range lines{
  if strings.Contains(line,req.Query){matches=append(matches,match{Line:req.Start+i,Text:line})}
 }
 writeJSON(w,http.StatusOK,map[string]any{
  "path":rel,"start_line":req.Start,"end_line":end,"matches":matches,
  "window_only":true,"scanned_lines":len(lines),"remaining":remaining,
 })
}
