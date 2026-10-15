package server

import (
 "encoding/json"
 "errors"
 "io"
 "net/http"

 "bletonfc/vscode_tasks_menu/internal/restrictedcommand"
)

// restrictedCommandAPI runs fixed, reviewed metadata-only commands in the
// server-owned workspace. It never creates PTYs or executes a shell.
func (s *Server) restrictedCommandAPI(w http.ResponseWriter,r *http.Request){
 if s.Config.Restricted==nil{http.NotFound(w,r);return}
 if r.Method!=http.MethodPost{
  w.Header().Set("Allow","POST")
  http.Error(w,"method not allowed",http.StatusMethodNotAllowed);return
 }
 w.Header().Set("Cache-Control","no-store")
 r.Body=http.MaxBytesReader(w,r.Body,4096)
 defer r.Body.Close()
 var req struct{Command string `json:"command"`}
 dec:=json.NewDecoder(r.Body)
 dec.DisallowUnknownFields()
 if err:=dec.Decode(&req);err!=nil {
  http.Error(w,"invalid restricted command JSON",http.StatusBadRequest);return
 }
 if err:=dec.Decode(new(any));err!=io.EOF {
  http.Error(w,"exactly one JSON object is required",http.StatusBadRequest);return
 }
 plan,err:=restrictedcommand.Parse(req.Command,s.Config.Restricted.AllowedCommands)
 if errors.Is(err,restrictedcommand.ErrDenied){
  http.Error(w,"command is blocked by Restricted Developer security policy",http.StatusForbidden);return
 }
 if err!=nil{
  http.Error(w,"invalid command",http.StatusBadRequest);return
 }
 root,err:=s.projectRoot()
 if err!=nil{
  http.Error(w,"workspace unavailable",http.StatusServiceUnavailable);return
 }
 result,err:=restrictedcommand.Run(r.Context(),root,plan)
 if err!=nil{
  http.Error(w,"restricted command execution unavailable",http.StatusServiceUnavailable);return
 }
 writeJSON(w,http.StatusOK,result)
}
