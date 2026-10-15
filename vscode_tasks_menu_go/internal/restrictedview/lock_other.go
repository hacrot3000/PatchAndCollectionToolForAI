//go:build !linux && !darwin && !freebsd

package restrictedview

import (
 "errors"
 "os"
)

// Fail closed until a comparable OS-level cross-process lock exists.
func lockExclusive(*os.File)error{return errors.New("restricted durable quota locking unsupported on this platform")}
func unlockExclusive(*os.File){}
