//go:build windows

package server

import (
 "os/exec"
 "strconv"
)

func scheduleConfigureProcess(cmd *exec.Cmd){}
func scheduleTerminateProcessGroup(cmd *exec.Cmd){
 if cmd==nil||cmd.Process==nil{return}
 // taskkill /T terminates the entire descendant tree rather than cmd.exe only.
 _=exec.Command("taskkill.exe","/T","/F","/PID",strconv.Itoa(cmd.Process.Pid)).Run()
 _=cmd.Process.Kill()
}
