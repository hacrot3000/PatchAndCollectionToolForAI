//go:build darwin

package server

import (
 "os"
 "path/filepath"
 "syscall"
)

// Darwin has no syscall.Openat wrapper in Go 1.20. Deny final-component
// symlinks with O_NOFOLLOW; the calling pre/post Lstat and SameFile checks
// independently verify the exact workspace and selected file.
func openRemoteFileNoFollow(root,name string)(*os.File,error){
 fd,err:=syscall.Open(filepath.Join(root,name),
  syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC|syscall.O_NONBLOCK,0)
 if err!=nil{return nil,err}
 return os.NewFile(uintptr(fd),name),nil
}
