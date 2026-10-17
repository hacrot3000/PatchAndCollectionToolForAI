//go:build windows

package server

import "os/exec"

func scheduleConfigureProcess(cmd *exec.Cmd){}
func scheduleTerminateProcessGroup(cmd *exec.Cmd){
 if cmd!=nil&&cmd.Process!=nil{_ = cmd.Process.Kill()}
}
