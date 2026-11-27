package server

import (
 "encoding/json"
 "fmt"
 "os"
 "path/filepath"
 "regexp"
 "runtime"
 "strings"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

// Local owner registration is not a model-supplied executable or an AI Pack.
// The actual build remains an approval-controlled native command-only PATCH.
type agentNimGatewayRegistration struct {
 ProjectRoot string `json:"project_root"`
 ProjectID string `json:"project_id"`
 Script string `json:"script"`
 SHA256 string `json:"sha256"`
}
var agentNimDefine=regexp.MustCompile(`^-d:[A-Za-z_][A-Za-z0-9_]*$`)
var agentNimSHA=regexp.MustCompile(`^[0-9a-f]{64}$`)
var agentNimPath=regexp.MustCompile(`^[A-Za-z0-9_./-]+$`)

func readAgentNimGateway(workspace,projectID string)(agentNimGatewayRegistration,error){
 var g agentNimGatewayRegistration
 deny:=fmt.Errorf("Nim gateway is not registered for this exact local project or its script digest changed; owner must register/review it")
 if runtime.GOOS=="windows"||projectID==""{return g,deny}
 home,err:=os.UserHomeDir();if err!=nil{return g,deny}
 cfg:=filepath.Join(home,".config","taskdeck","nim-gateway.json")
 st,err:=os.Lstat(cfg);if err!=nil||!st.Mode().IsRegular()||st.Mode().Perm()&0077!=0||st.Size()>8192{return g,deny}
 raw,err:=os.ReadFile(cfg);if err!=nil||json.Unmarshal(raw,&g)!=nil{return g,deny}
 root,err:=filepath.EvalSymlinks(workspace);if err!=nil||!filepath.IsAbs(g.ProjectRoot)||root!=g.ProjectRoot||g.ProjectID!=projectID||
 !agentNimSHA.MatchString(g.SHA256)||!agentNimPath.MatchString(g.Script)||!agentCommandOnlyRelativeScript(g.Script)||filepath.ToSlash(filepath.Clean(g.Script))!=g.Script||
 filepath.Base(g.Script)!="nim-agent.sh"{return g,deny}
 script:=filepath.Join(root,filepath.FromSlash(g.Script))
 resolved,err:=filepath.EvalSymlinks(script);if err!=nil||resolved!=script{return g,deny}
 st,err=os.Lstat(script);if err!=nil||!st.Mode().IsRegular()||st.Size()>1<<20{return g,deny}
 data,err:=os.ReadFile(script);if err!=nil||agentchat.BodyDigest(data)!=g.SHA256{return g,deny}
 return g,nil
}

func agentNimGatewayArgv(argv []string)bool{
 if len(argv)<2{return false}
 // Recognize this particular gateway regardless of direct invocation or the
 // supported script interpreter. This is not a sandbox for other scripts.
 return filepath.Base(argv[0])=="nim-agent.sh"||
  ((argv[0]=="bash"||argv[0]=="sh")&&filepath.Base(argv[1])=="nim-agent.sh")
}

func validAgentNimBuildArgs(argv []string)bool{
 if len(argv)<3||argv[0]!="bash"{return false}
 a:=argv[2:]
 buildProfiles:="|server|server-probe|server-verbose|battle|pvp|arenaserver|arenaserver-fake-data|local|release|"
 fileProfiles:="|debug|release|server|server-probe|server-verbose|battle|pvp|arenaserver|arenaserver-fake-data|"
 member:=func(set,v string)bool{return v!=""&&!strings.Contains(v,"|")&&strings.Contains(set,"|"+v+"|")}
 switch a[0]{
 case "nim-build":return len(a)==2&&member(buildProfiles,a[1])
 case "nim-check","nim-compile","nim-test","nim-test-suite":
  if len(a)<2||len(a)>100||!agentNimPath.MatchString(a[1])||!agentCommandOnlyRelativeScript(a[1])||filepath.Ext(a[1])!=".nim"{return false}
  rest:=a[2:]
  if len(rest)>0&&!strings.HasPrefix(rest[0],"-"){if !member(fileProfiles,rest[0]){return false};rest=rest[1:]}
  for _,flag:=range rest{if !agentNimDefine.MatchString(flag){return false}}
  return true
 }
 return false
}

func(s *Server) validateAgentNimGatewayProposal(args agentPatchArguments,projectID string)error{
 if strings.TrimSpace(args.PatchManifest)==""{return nil}
 m,err:=parseCommandOnlyManifest(args.PatchManifest);if err!=nil{return err}
 argv:=m.PostPatch.Commands[0].Argv
 if !agentNimGatewayArgv(argv){return nil}
 if s.Config.SharedServerEnabled{return fmt.Errorf("Nim gateway is local-only")}
 g,err:=readAgentNimGateway(s.Workspace,projectID);if err!=nil{return err}
 if len(argv)<3||argv[0]!="bash"||filepath.ToSlash(filepath.Clean(argv[1]))!=g.Script||!validAgentNimBuildArgs(argv){
  return fmt.Errorf("Nim gateway build action denied; use only registered nim-build, nim-check, nim-compile, nim-test or nim-test-suite; use COLLECT nim-gateway for receipt/log reads")
 }
 pinned:=false
 if m.Preflight!=nil{for _,f:=range m.Preflight.Files{if f.Path==g.Script&&f.SHA256==g.SHA256&&(f.Exists==nil||*f.Exists){pinned=true}}}
 if !pinned{return fmt.Errorf("Nim gateway requires preflight.files pin of the registered script path and SHA-256")}
 return nil
}

func(s *Server) agentNimGatewayInstructions(projectID string)string{
 if s.Config.SharedServerEnabled{return ""}
 g,err:=readAgentNimGateway(s.Workspace,projectID);if err!=nil{return ""}
 return fmt.Sprintf(" OWNER-REGISTERED NIM GATEWAY: approved gateway path %q, SHA-256 %s. This multi-action Docker-backed gateway is explicitly authorized through the native integration, not a generic shell exception. For build/test use command-only PATCH with argv [\"bash\",%q,\"nim-test\",\"actual/existing/test.nim\",\"server-probe\"] and preflight.files [{\"path\":%q,\"sha256\":%q,\"exists\":true}]. Keep the ordinary manifest and approval requirements. Allowed builds: nim-build server/server-probe/server-verbose/battle/pvp/arenaserver/arenaserver-fake-data/local/release; nim-check/nim-compile/nim-test/nim-test-suite FILE [PROFILE] [-d:NAME ...]. Do not use release-copy, arbitrary Docker, sudo, source-write or cleanup gateway actions. For external receipt/log use native COLLECT command action {\"type\":\"command\",\"transport\":\"local\",\"argv\":[\"nim-gateway\",\"nim-build-result\",\"EXACT_32_HEX_RUN_ID\"]}; also nim-build-status, nim-build-log and nim-artifacts with exact run ID. Do not read the external state directory directly or use latest. Use nim-gateway nim-profiles or nim-profile-info PROFILE to inspect flags; nim-doctor and nim-source-compare for bind mount evidence. After build, obtain exact RUN_ID from native output and fetch receipt. PASS requires actual test evidence, matching requested commands/flags, gateway SUCCESS, exit_code=0 and source_unchanged=true. Source hash is scoped, not a whole-workspace snapshot. Missing/truncated receipt or interrupted container execution is unverified; do not rerun uncertain builds. Do not request manual runs merely because this gateway invokes Docker.",g.Script,g.SHA256,g.Script,g.Script,g.SHA256)
}
