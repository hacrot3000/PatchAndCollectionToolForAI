//go:build windows

package server

import "os/exec"

// The auto installer itself is disabled on native Windows. WSL2 uses
// semantic_browser_install_proc_unix.go; keep the package buildable here.
func semanticConfigureProcessGroup(cmd *exec.Cmd){}
func semanticKillProcessGroup(cmd *exec.Cmd){
 if cmd!=nil&&cmd.Process!=nil{_ =cmd.Process.Kill()}
}
