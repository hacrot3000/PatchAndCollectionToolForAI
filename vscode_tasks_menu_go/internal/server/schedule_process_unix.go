//go:build !windows

package server

import (
 "os/exec"
 "syscall"
)

func scheduleConfigureProcess(cmd *exec.Cmd){
 cmd.SysProcAttr=&syscall.SysProcAttr{Setpgid:true}
}
func scheduleTerminateProcessGroup(cmd *exec.Cmd){
 if cmd!=nil&&cmd.Process!=nil{
  _=syscall.Kill(-cmd.Process.Pid,syscall.SIGKILL)
 }
}
