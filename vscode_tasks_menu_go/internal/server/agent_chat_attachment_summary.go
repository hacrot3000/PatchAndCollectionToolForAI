package server

import (
 "bytes"
 "encoding/base64"
 "encoding/json"
 "errors"
 "io"
 "net/http"
 "os"
 "path/filepath"
 "strings"
 "unicode/utf8"

 "bletonfc/vscode_tasks_menu/internal/agentchat/contextcompact"
)

const (
 agentAttachmentSourceLimit=16<<20
 agentAttachmentSourceBodyLimit=40<<20
)

// GET resolves a project-relative path using the normal guarded project
// resolver. POST accepts user-supplied local/editor bytes and NEVER opens
// the claimed path. Neither route executes code or egresses to a provider.
func (s *Server) agentAttachmentSummaryAPI(w http.ResponseWriter,r *http.Request){
 if s.restrictedPolicyForRequest(r)!=nil{
  http.Error(w,"attachment summary unavailable under restricted source policy",http.StatusForbidden)
  return
 }
 w.Header().Set("Cache-Control","no-store")
 w.Header().Set("X-Content-Type-Options","nosniff")
 switch r.Method{
 case http.MethodGet:
  rel,err:=cleanProjectRelativePath(r.URL.Query().Get("path"),false)
  if err!=nil{http.Error(w,"invalid project-relative path",http.StatusBadRequest);return}
  path,err:=s.resolveProjectPath(rel,false,false)
  if err!=nil{http.Error(w,"project file unavailable",http.StatusNotFound);return}
  f,err:=os.Open(path)
  if err!=nil{http.Error(w,"project file unavailable",http.StatusNotFound);return}
  defer f.Close()
  info,err:=f.Stat()
  if err!=nil||!info.Mode().IsRegular(){
   http.Error(w,"project file unavailable",http.StatusNotFound);return
  }
  result,err:=contextcompact.CompactAttachment(filepath.Base(rel),rel,"",info.Size(),f)
  if err!=nil{http.Error(w,"cannot summarize project file",http.StatusUnsupportedMediaType);return}
  writeJSON(w,http.StatusOK,result)
 case http.MethodPost:
  body:=http.MaxBytesReader(w,r.Body,agentAttachmentSourceBodyLimit)
  dec:=json.NewDecoder(body);dec.DisallowUnknownFields()
  var input struct{
   Name string `json:"name"`
   Path string `json:"path,omitempty"`
   MIME string `json:"mime,omitempty"`
   Size int64 `json:"size"`
   Source string `json:"source,omitempty"`
   SampleBase64 string `json:"sample_base64,omitempty"`
  }
  if err:=dec.Decode(&input);err!=nil{
   http.Error(w,"invalid or oversized attachment snapshot",http.StatusBadRequest);return
  }
  var extra any
  if err:=dec.Decode(&extra);!errors.Is(err,io.EOF){
   http.Error(w,"expected one attachment snapshot",http.StatusBadRequest);return
  }
  input.Name=strings.TrimSpace(input.Name)
  if input.Name==""||len(input.Name)>120||strings.ContainsAny(input.Name,"\x00\r\n")||
    input.Size<0||len(input.Source)>agentAttachmentSourceLimit||
    len(input.SampleBase64)>((int(contextcompact.AttachmentSampleLimit)+2)/3*4+8)||
    (input.Source!=""&&input.SampleBase64!=""){
   http.Error(w,"invalid attachment snapshot metadata or size",http.StatusBadRequest);return
  }
  var content []byte
  if input.SampleBase64!=""{
   decoded,err:=base64.StdEncoding.DecodeString(input.SampleBase64)
   if err!=nil||int64(len(decoded))>contextcompact.AttachmentSampleLimit{
    http.Error(w,"invalid image metadata sample",http.StatusBadRequest);return
   }
   content=decoded
  }else{
   if !utf8.ValidString(input.Source)||strings.ContainsRune(input.Source,0){
    http.Error(w,"source snapshot must be valid UTF-8",http.StatusUnsupportedMediaType);return
   }
   content=[]byte(input.Source)
  }
  if int64(len(content))>input.Size {
   http.Error(w,"source snapshot exceeds declared file size",http.StatusBadRequest);return
  }
  result,err:=contextcompact.CompactAttachment(input.Name,input.Path,input.MIME,input.Size,bytes.NewReader(content))
  if err!=nil{http.Error(w,"cannot summarize attachment snapshot",http.StatusBadRequest);return}
  writeJSON(w,http.StatusOK,result)
 default:
  http.Error(w,"method not allowed",http.StatusMethodNotAllowed)
 }
}
