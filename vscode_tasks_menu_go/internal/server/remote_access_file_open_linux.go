//go:build linux

package server

import (
 "os"
 "syscall"
)

// Open relative to an already opened workspace directory, and never follow
// the final component or block on a FIFO. A concurrent rename of the root
// path cannot redirect the read outside the pinned directory descriptor.
func openRemoteFileNoFollow(root,name string)(*os.File,error){
 dirFD,err:=syscall.Open(root,syscall.O_RDONLY|syscall.O_DIRECTORY|
  syscall.O_NOFOLLOW|syscall.O_CLOEXEC,0)
 if err!=nil{return nil,err}
 defer syscall.Close(dirFD)
 fileFD,err:=syscall.Openat(dirFD,name,
  syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC|syscall.O_NONBLOCK,0)
 if err!=nil{return nil,err}
 return os.NewFile(uintptr(fileFD),name),nil
}
