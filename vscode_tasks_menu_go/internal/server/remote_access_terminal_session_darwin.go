//go:build darwin

package server

import (
 "os"
 "os/exec"
 "strconv"
 "strings"
 "syscall"
)

// macOS does not provide /proc. Enumerate the PTY session using the OS ps
// binary, not untrusted PHP or caller-provided argv. The PTY starts a unique
// setsid() session; background process groups still share its session ID.
func remoteKillPTYSession(sid int){
 if sid<=1{return}
 out,err:=exec.Command("/bin/ps","-axo","pid=,sess=").Output()
 if err!=nil{return}
 self:=os.Getpid()
 for _,line:=range strings.Split(string(out),"\n"){
  parts:=strings.Fields(line)
  if len(parts)!=2{continue}
  pid,e1:=strconv.Atoi(parts[0])
  session,e2:=strconv.Atoi(parts[1])
  if e1!=nil||e2!=nil||pid<=1||pid==self||session!=sid{continue}
  _=syscall.Kill(pid,syscall.SIGKILL)
 }
}
