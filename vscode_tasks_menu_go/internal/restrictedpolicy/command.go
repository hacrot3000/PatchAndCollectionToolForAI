package restrictedpolicy

import (
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "sort"
 "strings"
)

// CommandRequest is a structured argv request, NOT a shell expression.
// This first audited gateway supports only non-executable metadata actions.
// Unsafe Git features remain unavailable even if "git" is in AllowedCommands.
type CommandRequest struct {
 Command string `json:"command"`
 Args []string `json:"args"`
}
type CommandResponse struct {
 Command string `json:"command"`
 Output string `json:"output"`
 Limited bool `json:"limited"`
}
var ErrCommandDenied=errors.New("restricted command denied")

func (p Policy) allows(name string) bool {
 for _,allowed:=range p.AllowedCommands {if name==allowed{return true}}
 return false
}
func validToken(token string) bool {
 if len(token)==0||len(token)>160{return false}
 for _,r:=range token{
  if r=='\n'||r=='\r'||r=='\x00'||r=='\x1b'||r<32||r==127{return false}
 }
 return true
}

func RunCommand(root string,p Policy,req CommandRequest) (CommandResponse,error) {
 if err:=p.Validate();err!=nil{return CommandResponse{},err}
 if !validToken(req.Command)||len(req.Args)>8 {
  return CommandResponse{},fmt.Errorf("%w: invalid command",ErrCommandDenied)
 }
 for _,arg:=range req.Args {if !validToken(arg){return CommandResponse{},fmt.Errorf("%w: invalid argument",ErrCommandDenied)}}
 if !p.allows(req.Command){return CommandResponse{},fmt.Errorf("%w: command group not allowed",ErrCommandDenied)}
 rootAbs,err:=filepath.Abs(root)
 if err!=nil{return CommandResponse{},err}
 info,err:=os.Stat(rootAbs)
 if err!=nil{return CommandResponse{},err}
 if !info.IsDir(){return CommandResponse{},errors.New("workspace is not a directory")}
 out:=CommandResponse{Command:req.Command}
 switch req.Command {
 case "pwd":
  if len(req.Args)!=0{return CommandResponse{},fmt.Errorf("%w: pwd takes no arguments",ErrCommandDenied)}
  out.Output="workspace: .\n"
 case "ls":
  if len(req.Args)!=0{return CommandResponse{},fmt.Errorf("%w: ls arguments are not enabled",ErrCommandDenied)}
  entries,err:=os.ReadDir(rootAbs)
  if err!=nil{return CommandResponse{},err}
  names:=make([]string,0,100)
  for _,entry:=range entries {
   name:=entry.Name()
   if strings.HasPrefix(name,".")||!validToken(name){continue}
   names=append(names,name)
   if len(names)>=100 {out.Limited=len(entries)>100;break}
  }
  sort.Strings(names)
  out.Output=strings.Join(names,"\n")+"\n"
 case "git":
  if len(req.Args)==2&&req.Args[0]=="branch"&&req.Args[1]=="--show-current"||
   len(req.Args)==3&&req.Args[0]=="rev-parse"&&req.Args[1]=="--abbrev-ref"&&req.Args[2]=="HEAD" {
   branch,err:=safeCurrentBranch(rootAbs)
   if err!=nil{return CommandResponse{},err}
   out.Output=branch+"\n"
  }else{
   return CommandResponse{},fmt.Errorf("%w: only git branch --show-current or git rev-parse --abbrev-ref HEAD are available in metadata-only gateway",ErrCommandDenied)
  }
 default:
  return CommandResponse{},fmt.Errorf("%w: not supported by audited gateway",ErrCommandDenied)
 }
 if len(out.Output)>16384 {out.Output=out.Output[:16384];out.Limited=true}
 return out,nil
}

func safeCurrentBranch(rootAbs string) (string,error) {
 dotGit:=filepath.Join(rootAbs,".git")
 info,err:=os.Lstat(dotGit)
 if err!=nil{return "",err}
 if !info.IsDir()||info.Mode()&os.ModeSymlink!=0 {
  return "",fmt.Errorf("%w: only local .git directories supported",ErrCommandDenied)
 }
 headPath:=filepath.Join(dotGit,"HEAD")
 info,err=os.Lstat(headPath)
 if err!=nil{return "",err}
 if !info.Mode().IsRegular()||info.Size()>512 {
  return "",fmt.Errorf("%w: unsafe HEAD",ErrCommandDenied)
 }
 b,err:=os.ReadFile(headPath)
 if err!=nil{return "",err}
 v:=strings.TrimSpace(string(b))
 const prefix="ref: refs/heads/"
 if !strings.HasPrefix(v,prefix){return "HEAD",nil}
 branch:=strings.TrimPrefix(v,prefix)
 if !validToken(branch)||strings.Contains(branch,"..")||strings.HasPrefix(branch,"/"){
  return "",fmt.Errorf("%w: invalid branch metadata",ErrCommandDenied)
 }
 return branch,nil
}
