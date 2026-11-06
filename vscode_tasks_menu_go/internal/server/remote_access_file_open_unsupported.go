//go:build !linux && !darwin

package server

import (
 "errors"
 "os"
)

func openRemoteFileNoFollow(root,name string)(*os.File,error){
 return nil,errors.New("remote file read not supported on this platform")
}
