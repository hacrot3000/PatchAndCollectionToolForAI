//go:build linux || darwin || freebsd

package restrictedview

import (
 "os"
 "syscall"
)

func lockExclusive(f *os.File)error{return syscall.Flock(int(f.Fd()),syscall.LOCK_EX)}
func unlockExclusive(f *os.File){_ = syscall.Flock(int(f.Fd()),syscall.LOCK_UN)}
