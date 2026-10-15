package server

import (
 "bytes"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "io"
 "net/http"
 "strings"
 "unicode/utf8"

 "bletonfc/vscode_tasks_menu/internal/restrictededit"
 "bletonfc/vscode_tasks_menu/internal/restrictedview"
)

// restrictedFileEditAPI edits ONLY previously viewed physical lines. It does
// not return source text, accept full-file contents, or permit line insertion
// and deletion. The ordinary full-file editor API stays denied in this mode.
func (s *Server) restrictedFileEditAPI(w http.ResponseWriter,r *http.Request){
 if s.Config.Restricted==nil{http.NotFound(w,r);return}
 if r.Method!=http.MethodPost{
  w.Header().Set("Allow","POST")
  http.Error(w,"method not allowed",http.StatusMethodNotAllowed);return
 }
 w.Header().Set("Cache-Control","no-store")
 r.Body=http.MaxBytesReader(w,r.Body,128<<10)
 defer r.Body.Close()
 var req struct{
  Path string `json:"path"`
  StartLine int `json:"start_line"`
  Lines []string `json:"lines"`
  ExpectedSHA256 string `json:"expected_sha256"`
 }
 dec:=json.NewDecoder(r.Body);dec.DisallowUnknownFields()
 if err:=dec.Decode(&req);err!=nil{
  http.Error(w,"invalid protected edit request",http.StatusBadRequest);return
 }
 if err:=dec.Decode(new(any));err!=io.EOF{
  http.Error(w,"exactly one protected edit request is required",http.StatusBadRequest);return
 }
 if req.StartLine<1||len(req.Lines)==0||len(req.Lines)>s.Config.Restricted.MaxVisibleLines{
  http.Error(w,"invalid protected edit range",http.StatusBadRequest);return
 }
 if len(req.ExpectedSHA256)!=sha256.Size*2{
  http.Error(w,"expected file SHA-256 is required",http.StatusBadRequest);return
 }
 if _,err:=hex.DecodeString(req.ExpectedSHA256);err!=nil{
  http.Error(w,"invalid expected file SHA-256",http.StatusBadRequest);return
 }
 // Tie edit authorization to the exact account whose protected view
 // was charged. Shared mode never falls back to the single-user identity.
 user:="single-instance"
 if s.Config.SharedServerEnabled{
  principal,authenticated:=PrincipalFromContext(r.Context())
  if !authenticated{http.Error(w,"authentication required",http.StatusUnauthorized);return}
  user=string(principal.ProjectID)+":"+string(principal.UserID)
 }
 rel,path,root,err:=s.restrictedSourcePath(req.Path)
 if err!=nil{http.Error(w,"protected source path is not accessible",http.StatusForbidden);return}
 store,err:=restrictedview.Open(s.Workspace)
 if err!=nil{http.Error(w,"protected source quota storage unavailable",http.StatusServiceUnavailable);return}

 // This lock also serializes ordinary file writes in shared-server mode.
 // No source bytes are sent back on conflicts or unauthorized edits.
 lease,ok:=s.acquireSharedMutation(w,r,"file.write",rel)
 if !ok{return}
 defer s.releaseSharedMutation(lease)
 // Serialize range edits on this restricted instance. With the existing
 // SHA precondition, a second stale concurrent request must conflict rather
 // than silently overwriting the first edit.
 s.restrictedEditMu.Lock()
 defer s.restrictedEditMu.Unlock()
 pinned,err:=openProjectPinnedFile(root,path)
 if err!=nil{http.Error(w,"source file unavailable",http.StatusNotFound);return}
 defer pinned.close()
 current,info,err:=pinned.readCurrent(projectEditableLimit)
 if err!=nil||info==nil||!info.Mode().IsRegular(){
  http.Error(w,"source file unavailable or too large",http.StatusRequestEntityTooLarge);return
 }
 if info.Mode().Perm()&0222==0{
  http.Error(w,"source file is read-only",http.StatusForbidden);return
 }
 if !utf8.Valid(current)||!projectTextBytesValid(current){
  http.Error(w,"source file is not editable UTF-8 text",http.StatusUnsupportedMediaType);return
 }
 hash:=sha256.Sum256(current)
 if !strings.EqualFold(hex.EncodeToString(hash[:]),req.ExpectedSHA256){
  http.Error(w,"source file changed; refresh the permitted window",http.StatusConflict);return
 }
 parts:=restrictededit.SplitLines(current)
 if req.StartLine>len(parts)||len(req.Lines)>len(parts)-req.StartLine+1{
  http.Error(w,"requested edit range is outside the file",http.StatusRequestedRangeNotSatisfiable);return
 }
 old:=make([]string,0,len(req.Lines))
 for _,line:=range parts[req.StartLine-1:req.StartLine-1+len(req.Lines)]{
  old=append(old,string(line))
 }
 viewed,err:=store.HasViewed(user,path,req.StartLine,old)
 if err!=nil{http.Error(w,"source authorization unavailable",http.StatusServiceUnavailable);return}
 if !viewed{
  http.Error(w,"edit denied: view this exact source window first",http.StatusForbidden);return
 }
 next,err:=restrictededit.ReplaceLines(current,req.StartLine,req.Lines,s.Config.Restricted.MaxVisibleLines)
 if err!=nil{
  http.Error(w,"invalid replacement lines; line count and endings cannot change",http.StatusBadRequest);return
 }
 if int64(len(next))>projectEditableLimit{
  http.Error(w,"edited source exceeds file size limit",http.StatusRequestEntityTooLarge);return
 }
 if bytes.Equal(next,current){
  writeJSON(w,http.StatusOK,map[string]any{
   "saved":false,"unchanged":true,"sha256":hex.EncodeToString(hash[:]),
   "start_line":req.StartLine,"line_count":len(req.Lines),
  })
  return
 }
 if err:=pinned.writeTemp(next,info);err!=nil{
  http.Error(w,"cannot prepare protected file save",http.StatusInternalServerError);return
 }
 latest,_,err:=pinned.readCurrent(projectEditableLimit)
 if err!=nil||!bytes.Equal(latest,current){
  http.Error(w,"source changed before save",http.StatusConflict);return
 }
 if err:=pinned.commitTemp();err!=nil{
  http.Error(w,"cannot finalize protected file save",http.StatusInternalServerError);return
 }
 newHash:=sha256.Sum256(next)
 s.auditSharedSuccess(r,"file.write","file",rel,map[string]any{"protected":true,"lines":len(req.Lines)})
 writeJSON(w,http.StatusOK,map[string]any{
  "saved":true,"sha256":hex.EncodeToString(newHash[:]),
  "start_line":req.StartLine,"line_count":len(req.Lines),
  "refresh_required":true,
 })
}
