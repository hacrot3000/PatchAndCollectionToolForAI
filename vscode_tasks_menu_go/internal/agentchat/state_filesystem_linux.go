//go:build linux

package agentchat

import (
 "fmt"
 "syscall"
)

// Linux statfs magic numbers are from the kernel UAPI. All of these mount
// types can have nonlocal/distributed locking, caching or durability behavior.
// FUSE is deliberately conservative: it may be local, but TaskDeck has no
// validated guarantee that it implements flock+rename+directory fsync safely.
func classifyAgentLinuxFilesystem(t uint64)error{
 var fs string
 switch t{
 case 0x6969:fs="NFS"
 case 0xff534d42:fs="CIFS/SMB"
 case 0xfe534d42:fs="SMB2"
 case 0x517b:fs="SMB"
 case 0x01021997:fs="9p"
 case 0x00c36400:fs="Ceph"
 case 0x73757245:fs="Coda"
 case 0x5346414f:fs="AFS"
 case 0x01161970:fs="GFS2"
 case 0x7461636f:fs="OCFS2"
 case 0x0bd00bd0:fs="Lustre"
 case 0x47504653:fs="GPFS"
 case 0x65735546:fs="FUSE"
 }
 if fs!=""{return unsupportedAgentStateFilesystem(fs)}
 return nil
}

func validateAgentStateFilesystem(dir string)error{
 var st syscall.Statfs_t
 if err:=syscall.Statfs(dir,&st);err!=nil{
  return fmt.Errorf("cannot verify Agent state filesystem: %w",err)
 }
 return classifyAgentLinuxFilesystem(uint64(uint32(st.Type)))
}
