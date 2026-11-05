package tasks

import (
 "os"
 "path/filepath"
 "runtime"
 "testing"
)

func TestRemoteExecutableAllowedIsIndependentFromDaemonPATH(t *testing.T) {
 if runtime.GOOS=="windows" {t.Skip("host-only Unix remote process hardening")}
 workspace:=t.TempDir()
 if !RemoteExecutableAllowed(workspace,"/bin/echo") {
  t.Fatal("standard absolute executable unexpectedly rejected")
 }
 if RemoteExecutableAllowed(workspace,"echo") {
  t.Fatal("daemon PATH lookup remains remotely executable")
 }
 if RemoteExecutableAllowed(workspace,filepath.Join(workspace,"missing")) {
  t.Fatal("missing executable admitted")
 }
}

func TestRemoteExecutableAllowedRejectsMutableWorkspaceAndSymlinkPaths(t *testing.T) {
 if runtime.GOOS=="windows" {t.Skip("Unix executable mode and symlinks")}
 workspace:=t.TempDir()
 malicious:=filepath.Join(workspace,"untrusted-command")
 if err:=os.WriteFile(malicious,[]byte("#!/bin/sh\nexit 0\n"),0700);err!=nil{t.Fatal(err)}
 if RemoteExecutableAllowed(workspace,malicious) {
  t.Fatal("workspace file can be replaced after host task digest approval")
 }
 symlinkInWorkspace:=filepath.Join(workspace,"trusted-looking-symlink")
 if err:=os.Symlink("/bin/echo",symlinkInWorkspace);err!=nil{t.Fatal(err)}
 if RemoteExecutableAllowed(workspace,symlinkInWorkspace) {
  t.Fatal("workspace-editable symlink to trusted executable admitted")
 }
 outside:=t.TempDir()
 symlinkOut:=filepath.Join(outside,"looks-like-outside")
 if err:=os.Symlink(malicious,symlinkOut);err!=nil{t.Fatal(err)}
 if RemoteExecutableAllowed(workspace,symlinkOut) {
  t.Fatal("external symlink resolved into workspace")
 }
 // Even an executable outside the workspace must not be writable by
 // another user; a valid SHA of tasks.json cannot pin its bytes.
 writable:=filepath.Join(outside,"world-writable")
 if err:=os.WriteFile(writable,[]byte("#!/bin/sh\nexit 0\n"),0777);err!=nil{t.Fatal(err)}
 if err:=os.Chmod(writable,0777);err!=nil{t.Fatal(err)}
 if RemoteExecutableAllowed(workspace,writable) {
  t.Fatal("group/world-writable executable admitted")
 }
 if err:=os.Chmod(writable,0700);err!=nil{t.Fatal(err)}
 if !RemoteExecutableAllowed(workspace,writable) {
  t.Fatal("approved outside-workspace owner-only executable rejected")
 }
}
