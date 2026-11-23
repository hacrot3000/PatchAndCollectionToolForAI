package server

import (
 "archive/zip"
 "bytes"
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "time"

 "bletonfc/vscode_tasks_menu/internal/patchtool"
 "bletonfc/vscode_tasks_menu/internal/session"
)

// Native COLLECT is the ONLY production Collection executor for Agent Chat.
// Requests go through the same /api/patch/paste queue, native Patch Tool
// session, prompt selection, Python collector, ZIP/TXT artifacts and History
// as manually submitted Collection requests. There is no second source reader.
const agentNativeCollectionTextBudget = 96 << 10

// The native COLLECT schema, not the Agent, is authoritative over actions,
// limits, search coverage and large (including decompile/IDA) source files.
// Typed function calls can omit project identity: TaskDeck supplies its exact
// authoritative identity. Any supplied identity must already match. Plain
// text requests must explicitly include BOTH values.
func agentNativeCollectionRequest(raw string, identity patchtool.ProjectIdentity, requireIdentity bool) ([]byte,error) {
 if len(raw)==0||len(raw)>256<<10 {return nil,errors.New("native COLLECT request is empty or exceeds 256 KiB")}
 dec:=json.NewDecoder(strings.NewReader(raw))
 var request map[string]json.RawMessage
 if err:=dec.Decode(&request);err!=nil{return nil,fmt.Errorf("invalid native COLLECT JSON: %w",err)}
 var trailing any
 if err:=dec.Decode(&trailing);!errors.Is(err,io.EOF){return nil,errors.New("native COLLECT must contain exactly one JSON object")}
 if len(request)==0 {return nil,errors.New("native COLLECT request must be a JSON object")}
 for key:=range request {
  switch key {case "id","title","project","ai_context","actions","limits":
  default:return nil,fmt.Errorf("unexpected native COLLECT field %q",key)}
 }
 if len(request["project"])!=0&&string(request["project"])!="null"{
  if err:=ensurePastedIdentity(request["project"],identity);err!=nil{return nil,err}
 }else if requireIdentity {
  return nil,errors.New("text COLLECT must include exact project.key and project.id")
 }
 var actions []json.RawMessage
 if json.Unmarshal(request["actions"],&actions)!=nil||len(actions)==0 {
  return nil,errors.New("native COLLECT requires nonempty actions array")
 }
 bound,_:=json.Marshal(identity)
 request["project"]=bound
 return json.Marshal(request)
}

func agentNativeCollectionTextRequest(text string,identity patchtool.ProjectIdentity)([]byte,bool){
 // No commands embedded in prose and no Python execution. Native pasted
 // requests remain an exact standalone JSON object (possibly fenced).
 raw:=unwrapPasteFence(text)
 if !strings.HasPrefix(raw,"{")||!strings.HasSuffix(raw,"}"){return nil,false}
 body,err:=agentNativeCollectionRequest(raw,identity,true)
 return body,err==nil
}

// Queue through the SAME validated native Paste text handler used by humans.
// On failure, do not invoke a fallback file reader or silently replay a run.
func (s *Server) agentQueueNativeCollection(ctx context.Context,body []byte)(patchUploadResult,error){
 data,err:=json.Marshal(patchPasteRequest{Kind:"collect",Name:"agent-collection",Text:string(body)})
 if err!=nil{return patchUploadResult{},err}
 req:=httptest.NewRequest(http.MethodPost,"/api/patch/paste",bytes.NewReader(data)).WithContext(ctx)
 req.Header.Set("Content-Type","application/json")
 w:=httptest.NewRecorder()
 s.patchPaste(w,req)
 if w.Code!=http.StatusCreated {
  return patchUploadResult{},fmt.Errorf("native COLLECT queue refused HTTP %d: %s",w.Code,scrubAIContext(w.Body.String(),500))
 }
 var receipt patchUploadResult
 if json.Unmarshal(w.Body.Bytes(),&receipt)!=nil||receipt.Name==""||receipt.QueuePath==""||len(receipt.SHA256)!=64{
  return patchUploadResult{},errors.New("native COLLECT queue did not provide a valid package receipt")
 }
 if err:=s.agentVerifyQueuedPackage(receipt);err!=nil{return patchUploadResult{},err}
 return receipt,nil
}

func agentNativeCollectionFinalItem(state session.ProtocolState, name string,index int)(session.ProtocolItemState,bool){
 for _,item:=range state.Items{
  if item.Name!=name||strings.ToUpper(item.Kind)!="COLLECT"||item.Index!=index||item.RunID==""{continue}
  switch strings.ToUpper(item.Status){
  case "PASS","FAIL","INCOMPLETE","PREFLIGHT_FAIL":
   return item,true
  }
 }
 return session.ProtocolItemState{},false
}

func agentNativeCollectionArtifacts(state session.ProtocolState,item session.ProtocolItemState)(string,string){
 var resultZip,resultText string
 for _,a:=range state.Artifacts {
  if a.ItemName!=item.Name||strings.ToUpper(a.ItemKind)!="COLLECT"||a.Index!=item.Index||
    (a.RunID!=""&&a.RunID!=item.RunID){continue}
  switch a.ArtifactKind{
  case "collect_result_zip":resultZip=a.Path
  case "collect_result_text":resultText=a.Path
  }
 }
 return resultZip,resultText
}

// Use only verified native COLLECT artifact paths. In particular, an attacker
// cannot ask the Agent to read arbitrary files by fabricating an artifact path.
func agentNativeCollectionArtifactPath(root,relative,ext string)(string,error){
 if filepath.IsAbs(relative)||strings.Contains(relative,"\\")||relative==""{return "",errors.New("invalid native COLLECT artifact path")}
 parts:=strings.Split(filepath.ToSlash(filepath.Clean(relative)),"/")
 if len(parts)!=3||parts[0]!="artifacts"||parts[1]!="patch_tool_code_collections"||
   !strings.HasPrefix(parts[2],"CODE_COLLECTION_RESULT_")||filepath.Ext(parts[2])!=ext{
  return "",errors.New("artifact is outside native COLLECT output directory")
 }
 base,err:=filepath.Abs(root);if err!=nil{return "",err}
 directory:=filepath.Join(base,"artifacts","patch_tool_code_collections")
 for _,check:=range []string{filepath.Join(base,"artifacts"),directory,filepath.Join(directory,parts[2])}{
  st,e:=os.Lstat(check);if e!=nil{return "",e}
  if st.Mode()&os.ModeSymlink!=0{return "",errors.New("native COLLECT artifact symlink refused")}
 }
 path:=filepath.Join(directory,parts[2])
 st,err:=os.Lstat(path)
 if err!=nil||!st.Mode().IsRegular(){return "",errors.New("native COLLECT artifact not a regular file")}
 return path,nil
}

func agentNativeCollectionManifest(root,relative string,identity patchtool.ProjectIdentity)(string,error){
 path,err:=agentNativeCollectionArtifactPath(root,relative,".zip")
 if err!=nil{return "",err}
 z,err:=zip.OpenReader(path)
 if err!=nil{return "",fmt.Errorf("native COLLECT ZIP unavailable: %w",err)}
 defer z.Close()
 for _,entry:=range z.File {
  if entry.Name!="COLLECTION_MANIFEST.json" {continue}
  if entry.UncompressedSize64>1<<20{return "",errors.New("native COLLECT manifest too large")}
  r,e:=entry.Open();if e!=nil{return "",e}
  raw,e:=io.ReadAll(io.LimitReader(r,(1<<20)+1));r.Close()
  if e!=nil||len(raw)>1<<20{return "",errors.New("native COLLECT manifest cannot be read")}
  var manifest struct{
   Project patchtool.ProjectIdentity `json:"project_identity"`
   Status string `json:"collection_status"`
  }
  if json.Unmarshal(raw,&manifest)!=nil||manifest.Project!=identity{
   return "",errors.New("native COLLECT manifest project identity is invalid or mismatched")
  }
  if manifest.Status!="PASS"&&manifest.Status!="INCOMPLETE"{
   return "",errors.New("native COLLECT manifest status is not PASS/INCOMPLETE")
  }
  return manifest.Status,nil
 }
 return "",errors.New("native COLLECT ZIP is missing COLLECTION_MANIFEST.json")
}

func agentNativeCollectionTextExcerpt(root,relative string)(string,bool,error){
 path,err:=agentNativeCollectionArtifactPath(root,relative,".txt");if err!=nil{return "",false,err}
 file,err:=os.Open(path);if err!=nil{return "",false,err};defer file.Close()
 st,err:=file.Stat();if err!=nil{return "",false,err}
 max:=int64(agentNativeCollectionTextBudget)
 if st.Size()<=max {
  data,e:=io.ReadAll(io.LimitReader(file,max+1))
  if e!=nil{return "",false,e}
  return scrubAIContext(strings.ToValidUTF8(string(data),""),agentNativeCollectionTextBudget),false,nil
 }
 // Preserve the manual TXT's identity/manifest opening AND final diagnostic
 // sections without ever buffering the complete (possibly huge) companion.
 prefix:=make([]byte,max/2)
 n,e:=io.ReadFull(file,prefix)
 if e!=nil&&e!=io.EOF&&e!=io.ErrUnexpectedEOF{return "",false,e}
 suffix:=make([]byte,max/2)
 k,e:=file.ReadAt(suffix,st.Size()-int64(len(suffix)))
 if e!=nil&&e!=io.EOF{return "",false,e}
 combined:=strings.ToValidUTF8(string(prefix[:n]),"")+"\n[TRUNCATED: native COLLECT TXT is larger than model budget; full ZIP/TXT preserved in Patch Tool History]\n"+strings.ToValidUTF8(string(suffix[:k]),"")
 return scrubAIContext(combined,agentNativeCollectionTextBudget+256),true,nil
}

func agentNativeCollectionObservation(root string,identity patchtool.ProjectIdentity, nativeSession string,
 item session.ProtocolItemState, zipPath,txtPath string) string {
 outcome:=map[string]any{
  "source":"native_patch_tool_collect","project":identity,
  "native_session_id":nativeSession,"run_id":item.RunID,"request_name":item.Name,
  "native_status":strings.ToUpper(item.Status),"status":"FAILED",
  "failure_reason":scrubAIContext(item.FailureReason,500),
  "diagnosis_kind":item.DiagnosisKind,
  "result_zip":zipPath,"result_text":txtPath,
  "evidence_verified":false,"partial_evidence_available":false,
 }
 if item.RC!=nil{outcome["rc"]=*item.RC}
 if zipPath!="" {
  status,err:=agentNativeCollectionManifest(root,zipPath,identity)
  if err==nil {
   // rc=3 may be presented as FAIL by an outer wrapper despite a genuine
   // INCOMPLETE native artifact. Never discard the verified partial result.
   outcome["status"]=status
   outcome["evidence_verified"]=true
   outcome["partial_evidence_available"]=status=="INCOMPLETE"
   if status=="PASS"&&strings.ToUpper(item.Status)!="PASS" {outcome["status"]="INCOMPLETE";outcome["partial_evidence_available"]=true}
  }else{outcome["artifact_error"]=scrubAIContext(err.Error(),240)}
 }
 if txtPath!="" {
  excerpt,truncated,err:=agentNativeCollectionTextExcerpt(root,txtPath)
  if err==nil{
   outcome["text_excerpt"]=excerpt
   outcome["text_excerpt_truncated"]=truncated
  }else{outcome["text_error"]=scrubAIContext(err.Error(),240)}
 }
 if outcome["status"]=="INCOMPLETE" {
  outcome["continuation"]="Usable partial native COLLECT evidence is available; do not infer absence from missing data. Use a narrower native COLLECT action (search/file/range/head/tail/symbol/decompile) based on the original result. Do not rerun an identical broad request."
 }else if outcome["status"]=="FAILED"{
  outcome["continuation"]="Native COLLECT did not provide a verified result ZIP. Inspect the existing native session/History; do not claim source inspection succeeded."
 }
 body,_:=json.Marshal(outcome)
 return string(body)
}

// One native run per one model tool call, with exact queue-name/index/run
// binding. No independent Go source reader, no re-execution on failure.
func (s *Server) agentRunNativeCollection(ctx context.Context,owner string,request []byte,identity patchtool.ProjectIdentity)(string,error){
 if s.Config.SharedServerEnabled||owner!="local" {
  return "",errors.New("native Agent COLLECT requires an authorized local workspace; shared-server execution is not enabled")
 }
 if s.Sessions==nil{return "",errors.New("native Patch Tool session service unavailable")}
 if ctx.Err()!=nil{return "",ctx.Err()}
 receipt,err:=s.agentQueueNativeCollection(ctx,request)
 if err!=nil{return "",err}
 req:=httptest.NewRequest(http.MethodPost,"/api/sessions",nil).WithContext(ctx)
 nativeID,err:=s.agentStartNativePatch(req)
 if err!=nil{return "",fmt.Errorf("COLLECT queued as %s but native session failed to start: %w",receipt.QueuePath,err)}
 _,index,err:=s.agentSelectNativeQueue(ctx,req,nativeID,receipt.Name,"COLLECT")
 if err!=nil{return "",fmt.Errorf("native COLLECT session %s could not select %s; inspect existing queue, no automatic rerun: %w",nativeID,receipt.Name,err)}
 provider,ok:=s.Sessions.(session.ProtocolStateProvider)
 if !ok{return "",errors.New("native COLLECT protocol state is unavailable")}
 ticker:=time.NewTicker(300*time.Millisecond)
 defer ticker.Stop()
 var final session.ProtocolItemState
 var found bool
 for{
  if ctx.Err()!=nil{
   // Stop affects the exact read-only native Collection session only.
   _=s.Sessions.Stop(nativeID)
   return "",ctx.Err()
  }
  state,e:=provider.ProtocolState(nativeID)
  if e==nil{
   final,found=agentNativeCollectionFinalItem(state,receipt.Name,index)
   if found {
    z,t:=agentNativeCollectionArtifacts(state,final)
    if z!=""&&t!="" {
     return agentNativeCollectionObservation(s.Workspace,identity,nativeID,final,z,t),nil
    }
    // A failed preflight has no ZIP; a native completion can publish artifact
    // events just AFTER item_finished, so allow the protocol to settle.
    if strings.EqualFold(final.Status,"PREFLIGHT_FAIL") {
     return agentNativeCollectionObservation(s.Workspace,identity,nativeID,final,z,t),nil
    }
    if meta,ok:=s.Sessions.Metadata(nativeID);ok&&!strings.EqualFold(meta.Status,"running"){
     return agentNativeCollectionObservation(s.Workspace,identity,nativeID,final,z,t),nil
    }
   }
  }
  select{case <-ctx.Done():case <-ticker.C:}
 }
}
