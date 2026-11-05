//go:build darwin && go1.25

package server

import (
 "errors"
 "os"
 "syscall"
)

// Go 1.25's os.Root keeps traversal relative to the trusted workspace.
// The resolved directory must be the same inode as the already pinned
// descriptor, preventing a symlink or rename race from changing the target.
func remotePinnedDirectoryEntryMode(parent *os.File,name,root,relativeDir string)(os.FileMode,error){
 base,err:=os.OpenRoot(root)
 if err!=nil{return 0,err}
 defer base.Close()
 directory,err:=base.OpenRoot(relativeDir)
 if err!=nil{return 0,err}
 defer directory.Close()
 pinnedInfo,err:=parent.Stat()
 if err!=nil{return 0,err}
 opened,err:=directory.Open(".")
 if err!=nil{return 0,err}
 actualInfo,statErr:=opened.Stat()
 _=opened.Close()
 if statErr!=nil||!os.SameFile(pinnedInfo,actualInfo){
  return 0,errRemoteDirectorySymlink
 }
 child,err:=directory.OpenFile(name,os.O_RDONLY|syscall.O_NOFOLLOW,0)
 if errors.Is(err,syscall.ELOOP){return 0,errRemoteDirectorySymlink}
 if err!=nil{return 0,err}
 defer child.Close()
 info,err:=child.Stat()
 if err!=nil{return 0,err}
 return info.Mode(),nil
}
