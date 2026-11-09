//go:build !linux && !darwin

package server

import "os"

// Native remote execution is currently explicitly denied outside Unix.
// Retain the existing in-process reservation for cross-build compatibility.
func acquireRemoteWorkspaceOSLock(_ string,_ os.FileInfo)(func(),error){
 return func(){},nil
}
