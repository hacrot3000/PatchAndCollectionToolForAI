// Package restrictedcommand implements a deliberately small non-shell gateway.
// Expanding Git support requires reviewing each new verb for file disclosure,
// hooks, network and arbitrary process execution; an allowed git group is
// never interpreted as an unconditional allowance for all Git subcommands.
package restrictedcommand

import (
 "context"
 "errors"
 "fmt"
 "io"
 "os"
 "os/exec"
 "path/filepath"
 "runtime"
 "sort"
 "strings"
 "time"
 "unicode/utf8"
)

const MaxOutputBytes=8192
const MaxInputBytes=192
const Timeout=8*time.Second

var ErrDenied=errors.New("command not permitted by restricted security policy")

type Plan struct{
 Original string `json:"command"`
 Program string `json:"program"`
 Args []string `json:"args"`
 Description string `json:"description"`
}
type Result struct{
 Command string `json:"command"`
 Output string `json:"output"`
 ExitCode int `json:"exit_code"`
 Truncated bool `json:"truncated"`
}

func allowed(groups []string,group string)bool{
 for _,v:=range groups{if v==group{return true}}
 return false
}

// Parse recognizes exact, reviewed commands. No flags, aliases, environment
// overrides, path arguments, shell operators, substitutions or pipelines.
func Parse(line string,groups []string)(Plan,error){
 if line==""||len(line)>MaxInputBytes||!utf8.ValidString(line)||
  strings.ContainsAny(line,"\x00\r\n\t\\\"'\x60$;&|<>(){}[]*?!#~"){
  return Plan{},ErrDenied
 }
 tokens:=strings.Fields(line)
 if len(tokens)==0||len(tokens)>4||!allowed(groups,tokens[0]){return Plan{},ErrDenied}
 p:=Plan{Original:strings.Join(tokens," "),Program:tokens[0]}
 switch p.Program{
 case "pwd":
  if len(tokens)!=1{return Plan{},ErrDenied}
  p.Description="Current workspace (relative)"
 case "ls":
  if len(tokens)!=1{return Plan{},ErrDenied}
  p.Description="List workspace names, not file contents"
 case "git":
  switch strings.Join(tokens[1:]," "){
  case "status","status --short":
   p.Args=[]string{"status","--short","--untracked-files=no"}
   p.Description="Show bounded Git status summary"
  case "branch","branch --list":
   p.Args=[]string{"branch","--list","--no-color"}
   p.Description="List local Git branch names"
  case "rev-parse --abbrev-ref HEAD":
   p.Args=[]string{"rev-parse","--abbrev-ref","HEAD"}
   p.Description="Show current branch name"
  default:return Plan{},ErrDenied
  }
 default:return Plan{},ErrDenied
 }
 return p,nil
}

type cappedOutput struct{
 bytes []byte
 truncated bool
}
func (b *cappedOutput)Write(p []byte)(int,error){
 left:=MaxOutputBytes-len(b.bytes)
 if left<=0{b.truncated=true;return len(p),nil}
 if len(p)>left{b.bytes=append(b.bytes,p[:left]...);b.truncated=true
 }else{b.bytes=append(b.bytes,p...)}
 return len(p),nil
}
func outputText(data []byte)string{
 // Never emit terminal control sequences (OSC, ANSI, carriage returns) to
 // the browser. JSON output is text only; no arbitrary HTML or terminal bytes.
 var out strings.Builder
 for len(data)>0{
  r,n:=utf8.DecodeRune(data)
  if r==utf8.RuneError&&n==1{out.WriteRune('?');data=data[1:];continue}
  data=data[n:]
  if r=='\n'||(r>=32&&r!=127&&r<0x7f)||r>=0xa0 {out.WriteRune(r)}
 }
 return out.String()
}
func gitBinary()string{
 candidates:=[]string{"/usr/bin/git","/bin/git"}
 if runtime.GOOS=="darwin"{candidates=[]string{"/usr/bin/git"}}
 if runtime.GOOS=="windows"{return ""}
 for _,path:=range candidates{
  if st,err:=os.Stat(path);err==nil&&!st.IsDir()&&st.Mode().Perm()&0111!=0{return path}
 }
 return ""
}

// Run never starts a shell or accepts an executable/argument directly from
// the request. The working directory is a server-owned canonical workspace.
func Run(ctx context.Context,workspace string,plan Plan)(Result,error){
 out:=Result{Command:plan.Original,ExitCode:0}
 dir,err:=filepath.EvalSymlinks(workspace)
 if err!=nil{return out,err}
 info,err:=os.Stat(dir)
 if err!=nil||!info.IsDir(){return out,errors.New("workspace directory unavailable")}
 switch plan.Program{
 case "pwd":out.Output=".\n";return out,nil
 case "ls":
  entries,err:=os.ReadDir(dir);if err!=nil{return out,err}
  names:=make([]string,0,len(entries))
  for _,e:=range entries{
   n:=e.Name()
   if n==".git"||n==".vscode"||strings.HasPrefix(n,"."){continue}
   names=append(names,n)
  }
  sort.Strings(names)
  var b cappedOutput
  for _,n:=range names{
   if len(n)>200{n=n[:200]}
   b.Write([]byte(n+"\n"))
  }
  out.Output=outputText(b.bytes);out.Truncated=b.truncated;return out,nil
 case "git":
  binary:=gitBinary()
  if binary==""{return out,errors.New("restricted git executable unavailable on this host")}
  if len(plan.Args)==0{return out,ErrDenied}
  timeout,cancel:=context.WithTimeout(ctx,Timeout);defer cancel()
  // Run fixed Git metadata-only operations. Do not inherit GIT_*, HOME,
  // LD_PRELOAD, PATH, SSH_ASKPASS or any other host/user environment.
  argv:=append([]string{"-c","core.fsmonitor=false","-c","core.pager=cat",
   "-c","core.hooksPath=/dev/null","--no-pager"},plan.Args...)
  cmd:=exec.CommandContext(timeout,binary,argv...)
  cmd.Dir=dir
  cmd.Env=[]string{"PATH=/usr/bin:/bin","HOME=/nonexistent","GIT_CONFIG_NOSYSTEM=1",
   "GIT_CONFIG_GLOBAL=/dev/null","GIT_TERMINAL_PROMPT=0",
   "GIT_NO_REPLACE_OBJECTS=1","GIT_OPTIONAL_LOCKS=0",
   "GIT_PAGER=cat","PAGER=cat","LC_ALL=C"}
  var captured cappedOutput
  cmd.Stdout=&captured;cmd.Stderr=&captured
  err:=cmd.Run()
  out.Output=outputText(captured.bytes);out.Truncated=captured.truncated
  if err!=nil{
   if timeout.Err()!=nil{return out,fmt.Errorf("restricted Git timed out: %w",timeout.Err())}
   var ex *exec.ExitError
   if errors.As(err,&ex){out.ExitCode=ex.ExitCode();return out,nil}
   return out,err
  }
  return out,nil
 default:return out,ErrDenied
 }
}

var _ io.Writer = (*cappedOutput)(nil)
