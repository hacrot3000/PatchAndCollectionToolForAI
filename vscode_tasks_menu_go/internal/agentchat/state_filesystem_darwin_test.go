//go:build darwin

package agentchat

import (
 "errors"
 "testing"
)

func TestAgentDarwinStateMountAdmissionRejectsRemoteAndFUSE(t *testing.T){
 for _,name:=range []string{"nfs","smbfs","afpfs","webdav","osxfuse",
  "macfuse","fusefs","autofs","ceph","Lustre","myCustomFUSE"}{
  if err:=classifyAgentDarwinFilesystem(name);!errors.Is(err,ErrUnsupportedAgentStateFilesystem){
   t.Fatalf("%s should be rejected: %v",name,err)
  }
 }
 for _,name:=range []string{"apfs","hfs","msdos","exfat"}{
  if err:=classifyAgentDarwinFilesystem(name);err!=nil{
   t.Fatalf("%s should be accepted: %v",name,err)
  }
 }
}
