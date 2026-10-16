//go:build !windows

package server

import (
 "os/exec"
 "syscall"
)

// npm, Playwright, and Git can spawn child processes. Cancellation kills the
// private process group rather than leaving child downloads running.
func semanticConfigureProcessGroup(cmd *exec.Cmd){
 cmd.SysProcAttr=&syscall.SysProcAttr{Setpgid:true}
}
func semanticKillProcessGroup(cmd *exec.Cmd){
 if cmd==nil||cmd.Process==nil{return}
 _=syscall.Kill(-cmd.Process.Pid,syscall.SIGKILL)
 _=cmd.Process.Kill()
}
