//go:build linux || darwin

package server

import (
 "errors"
 "os"
 "syscall"
)

// Query child type relative to an already pinned directory descriptor.
// Do NOT use os.DirEntry.Info(): when a directory is opened through
// openat with os.NewFile(fd, leafName), Info can resolve that bare name
// against the daemon working directory instead of the pinned directory.
func remotePinnedDirectoryEntryMode(parent *os.File,name string)(os.FileMode,error){
 fd,err:=syscall.Openat(int(parent.Fd()),name,
  syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_CLOEXEC|syscall.O_NOFOLLOW,0)
 if errors.Is(err,syscall.ELOOP){return 0,errRemoteDirectorySymlink}
 if err!=nil{return 0,err}
 child:=os.NewFile(uintptr(fd),name)
 if child==nil{_ = syscall.Close(fd);return 0,syscall.EBADF}
 defer child.Close()
 stat,err:=child.Stat()
 if err!=nil{return 0,err}
 return stat.Mode(),nil
}
