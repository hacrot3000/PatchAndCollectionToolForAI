package server

import (
 "archive/zip"
 "bytes"
 "encoding/json"
 "fmt"
 "io"
 "path"
 "strings"
 "unicode/utf8"

 "bletonfc/vscode_tasks_menu/internal/patchtool"
)

// Narrow subset of the native schema: no payload, batch, git, remote SSH,
// recovery, validation, resources or other execution stages. Native preflight
// remains the authority for allowed argv, existing scripts, paths, symlinks,
// project policy, predecessor handling and execution.
type agentCommandOnlyManifest struct {
 SchemaVersion int `json:"schema_version"`
 Project patchtool.ProjectIdentity `json:"project"`
 Patch struct {
  ID string `json:"id"`
  Summary string `json:"summary,omitempty"`
 } `json:"patch"`
 PostPatch struct {
  Commands []struct {
   Name string `json:"name,omitempty"`
   Argv []string `json:"argv"`
   Cwd string `json:"cwd,omitempty"`
   Timeout *int `json:"timeout_seconds,omitempty"`
  } `json:"commands"`
  RunWhenNoChanges bool `json:"run_when_no_changes"`
  NoChangeReason string `json:"no_change_reason"`
  SafetyProfile string `json:"safety_profile,omitempty"`
 } `json:"post_patch"`
 Preflight *struct {
  Files []struct {
   Path string `json:"path"`
   Exists *bool `json:"exists,omitempty"`
   SHA256 string `json:"sha256,omitempty"`
   Anchors []string `json:"anchors,omitempty"`
  } `json:"files,omitempty"`
  RequireCleanWorktree *bool `json:"require_clean_worktree,omitempty"`
 } `json:"preflight,omitempty"`
}

func parseCommandOnlyManifest(raw string)(agentCommandOnlyManifest,error){
 var m agentCommandOnlyManifest
 fail:=func()(agentCommandOnlyManifest,error){return agentCommandOnlyManifest{},fmt.Errorf("invalid command-only manifest: use schema_version=1, exact project, patch.id, one legacy_strict post_patch command, run_when_no_changes=true and 20-500 character reason; optional preflight only")}
 if len(raw)==0||len(raw)>64<<10||!utf8.ValidString(raw)||strings.ContainsRune(raw,0){return fail()}
 dec:=json.NewDecoder(strings.NewReader(raw));dec.DisallowUnknownFields()
 if err:=dec.Decode(&m);err!=nil{return fail()}
 var extra any
 if err:=dec.Decode(&extra);err!=io.EOF{return fail()}
 p:=m.PostPatch
 reason:=utf8.RuneCountInString(strings.TrimSpace(p.NoChangeReason))
 if m.SchemaVersion!=1||strings.TrimSpace(m.Project.Key)==""||strings.TrimSpace(m.Project.ID)==""||
  strings.TrimSpace(m.Patch.ID)==""||len(m.Patch.ID)>128||!p.RunWhenNoChanges||reason<20||reason>500||
  (p.SafetyProfile!=""&&p.SafetyProfile!="legacy_strict")||len(p.Commands)!=1{return fail()}
 cmd:=p.Commands[0]
 if len(cmd.Argv)==0||len(cmd.Argv)>128||strings.TrimSpace(cmd.Argv[0])==""||
  (cmd.Timeout!=nil&&(*cmd.Timeout<1||*cmd.Timeout>1800)){return fail()}
 for _,arg:=range cmd.Argv{if arg==""||strings.ContainsRune(arg,0)||len(arg)>4096{return fail()}}
 // Native legacy_strict validates script operands against the project root.
 // Keep actual execution cwd identical to that root in this Agent lane so
 // a same-named script in a subdirectory cannot substitute for the reviewed
 // script. Native preflight also rechecks existence and project containment.
 if cmd.Cwd!=""&&cmd.Cwd!="."{return agentCommandOnlyManifest{},fmt.Errorf("Agent command-only manifest requires cwd=. (project root)")}
 if !agentCommandOnlyArgvShape(cmd.Argv){return agentCommandOnlyManifest{},fmt.Errorf("command-only argv requires an allowlisted bare executable or an existing project-relative script; no inline execution")}
 return m,nil
}

func agentCommandOnlyRelativeScript(p string)bool{
 if p==""||strings.HasPrefix(p,"/")||strings.HasPrefix(p,"-")||strings.HasPrefix(p,"~")||strings.ContainsAny(p,"\\\\:"){return false}
 for _,part:=range strings.Split(p,"/"){if part==".."{return false}}
 return path.Clean(p)!="."
}

// A conservative structural gate, not an alternative native validator.
// Native legacy_strict still validates all command flags and file operands.
func agentCommandOnlyArgvShape(argv []string)bool{
 if len(argv)==0{return false}
 exe:=argv[0]
 switch exe {
 case "ls","tree","pwd","find","grep","rg","cat","head","tail","wc","sed","nl":
  return true
 case "python","python3","bash","sh","node","pwsh","powershell","powershell.exe":
  if len(argv)<2||!agentCommandOnlyRelativeScript(argv[1]){return false}
  ext:=strings.ToLower(path.Ext(argv[1]))
  switch exe {
  case "python","python3":return ext==".py"
  case "bash","sh":return ext==".sh"
  case "node":return ext==".js"||ext==".mjs"||ext==".cjs"
  default:return ext==".ps1"
  }
 default:
  // Only direct project-local script paths, not an arbitrary PATH program.
  // Native execution requires the file to exist and be executable.
  return strings.Contains(exe,"/")&&agentCommandOnlyRelativeScript(exe)
 }
}

// Re-encode the typed subset; the native parser never sees hidden or
// unknown execution stages. No Python entrypoint is created.
func pastedCommandOnlyPackage(raw string,identity patchtool.ProjectIdentity)([]byte,error){
 m,err:=parseCommandOnlyManifest(raw)
 if err!=nil{return nil,err}
 if m.Project!=identity{return nil,fmt.Errorf("command-only manifest project identity mismatch")}
 m.PostPatch.SafetyProfile="legacy_strict"
 m.PostPatch.Commands[0].Cwd="."
 body,err:=json.MarshalIndent(m,"","  ")
 if err!=nil{return nil,err}
 var packed bytes.Buffer
 zw:=zip.NewWriter(&packed)
 member,err:=zw.Create("PATCH_TOOL_MANIFEST.json")
 if err!=nil{return nil,err}
 if _,err=member.Write(append(body,'\n'));err!=nil{return nil,err}
 if err=zw.Close();err!=nil{return nil,err}
 return packed.Bytes(),nil
}

func (args agentPatchArguments) pastePayload()(string,string){
 if strings.TrimSpace(args.PatchManifest)!=""{return "patch-manifest",args.PatchManifest}
 return "patch-python",args.PatchPython
}

func agentPatchPackage(args agentPatchArguments,identity patchtool.ProjectIdentity)([]byte,error){
 kind,body:=args.pastePayload()
 if kind=="patch-manifest"{return pastedCommandOnlyPackage(body,identity)}
 return pastedPythonPackage(body,identity,patchPasteSlug(args.Name))
}
