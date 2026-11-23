//go:build linux

package agentchat

import (
 "errors"
 "testing"
)

func TestAgentLinuxStateMountAdmissionRejectsKnownRemoteAndClusterFilesystems(t *testing.T){
 rejected:=[]struct{magic uint64;name string}{
  {0x6969,"NFS"},{0xff534d42,"CIFS"},{0xfe534d42,"SMB2"},
  {0x517b,"SMB"},{0x01021997,"9p"},{0x00c36400,"Ceph"},
  {0x73757245,"Coda"},{0x5346414f,"AFS"},
  {0x01161970,"GFS2"},{0x7461636f,"OCFS2"},
  {0x0bd00bd0,"Lustre"},{0x47504653,"GPFS"},
  {0x65735546,"FUSE"},
 }
 for _,tc:=range rejected{
  t.Run(tc.name,func(t *testing.T){
   if e:=classifyAgentLinuxFilesystem(tc.magic);!errors.Is(e,ErrUnsupportedAgentStateFilesystem){
    t.Fatalf("remote/distributed %s mount was admitted: %v",tc.name,e)
   }
  })
 }
 supported:=[]struct{magic uint64;name string}{
  {0xef53,"ext4"},{0x58465342,"XFS"},{0x9123683e,"btrfs"},
  {0x01021994,"tmpfs"},{0x794c7630,"overlay"},
 }
 for _,tc:=range supported{
  t.Run(tc.name,func(t *testing.T){
   if e:=classifyAgentLinuxFilesystem(tc.magic);e!=nil{
    t.Fatalf("local mount %s was rejected: %v",tc.name,e)
   }
  })
 }
}
