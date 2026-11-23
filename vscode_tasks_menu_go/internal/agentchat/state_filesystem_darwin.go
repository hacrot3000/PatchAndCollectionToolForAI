//go:build darwin

package agentchat

import (
 "fmt"
 "strings"
 "syscall"
)

// macOS statfs exposes the mounted filesystem's kernel-reported name.
// Avoid assuming flock/rename/fsync on a remote/cloud/user-space mount is
// equivalent to local APFS/HFS+. Unsupported names fail closed.
func classifyAgentDarwinFilesystem(name string)error{
 fs:=strings.ToLower(strings.TrimSpace(name))
 switch fs{
 case "nfs","smbfs","cifs","afpfs","webdav","osxfuse",
  "macfuse","fusefs","fuse","autofs","ceph","lustre","9p":
  return unsupportedAgentStateFilesystem(fs)
 }
 if strings.Contains(fs,"fuse"){return unsupportedAgentStateFilesystem(fs)}
 return nil
}

func validateAgentStateFilesystem(dir string)error{
 var st syscall.Statfs_t
 if err:=syscall.Statfs(dir,&st);err!=nil{
  return fmt.Errorf("cannot verify Agent state filesystem: %w",err)
 }
 b:=make([]byte,0,len(st.Fstypename))
 for _,c:=range st.Fstypename{
  if c==0{break}
  b=append(b,byte(c))
 }
 if len(b)==0{return unsupportedAgentStateFilesystem("unknown macOS mount")}
 return classifyAgentDarwinFilesystem(string(b))
}
