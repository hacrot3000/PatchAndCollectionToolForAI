//go:build linux || darwin

package agentchat

import (
 "fmt"
 "os"
)

// syncAgentDirectory flushes the directory entry updates created by
// rename/unlink. Syncing the file contents alone does NOT make a renamed
// filename crash durable on POSIX filesystems.
func syncAgentDirectory(dir string)error{
 f,err:=os.Open(dir)
 if err!=nil{return err}
 defer f.Close()
 if err=f.Sync();err!=nil{
  return fmt.Errorf("fsync agent journal directory: %w",err)
 }
 return nil
}
