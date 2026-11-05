//go:build !linux && (!darwin || !go1.25)

package server

import "os"

// Remote Agent application transport supports Linux/macOS. Other platforms
// refuse metadata-ambiguous entries instead of following unsafe paths.
func remotePinnedDirectoryEntryMode(_ *os.File,_,_,_ string)(os.FileMode,error){
 return os.ModeIrregular,nil
}
