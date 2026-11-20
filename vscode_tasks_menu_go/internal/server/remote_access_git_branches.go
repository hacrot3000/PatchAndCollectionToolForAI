package server

import (
 "context"
 "io"
 "os"
 "os/exec"
 "path/filepath"
 "regexp"
 "runtime"
 "strings"
 "time"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

var remoteGitBranchShortName=regexp.MustCompile("^[A-Za-z0-9][A-Za-z0-9._/-]{0,63}$")

// Read a strictly bounded local branch overview; never accept remote
// selected refs, arguments, working directories, URLs or commands.
func (s *Server) readRemoteGitBranches(ctx context.Context)(string,error){
 denied:=remotetransport.ErrRemoteActionDenied
 if s==nil||ctx==nil||ctx.Err()!=nil||s.Workspace==""||
  runtime.GOOS=="windows"||os.Getenv("TASKDECK_REMOTE_GIT_BRANCHES")!="1"{
  return "",denied
 }
 root,err:=filepath.EvalSymlinks(s.Workspace)
 if err!=nil{return "",denied}
 root,err=filepath.Abs(root)
 if err!=nil{return "",denied}
 info,err:=os.Stat(root)
 if err!=nil||!info.IsDir(){return "",denied}
 env:=[]string{
  "PATH="+os.Getenv("PATH"),"HOME="+os.Getenv("HOME"),
  "LC_ALL=C","GIT_OPTIONAL_LOCKS=0",
  "GIT_CONFIG_NOSYSTEM=1","GIT_CONFIG_GLOBAL="+os.DevNull,
  "GIT_CONFIG_SYSTEM="+os.DevNull,"GIT_PROTOCOL_FROM_USER=0",
 }
 base:=[]string{"-c","core.fsmonitor=false","-c","core.pager=cat",
  "-c","protocol.ext.allow=never","-C",root}
 run:=func(limit int,args ...string)(string,error){
  if ctx.Err()!=nil{return "",denied}
  command:=exec.CommandContext(ctx,"git",append(append([]string(nil),base...),args...)...)
  command.Dir=root
  command.Env=env
  output:=&boundedCommandOutput{remaining:limit}
  command.Stdout=output
  command.Stderr=io.Discard
  prepareGitJobProcess(command)
  command.Cancel=func()error{killGitJobProcess(command);return nil}
  command.WaitDelay=time.Second
  err:=command.Run()
  killGitJobProcess(command)
  if err!=nil||ctx.Err()!=nil||output.truncated{return "",denied}
  return strings.TrimSpace(output.String()),nil
 }
 top,err:=run(4096,"rev-parse","--show-toplevel")
 if err!=nil{return "",denied}
 canonical,err:=filepath.EvalSymlinks(top)
 if err!=nil{return "",denied}
 canonical,err=filepath.Abs(canonical)
 if err!=nil||filepath.Clean(canonical)!=filepath.Clean(root){return "",denied}
 current,err:=run(256,"symbolic-ref","--quiet","--short","HEAD")
 if err!=nil||!validRemoteGitBranchName(current){return "",denied}
 lines,err:=run(2048,"for-each-ref","--count=5","--sort=refname",
  "--format=%(refname:short)","refs/heads/")
 if err!=nil||lines==""{return "",denied}
 branches:=strings.Split(lines,"\n")
 if len(branches)>5{return "",denied}
 for _,branch:=range branches{
  if !validRemoteGitBranchName(branch){return "",denied}
 }
 // Explicitly label a partial result; never imply that only four branches
 // exist if the host has more. Current branch is always shown separately.
 more:=len(branches)>4
 if more{branches=branches[:4]}
 var result strings.Builder
 result.WriteString("Current: ")
 result.WriteString(current)
 result.WriteString("\nLocal branches (first 4):")
 for _,branch:=range branches{
  result.WriteString("\n- ")
  result.WriteString(branch)
 }
 if more{result.WriteString("\n... more")}
 summary:=result.String()
 if !remotetransport.ValidRemoteGitLogText(summary){return "",denied}
 return summary,nil
}

func validRemoteGitBranchName(value string)bool{
 if len(value)==0||len(value)>64||!remoteGitBranchShortName.MatchString(value){
  return false
 }
 if strings.HasPrefix(value,".")||strings.Contains(value,"..")||
  strings.HasSuffix(value,"/")||strings.HasSuffix(value,".")||
  strings.Contains(value,"//")||strings.Contains(value,"/.")||
  strings.Contains(value,"@{"){return false}
 return true
}
