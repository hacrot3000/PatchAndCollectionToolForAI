package server

import (
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net/http"
 "os"
 "path/filepath"
 "runtime"
 "strings"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
 "bletonfc/vscode_tasks_menu/internal/patchtool"
)

const nimGatewayMaxBytes = 1 << 20

type patchNimCandidate struct {
 Script string `json:"script"`
 SHA256 string `json:"sha256"`
}

type patchNimGatewayView struct {
 ProjectRoot string `json:"project_root"`
 ProjectID string `json:"project_id"`
 Candidates []patchNimCandidate `json:"candidates"`
 Registered bool `json:"registered"`
 RegisteredScript string `json:"registered_script,omitempty"`
 RegistrationPresent bool `json:"registration_present"`
 Prompt string `json:"prompt,omitempty"`
 Status string `json:"status"`
}

func patchNimRegistrationPath()(string,error){
 home,err:=os.UserHomeDir()
 if err!=nil{return "",err}
 return filepath.Join(home,".config","taskdeck","nim-gateway.json"),nil
}

// Only an existing, regular, project-relative gateway can be offered to the
// owner. Discovery and GET never authorize, modify, or execute it.
func inspectPatchNimCandidate(root,rel string)(patchNimCandidate,error){
 var candidate patchNimCandidate
 if !agentNimPath.MatchString(rel)||!agentCommandOnlyRelativeScript(rel)||
  filepath.ToSlash(filepath.Clean(rel))!=rel||filepath.Base(rel)!="nim-agent.sh"{
  return candidate,fmt.Errorf("enter a project-relative path ending in nim-agent.sh")
 }
 script:=filepath.Join(root,filepath.FromSlash(rel))
 st,err:=os.Lstat(script)
 if err!=nil{return candidate,err}
 resolved,err:=filepath.EvalSymlinks(script)
 if err!=nil||resolved!=script||!st.Mode().IsRegular()||st.Size()==0||st.Size()>nimGatewayMaxBytes{
  return candidate,fmt.Errorf("gateway must be a regular non-symlink project file, at most 1 MiB")
 }
 data,err:=os.ReadFile(script)
 if err!=nil{return candidate,err}
 return patchNimCandidate{Script:rel,SHA256:agentchat.BodyDigest(data)},nil
}

func writePatchNimRegistration(g agentNimGatewayRegistration)error{
 cfg,err:=patchNimRegistrationPath()
 if err!=nil{return err}
 if err=os.MkdirAll(filepath.Dir(cfg),0700);err!=nil{return err}
 data,err:=json.MarshalIndent(g,"","  ")
 if err!=nil{return err}
 f,err:=os.CreateTemp(filepath.Dir(cfg),".nim-gateway-")
 if err!=nil{return err}
 defer os.Remove(f.Name())
 defer f.Close()
 if _,err=f.Write(append(data,'\n'));err!=nil{return err}
 if err=f.Sync();err!=nil{return err}
 if err=f.Close();err!=nil{return err}
 return os.Rename(f.Name(),cfg)
}

func(s *Server) patchNimGatewayView(root,extra string)patchNimGatewayView{
 v:=patchNimGatewayView{ProjectRoot:root,Candidates:[]patchNimCandidate{},Status:"Not registered. Select a reviewed gateway and register it."}
 identity,err:=patchtool.ReadProjectIdentity(root)
 if err==nil{v.ProjectID=identity.ID}
 cfg,err:=patchNimRegistrationPath()
 if err==nil{
  if _,e:=os.Lstat(cfg);e==nil{v.RegistrationPresent=true}
 }
 paths:=[]string{"nim-agent.sh","nim-gateway-v4/nim-agent.sh","tools/nim-agent.sh"}
 if g,err:=readAgentNimGateway(root,v.ProjectID);err==nil{
  v.Registered=true
  v.RegisteredScript=g.Script
  v.Status="Registered (script SHA-256 verified). Container/test execution has not been checked."
  paths=append([]string{g.Script},paths...)
  v.Prompt=s.patchNimGatewayPrompt(identity,g)
 }
 if extra!=""{paths=append([]string{extra},paths...)}
 seen:=map[string]bool{}
 for _,rel:=range paths{
  if seen[rel]{continue};seen[rel]=true
  if c,e:=inspectPatchNimCandidate(root,rel);e==nil{v.Candidates=append(v.Candidates,c)}
 }
 if !v.Registered&&v.RegistrationPresent{v.Status="Existing registration is stale or belongs to another project. Review the selected script before replacing it."}
 if len(v.Candidates)==0&&!v.Registered{v.Status="No gateway found in the standard locations. Select an existing relative path or install a reviewed nim-agent.sh file."}
 return v
}

func(s *Server) patchNimGatewayPrompt(identity patchtool.ProjectIdentity,g agentNimGatewayRegistration)string{
 manifest:=map[string]any{
  "schema_version":1,"project":identity,
  "patch":map[string]string{"id":"nim-test-example"},
  "preflight":map[string]any{"files":[]any{map[string]any{"path":g.Script,"exists":true,"sha256":g.SHA256}}},
  "post_patch":map[string]any{"safety_profile":"legacy_strict","run_when_no_changes":true,
   "no_change_reason":"Run the requested existing Nim test without editing project source.",
   "commands":[]any{map[string]any{"name":"nim-test","argv":[]string{"bash",g.Script,"nim-test","actual/existing/test.nim","server"},"cwd":".","timeout_seconds":1800}}},
 }
 body,_:=json.Marshal(manifest)
 args,_:=json.MarshalIndent(map[string]string{"name":"Run Nim test","patch_python":"","patch_manifest":string(body)},"","  ")
 collect:=map[string]any{"id":"nim-gateway-doctor","project":identity,"actions":[]any{map[string]any{"type":"command","transport":"local","argv":[]string{"nim-gateway","nim-doctor"},"max_output_bytes":65536}}}
 collectJSON,_:=json.MarshalIndent(collect,"","  ")
 return "Nim Gateway đã được chủ project đăng ký chính thức trong TaskDeck. Hãy tiếp tục task hiện tại, tự chạy build/test và đọc receipt/log qua native tools thay vì yêu cầu tôi chạy tay.\n\n"+
 s.agentNimGatewayInstructions(identity.ID)+"\n\nFirst use this native COLLECT request (use the advertised collection tool's actual argument schema):\n"+string(collectJSON)+
 "\n\nExample taskdeck_submit_patch arguments (replace test path, profile and unique patch ID with the actual task; keep exact project and script SHA):\n"+string(args)+
 "\n\nAfter native execution, collect nim-build-result and nim-build-log with the exact gateway RUN_ID. Never infer PASS from submission. Continue the original task using the verified result."
}

// Local browser owner only. No model tool advertises this endpoint. Existing
// same-origin, browser lease and restricted-mode middleware still apply.
func(s *Server) patchNimGateway(w http.ResponseWriter,r *http.Request){
 w.Header().Set("Cache-Control","no-store")
 if s.Config.SharedServerEnabled||runtime.GOOS=="windows"{
  http.Error(w,"Nim Gateway setup is available only in local Unix TaskDeck",http.StatusForbidden);return
 }
 root,err:=filepath.EvalSymlinks(s.Workspace)
 if err!=nil||!filepath.IsAbs(root){http.Error(w,"workspace unavailable",409);return}
 switch r.Method{
 case http.MethodGet:
  extra:=strings.TrimSpace(r.URL.Query().Get("script"))
  if extra!=""{if _,err:=inspectPatchNimCandidate(root,extra);err!=nil{http.Error(w,"gateway path unavailable: "+err.Error(),400);return}}
  writeJSON(w,200,s.patchNimGatewayView(root,extra));return
 case http.MethodPost:
 default:
  http.Error(w,"method not allowed",405);return
 }
 if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type"))),"application/json"){http.Error(w,"JSON Content-Type required",415);return}
 defer r.Body.Close()
 var req struct{
  Action string `json:"action"`
  ProjectRoot string `json:"project_root"`
  Script string `json:"script"`
  SHA256 string `json:"sha256"`
  Content string `json:"content"`
  Confirm bool `json:"confirm"`
 }
 dec:=json.NewDecoder(http.MaxBytesReader(w,r.Body,7*nimGatewayMaxBytes))
 dec.DisallowUnknownFields()
 var trailing any
 if dec.Decode(&req)!=nil||dec.Decode(&trailing)!=io.EOF||!req.Confirm||req.ProjectRoot!=root{
  http.Error(w,"explicit owner confirmation for this workspace required",400);return
 }
 switch req.Action{
 case "install":
  // Install only the owner-selected file at the fixed default path. Never
  // overwrite source, execute it, or implicitly authorize it.
  if req.Script!="nim-agent.sh"||req.SHA256!=""||len(req.Content)==0||len(req.Content)>nimGatewayMaxBytes||
   !strings.HasPrefix(req.Content,"#!")||strings.ContainsRune(req.Content,0){
   http.Error(w,"select a reviewed nim-agent.sh shell script (<=1 MiB)",400);return
  }
  target:=filepath.Join(root,"nim-agent.sh")
  f,e:=os.OpenFile(target,os.O_WRONLY|os.O_CREATE|os.O_EXCL,0700)
  if e!=nil{http.Error(w,"cannot install without overwriting existing file: "+e.Error(),409);return}
  _,e=f.WriteString(req.Content)
  if e==nil{e=f.Sync()}
  closeErr:=f.Close()
  if e==nil{e=closeErr}
  if e!=nil{http.Error(w,"gateway installation failed; inspect incomplete file before retry: "+e.Error(),500);return}
  writeJSON(w,200,s.patchNimGatewayView(root,"nim-agent.sh"));return
 case "register":
  if req.Content!=""||!agentNimSHA.MatchString(req.SHA256){http.Error(w,"reviewed script digest required",400);return}
  c,e:=inspectPatchNimCandidate(root,req.Script)
  if e!=nil||c.SHA256!=req.SHA256{http.Error(w,"gateway changed or unavailable; refresh and review before registering",409);return}
  identity,e:=patchtool.EnsureProjectIdentity(root)
  if e!=nil{http.Error(w,"project identity unavailable: "+e.Error(),409);return}
  if e=writePatchNimRegistration(agentNimGatewayRegistration{ProjectRoot:root,ProjectID:identity.ID,Script:c.Script,SHA256:c.SHA256});e!=nil{http.Error(w,"registration failed: "+e.Error(),500);return}
  v:=s.patchNimGatewayView(root,c.Script)
  if !v.Registered{http.Error(w,"registration written but verification failed; refresh status",409);return}
  writeJSON(w,200,v);return
 case "revoke":
  identity,e:=patchtool.ReadProjectIdentity(root)
  if e!=nil{http.Error(w,"project identity unavailable",409);return}
  cfg,e:=patchNimRegistrationPath()
  if e!=nil{http.Error(w,"registration unavailable",409);return}
  // Allow revocation even after a script changes, but never delete another
  // project's registration merely because this UI is open.
  st,e:=os.Lstat(cfg)
  if e!=nil||!st.Mode().IsRegular()||st.Size()>8192{http.Error(w,"registration unavailable",409);return}
  data,e:=os.ReadFile(cfg)
  var g agentNimGatewayRegistration
  if e!=nil||json.Unmarshal(data,&g)!=nil||g.ProjectRoot!=root||g.ProjectID!=identity.ID{http.Error(w,"registration belongs to another project or is invalid",409);return}
  if e=os.Remove(cfg);e!=nil&&!errors.Is(e,os.ErrNotExist){http.Error(w,"cannot revoke: "+e.Error(),500);return}
  writeJSON(w,200,s.patchNimGatewayView(root,""));return
 default:
  http.Error(w,"unknown Nim Gateway setup action",400)
 }
}
